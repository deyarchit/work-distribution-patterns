<!-- Commit: dbc0e450f41ec0f930cf88b8badcb7c47ca74646 | Files scanned: 25 | Token estimate: ~300 -->

# Dependencies

## Go Module (`go.mod`)

| Dependency | Version | Used by |
|------------|---------|---------|
| `github.com/labstack/echo/v4` | v4.15.0 | All patterns — HTTP router |
| `github.com/google/uuid` | v1.6.0 | shared/models — task ID generation |
| `github.com/gorilla/websocket` | v1.5.3 | Pattern 2 — API↔worker WebSocket |
| `github.com/nats-io/nats.go` | v1.48.0 | Pattern 3 — JetStream + KV |

## Runtime Infrastructure

| Pattern | Infrastructure | Notes |
|---------|---------------|-------|
| 01 | None | Single process, no Docker required |
| 02 | Docker Compose | 1 API + 3 workers |
| 03 | Docker Compose | 3 APIs + 3 workers + NATS server + nginx |

## Pattern 3 External Services

```
NATS Server (JetStream enabled)
  Stream:    TASKS  subjects: tasks.>  retention: WorkQueuePolicy
  KV Bucket: task-store  TTL: 24h
  Subjects:  progress.<taskID>    (Core NATS, pub from worker → sub on all APIs)
             task_status.<taskID> (Core NATS, pub from worker → sub on all APIs)

nginx
  Load-balances :8080 → api replicas :8081/:8082/:8083
  No sticky sessions — NATS Core fan-out handles cross-replica SSE delivery
```

## Environment Variables

| Env Var | Default | Used by |
|---------|---------|---------|
| `ADDR` | `:8080` | All API servers |
| `WORKERS` | `5` | Pattern 1 — pool worker count |
| `QUEUE_SIZE` | `20` | Pattern 1 — pool queue depth |
| `MAX_STAGE_DURATION` | `500` | All — max ms per stage (randomized per stage in [0, max]) |
| `API_URL` | `ws://localhost:8080/ws/register` | Pattern 2 worker |
| `NATS_URL` | `nats://localhost:4222` | Pattern 3 API + worker |

## Test Dependencies

```
tests/e2e/  — stdlib only (net/http, bufio, encoding/json)
tests/load/ — stdlib only (net/http, sync, time, flag)
```

## Build Outputs

```
bin/p1-server   patterns/01-goroutine-pool/cmd/server
bin/p2-api      patterns/02-websocket-hub/cmd/api
bin/p2-worker   patterns/02-websocket-hub/cmd/worker
bin/p3-api      patterns/03-nats-jetstream/cmd/api
bin/p3-worker   patterns/03-nats-jetstream/cmd/worker
```
