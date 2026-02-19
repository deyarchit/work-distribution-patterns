<!-- Commit: 9d2e9ebb8ac2895ba48208a39da72b1b4d012efd | Files scanned: 43 | Token estimate: ~500 -->

# Architecture

## Overview

Three patterns demonstrating different work distribution topologies, all sharing the same HTTP API surface and HTMX frontend.

## Shared Interfaces

```
dispatch.TaskManager  Submit(ctx, task) error                         — API → execution substrate
dispatch.TaskSource   Receive(ctx) (Task, error)                      — worker pulls from transport
executor.ProgressSink Publish(event) / PublishTaskStatus(id, status)  — executor → transport
store.TaskStore       Create/Get/List/SetStatus                        — task persistence
```

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
