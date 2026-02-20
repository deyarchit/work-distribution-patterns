<!-- Commit: 394144da8e51a3a4b807c8913f7bca4ab40e5b8e | Files scanned: 58 | Token estimate: ~680 -->

# Backend Codemap

## Package Roles

| Package | Path | Role |
|---------|------|------|
| `shared/api` | `handlers.go`, `server.go` | HTTP routes (`/tasks`, `/events`, `/health`), HTMX handlers |
| `shared/dispatch` | `manager.go`, `bus.go`, `worker_source.go`, `sink.go` | `TaskManager`, `WorkerBus`, `WorkerSource`, `ProgressSink` interfaces + sentinel errors |
| `shared/manager` | `manager.go` | Unified `Manager`: task lifecycle, deadline loop, routes bus events to store/hub |
| `shared/executor` | `executor.go` | Stage runner; emits to `dispatch.ProgressSink`; returns `TaskStatus` |
| `shared/models` | `task.go` | `Task`, `Stage`, `ProgressEvent`, `TaskStatusEvent`, status enums |
| `shared/sse` | `hub.go` | SSE fan-out; implements `dispatch.ProgressSink` |
| `shared/store` | `store.go`, `memory.go` | `TaskStore` interface + `MemoryStore` |
| `shared/templates` | `embed.go`, `index.html` | Embedded HTMX template |
| `p01/internal/bus` | `bus.go`, `worker.go` | `ChannelBus` (both `WorkerBus`+`WorkerSource`); `RunWorker`+`progressSink` |
| `p02/internal/bus` | `bus.go` | `WebSocketBus` (WorkerBus): manages worker conns; readPump/writePump |
| `p02/internal/worker` | `source.go` | `WebSocketSource` (WorkerSource): reconnect loop, `currentSend` indirection |
| `p03/internal/nats` | `bus.go`, `source.go`, `setup.go`, `store.go` | `NATSBus`, `NATSSource`, JetStream setup, KV task store |
| `p04/internal/bus` | `bus.go` | `NATSRedisBus` (WorkerBus): NATS JetStream dispatch + Redis PSubscribe |
| `p04/internal/worker` | `source.go` | `NATSRedisSource` (WorkerSource): NATS JetStream pull + Redis Publish |
| `p04/internal/nats` | `setup.go` | JetStream stream setup (unchanged) |
| `p04/internal/redis` | `store.go` | `RedisTaskStore` (unchanged) |

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
// dispatch/manager.go
type TaskManager interface { Submit(ctx context.Context, task models.Task) error }

// dispatch/bus.go — manager-side transport view
type WorkerBus interface {
    Start(ctx context.Context) error
    Dispatch(ctx context.Context, task models.Task) error
    ReceiveResult(ctx context.Context) (models.TaskStatusEvent, error)   // blocks
    ReceiveProgress(ctx context.Context) (models.ProgressEvent, error)  // blocks
}
var ErrDispatchFull = errors.New("dispatch queue full")   // → 429
var ErrNoWorkers    = errors.New("no workers available")  // → 503

// dispatch/worker_source.go — worker-side transport view
type WorkerSource interface {
    Connect(ctx context.Context) error
    Receive(ctx context.Context) (models.Task, error)   // blocks
    ReportResult(ctx context.Context, taskID string, status models.TaskStatus) error
    ReportProgress(ctx context.Context, event models.ProgressEvent) error
}

// dispatch/sink.go — UX-only, best-effort
type ProgressSink interface { Publish(event models.ProgressEvent) }

// manager/manager.go
func New(s store.TaskStore, bus dispatch.WorkerBus, hub *sse.Hub, deadline time.Duration) *Manager
func (m *Manager) Start(ctx context.Context)   // non-blocking; launches result/progress/deadline goroutines
func (m *Manager) Submit(ctx context.Context, task models.Task) error

// executor/executor.go
type Executor struct { MaxStageDuration time.Duration }
func (e *Executor) Run(ctx, task, sink dispatch.ProgressSink) models.TaskStatus

// store/store.go
type TaskStore interface {
    Create(task models.Task) error
    Get(id string) (models.Task, bool)
    List() []models.Task
    SetStatus(id string, status models.TaskStatus) error
}
```

## Pattern-Specific Notes

**Pattern 1** — `ChannelBus` implements both `WorkerBus` and `WorkerSource` (same process, shared channels). `RunWorker` loops: `Receive` → `exec.Run` → `ReportResult`. Deadline disabled (`deadline=0`). `progressSink` adapter forwards to `source.ReportProgress`.

**Pattern 2** — `WebSocketBus.Dispatch` round-robins to idle `workerConn`; returns `ErrNoWorkers` if none. Message types unexported in each package (bus-side and worker-side) with matching JSON fields. `WebSocketSource` uses `currentSend chan []byte` guarded by mutex; reconnect goroutine sets/clears it. Deadline disabled.

**Pattern 3** — `NATSBus.Start` NATS Core-subscribes to `progress.*`/`task_status.*`. `NATSSource.Connect` queue-subscribes to `tasks.new` JetStream with manual ACK. Synchronous worker loop (one task at a time). Deadline 30 s.

**Pattern 4** — `NATSRedisBus.Start` uses Redis PSubscribe on `progress:*`/`task_status:*`. `NATSRedisSource.Connect` NATS JetStream queue-subscribes. Results/progress via `rdb.Publish`. Store is `RedisTaskStore`. Deadline 30 s.
