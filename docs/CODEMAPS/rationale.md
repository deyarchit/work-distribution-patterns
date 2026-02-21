# Codemap Rationale Log

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
