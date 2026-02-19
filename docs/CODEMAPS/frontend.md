<!-- Commit: dbc0e450f41ec0f930cf88b8badcb7c47ca74646 | Files scanned: 25 | Token estimate: ~400 -->

# Frontend

Single embedded HTMX + vanilla JS page. No build step.

## Template Files

```
shared/templates/embed.go      — go:embed index.html; exposes var FS embed.FS
shared/templates/index.html    — defines two Go templates:
  {{define "index.html"}}      — full page
  {{define "task-card"}}       — HTMX fragment (rendered server-side on POST /tasks)
```

## Page Structure

```
<body>
  .container
    h1 "Work Distribution Patterns"
    .form-card
      <form hx-post="/tasks" hx-target="#task-list" hx-swap="afterbegin">
        input[name=name]         Task Name
        input[name=stage_count]  Stages 1–8
        button[submit]
    .tasks-header
      h2 "Tasks"
      #sse-status               ● Ready | ● Connected | ● Disconnected
    #task-list
      .task-card#task-<uuid>    (one per submitted task)
        .task-header  .badge[status]
        .overall-progress → .overall-progress-bar
        .stages → .stage-row[data-stage=N]
          .stage-dot  .stage-name  .stage-progress-track → .stage-progress-fill  .stage-pct
```

## JavaScript — SSE Connection Management

```
taskConnections: Map<taskID, EventSource>

openTaskSSE(taskID)        — opens /events?taskID=<id>; no-op if already open
closeTaskSSE(taskID)       — closes + deletes; shows "● Ready" when last closes
updateSSEStatus(connected) — drives #sse-status (true=Connected, false=Disconnected)

htmx:afterSwap on #task-list:
  taskID = first inserted card's id
  openTaskSSE(taskID)     ← scoped SSE connection
  syncCardState(taskID)   ← GET /tasks/:id to catch any missed events
```

## JavaScript — SSE Event Handlers

```
onmessage → JSON.parse → dispatch by ev.type:
  "stage_progress" → handleStageProgress(ev)
    card = #task-<ev.taskID>
    row  = [data-stage=ev.stageIdx]
    update: .stage-dot class, .stage-progress-fill width+class, .stage-pct text
    updateOverallProgress(card)
  "task_status" → handleTaskStatus(ev)
    update: .task-card class, .badge class+text
    updateOverallProgress(card)
    if completed|failed → closeTaskSSE(ev.taskID)
```

## SSE Status States

| Label | Class | Trigger |
|-------|-------|---------|
| ● Ready | `connected` (green) | page load; last task SSE closes normally |
| ● Connected | `connected` (green) | any task SSE opens (onopen) |
| ● Disconnected | `disconnected` (red) | onerror with no remaining connections |

## CSS Classes

```
.task-card[.running|.completed|.failed]  — border color
.badge[.pending|.running|.completed|.failed]
.stage-dot[.running|.completed|.failed]
.stage-progress-fill[.completed|.failed]
#sse-status[.connected|.disconnected]
```
