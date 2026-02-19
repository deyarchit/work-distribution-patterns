<!-- Generated: 2026-02-18 | Files scanned: 27 | Token estimate: ~650 -->

# Architecture

Single Go module (`work-distribution-patterns`) with a shared core and three
interchangeable pattern implementations. All three expose an identical HTTP API
and HTMX frontend; only the task dispatch mechanism differs.

## Key Abstraction

```
dispatch.TaskManager interface
  └── Submit(ctx, task) error
      (full lifecycle: persist → dispatch → route progress → persist terminal status)
```

`shared/api` depends only on this interface — never on pattern-specific code.
Each pattern implements TaskManager to handle its unique dispatch mechanics.

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
  │ GET /events?taskID=<id>                          Executor.Run()
  ◄── sse.Hub ◄──────────────────────────── progress msgs over WS

Lifecycle: API creates task → store.Create() → WorkerHub.Assign()
           Worker runs → wsSink sends progress over WS → API receives & forwards
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
  │                              worker consumes
  │                              Executor.Run() → natsSink
  │                                    │
  │                              NATS Core progress.* + task_status.*
  ◄── sse.Hub ◄── all API replicas subscribe to NATS Core

Lifecycle: API creates task → store.Create() → js.Publish("tasks.new")
           Worker consumes → Executor.Run() → natsSink publishes progress & status
           All APIs receive via NATS subscription → Hub broadcasts & store updates
```

- 2 binaries (`bin/p3-api`, `bin/p3-worker`)
- NATS KV (`task-store`) for cross-replica task state
- NATSTaskManager: persists task, publishes to JetStream, handles publish failure
- All API replicas: progress.* and task_status.* subscriptions update hub & store
- nginx load-balances; no sticky sessions needed

## Shared Package Map

```
shared/
  models/      Task, Stage, ProgressEvent, TaskStatus — pure data types
  dispatch/    TaskManager interface + contract documentation
  executor/    Executor.Run() — stage loop, 10 ticks/stage; returns TaskStatus
  sse/         Hub — task-scoped + global fan-out broadcaster
  store/       TaskStore interface + MemoryStore
  api/         Echo router + handlers (depends on TaskManager, Hub, TaskStore)
  templates/   Embedded index.html (HTMX + vanilla JS)
```

## Task Lifecycle (All Patterns)

```
1. Browser POST /tasks
2. API handler creates Task (uuid, stages, status=pending)
3. handler calls manager.Submit(ctx, task)
4. manager persists: store.Create(task)
5. manager dispatches work (pattern-specific)
6. worker receives task
7. worker runs Executor.Run(ctx, task, sink)
8. executor emits ProgressEvent, then terminal TaskStatus
9. sink routes events back to API (Hub, WebSocket, or NATS)
10. manager receives completion, persists: store.SetStatus(task.ID, status)
11. browser SSE stream receives task_status event, updates UI
```
