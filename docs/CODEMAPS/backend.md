# Backend Codemap

## Package Roles

| Package | Role |
|---------|------|
| `shared/api` | HTTP routes, HTMX handlers |
| `shared/contracts` | Interface definitions: `TaskManager`, `TaskDispatcher`, `TaskConsumer`; sentinel errors |
| `shared/manager` | Task lifecycle, deadline loop, event bus routing |
| `shared/executor` | Stage runner; emits events via `TaskConsumer` |
| `shared/models` | `Task`, `Stage`, `TaskEvent`, `TaskStatus` types |
| `shared/events` | `TaskEventBridge` + implementations: `MemoryBridge` (P1–P4), `NATSBridge` (P5) |
| `shared/sse` | SSE `Hub` (server), `Client` (subscriber) |
| `shared/store` | `TaskStore` interface + `MemoryStore` |
| `shared/templates` | Embedded HTMX template |
| `shared/testutil` | Test helpers (`PostTask`, `ListTasks`, etc.); `RunSuite` integration runner |
| `shared/client` | `RemoteTaskManager`: HTTP proxy to manager (P2–P6 APIs) |
| `p0N/internal/app` | Wiring: `NewManager`, `NewAPI`, `RunWorker` per pattern |
| P1–P6 `internal/*` | Pattern-specific `TaskDispatcher`/`TaskConsumer` implementations |

## API Routes

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| POST | `/tasks` | `SubmitTask` | Creates task, delegates to `TaskManager.Submit` |
| GET | `/tasks` | `ListTasks` | Proxies to `TaskManager.List` |
| GET | `/tasks/:id` | `GetTask` | Proxies to `TaskManager.Get` |
| GET | `/events` | `SSEStream` | Subscribes to SSE hub (`?taskID=` scoped; empty = global) |
| GET | `/health` | `Health` | Returns `200 ok`; logs skipped |
| GET | `/` | `Index` | Serves HTMX page |
| GET | `/ws/register` | P3 manager only — worker WebSocket registration |
| GET | `/work/next` | P2 manager only — worker polls for next task |
| POST | `/work/events` | P2 manager only — worker posts events |

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

## Detailed References

| File | Load when… |
|------|-----------|
| [details/backend-patterns.md](./details/backend-patterns.md) | Working on a specific pattern (P1–P6) wiring, transport implementation, or pattern-specific configuration |
