# Dependencies & Configuration

## Go Dependencies (`go.mod`)

| Dependency | Version | Why |
|-----------|---------|-----|
| `github.com/labstack/echo/v4` | v4.15.0 | HTTP router, middleware, template rendering |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket transport (Pattern 3) |
| `google.golang.org/grpc` | v1.x | gRPC framework (Pattern 4) |
| `google.golang.org/protobuf` | v1.x | Protocol buffer runtime (Pattern 4) |
| `github.com/nats-io/nats.go` | v1.48.0 | NATS JetStream client (Pattern 5/6) |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver + `pgxpool` connection pool (Pattern 5/6) |
| `github.com/google/uuid` | v1.6.0 | Task ID generation |
| `github.com/kelseyhightower/envconfig` | v1.4.0 | Struct-based env config loading (all patterns) |
| `gocloud.dev` | v0.41.0 | Cloud-agnostic abstraction for pub/sub (Pattern 6) |
| `gocloud.dev/pubsub/awssnssqs` | v0.41.0 | AWS SNS/SQS driver for gocloud (Pattern 6 AWS) |
| `github.com/pitabwire/natspubsub` | v0.0.x | NATS JetStream driver for gocloud (Pattern 6) |
| `github.com/aws/aws-sdk-go-v2/config` | v1.41+ | AWS SDK v2 config loader (Pattern 6 AWS) |
| `github.com/aws/aws-sdk-go-v2/service/sns` | v1.39+ | AWS SNS client (Pattern 6 AWS) |
| `github.com/aws/aws-sdk-go-v2/service/sqs` | v1.42+ | AWS SQS client (Pattern 6 AWS) |
| `github.com/testcontainers/testcontainers-go` | v0.x | Container orchestration for tests (integration tests) |
| `github.com/testcontainers/testcontainers-go/modules/nats` | v0.x | NATS test container (P5/P6 integration tests) — ⚠ do not pass `WithArgument("-js","")`: JetStream is already on by default in this module and adding it causes conflicts |
| `github.com/testcontainers/testcontainers-go/modules/postgres` | v0.x | PostgreSQL test container (P5/P6 integration tests) |

## Environment Variables

All env loading uses `envconfig.Process("", &cfg)` with a `config` struct and `default:` tags.

| Variable | Default | Pattern | Purpose |
|----------|---------|---------|---------|
| `ADDR` | `:8080` | All API/server | Listen address |
| `WORKERS` | `5` | P1 | Goroutine pool size |
| `QUEUE_SIZE` | `20` | P1 | Max queued tasks before HTTP 429 |
| `MAX_STAGE_DURATION` | `500` | P1, P2 worker, P3 worker, P5 worker | Max milliseconds per stage |
| `MANAGER_URL` | `http://localhost:8081` | P2 API, P2 worker, P3 API, P5 API | Manager process base URL (HTTP for API; P2 worker also reads this) |
| `WORKERS_QUEUE_SIZE` | `20` | P2 manager, P3 manager | Max queued tasks before HTTP 429 |
| `MANAGER_URL` (WS) | `ws://localhost:8081/ws/register` | P3 worker | WebSocket registration endpoint on Manager |
| `GRPC_ADDR` | `:9090` | P4 manager | gRPC server listen address |
| `NATS_URL` | `nats://127.0.0.1:4222` | P5 API, P5 manager | NATS server URL (API: event subscription; manager: dispatch + events) |
| `DATABASE_URL` | `postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable` | P5 manager, P6 manager | PostgreSQL connection string |
| `BROKER_URL` | `nats://nats:4222` | P6 API, P6 manager, P6 worker | Broker URL for gocloud (nats://, kafka://, or awssqs://) |
| `AWS_ENDPOINT_URL` | `http://localhost:4566` | P6 (AWS broker only) | LocalStack endpoint for local AWS testing |
| `AWS_REGION` | `us-east-1` | P6 (AWS broker only) | AWS region (used by SDKv2) |
| `AWS_ACCESS_KEY_ID` | `test` | P6 (AWS broker only) | AWS access key (dummy for LocalStack) |
| `AWS_SECRET_ACCESS_KEY` | `test` | P6 (AWS broker only) | AWS secret key (dummy for LocalStack) |

## Container Topology

| Pattern | Services | Notes |
|---------|----------|-------|
| P1 | `[server]` (binary) | Single process; `make run-p1` local (no Docker) |
| P2 | `[api]`, `[manager]`, `[worker×3]` | HTTP polling; `depends_on manager` |
| P3 | `[api]`, `[manager]`, `[worker×3]` | WebSocket push; manager owns hub |
| P4 | `[api]`, `[manager]` (HTTP + gRPC), `[worker×3]` | gRPC bidirectional streams |
| P5 | `[nginx]` → `[api×3]`, `[manager]`, `[worker×3]`, `[nats]`, `[postgres]` | Queue-and-store; APIs thin proxies; ⚠ nats.conf required (JetStream persistence) |
| P6 | `[nginx]` → `[api×3]`, `[manager]`, `[worker×3]`, `[broker]`, `[postgres]` | Broker-agnostic via gocloud; `BROKER=nats\|kafka\|aws` |

P5/P6 use persistent volumes (nats-jetstream for NATS; postgres always ephemeral).

## Build Targets

```bash
make build-all      # builds all 16 binaries into bin/
                    #   p1-server,
                    #   p2-api, p2-manager, p2-worker,
                    #   p3-api, p3-manager, p3-worker,
                    #   p4-api, p4-manager, p4-worker,
                    #   p5-api, p5-manager, p5-worker,
                    #   p6-api, p6-manager, p6-worker
make run-p1         # local run, no Docker
make run-p2         # docker compose up (Pattern 2: REST polling)
make run-p3         # docker compose up (Pattern 3: WebSocket hub)
make run-p4         # docker compose up (Pattern 4: gRPC bidirectional)
make run-p5         # docker compose up (Pattern 5: Queue-and-Store)
make run-p6         # docker compose up (Pattern 6: Cloud-Agnostic PubSub; BROKER=nats, kafka, or aws; default: nats)
make test-all       # build-all + E2E tests against all 6 patterns
make test-e2e       # E2E tests against BASE_URL (default :8080)
make test-load      # load test against BASE_URL
```
