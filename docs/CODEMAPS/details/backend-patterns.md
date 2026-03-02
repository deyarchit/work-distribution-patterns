<!-- Load when: Working on a specific pattern (P1–P6) wiring, transport implementation, or pattern-specific configuration -->

# Pattern-Specific Implementation Details

## Pattern 1 — Goroutine Pool (single process)

`ChannelDispatcher`+`ChannelConsumer` share a single `events chan TaskEvent` (created together by `New`). Directional channel types enforce ownership at compile time. `RunWorker` loops: `Receive` → `exec.Run(ctx, task, consumer)`; executor handles all event emission. Manager republishes events to MemoryBridge. Deadline disabled (`deadline=0`). Wiring: `p01/internal/app/app.go`. Integration test: `p01/integration_test.go` (single-process, no testcontainers needed).

## Pattern 2 — REST Polling (separate processes)

Three separate processes: API, Manager, Worker. `shared/client.RemoteTaskManager` (API-side) proxies `Submit/Get/List` over HTTP. Manager creates `MemoryBridge` and pumps it to SSE hub; API subscribes via `sse.Client`. Manager builds its Echo router manually (custom `POST /tasks` handler accepts full `models.Task` — API pre-creates the task with `models.NewTask`). `RESTDispatcher.HandleNext` (GET /work/next) does a non-blocking channel pop; workers poll at 500 ms idle / 2 s error backoff. Manager republishes events to MemoryBridge. Deadline disabled. Wiring: `p02/internal/app/{api,manager,worker}.go`. Integration test: `p02/integration_test.go` (uses testcontainers).

## Pattern 3 — WebSocket Hub (separate processes)

Three separate processes: API (:8080), Manager (:8081), Worker. API uses `shared/client.RemoteTaskManager`; Manager owns `WebSocketDispatcher`, `MemoryBridge`, and MemoryStore. Manager pumps `MemoryBridge` to SSE hub; API subscribes via `sse.Client`. `WebSocketDispatcher.Dispatch` round-robins to idle `workerConn`; returns `ErrNoWorkers` if none. `WebSocketConsumer` uses `currentSend chan []byte` guarded by mutex; reconnect goroutine sets/clears it. Worker process calls `exec.Run` in a goroutine per task. Manager republishes events to MemoryBridge. Deadline disabled. Wiring: `p03/internal/app/{api,manager,worker}.go`. Integration test: `p03/integration_test.go` (no testcontainers; starts 3 in-process worker goroutines — ⚠ must be exactly 3; `ConcurrentTasks` submits 3 simultaneous tasks and the busy flag means fewer workers cause a timeout). `WaitForWorker` required — ⚠ workers register asynchronously via WebSocket after HTTP server is ready.

## Pattern 4 — gRPC Bidirectional Streaming (separate processes)

Three separate processes: API (:8080), Manager (:8081), Worker. API uses `shared/client.RemoteTaskManager`; Manager owns `gRPCDispatcher`, `MemoryBridge`, and MemoryStore. Manager pumps `MemoryBridge` to SSE hub; API subscribes via `sse.Client`. `gRPCDispatcher.Start` listens for gRPC connections on configured address; `Dispatch` sends tasks over established streams. `gRPCConsumer` maintains persistent bidirectional stream with manager, emits events as gRPC messages. Manager republishes events to MemoryBridge. Deadline disabled. Uses protobuf-generated code from `work.proto`. Wiring: `p04/internal/app/{api,manager,worker}.go`. Integration test: `p04/integration_test.go` (no testcontainers; `NewManager` returns `ManagerComponents{Router, GRPCServer}` — ⚠ must start two separate listeners, both on random ports). `WaitForWorker` required — ⚠ gRPC stream registers asynchronously after HTTP server is ready.

## Pattern 5 — Queue-and-Store (horizontally scaled)

Three separate process types: API (:8080, ×3 replicas), Manager (:8081, ×1), Worker (×3). API uses `shared/client.RemoteTaskManager` — thin proxy only, no NATS/postgres. Manager owns NATS, postgres, SSE hub. `NATSDispatcher.Start` NATS Core-subscribes to `task.events.*`; `NATSConsumer.Emit` publishes to `task.events.<taskID>`. `NATSConsumer.Connect` queue-subscribes to `tasks.new` JetStream with manual ACK. Synchronous worker loop (one task at a time). Manager always republishes events to `NATSBridge` after processing worker events. Deadline 30 s. Store is `pgstore.Store` backed by PostgreSQL (`pgxpool`); schema (`tasks` table with JSONB `stages`) is created idempotently on startup. Wiring: `p05/internal/app/{api,manager,worker}.go`. Integration test: `p05/integration_test.go` (testcontainers: NATS via `tcnats.Run`, Postgres via `tcpostgres.Run`).

## Pattern 6 — Cloud-Agnostic PubSub (gocloud abstraction)

Three separate process types: API (:8080, ×3 replicas), Manager (:8081, ×1), Worker (×3). Uses `gocloud.dev/pubsub` abstraction with pluggable brokers (NATS JetStream, Kafka, AWS SNS/SQS). API uses `shared/client.RemoteTaskManager` + `CloudBridge` to subscribe to broker-specific event stream. Manager owns `CloudDispatcher`, `CloudBridge`, postgres.
  - **NATS/Kafka**: Two JetStream streams or Kafka topics: TASKS (WorkQueue/queue retention, persistent) and EVENTS (Interest/high-throughput, ephemeral). Durable consumers: manager (`manager-events`), workers (`workers` shared), APIs (ephemeral per instance).
  - **AWS**: Manager publishes tasks to SQS queue (`worker-tasks`), APIs dynamically create SQS queues subscribed to SNS topic (`api-events`) for fanout; workers consume shared SQS queue (load-balanced). Uses AWS SDK v2 + LocalStack for local testing.
  - Manager re-dispatches at 30 s deadline. Store: `pgstore.Store` (PostgreSQL). Wiring: `p06/internal/app/{api,manager,worker}.go`. Integration test: `p06/integration_test.go` (testcontainers: NATS + Postgres; pluggable broker selection via `BROKER` env).
