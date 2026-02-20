# Codemap Rationale Log

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
