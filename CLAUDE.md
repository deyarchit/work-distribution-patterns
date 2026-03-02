# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Exploring the Project

**CRITICAL:** Codemaps in `docs/CODEMAPS/` are the authoritative, token-efficient source of truth for this repository. Follow these rules exactly:

1. **At the start of every session, read every file found under `docs/CODEMAPS/` (including subdirectories) — do not skip any.**
2. **Answer directly from codemap knowledge. Do not read source files for questions the codemaps already cover.** Only read source files when the codemaps explicitly do not cover the topic, or when the user asks for implementation-level details beyond what the codemaps provide. **Do not use Glob, Grep, or Read on source files to "verify" or "supplement" codemap answers — this is wasteful and violates the codemap-first principle.**

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

# 4. Full unit test suite — all patterns must pass before work is done
make test
```
