<!-- Commit: 0617358258f210256f7fed182c9f649941ee2c33 | Files scanned: 38 | Token estimate: ~820 -->

# Backend Codemap

## Package Roles

| Package | Path | Role |
|---------|------|------|
| `shared/api` | `handlers.go`, `server.go` | HTTP routes (`/tasks`, `/events`, `/health`), HTMX handlers |
| `shared/contracts` | `manager.go`, `producer.go`, `consumer.go`, `sink.go` | `TaskManager`, `TaskProducer`, `TaskConsumer`, `EventSink` interfaces + sentinel errors |
| `shared/manager` | `manager.go` | Unified `Manager`: task lifecycle, deadline loop, single `runEventLoop` routes events to store/hub |
| `shared/executor` | `executor.go` | Stage runner; emits all events (running/progress/terminal) via `contracts.EventSink`; returns nothing |
| `shared/models` | `task.go` | `Task`, `Stage`, `TaskEvent`, `TaskStatus`, event-type constants |
| `shared/sse` | `hub.go` | SSE fan-out; `Publish(TaskEvent)` and `PublishTaskStatus` |
| `shared/store` | `store.go`, `memory.go` | `TaskStore` interface + `MemoryStore` |
| `shared/templates` | `embed.go`, `index.html` | Embedded HTMX template |
| `p01/internal/goroutine` | `producer.go`, `consumer.go` | `ChannelProducer`+`ChannelConsumer` (single shared `events chan TaskEvent`); constructed together by `New` |
| `p01/internal/worker` | `worker.go` | `RunWorker`: `Receive` → `exec.Run(ctx, task, source)` loop |
| `p02/internal/rest` (pkg `rest`) | `producer.go`, `consumer.go` | `RESTProducer` (TaskProducer): buffered chan + HTTP handlers `/work/next`, `/work/events`; `RESTConsumer` (TaskConsumer): polling loop |
| `p02/internal/client` (pkg `client`) | `manager.go` | `RemoteTaskManager`: implements `contracts.TaskManager` by proxying HTTP calls to manager process; `sseLoop` goroutine for cross-process event subscription |
| `p03/internal/websocket` (pkg `wsinternal`) | `producer.go`, `consumer.go` | `WebSocketProducer` (TaskProducer): manages worker conns, readPump/writePump, single `events` chan; `WebSocketConsumer` (TaskConsumer): reconnect loop, `currentSend` indirection |
| `p04/internal/nats` (pkg `natsinternal`) | `producer.go`, `consumer.go`, `setup.go` | `NATSProducer`, `NATSConsumer`, JetStream stream setup |
| `p04/internal/postgres` (pkg `pgstore`) | `store.go` | `Store`: PostgreSQL-backed `TaskStore`; schema auto-created on startup via `New(ctx, pool)` |

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
    Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)
}

// contracts/producer.go — manager-side transport view
type TaskProducer interface {
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

// contracts/sink.go — executor emits via this; TaskConsumer satisfies it automatically
type EventSink interface { Emit(ctx context.Context, event models.TaskEvent) error }

// shared/api/server.go
func NewRouter(hub *sse.Hub, tpl *template.Template, manager contracts.TaskManager) *echo.Echo

// manager/manager.go
func New(s store.TaskStore, bus contracts.TaskProducer, hub *sse.Hub, deadline time.Duration) *Manager
func (m *Manager) Start(ctx context.Context)   // non-blocking; launches runEventLoop + deadline goroutines
func (m *Manager) Submit(ctx context.Context, task models.Task) error
func (m *Manager) Get(_ context.Context, id string) (models.Task, bool)
func (m *Manager) List(_ context.Context) []models.Task
func (m *Manager) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)
```

## Pattern-Specific Notes

**Pattern 1** — `ChannelProducer`+`ChannelConsumer` share a single `events chan TaskEvent` (created together by `New`). Directional channel types enforce ownership at compile time. `RunWorker` loops: `Receive` → `exec.Run(ctx, task, source)`; executor handles all event emission. Deadline disabled (`deadline=0`).

**Pattern 2** — Three separate processes: API, Manager, Worker. `RemoteTaskManager` (API-side) proxies `Submit/Get/List` over HTTP and `Subscribe` via SSE pump. Manager builds its Echo router manually (custom `POST /tasks` handler accepts full `models.Task` — API pre-creates the task with `models.NewTask`). `RESTProducer.HandleNext` (GET /work/next) does a non-blocking channel pop; workers poll at 500 ms idle / 2 s error backoff. Deadline disabled.

**Pattern 3** — `WebSocketProducer.Dispatch` round-robins to idle `workerConn`; returns `ErrNoWorkers` if none. All message types are unexported and co-located in `internal/websocket` (package `wsinternal`). `WebSocketConsumer` uses `currentSend chan []byte` guarded by mutex; reconnect goroutine sets/clears it. Worker process calls `exec.Run` in a goroutine per task. Deadline disabled.

**Pattern 4** — `NATSProducer.Start` NATS Core-subscribes to `task.events.*`; `NATSConsumer.Emit` publishes to `task.events.<taskID>`. `NATSConsumer.Connect` queue-subscribes to `tasks.new` JetStream with manual ACK. Synchronous worker loop (one task at a time). Deadline 30 s. Store is `pgstore.Store` backed by PostgreSQL (`pgxpool`); schema (`tasks` table with JSONB `stages`) is created idempotently on startup.
