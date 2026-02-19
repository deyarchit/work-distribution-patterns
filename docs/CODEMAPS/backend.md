<!-- Commit: dbc0e450f41ec0f930cf88b8badcb7c47ca74646 | Files scanned: 25 | Token estimate: ~800 -->

# Backend

## Routes (shared/api)

```
POST /tasks          → SubmitTask(manager)
                         Bind submitRequest {name, stage_count}
                         Create Task via models.NewTask(name, stageCount)
                         manager.Submit(ctx, task)  ← TaskManager
                         HTMX: render "task-card" template fragment
                         JSON: {id: string}

GET  /tasks          → ListTasks(store)  → store.List()
GET  /tasks/:id      → GetTask(store)    → store.Get(id)
GET  /events         → SSEStream(hub)
                         ?taskID=<id>   → hub.Subscribe(taskID)  [scoped]
                         ?taskID=       → hub.Subscribe("")      [global]
                         streams: data: <json>\n\n + heartbeat every 15s
GET  /               → Index(tpl)       → index.html template
```

## Router Creation (`api.NewRouter`)

```
NewRouter(
  taskStore store.TaskStore,
  hub *sse.Hub,
  tpl *template.Template,
  manager dispatch.TaskManager   ← the seam
) *echo.Echo
```

Pattern implementations create the manager and pass it; `shared/api` has no pattern awareness.

## Middleware Chain

```
Echo.Logger → Echo.Recover → templateMiddleware(inject *template.Template)
```

## TaskManager Interface (dispatch/manager.go)

```
type TaskManager interface {
  Submit(ctx context.Context, task models.Task) error
}
```

Full lifecycle responsibility:
- Persist task to store (create)
- Dispatch work (pattern-specific: pool, WebSocket, NATS)
- Route progress from workers to SSE hub
- Persist terminal status (completed/failed)

## TaskSource Interface (dispatch/source.go)

```
type TaskSource interface {
  Receive(ctx context.Context) (models.Task, error)
}
```

Worker-side complement to TaskManager. Blocks until a task is available or ctx is cancelled.
Workers have no knowledge of stores, SSE, or browsers.

## Task Manager Implementations

| Pattern | Type | File | Mechanism |
|---------|------|------|-----------|
| 01 | `PoolTaskManager` | `patterns/01-goroutine-pool/internal/pool/manager.go` | Pool.Enqueue(fn) → runs on worker goroutine → calls Executor → hub broadcasts → SetStatus |
| 02 | `WSTaskManager` | `patterns/02-websocket-hub/internal/api/manager.go` | WorkerHub.Assign(task) → sends TaskMsg over WebSocket → worker executes → hub broadcasts → WorkerHub.readPump updates SetStatus |
| 03 | `NATSTaskManager` | `patterns/03-nats-jetstream/internal/nats/manager.go` | js.Publish("tasks.new") → worker consumes → Executor → natsSink publishes progress/status → API-side subscriptions update hub & store |

## Task Source Implementations

| Pattern | Type | File | Mechanism |
|---------|------|------|-----------|
| 02 | `WSTaskSource` | `patterns/02-websocket-hub/internal/worker/source.go` | Reconnect loop + WebSocket read; Sink() returns connection-scoped wsSink |
| 03 | `NATSTaskSource` | `patterns/03-nats-jetstream/internal/nats/source.go` | JetStream QueueSubscribe("tasks.new"); ACKs after task handed to Receive() |

## SSE Hub (`shared/sse/hub.go`)

```
Hub.taskSubs   map[taskID → set<chan []byte>>   // per-task subscribers
Hub.globalSubs set<chan []byte>                  // "" taskID → all events

Subscribe(taskID) → (chan []byte, unsub func())
Publish(ProgressEvent)            → broadcast(event.TaskID, data)
PublishTaskStatus(taskID, status) → broadcast(taskID, data)
broadcast(taskID, data)           → fan-out to taskSubs[taskID] + globalSubs
                                    non-blocking send; drops slow consumers
```

## Store (`shared/store`)

```
TaskStore interface
  Create(task)           error
  Get(id)                (Task, bool)
  List()                 []Task
  SetStatus(id, status)  error

MemoryStore — sync.RWMutex-guarded map[string]Task
  store/memory.go — Pattern 1 & 2 use this; Pattern 3 uses JetStreamStore (KV bucket)
```

## Executor (`shared/executor/executor.go`)

```
ProgressSink interface {
  Publish(ProgressEvent)
  PublishTaskStatus(id, status)
}

Executor struct {
  MaxStageDuration time.Duration   // set from MAX_STAGE_DURATION env (milliseconds)
}

Run(ctx, task, sink) TaskStatus:
  PublishTaskStatus(running)
  for each stage:
    pick random tickDuration in [0, MaxStageDuration] / 10
    Publish(running, 0%)
    10 ticks × sleep(tickDuration) → Publish(running, tick*10%)
    Publish(completed, 100%)
  PublishTaskStatus(completed)
  ctx.Done() at any tick → PublishTaskStatus(failed); return TaskFailed

  Returns TaskStatus (TaskCompleted or TaskFailed) for manager to persist
```

## Pattern 2 Worker (`patterns/02-websocket-hub/internal/worker/source.go`)

```
WSTaskSource — implements dispatch.TaskSource
  Connect(ctx)    — reconnect loop (exponential backoff up to 10s)
  Receive(ctx)    — blocks until task arrives on WebSocket
  Sink()          — returns wsSink for most recently received task

wsSink — implements executor.ProgressSink
  Publish(ProgressEvent)          → ProgressMsg over WebSocket
  PublishTaskStatus(taskID, status) → DoneMsg only on terminal status (completed/failed)

Worker main loop:
  go source.Connect(ctx)
  for { task = source.Receive(ctx); go exec.Run(ctx, task, source.Sink()) }
```

## Pattern 2 API WorkerHub (`patterns/02-websocket-hub/internal/api/hub.go`)

```
WorkerHub.Assign(task) — round-robin, skips busy workers
WorkerConn: writePump (WS write), readPump (WS read → sse.Hub.Publish)
Messages:
  TaskMsg:      {type: "task", task: Task}      — API → worker
  ProgressMsg:  {type: "progress", event: ProgressEvent}  — worker → API
  DoneMsg:      {type: "done", taskID: string}  — worker → API (terminal)
  ReadyMsg:     {type: "ready"}                 — worker announces readiness

readPump: receives DoneMsg, calls hub.PublishTaskStatus + store.SetStatus
```

## Pattern 3 NATS (`patterns/03-nats-jetstream/internal/nats/`)

```
setup.go:
  JetStream stream: "TASKS"  subjects: "tasks.>"  retention: WorkQueuePolicy
  KV bucket:        "task-store"  TTL: 24h
  Consumer durable: "workers"  — shared queue for all worker instances

source.go (NATSTaskSource):
  QueueSubscribe("tasks.new", "workers") → ACK after task handed to Receive()
  Compile-time: var _ dispatch.TaskSource = (*NATSTaskSource)(nil)

source.go (NATSSink — implements executor.ProgressSink):
  Publish(event)              → nc.Publish("progress.<taskID>", json)
  PublishTaskStatus(id, status) → nc.Publish("task_status.<id>", json)

manager.go (API-side NATS subscriptions, all API replicas):
  progress.* → json → hub.Publish(event)
  task_status.* → json → hub.PublishTaskStatus(taskID, status) + store.SetStatus(taskID, status)
```
