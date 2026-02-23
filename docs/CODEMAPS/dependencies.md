<!-- Commit: 0617358258f210256f7fed182c9f649941ee2c33 | Files scanned: 14 | Token estimate: ~600 -->

# Dependencies & Configuration

## Go Dependencies (`go.mod`)

| Dependency | Version | Why |
|-----------|---------|-----|
| `github.com/labstack/echo/v4` | v4.15.0 | HTTP router, middleware, template rendering |
| `github.com/gorilla/websocket` | v1.5.3 | WebSocket transport (Pattern 3) |
| `github.com/nats-io/nats.go` | v1.48.0 | NATS JetStream client (Pattern 4) |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver + `pgxpool` connection pool (Pattern 4) |
| `github.com/google/uuid` | v1.6.0 | Task ID generation |
| `github.com/kelseyhightower/envconfig` | v1.4.0 | Struct-based env config loading (all patterns) |

## Environment Variables

All env loading uses `envconfig.Process("", &cfg)` with a `config` struct and `default:` tags.

| Variable | Default | Pattern | Purpose |
|----------|---------|---------|---------|
| `ADDR` | `:8080` | All API/server | Listen address |
| `WORKERS` | `5` | P1 | Goroutine pool size |
| `QUEUE_SIZE` | `20` | P1 | Max queued tasks before HTTP 429 |
| `MAX_STAGE_DURATION` | `500` | P1, P2 worker, P3 worker, P4 worker | Max milliseconds per stage |
| `MANAGER_URL` | `http://localhost:8081` | P2 API, P2 worker | Manager process base URL |
| `WORKERS_QUEUE_SIZE` | `20` | P2 manager | Max queued tasks in RESTProducer before HTTP 429 |
| `API_URL` | `ws://localhost:8080/ws/register` | P3 worker | WebSocket registration endpoint |
| `NATS_URL` | `nats://127.0.0.1:4222` | P4 | NATS server URL |
| `DATABASE_URL` | `postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable` | P4 | PostgreSQL connection string |

## Container Topology

### Pattern 1 — Single process (no Docker required)
```
[server binary]   ← Dockerfile (single-stage)
make run-p1       ← runs locally without Docker
```

### Pattern 2 — Docker Compose (`02-rest-polling`)
```
[manager ×1]   ← Dockerfile.manager  (port 8081, healthcheck /health)
[api ×1]       ← Dockerfile.api      (port 8080, depends_on manager healthy)
[worker ×3]    ← Dockerfile.worker   (depends_on manager healthy)
```
Workers and API talk to manager via `http://manager:8081`.

### Pattern 3 — Docker Compose (`03-websocket-hub`)
```
[api ×1]       ← Dockerfile.api     (port 8080)
[worker ×3]    ← Dockerfile.worker
```
Workers connect to API via `ws://api:8080/ws/register`.

### Pattern 4 — Docker Compose (`04-queue-and-store`)
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
make build-all      # builds all 8 binaries into bin/
                    #   p1-server, p2-api, p2-manager, p2-worker,
                    #   p3-api, p3-worker, p4-api, p4-worker
make run-p1         # local run, no Docker
make run-p2         # docker compose up (Pattern 2: REST polling)
make run-p3         # docker compose up (Pattern 3: WebSocket hub)
make run-p4         # docker compose up (Pattern 4: Queue-and-Store)
make test-all       # build-all + E2E tests against all 4 patterns
make test-e2e       # E2E tests against BASE_URL (default :8080)
make test-load      # load test against BASE_URL
```
