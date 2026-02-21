<!-- Commit: e927fc3061e6071046447d9933c5d2161663f55b | Files scanned: 10 | Token estimate: ~540 -->

# Dependencies & Configuration

## Go Dependencies (`go.mod`)

| Dependency | Version | Why |
|-----------|---------|-----|
| `github.com/labstack/echo/v4` | v4.15.0 | HTTP router, middleware, template rendering |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket transport (Pattern 2) |
| `github.com/nats-io/nats.go` | v1.48.0 | NATS JetStream client (Pattern 3) |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver + `pgxpool` connection pool (Pattern 3) |
| `github.com/google/uuid` | v1.6.0 | Task ID generation |
| `github.com/kelseyhightower/envconfig` | v1.4.0 | Struct-based env config loading (all patterns) |

## Environment Variables

All env loading uses `envconfig.Process("", &cfg)` with a `config` struct and `default:` tags.

| Variable | Default | Pattern | Purpose |
|----------|---------|---------|---------|
| `ADDR` | `:8080` | All API/server | Listen address |
| `WORKERS` | `5` | P1 | Goroutine pool size |
| `QUEUE_SIZE` | `20` | P1 | Max queued tasks before HTTP 429 |
| `MAX_STAGE_DURATION` | `500` | P1, P2 worker, P3 worker | Max milliseconds per stage |
| `API_URL` | `ws://localhost:8080/ws/register` | P2 worker | WebSocket registration endpoint |
| `NATS_URL` | `nats://127.0.0.1:4222` | P3 | NATS server URL |
| `DATABASE_URL` | `postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable` | P3 | PostgreSQL connection string |

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

### Pattern 3 — Docker Compose (`03-queue-and-store`)
```
[nginx]        ← nginx/nginx.conf   (port 8080 → upstream api)
  ├─ [api ×3] ← Dockerfile.api     (depends_on: postgres healthy, nats started)
  └─ [worker ×3] ← Dockerfile.worker
[nats]         ← nats:latest + nats.conf (max_file_store: 1GB, store_dir: /data/jetstream)
               ← named volume: nats-jetstream (persistent across restarts)
[postgres]     ← postgres:17-alpine; NO named volume → ephemeral, wiped on `docker compose down`
```
No sticky sessions: all API replicas subscribe to `task.events.*` on NATS Core.
`pgstore.Store` (PostgreSQL) is the shared persistent store; schema created on startup.
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
