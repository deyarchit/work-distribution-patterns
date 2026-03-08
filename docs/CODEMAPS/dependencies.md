# Dependencies & Configuration

## Go Dependencies (`go.mod`)

| Dependency | Version | Why |
|-----------|---------|-----|
| `echo/v4` | v4.15.0 | HTTP router + HTMX templates |
| `gorilla/websocket` | v1.5.3 | P3 WebSocket transport |
| `google.golang.org/grpc` | v1.x | P4 gRPC |
| `protobuf` | v1.x | P4 protobuf |
| `nats.go` | v1.48.0 | P5/P6 JetStream |
| `pgx/v5` | v5.8.0 | P5/P6 PostgreSQL + pool |
| `uuid` | v1.6.0 | Task IDs |
| `envconfig` | v1.4.0 | Struct-based config |
| `gocloud.dev` | v0.41.0 | P6 pubsub abstraction |
| `aws-sdk-go-v2/*` | v1.x | P6 AWS SNS/SQS |
| `testcontainers-go` | v0.x | P5/P6 NATS/Postgres — ⚠ omit `WithArgument("-js","")`; JetStream on by default |

## Environment Variables

All env loading uses `envconfig.Process("", &cfg)` with `default:` tags.

| Variable | Default | Used by | Purpose |
|----------|---------|---------|---------|
| `ADDR` | `:8080` | All | Listen address |
| `WORKERS` | `5` | P1 | Pool size |
| `QUEUE_SIZE` | `20` | P1–P3 | Max before 429 |
| `MAX_STAGE_DURATION` | `500ms` | Worker | Per-stage max |
| `MANAGER_URL` | `http://localhost:8081` | P2–P5 | Manager URL |
| `GRPC_ADDR` | `:9090` | P4 | gRPC listen |
| `NATS_URL` | `nats://127.0.0.1:4222` | P5 | NATS broker |
| `DATABASE_URL` | `postgres://localhost/tasks` | P5–P7 | PostgreSQL |
| `BROKER_URL` | `nats://nats:4222` | P6–P7 | Gocloud URL |
| `AWS_*` | — | P6 | AWS SDK (LocalStack) |
| `BOOTSTRAP_ADDR` | `:8083` | P7 | mTLS listen |
| `TOKEN_SECRET` | `change-me-in-production` | P7 | HMAC key |
| `TOKEN_TTL_MINS` | `15` | P7 | TTL (min) |
| `SERVER_CERT_PATH` | `certs/server.crt` | P7 | Server cert |
| `SERVER_KEY_PATH` | `certs/server.key` | P7 | Server key |
| `CA_CERT_PATH` | `certs/ca.crt` | P7 | CA cert |
| `CERT_PATH` | `certs/client.crt` | P7 | Client cert |
| `KEY_PATH` | `certs/client.key` | P7 | Client key |

## Container Topology

| Pattern | Services | Notes |
|---------|----------|-------|
| P1 | `[server]` (binary) | Single process; `make run-p1` local (no Docker) |
| P2 | `[api]`, `[manager]`, `[worker×3]` | HTTP polling; `depends_on manager` |
| P3 | `[api]`, `[manager]`, `[worker×3]` | WebSocket push; manager owns hub |
| P4 | `[api]`, `[manager]` (HTTP + gRPC), `[worker×3]` | gRPC bidirectional streams |
| P5 | `[nginx]` → `[api×3]`, `[manager×3]`, `[worker×3]`, `[nats]`, `[postgres]` | NATS queue group ensures one manager per event; ⚠ nats.conf required |
| P6 | `[nginx]` → `[api×3]`, `[manager×3]`, `[worker×3]`, `[broker]`, `[postgres]` | Broker-agnostic gocloud; broker consumer groups; `BROKER=nats\|kafka\|aws` |
| P7 | `[api]`, `[manager]` (HTTP:8081 + mTLS:8083), `[worker×3]` | Worker bootstrap; discovers broker + token; mTLS device certs |

P5: nats-jetstream volume; P6–P7: postgres ephemeral.

## Build Targets

```bash
make build-all      # builds all 19 binaries into bin/
                    #   p1-server,
                    #   p2-api, p2-manager, p2-worker,
                    #   p3-api, p3-manager, p3-worker,
                    #   p4-api, p4-manager, p4-worker,
                    #   p5-api, p5-manager, p5-worker,
                    #   p6-api, p6-manager, p6-worker,
                    #   p7-api, p7-manager, p7-worker
make run-p1         # local run, no Docker
make run-p2         # docker compose up (Pattern 2: REST polling)
make run-p3         # docker compose up (Pattern 3: WebSocket hub)
make run-p4         # docker compose up (Pattern 4: gRPC bidirectional)
make run-p5         # docker compose up (Pattern 5: Queue-and-Store)
make run-p6         # docker compose up (Pattern 6: Cloud-Agnostic PubSub; BROKER=nats, kafka, or aws; default: nats)
make test-all       # build-all + E2E tests against all patterns
make test-e2e       # E2E tests against BASE_URL (default :8080)
make test-load      # load test against BASE_URL
```
