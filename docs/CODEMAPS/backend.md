<!-- Commit: 9d2e9ebb8ac2895ba48208a39da72b1b4d012efd | Files scanned: 43 | Token estimate: ~600 -->

# Backend Codemap

## Package Roles

| Package | Path | Role |
|---------|------|------|
| `shared/api` | `handlers.go`, `server.go` | HTTP routes, HTMX fragment handlers |
| `shared/dispatch` | `manager.go`, `source.go` | `TaskManager` and `TaskSource` interfaces |
| `shared/executor` | `executor.go` | Stage runner; emits to `ProgressSink` |
| `shared/models` | `task.go` | `Task`, `Stage`, `ProgressEvent`, status enums |
| `shared/sse` | `hub.go` | SSE fan-out; implements `ProgressSink` |
| `shared/store` | `store.go`, `memory.go` | `TaskStore` interface + `MemoryStore` |
| `shared/templates` | `embed.go`, `index.html` | Embedded HTMX template |
| `p01/internal/pool` | `pool.go`, `manager.go` | Bounded goroutine pool; `PoolTaskManager` |
| `p02/internal/api` | `hub.go`, `manager.go`, `messages.go` | WebSocket worker hub; `WSTaskManager` |
| `p03/internal/nats` | `manager.go`, `source.go`, `setup.go`, `store.go` | NATS manager, source+sink, KV store |

## API Routes (`shared/api`)

| Method | Path | Handler |
|--------|------|---------|
| POST | `/tasks` | `SubmitTask` — creates task, delegates to `TaskManager.Submit` |
| GET | `/tasks` | `ListTasks` — returns all tasks as JSON |
| GET | `/tasks/:id` | `GetTask` — returns single task as JSON |
| GET | `/events` | `SSEStream` — subscribes to `sse.Hub` (`?taskID=` for scoped; empty = global) |
| GET | `/` | `Index` — serves HTMX page |
| GET | `/ws/register` | Pattern 2 only — worker WebSocket registration |

## Key Type Signatures

```go
// dispatch/manager.go
type TaskManager interface { Submit(ctx context.Context, task models.Task) error }

// dispatch/source.go
type TaskSource interface { Receive(ctx context.Context) (models.Task, error) }

// executor/executor.go
type ProgressSink interface {
    Publish(event models.ProgressEvent)
    PublishTaskStatus(taskID string, status models.TaskStatus)
}
type Executor struct { MaxStageDuration time.Duration }
func (e *Executor) Run(ctx, task, sink) models.TaskStatus   // 10 ticks per stage

// store/store.go
type TaskStore interface {
    Create(task models.Task) error
    Get(id string) (models.Task, bool)
    List() []models.Task
    SetStatus(id string, status models.TaskStatus) error
}

// models/task.go
type Task struct { ID, Name string; Status TaskStatus; SubmittedAt time.Time; CompletedAt *time.Time; Stages []Stage }
type Stage struct { Index int; Name string; Status StageStatus; Progress int }
type ProgressEvent struct { TaskID string; StageIdx int; StageName string; Progress int; Status StageStatus }
```

## Pattern-Specific Notes

**Pattern 1** — `PoolTaskManager.Submit` enqueues a closure; `Executor.Run` uses `sse.Hub` directly as `ProgressSink`.

**Pattern 2** — `WorkerHub.readPump` is the receive side: `ProgressMsg` → `sseHub.Publish`; `DoneMsg` → `sseHub.PublishTaskStatus` + `store.SetStatus`. Workers report via WebSocket JSON messages.

**Pattern 3** — `NATSTaskManager` subscribes to `progress.*` and `task_status.*` on NATS Core at startup; all API replicas receive all worker events. `JetStreamStore` uses a NATS KV bucket for cross-replica state.
