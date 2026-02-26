# Codemap Rationale Log

## 8a14f57 — 2026-02-26
Commits: `3a7f9b2..8a14f57` (Add new pattern: Queues via gocloud abstraction)

### Decisions
- **Pattern 6: Cloud-Agnostic PubSub via gocloud.dev abstraction**: Extends P5 (NATS JetStream) by wrapping pub/sub behind `gocloud.dev/pubsub` interface. Same business logic; different transport abstraction. APIs and workers use identical code to work with AWS SNS/SQS, Google Pub/Sub, Azure Service Bus, or NATS JetStream. Demonstrates how cloud-agnostic patterns reduce vendor lock-in and cost.
- **Two JetStream streams**: TASKS (WorkQueue retention, file storage for reliability) and EVENTS (Interest retention, memory storage for ephemeral event fan-out). Separates durable queueing from transient messaging.
- **Durable consumers with separate strategies**: Manager uses `manager-events` consumer (durable, for crash recovery); workers share `workers` consumer (durable, work-stealing); API replicas use ephemeral consumers (one per instance, simple fan-out).
- **Transport URLs abstraction**: gocloud scheme `nats://localhost:4222/tasks.new?stream_name=TASKS` encapsulates JetStream specifics. Swapping NATS for another backend requires only URL + driver change in one place, not rewriting dispatcher/consumer logic.

## 3a7f9b2 — 2026-02-26
Commits: `473fff3..3a7f9b2` (Refactor TaskEventBus interface and implementations)

### Decisions
- **Updated `TaskEventBus` split to include P6**: Extended the Publisher/Subscriber/Bridge interface hierarchy (introduced in 9e8e54c) to P6's `CloudBridge` implementation. All six patterns now consistently use the split interface approach.
- **`CloudBridge` implements `TaskEventBridge`**: Like `MemoryBridge` (P1–P4) and `NATSBridge` (P5), the new `CloudBridge` satisfies both Publisher and Subscriber, enabling Manager dependency injection without caring about the underlying pub/sub implementation.

## 9e8e54c — 2026-02-25
Commits: `473fff3..9e8e54c` (Refactor TaskEventBus interface and implementations)

### Decisions
- **Split `TaskEventBus` into `TaskEventPublisher` and `TaskEventSubscriber`**: The original single interface mixed two concerns (publishing and subscribing). Splitting them enables Interface Segregation Principle: managers can accept just `TaskEventPublisher`, subscribers can depend on just `TaskEventSubscriber`. This decouples the two sides and simplifies dependency injection across patterns.
- **Introduced `TaskEventBridge` as the unified interface**: For components that need both publish and subscribe capabilities (e.g., `MemoryBridge`, `NATSBridge`), `TaskEventBridge` embeds both. This provides opt-in unification for implementations while maintaining fine-grained interfaces at the dependency level.
- **Updated all pattern managers and event implementations**: All `MemoryBridge`, `NATSBridge` implementations and their usages (`Manager.New` signatures) reflect the split interfaces, ensuring consistency across all five patterns.

## f5c7050 — 2026-02-24
Commits: `96111a2..f5c7050` (Add grpc pattern, rename pattern 04 → 05)

### Decisions
- **Add Pattern 4 (gRPC bidirectional streaming)**: Sits between P3 (WebSocket) and P5 (NATS+Postgres) in the architectural progression. gRPC provides: persistent bidirectional streams like WebSocket, but with HTTP/2 multiplexing; protobuf serialization for typed messages; stronger typing than raw TCP. Pattern demonstrates typed RPC-based work distribution — a middle ground between WebSocket's simplicity and NATS's queue guarantees.
- **Rename P4 → P5 (NATS JetStream pattern)**: "P4" is now gRPC, so the previous queue-and-store pattern (NATS+postgres) becomes P5. The progression is now: goroutines → REST → WebSocket → gRPC → NATS+postgres.
- **Protobuf schema for P4**: `work.proto` defines `Task`, `TaskEvent`, `DispatchRequest`, `EventStream` messages. Auto-generated `work.pb.go` and `work_grpc.pb.go` provide type safety and code generation benefits (compared to ad-hoc JSON over WebSocket).

## 756de21 — 2026-02-24
Commits: `f5c7050..756de21` (Simplify Manager and separate NATS subjects)

### Decisions
- **Remove conditional event republishing in Manager**: Removed the `republishWorkerEvents` flag from `Manager.New()`. The Manager now *always* republishes worker events to the event bus after processing and persisting them.
- **Separate NATS subjects for P5**:
    - **Worker-to-Manager**: Workers now emit to `worker.events.<taskID>`.
    - **Manager-to-API**: Manager republishes to `task.events.<taskID>`.
    This "Manager as Gateway" model ensures that all UI updates (SSE) reflect the state already persisted in the database, providing better consistency and decoupling.

## 4751c24 — 2026-02-24
Commits: `40d4c7d..4751c24` (Fix bug in manager for nats pattern)

### Decisions
- **Added `republishWorkerEvents` flag to `Manager.New()`**: (Legacy decision - superseded by 756de21) Used to control whether worker events are republished to the event bus. This has since been removed in favor of a unified republishing model with separate subjects.

## 89762b6 — 2026-02-24
Commits: `420cf39..89762b6` (Rename pattern folders, configure repomix, update codemaps)

### Decisions
- **Rename pattern directories for clarity**: `patterns/01-goroutine-pool` → `patterns/p01`, `patterns/02-rest-polling` → `patterns/p02`, etc. Shorthand names (`p01`, `p02`, `p03`, `p05`) reduce verbosity in Makefiles, Dockerfiles, and documentation while preserving the descriptive names in code comments and codemaps. This mirrors common naming in CLI tools and makes shell history/scripts more readable.

## 420cf39 — 2026-02-23
Commits: `0f2a79b..420cf39` (Remove polling; extract event bus abstraction)

### Decisions
- **Remove `TaskManager.Subscribe()` and extract `events.TaskEventBridge`**: Previously, `TaskManager` owned event subscription — remote APIs called `RemoteTaskManager.Subscribe()` which internally used SSE polling. This mixed task management with event transport. The new `TaskEventBridge` interface (split into `TaskEventPublisher` and `TaskEventSubscriber`) provides clean separation: managers publish to the bridge, wiring in `main.go` pumps it to the SSE hub; APIs subscribe via transport-specific clients or directly.

- **`shared/sse/client.go` replaces `RemoteTaskManager.Subscribe`**: P2/P3 APIs now use `sse.Client` to subscribe to manager's SSE endpoint. Reconnection logic (exponential backoff) is centralized in `Client.streamWithReconnect`, removing duplicated SSE parsing from `RemoteTaskManager`.

- **Pattern-specific event wiring in `main.go`**: P1–P4 use `MemoryBridge` + SSE; P5 uses `NATSBridge` + direct NATS subscription in API replicas. Event flow is now explicit and visible at startup, not buried in cross-process HTTP calls.

- **P5 API requires `NATS_URL`**: APIs subscribe directly to NATS `task.events.*` instead of polling manager's SSE endpoint, eliminating the manager as event bottleneck and enabling true distributed pub/sub.

## 0f2a79c — 2026-02-23
Commits: `0f2a79b..0f2a79c` (Rename TaskProducer → TaskDispatcher, update TaskConsumer terminology)

### Decisions
- **Rename `TaskProducer` → `TaskDispatcher`**: More accurately describes the manager's role in routing tasks to workers. "Producer" was technically correct (producing tasks for a queue) but "Dispatcher" aligns better with the `TaskManager`'s orchestrating role.

- **Prefer `TaskConsumer` over `TaskWorker`**: While "Worker" is a standard term for the execution entity, "Consumer" is the idiomatic term in message-based systems (especially NATS). Using `TaskDispatcher` + `TaskConsumer` provides a clear model: the manager dispatches work, and the consumer takes it from the transport to execute it.

- **Remove `EventSink` interface**: Previously, `TaskConsumer` satisfied `EventSink` via its `Emit` method. The `Executor` used `EventSink` to remain unaware of the other consumer methods (`Connect`, `Receive`). Removing this intermediate interface simplifies the `contracts` package and provides a single, unified view from the worker side. The `Executor` now accepts `TaskConsumer` directly.

- **Rename implementation types and variables for consistency**: `ChannelProducer` → `ChannelDispatcher`, `NATSConsumer` → `NATSWorker`, etc. Variable names like `source` and `bus` were renamed to `worker` and `dispatcher` throughout the codebase.

## 0f2a79b — 2026-02-23
Commits: `0617358..0f2a79b` (Separate manager process for P3 and P5; extract shared/client)

### Decisions
- **Extract `RemoteTaskManager` to `shared/client`**: Previously lived in `p02/internal/client`. P3 and P5 APIs now also use it (they were restructured to be thin proxies like P2). Moving it to `shared/client` avoids duplication and makes the shared transport contract explicit.

- **P3: Split API (:8080) and Manager (:8081) into separate processes**: Previously the P3 API owned the WebSocket hub and MemoryStore directly (single-process). Splitting to API+Manager+Worker makes P3's topology match P2 and P5 — each pattern cleanly separates "HTTP frontend" from "task orchestration". Workers now register to `ws://manager:8081/ws/register` instead of the API.

- **P5: API replicas become thin proxies (like P2/P3)**: Previously each API replica connected directly to NATS and postgres. All NATS and postgres ownership moved into the dedicated manager process. API replicas only hold `RemoteTaskManager` + a local `sse.Hub` fed by the manager's SSE stream. This eliminates connection pool multiplicity and clarifies responsibility boundaries.

## 0617358 — 2026-02-23
Commits: `a3caca0..0617358` (Pattern 2 REST polling, renumber patterns, expand TaskManager, API layer refactor)

### Decisions
- **Insert REST Polling as Pattern 2, renumber old P2→P3, P3→P5**: Introduces a more gradual architectural progression — goroutines → REST polling → WebSocket → NATS+Postgres. Each step adds exactly one new complexity (cross-process boundary, long-lived connections, persistent queuing).

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
Commits: `9d2e9eb..df20ceb` (+ uncommitted Pattern 5 work)

### Decisions
- **Pattern 5 renamed `04-redis-pubsub` → `04-nats-redis`**: The pattern was redesigned
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

- **NATS `nats.conf` for P3/P5**: NATS 2.12+ changed JetStream defaults — `Max Storage: 0 B`
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
  `deadline=0` disables the loop entirely — P1/P2 skip it; P3/P5 use 30 s.

- **`context.Background()` in P1 server (no signal handling)**: Adding `signal.NotifyContext`
  caused SIGTERM to be intercepted, keeping the Echo server alive and blocking port 8080
  between E2E test runs. Using `context.Background()` lets `pkill` kill the process
  immediately — acceptable for a single-process demo pattern.
