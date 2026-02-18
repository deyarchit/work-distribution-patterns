<!-- Generated: 2026-02-18 | Files scanned: 27 | Token estimate: ~600 -->

# Architecture

Single Go module (`work-distribution-patterns`) with a shared core and three
interchangeable pattern implementations. All three expose an identical HTTP API
and HTMX frontend; only the task dispatch mechanism differs.

## Key Abstraction

```
dispatch.Dispatcher interface
  └── Submit(ctx, task) error
```

`shared/api` depends only on this interface — never on pattern-specific code.

## Pattern Topologies

### Pattern 1 — Goroutine Pool (single process)

```
Browser
  │ POST /tasks
  ▼
Echo API ──► PoolDispatcher ──► Pool.Enqueue()
  │                                  │
  │ GET /events?taskID=<id>     goroutine worker
  ◄── sse.Hub ◄────────────────── executor.Run() → Hub.Publish()
```

- 1 binary (`bin/p1-server`)
- Bounded channel pool; 429 when queue full

### Pattern 2 — WebSocket Worker Hub (1 API + N workers)

```
Browser
  │ POST /tasks
  ▼
Echo API ──► WSDispatcher ──► WorkerHub.Assign() ──► worker (WebSocket)
  │                                                        │
  │ GET /events?taskID=<id>                          executor.Run()
  ◄── sse.Hub ◄──────────────────────────── progress msgs over WS
```

- 2 binaries (`bin/p2-api`, `bin/p2-worker`)
- Round-robin assignment; 503 when all workers busy

### Pattern 3 — NATS JetStream (N APIs + N workers + NATS)

```
Browser
  │ POST /tasks (any API replica)
  ▼
Echo API ──► NATSDispatcher ──► JetStream "tasks.new"
  │                                    │
  │                              worker consumes
  │                              executor.Run()
  │                                    │
  │                              NATS Core "progress.<taskID>"
  ◄── sse.Hub ◄── all API replicas subscribe to NATS Core progress
```

- 2 binaries (`bin/p3-api`, `bin/p3-worker`)
- NATS KV (`task-store`) for cross-replica task state
- nginx load-balances; no sticky sessions needed

## Shared Package Map

```
shared/
  models/      Task, Stage, ProgressEvent, TaskAssignment — pure data types
  dispatch/    Dispatcher interface (the seam)
  executor/    Executor.Run() — stage loop, 10 ticks/stage; ProgressSink interface
  sse/         Hub — task-scoped + global fan-out broadcaster
  store/       TaskStore interface + MemoryStore
  api/         Echo routes + handlers (depends on Dispatcher, Hub, TaskStore)
  templates/   Embedded index.html (HTMX + vanilla JS)
```
