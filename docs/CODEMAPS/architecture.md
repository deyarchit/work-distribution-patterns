<!-- Commit: 037cf2a0a6aad9ab680755e81d64b2bce033fac2 | Files scanned: 26 | Token estimate: ~720 -->

# Architecture

## Overview

Three patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
contracts.TaskManager   Submit(ctx, task) error                       — API → Manager (unchanged)
contracts.TaskProducer  Start/Dispatch/ReceiveResult/ReceiveProgress  — manager-side transport view
contracts.TaskConsumer  Connect/Receive/ReportResult/ReportProgress   — worker-side transport view
contracts.ProgressSink  ReportProgress(ctx, event) error              — stage progress (UX, best-effort)
store.TaskStore         Create/Get/List/SetStatus                     — task persistence
```

`TaskProducer` and `TaskConsumer` are the variation points; all other logic lives in `shared/manager.Manager`.
`TaskConsumer` automatically satisfies `ProgressSink` (same `ReportProgress` signature).
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
                                                          │ buffered chan (directional)
                         sse.Hub ◄── Manager.runResultLoop ◄── ChannelProducer.results
                         sse.Hub ◄── Manager.runProgressLoop ◄─ ChannelProducer.progress
                            │
                         RunWorker goroutines ◄── ChannelConsumer.Receive()
Browser ◄── GET /events ───┘
```

- `ChannelProducer`+`ChannelConsumer` created together by `goroutine.New`; share directional channels
- Store: `MemoryStore`; Backpressure: HTTP 429; Deadline loop: disabled (`deadline=0`)
- Env: `WORKERS`, `QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 2: WebSocket Hub (API + remote workers)

```
Browser ──POST /tasks──► shared/api ──► Manager ──► WebSocketProducer.Dispatch()
                                                          │ WebSocket
                         sse.Hub ◄── Manager.runResultLoop ◄── WebSocketProducer.results
                            │                          (readPump pushes to chan)
                         Worker process ◄── WebSocketConsumer.Receive()
Browser ◄── GET /events ───┘
```

- Store: `MemoryStore`; Backpressure: HTTP 503; Deadline loop: disabled
- Worker registration: `GET /ws/register` → `WebSocketProducer.Register(conn)`

## Pattern 3: NATS JetStream (horizontally scaled)

```
Browser ──POST /tasks──► nginx ──► API replica ──► Manager ──► NATSBus.Dispatch()
                                       │                              │ JetStream
                          NATS Core ◄──┘                     Worker NATSSource.Receive()
                    (progress.* / task_status.*)              executor.Run()
                              │                               NATSSource.ReportResult/Progress
                    ALL API replicas: NATSBus.Start() subscribes
                    Manager routes to hub + store

Browser ◄── GET /events ───┘ (any replica — no sticky sessions needed)
```

- Store: `JetStreamStore` (NATS KV — shared); Deadline: 30 s re-dispatch
- Env: `NATS_URL`

