# Work Distribution Patterns

A Go demo exploring various work-distribution patterns with progressively increasing scalability and decoupling.

![Architecture Overview](docs/overview.excalidraw.png)

## Patterns

| Pattern | Topology | Communication Style | Full-Stack Layering |
|---|---|---|---|
| **p01: Local-Channels** | Single process | In-process channels | Embedded Monolith |
| **p02: Pull-REST** | 1 API + N workers | HTTP Long-polling | Tiered Remote Polling |
| **p03: Push-WebSocket** | 1 API + N workers | Persistent WebSockets | Tiered Remote Push |
| **p05: Brokered-NATS** | N APIs + N workers | NATS + PostgreSQL | Distributed Event-Driven |

All patterns expose an **identical HTTP API** and **identical HTMX frontend**. Only the internal dispatch mechanism and layering changes.

## Quick Start

### Pattern 1: Local-Channels (no Docker needed)

```bash
make run-p1
# open http://localhost:8080
```

### Pattern 2: Pull-REST (Docker)

```bash
make run-p2
# open http://localhost:8080
```

### Pattern 3: Push-WebSocket (Docker)

```bash
make run-p3
# open http://localhost:8080
```

### Pattern 5: Brokered-NATS (Docker)

```bash
make run-p5
# open http://localhost:8080
```

## Architecture

```
Browser
  │  HTTP POST /tasks, GET /tasks, GET /tasks/{id}
  │  SSE  GET /events
  ▼
┌─────────────────────────────────────────────────────┐
│  shared/api  (HTTP handlers + SSE — 100% shared)    │
│  POST /tasks → manager.Submit(task)                 │
│  GET  /events → sse.Hub.Subscribe()                 │
└──────────────────┬──────────────────────────────────┘
                   │ contracts.TaskDispatcher interface
        ┌──────────┴─────────────────────────┐
        │                                    │
      p01                                 p02 / p03 / p05
  ChannelDispatcher              REST/WS/NATSDispatcher
  (in-process)                (routes to external workers)
```

## API Contract

```
POST /tasks          {"name", "stage_count" (1-8, default 3)}
                     → 202 {"id":"..."}
GET  /tasks          → []Task
GET  /tasks/{id}     → Task
GET  /events         SSE stream
GET  /               HTMX frontend
```

## Testing

```bash
# E2E tests (requires a running server)
BASE_URL=http://localhost:8080 make test-e2e

# Load test
BASE_URL=http://localhost:8080 make test-load

# Build all binaries
make build-all

# Run all patterns end-to-end
make test-all
```

## Project Structure

```
shared/
├── api/          All HTTP handlers (shared across all patterns)
├── manager/      Unified task lifecycle management
├── contracts/    Interfaces (TaskDispatcher, TaskConsumer, TaskManager)
├── models/       Task, Stage, TaskEvent data types
├── executor/     Stage runner (worker-side logic)
├── store/        Persistence (Memory, PostgreSQL)
├── events/       Event Bus (Memory, NATS)
├── client/       Remote proxy for API-to-Manager communication
├── sse/          SSE hub — broadcaster to browser connections
└── templates/    Embedded HTMX frontend

patterns/
├── p01/          Local-Channels: Bounded goroutine pool (in-process)
├── p02/          Pull-REST: REST-based worker polling
├── p03/          Push-WebSocket: WebSocket dispatch to external workers
└── p05/          Brokered-NATS: NATS JetStream (queue) + PostgreSQL (store)
```
