<!-- Commit: 7fe066ab6730595a6c51680b8324893cbae27fa5 | Files scanned: 9 | Token estimate: ~490 -->

# Dependencies & Configuration

## Go Dependencies (`go.mod`)

| Dependency | Version | Why |
|-----------|---------|-----|
| `github.com/labstack/echo/v4` | v4.15.0 | HTTP router, middleware, template rendering |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket transport (Pattern 2) |
| `github.com/nats-io/nats.go` | v1.48.0 | NATS JetStream client (Pattern 3) |
| `github.com/google/uuid` | v1.6.0 | Task ID generation |

## Environment Variables

| Variable | Default | Pattern | Purpose |
|----------|---------|---------|---------|
| `ADDR` | `:8080` | All | Listen address for API/server |
| `WORKERS` | `5` | P1 | Goroutine pool size |
| `QUEUE_SIZE` | `20` | P1 | Max queued tasks before HTTP 429 |
| `MAX_STAGE_DURATION` | `500` | P1, P2 worker | Max milliseconds per stage (randomized per stage) |
| `NATS_URL` | `nats://localhost:4222` | P3 | NATS server URL |

## Container Topology

### Pattern 1 — Single process (no Docker required)
```
[server binary]   ← Dockerfile (single-stage)
make run-p1       ← runs locally without Docker
```

### Pattern 2 — Docker Compose
```
[api ×1]       ← Dockerfile.api     (port 8080)
[worker ×3]    ← Dockerfile.worker
```
Workers connect to API via `ws://api:8080/ws/register`.

### Pattern 3 — Docker Compose
```
[nginx]        ← nginx/nginx.conf   (port 8080 → upstream api)
  ├─ [api ×3] ← Dockerfile.api
  └─ [worker ×3] ← Dockerfile.worker
[nats]         ← nats:latest + nats.conf (explicit max_file_store: 1GB, store_dir: /data/jetstream)
               ← named volume: nats-jetstream
```
No sticky sessions: all API replicas subscribe to `progress.*` and `task_status.*` on NATS Core.
`JetStreamStore` uses NATS KV bucket (`tasks`) as shared state across API replicas.
**Note:** `nats.conf` is required — NATS 2.12+ defaults `Max Storage: 0 B` without explicit config.

## Build Targets

```bash
make build-all      # builds all 5 binaries into bin/
make run-p1         # local run, no Docker
make run-p2         # docker compose up (Pattern 2)
make run-p3         # docker compose up (Pattern 3)
make test-all       # build-all + E2E tests against all 3 patterns
make test-e2e       # E2E tests against BASE_URL (default :8080)
make test-load      # load test against BASE_URL
```
