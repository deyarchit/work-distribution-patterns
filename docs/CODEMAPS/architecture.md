<!-- Commit: f5c70505b68226bac66d88c059907dd521ec813f | Files scanned: 58 | Token estimate: ~1050 -->

# Architecture

## Overview

Five patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
contracts.TaskManager      Submit/Get/List                       — API → Manager
contracts.TaskDispatcher   Start/Dispatch/ReceiveEvent           — manager-side transport view
contracts.TaskConsumer     Connect/Receive/Emit                  — worker-side transport view
events.TaskEventBus        Publish/Subscribe                     — event streaming abstraction
store.TaskStore            Create/Get/List/SetStatus             — task persistence
```

`TaskDispatcher` and `TaskConsumer` are the variation points; all other logic lives in `shared/manager.Manager`.
`TaskConsumer` is the single view from the worker side, used by the executor to emit events.
Sentinel errors from `Dispatch`: `ErrDispatchFull` → HTTP 429, `ErrNoWorkers` → HTTP 503.

`TaskManager.Get/List` let the API query task state without direct store access.
Event streaming is wired explicitly in `main.go`: managers publish to `TaskEventBus`, which is pumped to SSE hub; APIs subscribe via `sse.Client` (P2/P3/P4) or NATS (P5).
`shared/client.RemoteTaskManager` implements `TaskManager` by proxying Submit/Get/List over HTTP; used by P2/P3/P4 APIs.

## Process Topology

| Pattern | API | Manager | Worker | Transport |
|---------|-----|---------|--------|-----------|
| P1 | single process | same | goroutines | in-process channels |
| P2 | :8080 | :8081 | separate process | REST polling |
| P3 | :8080 | :8081 | separate process | WebSocket push |
| P4 | :8080 | :8081 | separate process | gRPC bidirectional stream |
| P5 | :8080 (×3) | :8081 (×1) | separate process (×3) | NATS JetStream |

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
                         sse.Hub ◄── pump ◄── MemoryEventBus ◄──┐
                            │                                    │
                         RunWorker goroutines                    Manager.runEventLoop (republishes=true)
                                    └── exec.Run(ctx, task, consumer)  [consumer = TaskConsumer]
                                           │ event emission ────────┘
Browser ◄── GET /events ───┘
```

- `ChannelDispatcher`+`ChannelWorker` created together by `goroutine.New`; share a single `events` channel
- Store: `MemoryStore`; Backpressure: HTTP 429; Deadline loop: disabled (`deadline=0`)
- Env: `WORKERS`, `QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 2: REST Polling (API + manager + workers — separate processes)

```
Browser ──POST /tasks──► API (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                                                                            │ RESTDispatcher.Dispatch
Browser ◄── GET /events ── local hub ◄── pump ◄── sse.Client ◄── GET /events (SSE) ◄── mgr hub ◄── MemoryEventBus ◄──┐
                                                                                                                         │
                           Worker ──polls── GET /work/next ◄─────────────────────────────────────────────────────────┤
                                 └──── POST /work/events ──────────────────────────────────► RESTDispatcher ──►┐       │
                                                                                                Manager.runEventLoop (republishes=true)
```

- `shared/client.RemoteTaskManager` proxies Submit/Get/List to manager over HTTP
- Manager pumps `MemoryEventBus` → SSE hub; API subscribes via `sse.Client`
- `RESTDispatcher`: non-blocking `Dispatch` to buffered chan; blocking GET /work/next for workers
- Store: `MemoryStore` (manager-local); Backpressure: HTTP 429; Deadline loop: disabled
- Env: `MANAGER_URL`, `WORKERS_QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 3: WebSocket Hub (API + manager + workers — separate processes)

```
Browser ──POST /tasks──► API (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                                                                            │ WebSocketDispatcher.Dispatch
Browser ◄── GET /events ── local hub ◄── pump ◄── sse.Client ◄── GET /events (SSE) ◄── mgr hub ◄── MemoryEventBus ◄──┐
                                                                                                                         │
                           Worker process ◄── WebSocketConsumer.Receive() ◄── WebSocket ────────────────┐             │
                                    └── exec.Run(ctx, task, consumer)  [consumer = TaskConsumer]        │             │
                                           │ event emission ───────────────────► Manager.runEventLoop (republishes=true)
```

- `shared/client.RemoteTaskManager` proxies Submit/Get/List to manager over HTTP
- Manager pumps `MemoryEventBus` → SSE hub; API subscribes via `sse.Client`
- Store: `MemoryStore` (manager-local); Backpressure: HTTP 503; Deadline loop: disabled
- Worker registration: `GET /ws/register` on Manager → `WebSocketDispatcher.Register(conn)`

## Pattern 4: gRPC Bidirectional Streaming (API + manager + workers — separate processes)

```
Browser ──POST /tasks──► API (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                                                                            │ gRPCDispatcher.Dispatch
Browser ◄── GET /events ── local hub ◄── pump ◄── sse.Client ◄── GET /events (SSE) ◄── mgr hub ◄── MemoryEventBus ◄──┐
                                                                                                                         │
                           Worker process ◄── gRPCConsumer.Receive() ◄── gRPC stream ────────────────┐             │
                                    └── exec.Run(ctx, task, consumer)  [consumer = TaskConsumer]        │             │
                                           │ event emission ───────────────────► Manager.runEventLoop (republishes=true)
```

- `shared/client.RemoteTaskManager` proxies Submit/Get/List to manager over HTTP
- Manager pumps `MemoryEventBus` → SSE hub; API subscribes via `sse.Client`
- `gRPCDispatcher`: maintains persistent bidirectional gRPC streams with workers; `Dispatch` sends tasks over stream
- `gRPCConsumer`: connects via gRPC, receives tasks and sends events bidirectionally
- Store: `MemoryStore` (manager-local); Backpressure: HTTP 503 if no workers; Deadline loop: disabled
- Env: `MANAGER_URL`, `GRPC_ADDR` (manager gRPC listen), `MAX_STAGE_DURATION`

## Pattern 5: Queue-and-Store (horizontally scaled)

```
Browser ──POST /tasks──► nginx ──► API replica (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                       │                                                                       │ NATSDispatcher.Dispatch
Browser ◄── GET /events ◄── NATS sub (task.events.*) ──────────────────────┐               JetStream (tasks.new)
                                      (direct, no hub pump)                 │                       │
                                                                      NATSEventBus              │
                                                                            ▲                   │
                                                                            │              Worker NATSConsumer.Receive()
                                                                            │              exec.Run → Emit → task.events.<id>
                                                                            │              (Manager.runEventLoop does NOT republish;
                                                                            │               republishWorkerEvents=false)
                         Manager.runEventLoop → PostgreSQL
```

- API replicas are thin proxies; Manager owns NATS, postgres, `NATSEventBus`
- APIs subscribe directly to NATS `task.events.*` (no SSE hub needed); Manager does NOT republish to event bus
- Store: `pgstore.Store` (PostgreSQL — shared across replicas); Deadline: 30 s re-dispatch
- NATS used for both queueing (JetStream tasks.new) and event streaming (Core task.events.*)
- Env (API): `MANAGER_URL`, `NATS_URL`; Env (manager): `NATS_URL`, `DATABASE_URL`
