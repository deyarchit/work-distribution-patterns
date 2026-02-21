<!-- Commit: 5054cca620340baf0cbf3f62ec91c38a00d213b1 | Files scanned: 26 | Token estimate: ~680 -->

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
| `p02/internal/websocket` (pkg `wsinternal`) | `producer.go`, `consumer.go` | `WebSocketProducer` (TaskProducer): manages worker conns, readPump/writePump, single `events` chan; `WebSocketConsumer` (TaskConsumer): reconnect loop, `currentSend` indirection |
| `p03/internal/nats` (pkg `natsinternal`) | `producer.go`, `consumer.go`, `setup.go`, `store.go` | `NATSProducer`, `NATSConsumer`, JetStream setup, KV task store |

## API Routes (`shared/api`)

| Method | Path | Handler |
|--------|------|---------|
| POST | `/tasks` | `SubmitTask` — creates task, delegates to `TaskManager.Submit` |
| GET | `/tasks` | `ListTasks` — returns all tasks as JSON |
| GET | `/tasks/:id` | `GetTask` — returns single task as JSON |
| GET | `/events` | `SSEStream` — subscribes to `sse.Hub` (`?taskID=` for scoped; empty = global) |
| GET | `/health` | `Health` — returns `200 ok`; excluded from Echo request logs via skipper |
| GET | `/` | `Index` — serves HTMX page |
| GET | `/ws/register` | Pattern 2 only — worker WebSocket registration |

## Key Type Signatures

```go
// contracts/manager.go
type TaskManager interface { Submit(ctx context.Context, task models.Task) error }

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

// manager/manager.go
func New(s store.TaskStore, bus contracts.TaskProducer, hub *sse.Hub, deadline time.Duration) *Manager
func (m *Manager) Start(ctx context.Context)   // non-blocking; launches runEventLoop + deadline goroutines
func (m *Manager) Submit(ctx context.Context, task models.Task) error

// executor/executor.go
type Executor struct { MaxStageDuration time.Duration }
func (e *Executor) Run(ctx context.Context, task models.Task, sink contracts.EventSink)
// Emits: task_status=running → progress per stage → task_status=completed|failed

// models/task.go — unified event type
type TaskEvent struct {
    Type      string  // EventProgress | EventTaskStatus
    TaskID    string
    StageName string  // EventProgress only
    Progress  int     // 0–100, EventProgress only
    Status    string  // EventTaskStatus only
}
const EventProgress = "progress"; const EventTaskStatus = "task_status"

// store/store.go
type TaskStore interface {
    Create(task models.Task) error
    Get(id string) (models.Task, bool)
    List() []models.Task
    SetStatus(id string, status models.TaskStatus) error
}
```

## Pattern-Specific Notes

**Pattern 1** — `ChannelProducer`+`ChannelConsumer` share a single `events chan TaskEvent` (created together by `New`). Directional channel types enforce ownership at compile time. `RunWorker` loops: `Receive` → `exec.Run(ctx, task, source)`; executor handles all event emission. Deadline disabled (`deadline=0`).

**Pattern 2** — `WebSocketProducer.Dispatch` round-robins to idle `workerConn`; returns `ErrNoWorkers` if none. All message types are unexported and co-located in `internal/websocket` (package `wsinternal`). `WebSocketConsumer` uses `currentSend chan []byte` guarded by mutex; reconnect goroutine sets/clears it. Worker process calls `exec.Run` in a goroutine per task. Deadline disabled.

**Pattern 3** — `NATSProducer.Start` NATS Core-subscribes to a single `task.events.*` subject. `NATSConsumer.Emit` publishes to `task.events.<taskID>`. `NATSConsumer.Connect` queue-subscribes to `tasks.new` JetStream with manual ACK. Synchronous worker loop (one task at a time). Deadline 30 s.

