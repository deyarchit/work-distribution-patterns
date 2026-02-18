# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Exploring the Project

Use `tree` to get the full nested structure before navigating files — it prevents stale path assumptions:

```bash
tree .
```

## Commands

```bash
# Run Pattern 1 locally (no Docker)
make run-p1

# Run Pattern 2 with Docker Compose (1 API + 3 workers)
make run-p2 / make stop-p2

# Run Pattern 3 with Docker Compose (3 APIs + 3 workers + NATS + nginx)
make run-p3 / make stop-p3

# Build all five binaries into bin/
make build-all

# E2E tests — requires a running server
BASE_URL=http://localhost:8080 make test-e2e

# Run a single E2E test
BASE_URL=http://localhost:8080 go test ./tests/e2e/... -v -run TestSingleTask

# Load test
BASE_URL=http://localhost:8080 make test-load

# Tidy modules
make tidy
```

## Architecture

All three patterns expose an **identical HTTP API and HTMX frontend**. Only the dispatch mechanism differs. The `shared/` package contains everything that never changes across patterns.

### The Key Abstraction: `dispatch.Dispatcher`

`shared/dispatch/dispatcher.go` defines the single interface that separates the HTTP layer from execution:

```go
type Dispatcher interface {
    Submit(ctx context.Context, task models.Task) error
}
```

`shared/api` never imports pattern-specific code — it only depends on this interface. Each pattern (`01`, `02`, `03`) provides a concrete implementation.

### Shared Package Roles

| Package | Role |
|---|---|
| `shared/models` | `Task`, `Stage`, `ProgressEvent`, `TaskAssignment` data types |
| `shared/dispatch` | `Dispatcher` interface (the seam between HTTP and execution) |
| `shared/executor` | `Executor.Run()` — runs stages sequentially, 10 ticks per stage. Accepts a `ProgressSink` interface |
| `shared/sse` | `Hub` — fan-out broadcaster to all SSE subscribers. Satisfies `executor.ProgressSink` directly |
| `shared/store` | `TaskStore` interface + `MemoryStore` implementation |
| `shared/api` | All Echo HTTP handlers, wired with a `Dispatcher` and `TaskStore` |
| `shared/templates` | Single embedded HTMX `index.html` |

### Pattern Topologies

- **Pattern 1** (`01-goroutine-pool`): Single process. `PoolDispatcher` submits tasks to a bounded goroutine pool. `sse.Hub` satisfies `ProgressSink` directly.
- **Pattern 2** (`02-websocket-hub`): 1 API + N workers. `WSDispatcher` sends `TaskAssignment` to a worker over WebSocket. Workers call back via WebSocket with progress events.
- **Pattern 3** (`03-nats-jetstream`): N APIs + N workers + NATS. `NATSDispatcher` publishes tasks to a JetStream stream. All API replicas subscribe to NATS Core progress subjects so any replica can serve any SSE client — no sticky sessions needed. NATS KV is used to track task state across replicas.

### Adding a New Pattern

1. Create `patterns/0N-name/` with `cmd/` entrypoints and `internal/` implementation.
2. Implement `dispatch.Dispatcher` — that is the only contract with `shared/api`.
3. Wire `shared/api.NewServer(dispatcher, store, hub)` in `main.go`.
4. Wire `executor.Executor` to emit into `ProgressSink` (either `sse.Hub` directly or an adapter).
5. Add `run-pN` / `stop-pN` / build targets to `Makefile`.

## Key Design Constraints

- `shared/api` must not import any pattern-specific package.
- `StageDurationSecs` lives on the `Task` struct — do not add it as a separate parameter to `Dispatcher.Submit`.
- `sse.Hub` drops events for slow consumers (non-blocking send) — do not change this to blocking.
- `MemoryStore` is the only store implementation; it is not safe to share across processes (Patterns 2/3 store state only in the API process that received the task, or in NATS KV for Pattern 3).
