# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Exploring the Project

Codemaps in `docs/CODEMAPS/` are token-efficient summaries — load only the ones relevant to your task and keep them in context throughout. Prefer them over broad file exploration.

| Task type | Codemap(s) to load |
|---|---|
| Understanding overall system design, how patterns differ, or data flow | `docs/CODEMAPS/architecture.md` |
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

# 4. Full E2E suite — all three patterns must pass before work is done
make test-all
```

For small, isolated changes (e.g. a comment or doc edit) you may skip steps 3–4, but always run `make fmt` and `make lint` first. Never consider a change complete while lint warnings or test failures remain.