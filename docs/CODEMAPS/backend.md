<!-- Commit: 5154530 | Files scanned: 75 | Token estimate: ~1200 -->

# Backend Codemap

## Package Roles

| Package | Path | Role |
|---------|------|------|
| `shared/api` | `handlers.go`, `server.go` | HTTP routes (`/tasks`, `/events`, `/health`), HTMX handlers |
| `shared/contracts` | `manager.go`, `dispatcher.go`, `consumer.go` | `TaskManager`, `TaskDispatcher`, `TaskConsumer` interfaces + sentinel errors |
| `shared/manager` | `manager.go` | Unified `Manager`: task lifecycle, deadline loop, single `runEventLoop` conditionally routes worker events to event bus (true for MemoryEventBus, false for NATS) |
| `shared/executor` | `executor.go` | Stage runner; emits all events (running/progress/terminal) via `contracts.TaskConsumer`; returns nothing |
| `shared/models` | `task.go` | `Task`, `Stage`, `TaskEvent`, `TaskStatus`, event-type constants |
| `shared/events` | `events.go`, `memory.go`, `nats.go` | `TaskEventBridge` interface (split into Publisher/Subscriber); `MemoryBridge` (P1–P4), `NATSBridge` (P5) |
| `shared/sse` | `hub.go`, `client.go` | `Hub`: SSE fan-out; `Client`: SSE subscriber (used by P2/P3 APIs) |
| `shared/store` | `store.go`, `memory.go` | `TaskStore` interface + `MemoryStore` |
| `shared/templates` | `embed.go`, `index.html` | Embedded HTMX template |
| `p01/internal/goroutine` | `dispatcher.go`, `consumer.go` | `ChannelDispatcher`+`ChannelConsumer` (single shared `events chan TaskEvent`); constructed together by `New` |
| `p01/internal/worker` | `worker.go` | `RunWorker`: `Receive` → `exec.Run(ctx, task, consumer)` loop |
| `shared/client` (pkg `client`) | `manager.go` | `RemoteTaskManager`: implements `contracts.TaskManager` by proxying Submit/Get/List over HTTP. Used by P2, P3, and P5 APIs. |
| `p02/internal/rest` (pkg `rest`) | `dispatcher.go`, `consumer.go` | `RESTDispatcher` (TaskDispatcher): buffered chan + HTTP handlers `/work/next`, `/work/events`; `RESTConsumer` (TaskConsumer): polling loop |
| `p03/internal/websocket` (pkg `wsinternal`) | `dispatcher.go`, `consumer.go` | `WebSocketDispatcher` (TaskDispatcher): manages worker conns, readPump/writePump, single `events` chan; `WebSocketConsumer` (TaskConsumer): reconnect loop, `currentSend` indirection |
| `p04/internal/grpc` (pkg `grpc`) | `client.go`, `server.go`, `dispatcher.go`, `consumer.go`, `converter.go` | `gRPCDispatcher` (TaskDispatcher): bidirectional stream management; `gRPCConsumer` (TaskConsumer): gRPC client; protobuf types in `proto/work.proto` |
| `p04/proto` | `work.proto`, `work.pb.go`, `work_grpc.pb.go` | Protocol buffer definitions for gRPC messages; auto-generated bindings |
| `p05/internal/nats` (pkg `natsinternal`) | `dispatcher.go`, `consumer.go`, `setup.go` | `NATSDispatcher`, `NATSConsumer`, JetStream stream setup |
| `p05/internal/postgres` (pkg `pgstore`) | `store.go` | `Store`: PostgreSQL-backed `TaskStore`; schema auto-created on startup via `New(ctx, pool)` |
| `p06/internal/pubsub` (pkg `pubsubinternal`) | `dispatcher.go`, `consumer.go`, `bridge.go`, `setup.go`, `aws.go` | `CloudDispatcher`, `CloudConsumer`, `CloudBridge`; gocloud.dev/pubsub abstraction; broker-agnostic setup for NATS/Kafka/AWS; `openAWSAPISubscription` dynamically creates SQS queues for APIs |
| `p06/internal/postgres` (pkg `pgstore`) | `store.go` | `Store`: PostgreSQL-backed `TaskStore` (same as P5) |

## API Routes (`shared/api`)

| Method | Path | Handler |
|--------|------|---------|
| POST | `/tasks` | `SubmitTask` — creates task via `models.NewTask`, delegates to `TaskManager.Submit` |
| GET | `/tasks` | `ListTasks(manager)` — proxies to `TaskManager.List(ctx)` |
| GET | `/tasks/:id` | `GetTask(manager)` — proxies to `TaskManager.Get(ctx, id)` |
| GET | `/events` | `SSEStream` — subscribes to `sse.Hub` (`?taskID=` for scoped; empty = global) |
| GET | `/health` | `Health` — returns `200 ok`; excluded from Echo request logs via skipper |
| GET | `/` | `Index` — serves HTMX page |
| GET | `/ws/register` | Pattern 3 only — worker WebSocket registration |
| GET | `/work/next` | Pattern 2 manager only — worker polls for next task |
| POST | `/work/events` | Pattern 2 manager only — worker posts progress/status events |

## Key Type Signatures

```go
// contracts/manager.go
type TaskManager interface {
    Submit(ctx context.Context, task models.Task) error
    Get(ctx context.Context, id string) (models.Task, bool)
    List(ctx context.Context) []models.Task
}

// contracts/dispatcher.go — manager-side transport view
type TaskDispatcher interface {
    Start(ctx context.Context) error
    Dispatch(ctx context.Context, task models.Task) error
    ReceiveEvent(ctx context.Context) (models.TaskEvent, error)  // blocks
}
var ErrDispatchFull = errors.New("dispatch queue full")   // → 429
var ErrNoWorkers    = errors.New("no workers available")  // → 503

// contracts/consumer.go — worker-side transport view
type TaskConsumer interface {
    Connect(ctx context.Context) error
    Receive(ctx context.Context) (models.Task, error)            // blocks
    Emit(ctx context.Context, event models.TaskEvent) error
}

// events/events.go
type TaskEventPublisher interface { Publish(models.TaskEvent) }
type TaskEventSubscriber interface { Subscribe(context.Context) (<-chan models.TaskEvent, error) }
type TaskEventBridge interface { TaskEventPublisher; TaskEventSubscriber }

// shared/api/server.go
func NewRouter(hub *sse.Hub, tpl *template.Template, manager contracts.TaskManager) *echo.Echo

// manager/manager.go
func New(s store.TaskStore, d contracts.TaskDispatcher, evs events.TaskEventPublisher, deadline time.Duration) *Manager
func (m *Manager) Start(ctx context.Context)   // non-blocking; launches runEventLoop + deadline goroutines
func (m *Manager) Submit(ctx context.Context, task models.Task) error
func (m *Manager) Get(_ context.Context, id string) (models.Task, bool)
func (m *Manager) List(_ context.Context) []models.Task

// sse/client.go
func (c *Client) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)  // SSE streaming with auto-reconnect
```

## Pattern-Specific Notes

**Pattern 1** — `ChannelDispatcher`+`ChannelConsumer` share a single `events chan TaskEvent` (created together by `New`). Directional channel types enforce ownership at compile time. `RunWorker` loops: `Receive` → `exec.Run(ctx, task, consumer)`; executor handles all event emission. Manager republishes events to MemoryBridge. Deadline disabled (`deadline=0`).

**Pattern 2** — Three separate processes: API, Manager, Worker. `shared/client.RemoteTaskManager` (API-side) proxies `Submit/Get/List` over HTTP. Manager creates `MemoryBridge` and pumps it to SSE hub; API subscribes via `sse.Client`. Manager builds its Echo router manually (custom `POST /tasks` handler accepts full `models.Task` — API pre-creates the task with `models.NewTask`). `RESTDispatcher.HandleNext` (GET /work/next) does a non-blocking channel pop; workers poll at 500 ms idle / 2 s error backoff. Manager republishes events to MemoryBridge. Deadline disabled.

**Pattern 3** — Three separate processes: API (:8080), Manager (:8081), Worker. API uses `shared/client.RemoteTaskManager`; Manager owns `WebSocketDispatcher`, `MemoryBridge`, and MemoryStore. Manager pumps `MemoryBridge` to SSE hub; API subscribes via `sse.Client`. `WebSocketDispatcher.Dispatch` round-robins to idle `workerConn`; returns `ErrNoWorkers` if none. `WebSocketConsumer` uses `currentSend chan []byte` guarded by mutex; reconnect goroutine sets/clears it. Worker process calls `exec.Run` in a goroutine per task. Manager republishes events to MemoryBridge. Deadline disabled.

**Pattern 4** — Three separate processes: API (:8080), Manager (:8081), Worker. API uses `shared/client.RemoteTaskManager`; Manager owns `gRPCDispatcher`, `MemoryBridge`, and MemoryStore. Manager pumps `MemoryBridge` to SSE hub; API subscribes via `sse.Client`. `gRPCDispatcher.Start` listens for gRPC connections on configured address; `Dispatch` sends tasks over established streams. `gRPCConsumer` maintains persistent bidirectional stream with manager, emits events as gRPC messages. Manager republishes events to MemoryBridge. Deadline disabled. Uses protobuf-generated code from `work.proto`.

**Pattern 5** — Three separate process types: API (:8080, ×3 replicas), Manager (:8081, ×1), Worker (×3). API uses `shared/client.RemoteTaskManager` — thin proxy only, no NATS/postgres. Manager owns NATS, postgres, SSE hub. `NATSDispatcher.Start` NATS Core-subscribes to `task.events.*`; `NATSConsumer.Emit` publishes to `task.events.<taskID>`. `NATSConsumer.Connect` queue-subscribes to `tasks.new` JetStream with manual ACK. Synchronous worker loop (one task at a time). Manager always republishes events to `NATSBridge` after processing worker events. Deadline 30 s. Store is `pgstore.Store` backed by PostgreSQL (`pgxpool`); schema (`tasks` table with JSONB `stages`) is created idempotently on startup.

**Pattern 6** — Three separate process types: API (:8080, ×3 replicas), Manager (:8081, ×1), Worker (×3). Uses `gocloud.dev/pubsub` abstraction with pluggable brokers (NATS JetStream, Kafka, AWS SNS/SQS). API uses `shared/client.RemoteTaskManager` + `CloudBridge` to subscribe to broker-specific event stream. Manager owns `CloudDispatcher`, `CloudBridge`, postgres.
  - **NATS/Kafka**: Two JetStream streams or Kafka topics: TASKS (WorkQueue/queue retention, persistent) and EVENTS (Interest/high-throughput, ephemeral). Durable consumers: manager (`manager-events`), workers (`workers` shared), APIs (ephemeral per instance).
  - **AWS**: Manager publishes tasks to SQS queue (`worker-tasks`), APIs dynamically create SQS queues subscribed to SNS topic (`api-events`) for fanout; workers consume shared SQS queue (load-balanced). Uses AWS SDK v2 + LocalStack for local testing.
  - Manager re-dispatches at 30 s deadline. Store: `pgstore.Store` (PostgreSQL).
