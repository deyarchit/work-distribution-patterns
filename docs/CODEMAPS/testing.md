# Testing Codemap

## Test Commands

| Target | Scope | Infra |
|--------|-------|-------|
| `make test` | All 6 patterns (~40 s) | In-process only |
| `make test-e2e` | `RunSuite` vs external API | `BASE_URL=<url>` required |
| `make test-load` | Load test | `BASE_URL=<url>` required |
| `make test-integration` | Full E2E all patterns (slow) | Docker Compose stacks |

Use `make test` as inner loop; builds coverage report.

## Test Structure

- `patterns/p0N/integration_test.go` — per-pattern integration test
- `tests/e2e/task_test.go` — E2E wrapper (reads `BASE_URL`, calls `RunSuite`)
- `shared/testutil/` — helpers (`RunSuite`, `SSEClient`, `WaitReady`, `WaitForWorker`) and suite

**Setup**: Start infra → Manager → Worker(s) → API → `WaitReady` + `WaitForWorker` → `RunSuite`.

**RunSuite subtests**: `SingleTask` (verify task lifecycle), `ConcurrentTasks` (3 parallel), `StatusTransitions` (SSE sequence + final state).

## Test Infrastructure

| Pattern | Containers | Components |
|---------|-----------|-----------|
| P1 | None | Single fused Echo (manager + workers + API) |
| P2 | None | Manager, 1 worker, API on random ports |
| P3 | None | Manager, **3 worker goroutines**, API |
| P4 | None | Manager HTTP + **gRPC server**, 1 worker, API |
| P5 | NATS, Postgres | Manager, 1 worker, API (testcontainers) |
| P6 | NATS, Postgres | Manager, 1 worker, API (testcontainers) |

P5/P6 use `testcontainers-go`: `tcnats.Run`, `tcpostgres.Run` with `BasicWaitStrategies()`. All cleanup via `t.Cleanup`.

## Key Helpers (`shared/testutil`)

| Function | Signature | Purpose |
|----------|-----------|---------|
| `RunSuite` | `(t, baseURL string)` | Runs SingleTask, ConcurrentTasks, StatusTransitions |
| `SSEClient` | `(ctx, t, baseURL, taskID string) <-chan SSEEvent` | Connect to `/events`; buffered (256) |
| `PostTask` | `(t, baseURL, name string, stageCount int) string` | POST /tasks; returns task ID |
| `GetTask`, `ListTasks` | HTTP helpers | GET /tasks/:id, GET /tasks |
| `WaitReady` | `(t, baseURL string)` | Poll /health until 200 (10 s timeout) |
| `WaitForWorker` | `(t, baseURL string)` | Submit probe task, wait for completion (5 s accept + 10 s complete) — ⚠ critical for async registration |
| `CollectEventsUntilQuiet` | `(ctx, t, events, taskIDs, quietDuration) CollectedEvents` | Aggregate SSE events until 1 s quiet |

## Non-Obvious Gotchas

- **P3: exactly 3 workers required** — `ConcurrentTasks` submits 3 simultaneous tasks; `busy` flag limits one task per worker.
- **P3/P4: `WaitForWorker` mandatory** — Workers register asynchronously; omitting it causes 503 on first `POST /tasks`. ⚠ Do not remove.
- **`WaitForWorker` waits for completion** — Polls `GET /tasks/<id>` until terminal; ensures worker idle before suite.
- **NATS: no manual `-js` flag** — `tcnats.Run` enables JetStream by default; explicit flag causes conflicts.
- **P4: dual listeners** — `NewManager` returns `{Router, GRPCServer}`; start both on random ports.
- **`make test-e2e` requires running API** — Default `BASE_URL=http://localhost:8080`.
- **AWS/Kafka in `test-integration` only** — `make test` uses NATS for P6; brokers via docker-compose + `BROKER=` env.
