# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Exploring the Project

**CRITICAL:** Codemaps in `docs/CODEMAPS/` are the authoritative, token-efficient source of truth for this repository. **Always load the relevant codemaps as your first action** when starting a new task, before performing broad file exploration or grep searches.

| Task type | Codemap(s) to load |
|---|---|
| Understanding overall system design, how patterns differ, or data flow | `docs/CODEMAPS/architecture.md` |
| Deep dive into architectural decisions and trade-offs | `docs/CODEMAPS/rationale.md` |
| Working on Go backend code: packages, types, interfaces, dispatch, executor | `docs/CODEMAPS/backend.md` |
| Working on UI, SSE streaming, or HTMX templates | `docs/CODEMAPS/frontend.md` |
| Adding or auditing external libraries | `docs/CODEMAPS/dependencies.md` |
| Adding a new pattern | `docs/CODEMAPS/architecture.md` + `docs/CODEMAPS/backend.md` |
| Debugging a full request path (API → dispatch → SSE) | all four codemaps |

Use `tree .` for the full nested file structure when needed.

## Code Change Workflow

After any non-trivial code change, run these steps in order:

```bash
# 1. Format code to match repository style
make fmt

# 2. Lint — must pass with zero warnings
make lint

# 3. Build all binaries to catch compilation errors across patterns
make build-all

# 4. Full E2E suite — all patterns must pass before work is done
make test-all
```

For small, isolated changes (e.g. a comment or doc edit) you may skip steps 3–4, but always run `make fmt` and `make lint` first. Never consider a change complete while lint warnings or test failures remain.

## Testing a Specific Pattern

When working on a single pattern and you want faster iteration without running the full `test-all` suite, use this workflow:

```bash
# Start the pattern (in foreground, shows logs with --tail=30)
make run-p<num>

# In a separate terminal, run E2E tests against it
make test-e2e

# Stop the pattern
make stop-p<num>
```

For example, to test pattern 5:
```bash
make run-p5       # Terminal 1: start services
make test-e2e     # Terminal 2: run tests
make stop-p5      # Terminal 1: cleanup
```

This is useful during active development on a specific pattern. Before merging, always run the full `make test-all` suite to ensure all patterns still work correctly.