# Testing Codemap

## Test Commands

| Target | Command | Covers | Needs running infra? |
|--------|---------|--------|----------------------|
| `make test` | `go test ./patterns/... -timeout 300s -coverprofile=coverage.txt` | All 6 pattern integration tests; generates `coverage.txt` + `coverage.html` | No — spins up everything in-process |
| `make test-e2e` | `go clean -testcache && BASE_URL=<url> go test ./tests/e2e/...` | `RunSuite` against an external server | Yes — `BASE_URL` must point to a running API |
| `make test-load` | `go run ./tests/load/load.go -url <url>` | Load test | Yes — `BASE_URL` must point to a running API |
| `make test-integration` | `build-all` + docker-compose bring-up for all 6 patterns sequentially | Full E2E for all patterns; slow | No — brings up Docker Compose stacks |

`make test` is the fast inner loop (~40 s for all 6 patterns). Use it after any code change.

## Test Structure

```
patterns/p0N/integration_test.go   — one integration test per pattern (package p0N_test)
tests/e2e/task_test.go             — thin E2E wrapper: reads BASE_URL, calls RunSuite
shared/testutil/helpers.go         — HTTP helpers, SSE client, WaitReady, WaitForWorker
shared/testutil/suite.go           — RunSuite + the three subtest implementations
```

All integration tests follow the same shape:
1. Start required infrastructure (containers for P5/P6; in-process for P1–P4)
2. Start manager, then worker(s), then API — in that order
3. Call `WaitReady` (and `WaitForWorker` for P3/P4)
4. Call `testutil.RunSuite(t, apiURL)`

`RunSuite` runs three subtests in sequence: `SingleTask`, `ConcurrentTasks`, `StatusTransitions`.

| Subtest | What it verifies |
|---------|-----------------|
| `SingleTask` | 3-stage task emits `stageCount*2` progress events and reaches `completed` |
| `ConcurrentTasks` | 3 simultaneous tasks all complete with correct per-task event counts |
| `StatusTransitions` | SSE status sequence is exactly `["running", "completed"]`; GET /tasks/:id and GET /tasks reflect final state |

## Test Infrastructure

| Pattern | Containers | In-process components |
|---------|-----------|----------------------|
| P1 | None | Single Echo server (manager + workers + API fused) |
| P2 | None | Manager Echo, 1 worker goroutine, API Echo — all on random ports |
| P3 | None | Manager Echo, **3 worker goroutines**, API Echo |
| P4 | None | Manager HTTP Echo + **gRPC server** (both random ports), 1 worker goroutine, API Echo |
| P5 | `nats:2-alpine` (JetStream), `postgres:16-alpine` | Manager Echo, 1 worker goroutine, API Echo |
| P6 | `nats:2-alpine` (JetStream), `postgres:16-alpine` | Manager Echo, 1 worker goroutine, API Echo |

Container startup uses `testcontainers-go`:
- NATS: `tcnats.Run(ctx, "nats:2-alpine")` — JetStream is on by default in this module
- Postgres: `tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.BasicWaitStrategies())`

All containers and servers are cleaned up via `t.Cleanup`.

## Key Helpers (`shared/testutil`)

| Function | Signature | Description |
|----------|-----------|-------------|
| `RunSuite` | `(t, baseURL string)` | Runs SingleTask, ConcurrentTasks, StatusTransitions subtests |
| `SSEClient` | `(ctx, t, baseURL, taskID string) <-chan SSEEvent` | Connects to `/events` or `/events?taskID=`; returns buffered (256) channel; closes on ctx cancel or stream drop |
| `PostTask` | `(t, baseURL, name string, stageCount int) string` | POST /tasks; returns task ID; fatals on non-202 |
| `GetTask` | `(t, baseURL, id string) TaskResponse` | GET /tasks/:id |
| `ListTasks` | `(t, baseURL string) []TaskResponse` | GET /tasks |
| `WaitReady` | `(t, baseURL string)` | Polls /health until 200; timeout 10 s |
| `WaitForWorker` | `(t, baseURL string)` | Submits probe task, retries until 202 accepted, then waits for completion; timeout 5 s accept + 10 s complete |
| `CollectEventsUntilQuiet` | `(ctx, t, events <-chan SSEEvent, taskIDs []string, quietDuration time.Duration) CollectedEvents` | Collects SSE events for the given task IDs until `quietDuration` silence; returns counts, status sequence, per-task breakdown |
| `DoGet` | `(url string) (int, error)` | Raw GET; used internally by WaitReady |

`CollectedEvents` fields used in assertions:
- `EventCounts map[string]int` — event type → total count across all tasks
- `PerTask map[string]map[string]int` — taskID → event type → count
- `StatusSeq []string` — ordered slice of `task_status` event statuses
- `SeenCompleted bool` — true if any `task_status=completed` was received

## Non-Obvious Gotchas

- **P3 requires exactly 3 workers.** `ConcurrentTasks` submits 3 tasks; P3's `busy` flag is one task per worker. <3 workers → timeout.
- **P3/P4 require `WaitForWorker`.** Workers register asynchronously. Without it, first `POST /tasks` returns 503. ⚠ Do not remove.
- **`WaitForWorker` waits for completion, not just acceptance.** Polls `GET /tasks/<id>` until terminal — ensures worker is idle before suite runs.
- **NATS testcontainer: no manual `-js` flag.** `tcnats.Run` enables JetStream by default; manual `WithArgument("-js","")` causes conflicts.
- **`CollectEventsUntilQuiet` quiescence is 1 second.** Slow CI may fire early; increase `quietDuration` instead of adding sleep.
- **P4: dual listeners.** `NewManager` returns `{Router, GRPCServer}`; start both HTTP and gRPC listeners on random ports.
- **`make test-e2e` requires running API.** Tests against `BASE_URL` (default `http://localhost:8080`); fails on `WaitReady` if no server.
- **AWS/Kafka only in `make test-integration`.** `make test` uses NATS for P6; AWS/Kafka tested via docker-compose with `BROKER=` override.
