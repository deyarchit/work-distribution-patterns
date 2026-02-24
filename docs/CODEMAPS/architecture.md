<!-- Commit: 0f2a79be70e27faae9a536f6a02ab610528f049f | Files scanned: 40 | Token estimate: ~880 -->

# Architecture

## Overview

Four patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
contracts.TaskManager      Submit/Get/List/Subscribe             — API → Manager
contracts.TaskDispatcher   Start/Dispatch/ReceiveEvent           — manager-side transport view
contracts.TaskConsumer     Connect/Receive/Emit                  — worker-side transport view
store.TaskStore            Create/Get/List/SetStatus             — task persistence
```

`TaskDispatcher` and `TaskConsumer` are the variation points; all other logic lives in `shared/manager.Manager`.
`TaskConsumer` is the single view from the worker side, used by the executor to emit events.
Sentinel errors from `Dispatch`: `ErrDispatchFull` → HTTP 429, `ErrNoWorkers` → HTTP 503.

`TaskManager.Get/List` let the API query task state without direct store access.
`TaskManager.Subscribe` streams `TaskEvent` from the manager's hub — used by P2/P3/P4 APIs to pump events cross-process.
`shared/client.RemoteTaskManager` implements `TaskManager` by proxying HTTP to a separate manager process; used by all three remote patterns.

## Process Topology

| Pattern | API | Manager | Worker | Transport |
|---------|-----|---------|--------|-----------|
| P1 | single process | same | goroutines | in-process channels |
| P2 | :8080 | :8081 | separate process | REST polling |
| P3 | :8080 | :8081 | separate process | WebSocket push |
| P4 | :8080 (×3) | :8081 (×1) | separate process (×3) | NATS JetStream |

## Three-Layer Structure

```
API layer    shared/api          HTTP transport, unchanged
Manager      shared/manager      task lifecycle, deadline loop, event routing
Transport    per-pattern         dispatcher.go (TaskDispatcher) + consumer.go (TaskConsumer)
```

## Pattern 1: Goroutine Pool (single process)

```
Browser ──POST /tasks──► shared/api ──► Manager ──► ChannelDispatcher.Dispatch()
                                                          │ buffered events chan (directional)
                         sse.Hub ◄── Manager.runEventLoop ◄── ChannelDispatcher.events
                            │
                         RunWorker goroutines ◄── ChannelConsumer.Receive()
                                    └── exec.Run(ctx, task, consumer)  [consumer = TaskConsumer]
Browser ◄── GET /events ───┘
```

- `ChannelDispatcher`+`ChannelWorker` created together by `goroutine.New`; share a single `events` channel
- Store: `MemoryStore`; Backpressure: HTTP 429; Deadline loop: disabled (`deadline=0`)
- Env: `WORKERS`, `QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 2: REST Polling (API + manager + workers — separate processes)

```
Browser ──POST /tasks──► API (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                                                                            │ RESTDispatcher.Dispatch
Browser ◄── GET /events ── local hub ◄── pump goroutine ◄── Subscribe (SSE /events)       │
                                                                                            │
                           Worker ──polls── GET /work/next ◄────────────────────────────────┤
                                 └──── POST /work/events ──────────────────────────────────► RESTDispatcher
```

- `shared/client.RemoteTaskManager` proxies all API calls to manager over HTTP
- `RESTDispatcher`: non-blocking `Dispatch` to buffered chan; blocking GET /work/next for workers
- Store: `MemoryStore` (manager-local); Backpressure: HTTP 429; Deadline loop: disabled
- Env: `MANAGER_URL`, `WORKERS_QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 3: WebSocket Hub (API + manager + workers — separate processes)

```
Browser ──POST /tasks──► API (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                                                                            │ WebSocketDispatcher.Dispatch
Browser ◄── GET /events ── local hub ◄── pump goroutine ◄── Subscribe (SSE /events)       │ (WebSocket)
                                                                                            │
                           Worker process ◄── WebSocketConsumer.Receive()                  │ GET /ws/register
                                    └── exec.Run(ctx, task, consumer)  [consumer = TaskConsumer] │
```

- `shared/client.RemoteTaskManager` proxies all API calls to manager over HTTP
- Store: `MemoryStore` (manager-local); Backpressure: HTTP 503; Deadline loop: disabled
- Worker registration: `GET /ws/register` on Manager → `WebSocketDispatcher.Register(conn)`

## Pattern 4: Queue-and-Store (horizontally scaled)

```
Browser ──POST /tasks──► nginx ──► API replica (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                       │                                                                       │ NATSDispatcher.Dispatch
Browser ◄── GET /events ── local hub ◄── pump goroutine ◄── Subscribe (SSE /events)         JetStream (tasks.new)
                                                                                                  │
                                                                                        Worker NATSConsumer.Receive()
                                                                                        exec.Run → Emit → task.events.<id>
                                                                                        Manager.runEventLoop → hub + PostgreSQL
```

- API replicas are thin proxies; Manager owns NATS, postgres, SSE hub
- Store: `pgstore.Store` (PostgreSQL — shared across replicas); Deadline: 30 s re-dispatch
- NATS used for queuing only (JetStream dispatch + NATS Core event routing)
- Env (API): `MANAGER_URL`; Env (manager): `NATS_URL`, `DATABASE_URL`
