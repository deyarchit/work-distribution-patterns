<!-- Generated: 2026-02-18 | Files scanned: 27 | Token estimate: ~550 -->

# Backend

## Routes (shared/api)

```
POST /tasks          → SubmitTask(store, dispatcher)
                         Bind submitRequest {name, stage_count}
                         Create Task (uuid, stages 1–8)
                         store.Create → dispatcher.Submit
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

## Middleware Chain

```
Echo.Logger → Echo.Recover → templateMiddleware(inject *template.Template)
```

## Dispatcher Implementations

| Pattern | File | Mechanism | Back-pressure |
|---------|------|-----------|---------------|
| 01 | `patterns/01-goroutine-pool/internal/pool/dispatcher.go` | `Pool.Enqueue(fn)` | HTTP 429 |
| 02 | `patterns/02-websocket-hub/internal/api/dispatcher.go` | `WorkerHub.Assign(task)` | HTTP 503 |
| 03 | `patterns/03-nats-jetstream/internal/nats/dispatcher.go` | `js.Publish("tasks.new", payload)` | NATS flow control |

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
  UpdateStage(id, idx, stage) error

MemoryStore — sync.RWMutex-guarded map[string]Task
  store/memory.go
```

## Executor (`shared/executor/executor.go`)

```
ProgressSink interface { Publish(ProgressEvent); PublishTaskStatus(id, status) }
Executor.StageDuration  time.Duration   // configurable via STAGE_DURATION_SECS

Run(ctx, task, sink):
  PublishTaskStatus(running)
  for each stage:
    Publish(running, 0%)
    10 ticks × sleep(StageDuration/10) → Publish(running, tick*10%)
    Publish(completed, 100%)
  PublishTaskStatus(completed)
  ctx.Done() at any tick → PublishTaskStatus(failed)
```

## Pattern 2 Worker Hub (`patterns/02-websocket-hub/internal/api/hub.go`)

```
WorkerHub.Assign(task) — round-robin, skips busy workers
WorkerConn: writePump (WS write), readPump (WS read → sse.Hub.Publish)
Messages: MsgTypeTask | MsgTypeProgress | MsgTypeDone | MsgTypeReady
```

## Pattern 3 NATS Setup (`patterns/03-nats-jetstream/internal/nats/setup.go`)

```
JetStream stream: "TASKS"  subjects: "tasks.>"  retention: WorkQueuePolicy
KV bucket:        "task-store"  TTL: 24h
Consumer durable: "workers"
Progress subjects: "progress.<taskID>"  (NATS Core, not JetStream)
```
