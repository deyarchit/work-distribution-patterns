<!-- Commit: 5154530 | Files scanned: 28 | Token estimate: ~1050 -->

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
| `github.com/testcontainers/testcontainers-go/modules/nats` | v0.x | NATS test container (P5/P6 integration tests) |
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

### Pattern 1 — Single process (no Docker required)
```
[server binary]   ← patterns/p01/Dockerfile (single-stage)
make run-p1       ← runs locally without Docker
```

### Pattern 2 — Docker Compose (`patterns/p02`)
```
[manager ×1]   ← patterns/p02/Dockerfile.manager  (port 8081, healthcheck /health)
[api ×1]       ← patterns/p02/Dockerfile.api      (port 8080, depends_on manager healthy)
[worker ×3]    ← patterns/p02/Dockerfile.worker   (depends_on manager healthy)
```
Workers and API talk to manager via `http://manager:8081`.

### Pattern 3 — Docker Compose (`patterns/p03`)
```
[manager ×1]   ← patterns/p03/Dockerfile.manager  (port 8081, healthcheck /health; owns WebSocket hub + MemoryStore)
[api ×1]       ← patterns/p03/Dockerfile.api      (port 8080, depends_on manager healthy; MANAGER_URL=http://manager:8081)
[worker ×3]    ← patterns/p03/Dockerfile.worker   (MANAGER_URL=ws://manager:8081/ws/register)
```
Workers connect to Manager (not API) via WebSocket.

### Pattern 4 — Docker Compose (`patterns/p04`)
```
[manager ×1]   ← patterns/p04/Dockerfile.manager  (port 8081 HTTP, port 9090 gRPC; healthcheck /health; owns gRPC hub + MemoryStore)
[api ×1]       ← patterns/p04/Dockerfile.api      (port 8080, depends_on manager healthy; MANAGER_URL=http://manager:8081)
[worker ×3]    ← patterns/p04/Dockerfile.worker   (MANAGER_URL=http://manager:8081 for gRPC dial)
```
Workers connect to Manager via gRPC bidirectional streams.

### Pattern 5 — Docker Compose (`patterns/p05`)
```
[nginx]        ← patterns/p05/nginx/nginx.conf   (port 8080 → upstream api)
  ├─ [api ×3] ← patterns/p05/Dockerfile.api     (MANAGER_URL=http://manager:8081, NATS_URL=nats://nats:4222, depends_on manager healthy)
[manager ×1]   ← patterns/p05/Dockerfile.manager  (port 8081; NATS_URL, DATABASE_URL; owns NATSBridge, postgres, SSE hub)
[worker ×3]    ← patterns/p05/Dockerfile.worker
[nats]         ← nats:latest + patterns/p05/nats.conf (max_file_store: 1GB, store_dir: /data/jetstream)
               ← named volume: nats-jetstream (persistent across restarts)
[postgres]     ← postgres:17-alpine; NO named volume → ephemeral, wiped on `docker compose down`
```
No sticky sessions: API replicas subscribe directly to NATS `task.events.*` for distributed event streaming.
Manager uses `NATSBridge` to publish events; `pgstore.Store` (PostgreSQL) is the shared persistent store; schema created on startup.
**Note:** `nats.conf` is required — NATS 2.12+ defaults `Max Storage: 0 B` without explicit config.

### Pattern 6 — Docker Compose (`patterns/p06`)
```
[nginx]        ← patterns/p06/nginx/nginx.conf   (port 8080 → upstream api)
  ├─ [api ×3] ← patterns/p06/Dockerfile.api     (MANAGER_URL=http://manager:8081, BROKER_URL=..., depends_on manager healthy)
[manager ×1]   ← patterns/p06/Dockerfile.manager  (port 8081; BROKER_URL, DATABASE_URL; owns CloudDispatcher, postgres, SSE hub)
[worker ×3]    ← patterns/p06/Dockerfile.worker   (BROKER_URL, MAX_STAGE_DURATION)
[broker]       ← NATS (default), Kafka, or LocalStack (for AWS) — configurable via BROKER variable
               ← docker-compose.base.yml + docker-compose.{nats,kafka,aws}.yml
               ← Named volume: nats-jetstream (NATS only; persistent across restarts)
[postgres]     ← postgres:17-alpine; NO named volume → ephemeral, wiped on `docker compose down`
```

**Broker Selection:** `make run-p6 BROKER=nats` (default), `make run-p6 BROKER=kafka`, or `make run-p6 BROKER=aws`.
- **NATS:** gocloud `natspubsub` driver; two JetStream streams (TASKS: WorkQueue, EVENTS: Interest); durable consumers (manager, workers)
- **Kafka:** gocloud `kafkapubsub` driver; topics for tasks and events; consumer groups for load balancing
- **AWS:** gocloud `awssnssqs` driver + LocalStack; Manager publishes tasks to SQS queue, APIs dynamically create SQS queues subscribed to SNS topic for fanout
- API replicas subscribe directly; no sticky sessions. Manager owns all connections and state.

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
