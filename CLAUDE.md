# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Exploring the Project

Start with the codemaps in `docs/CODEMAPS/` for a quick orientation before navigating files:

| Codemap | Contents |
|---|---|
| `docs/CODEMAPS/architecture.md` | System design, pattern topologies, data flow |
| `docs/CODEMAPS/backend.md` | Package roles, key types, interface contracts |
| `docs/CODEMAPS/frontend.md` | HTMX frontend, SSE integration, templates |
| `docs/CODEMAPS/dependencies.md` | External dependencies and why each is used |

Use `tree .` for the full nested file structure when needed.

## Commands

```bash
# Run Pattern 1 locally (no Docker)
make run-p1

# Run Pattern 2 with Docker Compose (1 API + 3 workers)
make run-p2 / make stop-p2

# Run Pattern 3 with Docker Compose (3 APIs + 3 workers + NATS + nginx)
make run-p3 / make stop-p3

# Build all five binaries into bin/
make build-all

# Build + E2E tests against all three patterns (full validation)
make test-all

# E2E tests against a specific already-running server
BASE_URL=http://localhost:8080 make test-e2e

# Run a single E2E test
BASE_URL=http://localhost:8080 go test ./tests/e2e/... -v -run TestSingleTask

# Load test
BASE_URL=http://localhost:8080 make test-load

# Tidy modules
make tidy
```

## Adding a New Pattern

1. Create `patterns/0N-name/` with `cmd/` entrypoints and `internal/` implementation.
2. Implement `dispatch.Dispatcher` — that is the only contract with `shared/api`.
3. Wire `shared/api.NewServer(dispatcher, store, hub)` in `main.go`.
4. Wire `executor.Executor` to emit into `ProgressSink` (either `sse.Hub` directly or an adapter).
5. Add `run-pN` / `stop-pN` / build targets to `Makefile`.

## After Every Code Change or Feature Implementation

After completing any code change session or feature implementation, verify that all tests pass:

```bash
make test-all
```

This builds all binaries and runs the full E2E suite against all three patterns in sequence. All three patterns must pass before the work is considered complete.

## Key Design Constraints

- `shared/api` must not import any pattern-specific package.
- `StageDurationSecs` lives on the `Task` struct — do not add it as a separate parameter to `Dispatcher.Submit`.
- `sse.Hub` drops events for slow consumers (non-blocking send) — do not change this to blocking.
- `MemoryStore` is the only store implementation; it is not safe to share across processes (Patterns 2/3 store state only in the API process that received the task, or in NATS KV for Pattern 3).
