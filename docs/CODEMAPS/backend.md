<!-- Commit: 7fe066ab6730595a6c51680b8324893cbae27fa5 | Files scanned: 51 | Token estimate: ~720 -->

# Backend Codemap

## Package Roles

| Package | Path | Role |
|---------|------|------|
| `shared/api` | `handlers.go`, `server.go` | HTTP routes (`/tasks`, `/events`, `/health`), HTMX handlers |
| `shared/dispatch` | `manager.go`, `source.go`, `sink.go`, `result.go` | `TaskManager`, `TaskSource`, `ProgressSink`, `ResultSink` interfaces |
| `shared/executor` | `executor.go` | Stage runner; emits to `dispatch.ProgressSink`; returns `TaskStatus` |
| `shared/models` | `task.go` | `Task`, `Stage`, `ProgressEvent`, `TaskStatusEvent`, status enums |
| `shared/sse` | `hub.go` | SSE fan-out; implements `dispatch.ProgressSink` |
| `shared/store` | `store.go`, `memory.go` | `TaskStore` interface + `MemoryStore` |
| `shared/templates` | `embed.go`, `index.html` | Embedded HTMX template |
| `p01/internal/pool` | `pool.go`, `manager.go` | Bounded goroutine pool; `PoolTaskManager`; `poolResultSink` |
| `p02/internal/api` | `hub.go`, `manager.go`, `messages.go` | WebSocket worker hub; `WSTaskManager`; `StatusMsg`/`DoneMsg` |
| `p03/internal/nats` | `manager.go`, `source.go`, `setup.go`, `store.go` | NATS manager, `NATSTaskSource`+`NATSSink` (both sinks), KV store |
| `p04/internal/nats` | `setup.go`, `source.go` | JetStream setup + `NATSTaskSource` (injects `ProgressSink`+`ResultSink`) |
| `p04/internal/redis` | `manager.go`, `sink.go`, `store.go` | `RedisSink` (implements both sinks), Redis store, SSE fan-out manager |

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

// dispatch/source.go
type TaskSource interface { Receive(ctx context.Context) (models.Task, ProgressSink, ResultSink, error) }

// dispatch/sink.go — UX-only, best-effort
type ProgressSink interface { Publish(event models.ProgressEvent) }

// dispatch/result.go — reliable task-level status path
type ResultSink interface { Record(taskID string, status models.TaskStatus) error }

// executor/executor.go
type Executor struct { MaxStageDuration time.Duration }
func (e *Executor) Run(ctx, task, sink dispatch.ProgressSink) models.TaskStatus  // returns terminal status; caller calls Record

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
type TaskStatusEvent struct { TaskID string; Status TaskStatus }  // transport payload for task_status channels
```

## Pattern-Specific Notes

**Pattern 1** — `PoolTaskManager.Submit` enqueues a closure. `poolResultSink` wraps `sse.Hub` + `store`. Calls `Record(TaskRunning)` before `exec.Run` and `Record(status)` after. `sse.Hub` is the `ProgressSink` directly.

**Pattern 2** — `wsSink` implements both `ProgressSink` and `ResultSink`. `Receive` returns the task's paired `wsSink` as both sinks. `readPump` handles `status` (non-terminal) and `done` (terminal, uses `msg.Status`) WebSocket messages. Workers send `running` before `exec.Run` via `Record`.

**Pattern 3** — `NATSSink` implements both sinks. `NATSTaskSource` holds a `*NATSSink` injected at construction; returns it from `Receive` as both sinks. Manager subscribes to `task_status.*` using `models.TaskStatusEvent`. Workers call `Record(TaskRunning)` before `exec.Run` (synchronous, one task at a time).

**Pattern 4** — `RedisSink` implements both sinks. `NATSTaskSource` (P4) accepts `dispatch.ProgressSink`+`dispatch.ResultSink` interfaces at construction (no cross-package import). Manager subscribes to Redis `task_status:*` using `models.TaskStatusEvent`. Workers call `Record(TaskRunning)` before `exec.Run` (synchronous).
