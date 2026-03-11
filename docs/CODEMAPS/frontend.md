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
- `htmx:afterSwap` triggers `syncCardState(taskID)`

## SSE Event Flow

`GET /events` → single user-scoped `EventSource` opened at page load (browser sends `user_id` cookie automatically); client demuxes by `taskID`.

| Event type | Payload fields | Handler | DOM effect |
|------------|---------------|---------|------------|
| `progress` | taskID, stageIdx, stageName, progress, status | `handleProgress()` | `.stage-dot` class, `.stage-progress-fill` width, `.stage-pct` text; `updateOverallProgress()` |
| `task_status` | taskID, status | `handleTaskStatus()` | `.task-card` class + `.badge` text/class |

- Single `EventSource` per browser session (not per task); never explicitly closed
- `syncCardState(taskID)` fetches `GET /tasks/:id` on card insertion to catch missed events
- Heartbeat: server sends `: heartbeat\n\n` every 15 s to keep connections alive
- UserID identity: `user_id` UUID cookie minted by `Index` handler on first visit; shared across tabs in same browser

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
