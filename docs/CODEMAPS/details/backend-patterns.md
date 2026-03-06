<!-- Load when: Working on a specific pattern (P1–P6) wiring, transport implementation, or pattern-specific configuration -->

# Pattern-Specific Implementation Details

## Pattern 1 — Goroutine Pool (single process)

- **Transport**: `ChannelDispatcher`/`ChannelConsumer` share single `events chan`.
- **Deadline**: Disabled. **Wiring**: `p01/internal/app/app.go`.

## Pattern 2 — REST Polling (separate processes)

- **API**: HTTP proxy via `RemoteTaskManager`. **Manager**: `POST /tasks` handler accepts full `models.Task` (API pre-creates). **Worker**: Polls `GET /work/next` at 500 ms idle / 2 s error backoff.
- **Deadline**: Disabled. **Wiring**: `p02/internal/app/{api,manager,worker}.go`.

## Pattern 3 — WebSocket Hub (separate processes)

- **Transport**: `WebSocketDispatcher` round-robins to idle workers; `WebSocketConsumer` uses `currentSend chan` guarded by mutex.
- **Gotcha**: ⚠ Exactly 3 workers required; workers register asynchronously. **Deadline**: Disabled. **Wiring**: `p03/internal/app/{api,manager,worker}.go`.

## Pattern 4 — gRPC Bidirectional Streaming (separate processes)

- **Transport**: `gRPCDispatcher.Start` listens on configured gRPC address; `gRPCConsumer` maintains persistent bidirectional stream.
- **Gotcha**: ⚠ `NewManager` returns `{Router, GRPCServer}` — start two separate listeners. **Deadline**: Disabled. **Wiring**: `p04/internal/app/{api,manager,worker}.go`.

## Pattern 5 — Queue-and-Store (horizontally scaled)

- **Transport**: NATS JetStream. **API**: Thin proxy (`RemoteTaskManager`). **Manager/Store**: PostgreSQL + NATS. **Worker**: One task at a time (queue-subscribe `tasks.new`).
- **Deadline**: 30 s. **Store**: `pgstore.Store` (Postgres JSONB `stages` table). **Wiring**: `p05/internal/app/{api,manager,worker}.go`.

## Pattern 6 — Cloud-Agnostic PubSub (gocloud abstraction)

- **Transport**: `gocloud.dev/pubsub` (pluggable: NATS JetStream, Kafka, AWS SNS/SQS via broker URL scheme).
- **Deadline**: 30 s. **Store**: `pgstore.Store` (PostgreSQL). **Wiring**: `p06/internal/app/{api,manager,worker}.go`.
- **NATS/Kafka**: Two streams/topics (TASKS persistent, EVENTS ephemeral); durable consumers (manager, shared workers, per-instance APIs).
- **AWS**: Manager→SQS worker queue, APIs→SNS topic (fanout), load-balanced workers. Uses SDK v2 + LocalStack.
