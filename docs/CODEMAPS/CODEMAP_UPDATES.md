# Codemap Updates — 2026-02-18

## Summary of Changes

The `simplify-interfaces` branch refactored the work distribution patterns to consolidate task lifecycle management into a single abstraction: **TaskManager**.

### Key Changes

#### 1. New TaskManager Interface (dispatch/manager.go)

**OLD**: `Dispatcher` interface with limited responsibility
```go
type Dispatcher interface {
  Submit(ctx context.Context, task models.Task) error
}
```

**NEW**: `TaskManager` interface with full lifecycle responsibility
```go
type TaskManager interface {
  Submit(ctx context.Context, task models.Task) error
}
```

The interface signature is identical, but the contract is now explicitly full-lifecycle:
- Persist task to store (Create)
- Dispatch work (pattern-specific: pool, WebSocket, NATS)
- Route progress from workers to SSE hub
- Persist terminal status (completed/failed) via SetStatus

#### 2. Executor.Run Now Returns TaskStatus

**OLD**: `Run(ctx, task, sink) error` — only reported errors
```go
func (e *Executor) Run(ctx context.Context, task models.Task, sink ProgressSink) error
```

**NEW**: `Run(ctx, task, sink) TaskStatus` — returns terminal status
```go
func (e *Executor) Run(ctx context.Context, task models.Task, sink ProgressSink) models.TaskStatus
```

Benefits:
- Managers can persist terminal state without wrapping the sink
- Clear signal: execution complete, here's the final status
- No task manager needs to interpret error codes

#### 3. TaskStore Simplified

**OLD**: `UpdateStage(id, idx, stage) error` — granular updates
**NEW**: Removed; only `SetStatus(id, status) error` remains

Rationale: Stages are updated via ProgressEvent events through the SSE hub; terminal status is the only thing persisted to the store.

#### 4. New Manager Implementations

Each pattern now implements `TaskManager` in its own package:

| Pattern | Implementation | File |
|---------|---|---|
| 01 | `PoolTaskManager` | `patterns/01-goroutine-pool/internal/pool/manager.go` |
| 02 | `WSTaskManager` | `patterns/02-websocket-hub/internal/api/manager.go` |
| 03 | `NATSTaskManager` | `patterns/03-nats-jetstream/internal/nats/manager.go` |

Each manager:
- Persists the task: `store.Create(task)`
- Dispatches it (pattern-specific)
- Handles failure cases with appropriate HTTP status codes
- Receives completion via pattern-specific means and calls `store.SetStatus(taskID, status)`

#### 5. API Routes Simplified

**OLD**: `SubmitTask(store, dispatcher)` — handler wired dispatcher and store
**NEW**: `SubmitTask(manager)` — handler uses manager as the seam

```go
// New signature
func SubmitTask(manager dispatch.TaskManager) echo.HandlerFunc {
  return func(c echo.Context) error {
    // ... create task ...
    if err := manager.Submit(c.Request().Context(), task); err != nil {
      return err
    }
    // ... render response ...
  }
}

// Called in api.NewRouter
func NewRouter(
  taskStore store.TaskStore,
  hub *sse.Hub,
  tpl *template.Template,
  manager dispatch.TaskManager,  // ← TaskManager is the seam
) *echo.Echo {
  // ...
  e.POST("/tasks", SubmitTask(manager))
  // ...
}
```

#### 6. Pattern Entry Points Updated

Each pattern's `main.go` now:
1. Creates the manager (PoolTaskManager, WSTaskManager, or NATSTaskManager)
2. Passes it to `api.NewRouter(..., manager)`
3. Manager handles the full task lifecycle

Example (Pattern 1):
```go
manager := pool.NewPoolTaskManager(p, hub, exec, taskStore)
e := api.NewRouter(taskStore, hub, tpl, manager)
```

### Deleted Files

- `shared/dispatch/dispatcher.go` — replaced by manager.go
- `shared/executor/sink.go` — ProgressSink interface moved to executor.go
- `patterns/01-goroutine-pool/internal/pool/dispatcher.go`
- `patterns/02-websocket-hub/internal/api/dispatcher.go`
- `patterns/03-nats-jetstream/internal/nats/dispatcher.go`

### Modified Files

Core shared packages:
- `shared/dispatch/manager.go` — new (interface + docs)
- `shared/executor/executor.go` — Run now returns TaskStatus
- `shared/models/task.go` — no changes
- `shared/store/store.go` — SetStatus only, no UpdateStage
- `shared/store/memory.go` — simplified
- `shared/api/handlers.go` — SubmitTask takes manager
- `shared/api/server.go` — NewRouter signature updated

Pattern-specific:
- `patterns/01-goroutine-pool/internal/pool/manager.go` — new
- `patterns/01-goroutine-pool/cmd/server/main.go` — uses NewPoolTaskManager
- `patterns/02-websocket-hub/internal/api/manager.go` — new
- `patterns/02-websocket-hub/cmd/api/main.go` — uses NewWSTaskManager
- `patterns/03-nats-jetstream/internal/nats/manager.go` — new
- `patterns/03-nats-jetstream/cmd/api/main.go` — uses NewNATSTaskManager

## Design Benefits

1. **Clear Separation**: TaskManager owns full lifecycle; shared/api owns just HTTP routing.
2. **Testability**: Each manager can be tested independently with mocks.
3. **Error Handling**: Back-pressure errors (429, 503) handled per-pattern without shared code.
4. **Extensibility**: New patterns just implement TaskManager; no changes to shared/api.
5. **Immutability**: Executor returns status; no mutable sink wrapping needed.

## Codemaps Updated

All codemaps refreshed to reflect:
- TaskManager as the central abstraction
- Full lifecycle flow in each pattern
- Simplified task lifecycle documentation
- Clear responsibility boundaries

Files updated:
- `docs/CODEMAPS/architecture.md`
- `docs/CODEMAPS/backend.md`
- `docs/CODEMAPS/frontend.md`
- `docs/CODEMAPS/dependencies.md`

## Next Steps

Run `make test-all` to verify all patterns still work correctly:
```bash
make test-all
```

This builds all binaries and runs the full E2E suite against all three patterns.
