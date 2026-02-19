<!-- Commit: 9d2e9ebb8ac2895ba48208a39da72b1b4d012efd | Files scanned: 2 | Token estimate: ~350 -->

# Frontend Codemap

## Stack

- **HTMX 1.9.12** (CDN) for form submission and fragment swapping
- **Vanilla JS** for SSE event handling and DOM mutation
- **Go `html/template`** with embedded FS (`shared/templates/embed.go`)

## Templates (`shared/templates/index.html`)

| Template | Usage |
|----------|-------|
| `index.html` | Full page shell; rendered by `GET /` |
| `task-card` | Task fragment; returned by `POST /tasks` (HTMX swap) |

## HTMX Integration

```html
<form hx-post="/tasks" hx-target="#task-list" hx-swap="afterbegin">
```

- Form POST triggers `SubmitTask` handler (detects `HX-Request: true`)
- Response is the `task-card` template fragment, prepended to `#task-list`
- `htmx:afterSwap` triggers `openTaskSSE(taskID)` and `syncCardState(taskID)`

## SSE Event Flow

```
GET /events?taskID=<id>  ─► per-task EventSource (stored in taskConnections Map)

  data: {"type":"stage_progress", taskID, stageIdx, stageName, progress, status}
       → handleStageProgress() → .stage-dot class, .stage-progress-fill width, .stage-pct text
                               → updateOverallProgress() (average of all stage fills)

  data: {"type":"task_status", taskID, status}
       → handleTaskStatus() → .task-card class + .badge text/class
                            → closeTaskSSE(taskID) on terminal status
```

- One `EventSource` per active task; closed on `completed` or `failed`
- `syncCardState(taskID)` fetches `GET /tasks/:id` on card insertion to catch missed events
- Heartbeat: server sends `: heartbeat\n\n` every 15 s to keep connections alive

## Card DOM Structure

```
.task-card#task-{id}  (.running | .completed | .failed)
  .task-header
    .task-name  /  .task-meta (short ID + time)
    .badge  (.pending | .running | .completed | .failed)
  .overall-progress > .overall-progress-bar
  .stages
    .stage-row[data-stage=N]  (one per stage)
      .stage-dot  /  .stage-name  /  .stage-progress-track > .stage-progress-fill  /  .stage-pct
```
