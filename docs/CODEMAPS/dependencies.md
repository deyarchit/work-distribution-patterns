<!-- Generated: 2026-02-18 | Files scanned: 27 | Token estimate: ~300 -->

# Dependencies

## Go Module (`go.mod`)

| Dependency | Version | Used by |
|------------|---------|---------|
| `github.com/labstack/echo/v4` | v4.15.0 | All patterns — HTTP router |
| `github.com/google/uuid` | v1.6.0 | shared/api — task ID generation |
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
  Subjects:  progress.<taskID>  (Core NATS, pub from worker → sub on all APIs)

nginx
  Load-balances :8080 → api replicas :8081/:8082/:8083
  No sticky sessions — NATS Core fan-out handles cross-replica SSE delivery
```

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
