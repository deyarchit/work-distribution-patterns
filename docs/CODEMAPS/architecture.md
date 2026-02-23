<!-- Commit: 0617358258f210256f7fed182c9f649941ee2c33 | Files scanned: 38 | Token estimate: ~860 -->

# Architecture

## Overview

Four patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
contracts.TaskManager   Submit/Get/List/Subscribe             — API → Manager
contracts.TaskProducer  Start/Dispatch/ReceiveEvent           — manager-side transport view
contracts.TaskConsumer  Connect/Receive/Emit                  — worker-side transport view
contracts.EventSink     Emit(ctx, TaskEvent) error            — executor emits to this (TaskConsumer satisfies it)
store.TaskStore         Create/Get/List/SetStatus             — task persistence
```

`TaskProducer` and `TaskConsumer` are the variation points; all other logic lives in `shared/manager.Manager`.
`TaskConsumer` automatically satisfies `EventSink` (same `Emit` signature).
Sentinel errors from `Dispatch`: `ErrDispatchFull` → HTTP 429, `ErrNoWorkers` → HTTP 503.

`TaskManager.Get/List` let the API query task state without direct store access.
`TaskManager.Subscribe` streams `TaskEvent` from the manager's hub — used by Pattern 2's API to pump events cross-process.

## Three-Layer Structure

```
API layer    shared/api          HTTP transport, unchanged
Manager      shared/manager      task lifecycle, deadline loop, event routing
Transport    per-pattern         producer.go (TaskProducer) + consumer.go (TaskConsumer)
```

## Pattern 1: Goroutine Pool (single process)

```
Browser ──POST /tasks──► shared/api ──► Manager ──► ChannelProducer.Dispatch()
                                                          │ buffered events chan (directional)
                         sse.Hub ◄── Manager.runEventLoop ◄── ChannelProducer.events
                            │
                         RunWorker goroutines ◄── ChannelConsumer.Receive()
                                    └── exec.Run(ctx, task, source)  [source = EventSink]
Browser ◄── GET /events ───┘
```

- `ChannelProducer`+`ChannelConsumer` created together by `goroutine.New`; share a single `events` channel
- Store: `MemoryStore`; Backpressure: HTTP 429; Deadline loop: disabled (`deadline=0`)
- Env: `WORKERS`, `QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 2: REST Polling (API + manager + workers — separate processes)

```
Browser ──POST /tasks──► API (:8080) ──► RemoteTaskManager.Submit ──► POST /tasks ──► Manager (:8081)
                                                                                            │ RESTProducer.Dispatch
Browser ◄── GET /events ── local hub ◄── pump goroutine ◄── Subscribe (SSE /events)       │
                                                                                            │
                           Worker ──polls── GET /work/next ◄────────────────────────────────┤
                                 └──── POST /work/events ──────────────────────────────────► RESTProducer
```

- `RemoteTaskManager` (internal/client) proxies all API calls to manager over HTTP
- `RESTProducer`: non-blocking `Dispatch` to buffered chan; blocking GET /work/next for workers
- Store: `MemoryStore` (manager-local); Backpressure: HTTP 429; Deadline loop: disabled
- Env: `MANAGER_URL`, `WORKERS_QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 3: WebSocket Hub (API + remote workers)

```
Browser ──POST /tasks──► shared/api ──► Manager ──► WebSocketProducer.Dispatch()
                                                          │ WebSocket
                         sse.Hub ◄── Manager.runEventLoop ◄── WebSocketProducer.events
                            │                          (readPump pushes to chan)
                         Worker process ◄── WebSocketConsumer.Receive()
                                    └── exec.Run(ctx, task, source)  [source = EventSink]
Browser ◄── GET /events ───┘
```

- Store: `MemoryStore`; Backpressure: HTTP 503; Deadline loop: disabled
- Worker registration: `GET /ws/register` → `WebSocketProducer.Register(conn)`

## Pattern 4: Queue-and-Store (horizontally scaled)

```
Browser ──POST /tasks──► nginx ──► API replica ──► Manager ──► NATSProducer.Dispatch()
                                       │                              │ JetStream (tasks.new)
                          NATS Core ◄──┘                     Worker NATSConsumer.Receive()
                       (task.events.*)                        exec.Run(ctx, task, source)
                              │                               NATSConsumer.Emit → task.events.<id>
                    ALL API replicas: NATSProducer.Start() subscribes to task.events.*
                    Manager.runEventLoop routes to hub + PostgreSQL store

Browser ◄── GET /events ───┘ (any replica — no sticky sessions needed)
```

- Store: `pgstore.Store` (PostgreSQL — shared across replicas); Deadline: 30 s re-dispatch
- NATS used for queuing only (JetStream dispatch + NATS Core event routing)
- Env: `NATS_URL`, `DATABASE_URL`
