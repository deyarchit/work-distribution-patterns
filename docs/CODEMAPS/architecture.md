<!-- Commit: 394144da8e51a3a4b807c8913f7bca4ab40e5b8e | Files scanned: 58 | Token estimate: ~720 -->

# Architecture

## Overview

Four patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
dispatch.TaskManager   Submit(ctx, task) error                       — API → Manager (unchanged)
dispatch.WorkerBus     Start/Dispatch/ReceiveResult/ReceiveProgress  — manager-side transport view
dispatch.WorkerSource  Connect/Receive/ReportResult/ReportProgress   — worker-side transport view
dispatch.ProgressSink  Publish(event)                                — stage progress (UX, best-effort)
store.TaskStore        Create/Get/List/SetStatus                     — task persistence
```

`WorkerBus` and `WorkerSource` are the variation points; all other logic lives in `shared/manager.Manager`.
Sentinel errors from `Dispatch`: `ErrDispatchFull` → HTTP 429, `ErrNoWorkers` → HTTP 503.

## Three-Layer Structure

```
API layer    shared/api          HTTP transport, unchanged
Manager      shared/manager      task lifecycle, deadline loop, event routing
Worker       per-pattern         bus.go (WorkerBus) + source.go (WorkerSource)
```

## Pattern 1: Goroutine Pool (single process)

```
Browser ──POST /tasks──► shared/api ──► Manager ──► ChannelBus.Dispatch()
                                                          │ buffered chan
                         sse.Hub ◄── Manager.runResultLoop ◄── ChannelBus.results
                         sse.Hub ◄── Manager.runProgressLoop ◄─ ChannelBus.progress
                            │
                         RunWorker goroutines ◄── ChannelBus.Receive()
Browser ◄── GET /events ───┘
```

- `ChannelBus` implements both `WorkerBus` and `WorkerSource` (same process, shared channels)
- Store: `MemoryStore`; Backpressure: HTTP 429; Deadline loop: disabled (`deadline=0`)
- Env: `WORKERS`, `QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 2: WebSocket Hub (API + remote workers)

```
Browser ──POST /tasks──► shared/api ──► Manager ──► WebSocketBus.Dispatch()
                                                          │ WebSocket
                         sse.Hub ◄── Manager.runResultLoop ◄── WebSocketBus.results
                            │                          (readPump pushes to chan)
                         Worker process ◄── WebSocketSource.Receive()
Browser ◄── GET /events ───┘
```

- Store: `MemoryStore`; Backpressure: HTTP 503; Deadline loop: disabled
- Worker registration: `GET /ws/register` → `WebSocketBus.Register(conn)`

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

## Pattern 4: NATS + Redis (hybrid — NATS queue, Redis SSE fan-out)

```
Browser ──POST /tasks──► nginx ──► API replica ──► Manager ──► NATSRedisBus.Dispatch()
                           │                                           │ JetStream
                        Redis Pub/Sub ◄── NATSRedisSource.ReportResult/Progress
                      (PSubscribe progress:*                  (via redis.Publish)
                       task_status:*)
                              │
                    ALL API replicas: NATSRedisBus.Start() PSubscribes
                    Manager routes to hub + store

Browser ◄── GET /events ───┘ (any replica — no sticky sessions needed)
```

- Store: `RedisTaskStore`; Workers: NATS JetStream queue-subscribe; Deadline: 30 s re-dispatch
- nginx uses `resolver 127.0.0.11 valid=5s` + `set $upstream` for true round-robin
- Env: `NATS_URL`, `REDIS_ADDR`
