# Codemap Rationale Log

## 0f2a79c — 2026-02-23
Commits: `0f2a79b..0f2a79c` (Rename TaskProducer → TaskDispatcher, update TaskConsumer terminology)

### Decisions
- **Rename `TaskProducer` → `TaskDispatcher`**: More accurately describes the manager's role in routing tasks to workers. "Producer" was technically correct (producing tasks for a queue) but "Dispatcher" aligns better with the `TaskManager`'s orchestrating role.

- **Prefer `TaskConsumer` over `TaskWorker`**: While "Worker" is a standard term for the execution entity, "Consumer" is the idiomatic term in message-based systems (especially NATS). Using `TaskDispatcher` + `TaskConsumer` provides a clear model: the manager dispatches work, and the consumer takes it from the transport to execute it.

- **Remove `EventSink` interface**: Previously, `TaskConsumer` satisfied `EventSink` via its `Emit` method. The `Executor` used `EventSink` to remain unaware of the other consumer methods (`Connect`, `Receive`). Removing this intermediate interface simplifies the `contracts` package and provides a single, unified view from the worker side. The `Executor` now accepts `TaskConsumer` directly.

- **Rename implementation types and variables for consistency**: `ChannelProducer` → `ChannelDispatcher`, `NATSConsumer` → `NATSWorker`, etc. Variable names like `source` and `bus` were renamed to `worker` and `dispatcher` throughout the codebase.

## 0f2a79b — 2026-02-23
Commits: `0617358..0f2a79b` (Separate manager process for P3 and P4; extract shared/client)

### Decisions
- **Extract `RemoteTaskManager` to `shared/client`**: Previously lived in `p02/internal/client`. P3 and P4 APIs now also use it (they were restructured to be thin proxies like P2). Moving it to `shared/client` avoids duplication and makes the shared transport contract explicit.

- **P3: Split API (:8080) and Manager (:8081) into separate processes**: Previously the P3 API owned the WebSocket hub and MemoryStore directly (single-process). Splitting to API+Manager+Worker makes P3's topology match P2 and P4 — each pattern cleanly separates "HTTP frontend" from "task orchestration". Workers now register to `ws://manager:8081/ws/register` instead of the API.

- **P4: API replicas become thin proxies (like P2/P3)**: Previously each API replica connected directly to NATS and postgres. All NATS and postgres ownership moved into the dedicated manager process. API replicas only hold `RemoteTaskManager` + a local `sse.Hub` fed by the manager's SSE stream. This eliminates connection pool multiplicity and clarifies responsibility boundaries.

## 0617358 — 2026-02-23
Commits: `a3caca0..0617358` (Pattern 2 REST polling, renumber patterns, expand TaskManager, API layer refactor)

### Decisions
- **Insert REST Polling as Pattern 2, renumber old P2→P3, P3→P4**: Introduces a more gradual architectural progression — goroutines → REST polling → WebSocket → NATS+Postgres. Each step adds exactly one new complexity (cross-process boundary, long-lived connections, persistent queuing).

- **Expand `contracts.TaskManager` with `Get/List/Subscribe`**: The API layer should never access the store directly — all reads flow through the manager. This discipline is essential for Pattern 2 where API and manager are separate processes. `Subscribe` streams `TaskEvent` from the manager's hub over SSE, letting the API pump events into its local hub without the manager knowing about the API.

- **Pattern 2 manager uses custom `POST /tasks` handler (not `api.NewRouter`)**: `api.NewRouter` uses `SubmitTask` which creates a new Task from `{name, stage_count}`. The API process already called `models.NewTask(...)` — the manager must accept a fully-formed Task to preserve the UUID. Using `api.NewRouter` would create a second task with a different ID.
  ```go
  // manager: accept full Task, not submitRequest
  e.POST("/tasks", func(c echo.Context) error { c.Bind(&task); mgr.Submit(ctx, task) })
  ```

- **`RemoteTaskManager.sseLoop` uses a separate `http.Client{}` with no timeout**: The main client has a 10 s timeout (correct for request/response calls). An SSE connection is intentionally long-lived — applying a timeout would disconnect it.

## e927fc3 — 2026-02-20
Commits: `5054cca..e927fc3` (+ uncommitted rename + PostgreSQL work)

### Decisions
- **Pattern 3 renamed `03-nats-jetstream` → `03-queue-and-store`**: The name reflects the
  two-component separation — NATS JetStream for queuing, PostgreSQL for state — rather than
  pinning to a single technology.

- **NATS KV replaced with PostgreSQL (`pgx/v5`)**: NATS KV is a convenience layer on top of
  JetStream; it adds a 24 h TTL (tasks expire) and lacks query, backup, and migration
  tooling. PostgreSQL is the natural fit for persistent, durable task state. NATS now has a
  single responsibility: work distribution (JetStream) and event fan-out (NATS Core).

- **Ephemeral Postgres container (no named volume)**: Omitting the volume from the Postgres
  service means `docker compose down` wipes the database. Each `docker compose up` starts
  clean — by design for repeatable E2E test runs.

- **Schema created inline on startup (`New(ctx, pool)`)**: A single `CREATE TABLE IF NOT
  EXISTS` keeps the setup self-contained without requiring a migration tool or init script.

## df20ceb — 2026-02-19
Commits: `9d2e9eb..df20ceb` (+ uncommitted Pattern 4 work)

### Decisions
- **Pattern 4 renamed `04-redis-pubsub` → `04-nats-redis`**: The pattern was redesigned
  to use NATS JetStream for API→Worker task delivery (at-least-once, same as Pattern 3)
  and Redis Pub/Sub only for the SSE fan-out layer within the API tier. Workers use
  `RedisSink` to publish progress directly to Redis; all API replicas PSubscribe and
  deliver to their local SSE hubs. This gives Redis a single clear responsibility
  (cross-replica SSE routing) while NATS owns work distribution.

- **nginx `resolver 127.0.0.11` + `set $upstream` variable**: Fixes the Pattern 3 bug
  where nginx resolves `api:8080` once at startup and caches one container IP. Using a
  variable forces per-connection DNS resolution via Docker's internal resolver, achieving
  true round-robin across all healthy replicas.
  ```nginx
  resolver 127.0.0.11 valid=5s ipv6=off;
  set $upstream http://api:8080;
  proxy_pass $upstream;
  ```

- **`RedisTaskStore` uses Set + String**: `tasks:all` Set holds IDs for `List()` without
  a full scan; each task is a JSON String with 24 h TTL — mirrors the JetStreamStore KV
  approach from Pattern 3.

## 7fe066a — 2026-02-20
Commits: `40c16d7..7fe066a`

### Decisions
- **`ProgressSink` / `ResultSink` split**: The original `executor.ProgressSink` conflated
  stage progress (best-effort UX) with task-level status (reliable, determines final state).
  Splitting into two interfaces in `dispatch` lets readers immediately see which path is
  reliable and which is fire-and-forget. `executor` now has no task-status responsibility.

- **`ResultSink.Record` not `Publish`**: Go prohibits a type having two methods with the
  same name and different signatures. Transport types (`wsSink`, `NATSSink`, `RedisSink`)
  implement both interfaces simultaneously — `Record` prevents a name collision with
  `ProgressSink.Publish`.

- **`Receive` returns `(Task, ProgressSink, ResultSink, error)`**: Eliminated the
  `WSTaskSource.Sink()` side-channel (implicit ordering: call `Sink()` immediately after
  `Receive` or get wrong sink). Returning sinks directly from `Receive` makes the pairing
  enforced by the type system rather than by convention.

- **`DoneMsg` `Status` field + `StatusMsg` type (P2 bug fix)**: `readPump` was hardcoding
  `models.TaskCompleted` for all `done` messages — failed tasks appeared as completed.
  Added `Status` field to `DoneMsg`; added `StatusMsg` for non-terminal `running` status.

- **NATS `nats.conf` for P3/P4**: NATS 2.12+ changed JetStream defaults — `Max Storage: 0 B`
  without explicit config. Running with `-js` flag alone disables file storage. Explicit
  `nats.conf` with `max_file_store: 1GB` and a named Docker volume is required.

## 394144d — 2026-02-20
Commits: `7fe066a..394144d`

### Decisions
- **Three-layer architecture (`WorkerBus` / `WorkerSource`)**: Replaced four bespoke
  `*TaskManager` structs with a single `shared/manager.Manager`. The variation between
  patterns is fully captured by two interfaces: `WorkerBus` (manager-side) and
  `WorkerSource` (worker-side). Sentinel errors (`ErrDispatchFull`, `ErrNoWorkers`) are
  returned from `Dispatch` and mapped to HTTP 429/503 in one place.

- **`ChannelBus` implements both sides (P1)**: In a single-process pattern, splitting
  into separate bus/source types adds no value. One struct with three channels satisfies
  both interfaces; `Start`/`Connect` are no-ops.

- **Deadline loop in shared Manager**: Re-dispatch logic (scan non-terminal tasks, re-enqueue
  if `now - dispatchTime > deadline`) lives once in `Manager.runDeadlineLoop`. Passing
  `deadline=0` disables the loop entirely — P1/P2 skip it; P3/P4 use 30 s.

- **`context.Background()` in P1 server (no signal handling)**: Adding `signal.NotifyContext`
  caused SIGTERM to be intercepted, keeping the Echo server alive and blocking port 8080
  between E2E test runs. Using `context.Background()` lets `pkill` kill the process
  immediately — acceptable for a single-process demo pattern.
