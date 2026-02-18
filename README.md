# Work Distribution Patterns

A Go demo exploring three work-dispatching patterns with progressively increasing scalability.

![Architecture Overview](docs/overview.excalidraw.png)

## Patterns

| Pattern | Topology | Use When |
|---|---|---|
| **1 — Goroutine Pool** | Single process | Low traffic, simple ops |
| **2 — WebSocket Hub** | 1 API + N workers | Moderate traffic, external workers |
| **3 — NATS JetStream** | N APIs + N workers + NATS | High traffic, full distribution |

All three expose an **identical HTTP API** and **identical HTMX frontend**. Only the dispatch mechanism changes.

## Quick Start

### Pattern 1 — Goroutine Pool (no Docker needed)

```bash
make run-p1
# open http://localhost:8080
```

```bash
# Submit a task via curl
curl -X POST localhost:8080/tasks \
  -H "Content-Type: application/json" \
  -d '{"name":"hello","stage_count":3,"stage_duration_secs":2}'

# Stream SSE events
curl -N localhost:8080/events
```

### Pattern 2 — WebSocket Worker Hub

```bash
make run-p2
# 1 API + 3 worker replicas
# open http://localhost:8080
```

### Pattern 3 — NATS JetStream

```bash
make run-p3
# 3 API replicas + 3 workers + NATS + nginx
# open http://localhost:8080
# NATS monitoring: http://localhost:8222
```

## Architecture

```
Browser
  │  HTTP POST /tasks, GET /tasks, GET /tasks/{id}
  │  SSE  GET /events
  ▼
┌─────────────────────────────────────────────────────┐
│  shared/api  (HTTP handlers + SSE — 100% shared)    │
│  POST /tasks → dispatcher.Submit(task)              │
│  GET  /events → sse.Hub.Subscribe()                 │
└──────────────────┬──────────────────────────────────┘
                   │ dispatch.Dispatcher interface
        ┌──────────┴─────────────────────────┐
        │                                    │
  Pattern 1                          Pattern 2 / 3
  PoolDispatcher              WSDispatcher / NATSDispatcher
  (in-process)                (routes to external workers)
```

Progress always terminates at `sse.Hub.Publish()` — the SSE layer never changes.

## API Contract

```
POST /tasks          {"name", "stage_count" (1-8), "stage_duration_secs"}
                     → 202 {"id":"..."}
GET  /tasks          → []Task
GET  /tasks/{id}     → Task
GET  /events         SSE stream
GET  /               HTMX frontend
```

SSE event types:
```json
{"type":"stage_progress","taskID":"...","stageIdx":2,"stageName":"Validation","progress":67,"status":"running"}
{"type":"task_status","taskID":"...","status":"completed"}
```

## Testing

```bash
# E2E tests (requires a running server)
BASE_URL=http://localhost:8080 make test-e2e

# Load test
BASE_URL=http://localhost:8080 make test-load

# Build all binaries
make build-all
```

## Project Structure

```
shared/
├── models/       Task, Stage, ProgressEvent data types
├── dispatch/     Dispatcher interface (THE key abstraction)
├── sse/          SSE hub — broadcaster to all browser connections
├── executor/     Stage runner — emits 10 progress ticks per stage
├── store/        TaskStore interface + MemoryStore
├── api/          All HTTP handlers (shared across all patterns)
└── templates/    Single embedded HTMX frontend

patterns/
├── 01-goroutine-pool/    Bounded goroutine pool (in-process)
├── 02-websocket-hub/     WebSocket dispatch to external workers
└── 03-nats-jetstream/    NATS JetStream + KV for full distribution
```

## Key Design Decisions

- **`dispatch.Dispatcher`** is the only seam between HTTP and execution — `shared/api` never imports pattern-specific code.
- **Stage duration** (`StageDurationSecs`) is part of the task, not an interface parameter — it flows through naturally.
- **`sse.Hub`** satisfies `executor.ProgressSink` directly in Pattern 1; Patterns 2/3 use adapter types.
- **Pattern 3** has all API replicas subscribe to NATS Core progress subjects, so any replica can serve any SSE client — no sticky sessions.
