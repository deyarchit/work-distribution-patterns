<!-- Commit: 7fe066ab6730595a6c51680b8324893cbae27fa5 | Files scanned: 57 | Token estimate: ~730 -->

# Architecture

## Overview

Four patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
dispatch.TaskManager   Submit(ctx, task) error                                              — API → execution substrate
dispatch.TaskSource    Receive(ctx) (Task, ProgressSink, ResultSink, error)                 — worker pulls task + paired sinks
dispatch.ProgressSink  Publish(event)                                                       — stage progress (UX, best-effort)
dispatch.ResultSink    Record(taskID, status) error                                         — task status transitions (reliable)
store.TaskStore        Create/Get/List/SetStatus                                            — task persistence
```

`ProgressSink` and `ResultSink` are always returned paired with the task from `Receive`. Each transport type (`wsSink`, `NATSSink`, `RedisSink`) implements both, so a single value satisfies both interfaces. In P1 the caller constructs `poolResultSink` explicitly (no `TaskSource`).

## Pattern 1: Goroutine Pool (single process)

```
Browser ──POST /tasks──► shared/api ──► PoolTaskManager ──► pool.Pool
                                                                  │ goroutine
                         sse.Hub ◄──────── executor.Executor ◄───┘
                            │
Browser ◄── GET /events ───┘
```

- Store: `MemoryStore` (in-process)
- Backpressure: HTTP 429 when queue full
- Env: `WORKERS`, `QUEUE_SIZE`, `MAX_STAGE_DURATION`

## Pattern 2: WebSocket Hub (API + remote workers)

```
Browser ──POST /tasks──► shared/api ──► WSTaskManager ──► WorkerHub.Assign()
                                                                   │ WebSocket
                         sse.Hub ◄── WorkerHub.readPump() ◄── Worker process
                            │                                  (executor + WSSink)
Browser ◄── GET /events ───┘
```

- Store: `MemoryStore` (per API process — not shared across processes)
- Backpressure: HTTP 503 when no idle workers
- Worker registration: `GET /ws/register`

## Pattern 3: NATS JetStream (horizontally scaled)

```
Browser ──POST /tasks──► nginx ──► API replica ──► NATSTaskManager ──► JetStream "tasks.new"
                                       │                                        │
                          NATS Core ◄──┘                         Worker (NATSTaskSource + NATSSink)
                    (progress.* / task_status.*)                          executor.Run()
                              │
                    ALL API replicas subscribe
                    hub.Publish() / store.SetStatus()

Browser ◄── GET /events ───┘ (any replica — no sticky sessions needed)
```

- Store: `JetStreamStore` (NATS KV bucket — shared across all replicas)
- Workers: queue-subscribe on `tasks.new`; at-least-once delivery
- Env: `NATS_URL`

## Pattern 4: NATS + Redis (hybrid — NATS queue, Redis SSE fan-out)

```
Browser ──POST /tasks──► nginx ──► API replica ──► RedisTaskManager ──► JetStream "tasks.new"
                           │   (resolver 127.0.0.11;                          │
                           │    round-robins across                  Worker (NATSTaskSource + RedisSink)
                           │    all healthy replicas)               executor.Run()
                           │                                              │
                        Redis Pub/Sub ◄──────────────────────────── PUBLISH progress:<id>
                      (PSUBSCRIBE progress:*                        PUBLISH task_status:<id>
                       task_status:*)
                              │
                     ALL API replicas receive every event
                     hub.Publish() / store.SetStatus()

Browser ◄── GET /events ───┘ (any replica — no sticky sessions needed)
```

- Store: `RedisTaskStore` (Redis Strings + Set — shared across all replicas)
- Workers: NATS JetStream queue-subscribe on `tasks.new`; at-least-once delivery
- Workers publish progress to Redis Pub/Sub (`RedisSink`); API layer owns SSE routing
- nginx uses `resolver 127.0.0.11 valid=5s` + `set $upstream` variable for true round-robin
- Env: `NATS_URL`, `REDIS_ADDR`
