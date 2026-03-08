# Backend Codemap

## Package Roles

| Package | Role |
|---------|------|
| `shared/api` | HTTP routes, HTMX handlers |
| `shared/contracts` | `TaskManager`, `TaskDispatcher`, `TaskConsumer` + errors |
| `shared/manager` | Task lifecycle, deadline loop, event routing |
| `shared/executor` | Stage runner; emits via `TaskConsumer` |
| `shared/models` | `Task`, `Stage`, `TaskEvent`, `TaskStatus` |
| `shared/events` | `TaskEventBridge` + `MemoryBridge`, `NATSBridge` |
| `shared/sse` | SSE `Hub`, `Client` |
| `shared/store` | `TaskStore` + `MemoryStore` |
| `shared/templates` | Embedded HTMX |
| `shared/testutil` | Test helpers + `RunSuite` runner |
| `shared/client` | `RemoteTaskManager` HTTP proxy |
| `p0N/internal/app` | Per-pattern: `NewManager`, `NewAPI`, `RunWorker` |
| P1–P6 `internal/*` | Pattern-specific transport impls |
| `p07/internal/bootstrap` | Token vending, revocation, TLS loading |
| `p07/internal/pubsub` | Gocloud pubsub (from P6) |
| `p07/internal/postgres` | Postgres store (from P6) |

## API Routes

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| POST | `/tasks` | Create task → `TaskManager.Submit` |
| GET | `/tasks` | List → `TaskManager.List` |
| GET | `/tasks/:id` | Get → `TaskManager.Get` |
| GET | `/events` | SSE (scoped by `?taskID=`) |
| GET | `/health` | `200 ok` |
| GET | `/` | HTMX page |
| GET | `/ws/register` | P3 manager — WS registration |
| GET | `/work/next` | P2 manager — worker poll |
| POST | `/work/events` | P2 manager — worker events |
| GET | `/worker/bootstrap` | P7 mTLS only — ⚠ broker URL + token |

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
func (c *Client) Subscribe(ctx context.Context) (<-chan models.TaskEvent, error)
```

## Detailed References

| File | Load when… |
|------|-----------|
| [details/backend-patterns.md](./details/backend-patterns.md) | Working on P1–P6 wiring or transport implementations |
| [details/backend-p07-bootstrap.md](./details/backend-p07-bootstrap.md) | Working on P7 bootstrap, mTLS, token vending, or device revocation |
