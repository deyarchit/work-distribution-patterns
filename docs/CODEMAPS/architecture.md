<!-- Commit: dbc0e450f41ec0f930cf88b8badcb7c47ca74646 | Files scanned: 25 | Token estimate: ~700 -->

# Architecture

Single Go module (`work-distribution-patterns`) with a shared core and three
interchangeable pattern implementations. All three expose an identical HTTP API
and HTMX frontend; only the task dispatch mechanism differs.

## Key Abstractions

```
dispatch.TaskManager interface          (API side)
  └── Submit(ctx, task) error
      (full lifecycle: persist → dispatch → route progress → persist terminal status)

dispatch.TaskSource interface           (worker side)
  └── Receive(ctx) (Task, error)
      (blocks until a task is available or ctx is cancelled)
```

`shared/api` depends only on `TaskManager` — never on pattern-specific code.
Workers depend only on `TaskSource` — they have no knowledge of stores, SSE, or browsers.
Each pattern implements both sides for its unique dispatch mechanics.

## Pattern Topologies

### Pattern 1 — Goroutine Pool (single process)

```
Browser
  │ POST /tasks
  ▼
Echo API ──► PoolTaskManager ──► Pool.Enqueue(fn)
  │                                  │
  │ GET /events?taskID=<id>     goroutine worker
  ◄── sse.Hub ◄────────────────── Executor.Run() → Hub.Publish()

Lifecycle: API creates task → store.Create() → Pool.Enqueue()
           Worker runs → Hub broadcasts → API marks terminal status
```

- 1 binary (`bin/p1-server`)
- Bounded channel pool; 429 when queue full
- PoolTaskManager: persists task, enqueues, handles queue-full failure

### Pattern 2 — WebSocket Worker Hub (1 API + N workers)

```
Browser
  │ POST /tasks
  ▼
Echo API ──► WSTaskManager ──► WorkerHub.Assign() ──► worker (WebSocket)
  │                                                        │
  │ GET /events?taskID=<id>                          WSTaskSource.Receive()
  ◄── sse.Hub ◄──────────────────────────── wsSink sends progress over WS

Lifecycle: API creates task → store.Create() → WorkerHub.Assign()
           Worker receives via source.Receive() → exec.Run(task, source.Sink())
           wsSink sends progress over WS → API receives & forwards to hub
           API marks terminal status when DoneMsg arrives
```

- 2 binaries (`bin/p2-api`, `bin/p2-worker`)
- Round-robin assignment; 503 when all workers busy
- WSTaskManager: persists task, assigns to worker, handles no-workers failure
- WorkerHub.readPump: receives DoneMsg, updates SSE hub and store

### Pattern 3 — NATS JetStream (N APIs + N workers + NATS)

```
Browser
  │ POST /tasks (any API replica)
  ▼
Echo API ──► NATSTaskManager ──► JetStream "tasks.new"
  │                                    │
  │                              NATSTaskSource.Receive()
  │                              Executor.Run() → NATSSink
  │                                    │
  │                              NATS Core progress.* + task_status.*
  ◄── sse.Hub ◄── all API replicas subscribe to NATS Core

Lifecycle: API creates task → store.Create() → js.Publish("tasks.new")
           Worker receives via source.Receive() → exec.Run(task, natsSink)
           natsSink publishes progress & status → all APIs receive via NATS → hub broadcasts & store updates
```

- 2 binaries (`bin/p3-api`, `bin/p3-worker`)
- NATS KV (`task-store`) for cross-replica task state
- NATSTaskManager: persists task, publishes to JetStream, handles publish failure
- All API replicas: progress.* and task_status.* subscriptions update hub & store
- nginx load-balances; no sticky sessions needed

## Shared Package Map

```
shared/
  models/      Task, Stage, ProgressEvent, TaskStatus — pure data types; NewTask() constructor
  dispatch/    TaskManager interface (API) + TaskSource interface (worker)
  executor/    ProgressSink interface + Executor.Run() — stage loop, randomized duration per stage
  sse/         Hub — task-scoped + global fan-out broadcaster
  store/       TaskStore interface + MemoryStore
  api/         Echo router + handlers (depends on TaskManager, Hub, TaskStore)
  templates/   Embedded index.html (HTMX + vanilla JS)
```

## Task Lifecycle (All Patterns)

```
1. Browser POST /tasks
2. API handler creates Task via models.NewTask(name, stageCount)
3. handler calls manager.Submit(ctx, task)
4. manager persists: store.Create(task)
5. manager dispatches work (pattern-specific)
6. worker receives via TaskSource.Receive(ctx)
7. worker runs Executor.Run(ctx, task, sink)
8. executor emits ProgressEvent per tick, then terminal TaskStatus
9. sink routes events back to API (Hub direct, WebSocket, or NATS)
10. manager receives completion, persists: store.SetStatus(task.ID, status)
11. browser SSE stream receives task_status event, updates UI
```
