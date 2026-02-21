<!-- Commit: e927fc3061e6071046447d9933c5d2161663f55b | Files scanned: 27 | Token estimate: ~730 -->

# Architecture

## Overview

Three patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
contracts.TaskManager   Submit(ctx, task) error               — API → Manager (unchanged)
contracts.TaskProducer  Start/Dispatch/ReceiveEvent           — manager-side transport view
contracts.TaskConsumer  Connect/Receive/Emit                  — worker-side transport view
contracts.EventSink     Emit(ctx, TaskEvent) error            — executor emits to this (TaskConsumer satisfies it)
store.TaskStore         Create/Get/List/SetStatus             — task persistence
```

`TaskProducer` and `TaskConsumer` are the variation points; all other logic lives in `shared/manager.Manager`.
`TaskConsumer` automatically satisfies `EventSink` (same `Emit` signature).
Sentinel errors from `Dispatch`: `ErrDispatchFull` → HTTP 429, `ErrNoWorkers` → HTTP 503.

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

## Pattern 2: WebSocket Hub (API + remote workers)

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

## Pattern 3: Queue-and-Store (horizontally scaled)

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

