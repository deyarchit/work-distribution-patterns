---
name: update-codemaps
description: Incrementally update architecture codemaps using git diffs since the last processed commit
tools: ["Bash", "Read", "Edit", "Write", "Glob", "Grep"]
model: haiku
---

# Update Codemaps

Incrementally update architecture codemaps using git diffs since the last processed commit.

## Step 1: Collect Change Context

Run the helper script to collect all git context in one shot:

```bash
bash ./scripts/collect-diff.sh
```

This script will:
- Extract the last processed commit SHA from `.reports/codemap-diff.txt`
- Check if there are new commits since last run
- Exit early if codemaps are up to date
- Collect commit log and file changes
- Output structured metadata, commits, changed files, and diff stats

If the script exits with "Codemaps are up to date", stop here.

## Step 2: Analyse What Changed

From the diff, identify:
- Files added / removed / renamed
- Interface, type, class, or function signature changes
- New packages, modules, or entry points
- Configuration or environment variable changes
- Container / build tooling topology changes
- New external dependencies

## Step 3: Update Codemaps

Read only the codemaps affected by the identified changes and apply targeted edits — change only the sections that reflect the diff. Do not rewrite unaffected sections. Keep each codemap under 1000 tokens.

| Codemap | Trigger |
|---------|---------|
| `docs/CODEMAPS/architecture.md` | interface/contract changes, new components, data-flow changes |
| `docs/CODEMAPS/backend.md` | new/removed source files or packages, API route changes |
| `docs/CODEMAPS/frontend.md` | changes in UI templates, client-side logic, or component structure |
| `docs/CODEMAPS/dependencies.md` | dependency manifest changes, new env vars, container/build changes |

### Rationale Log (optional)

Only if the diff contains decisions that materially affect architecture or interfaces — append a new entry to `docs/CODEMAPS/rationale.md` (create it if absent). Skip this entirely for routine changes (refactors, simplifications, config tweaks, test fixes).

Focus on **why** the decision was made — constraints considered, alternatives rejected, trade-offs accepted:

```markdown
## <HEAD_SHA_SHORT> — <YYYY-MM-DD>
Commits: `<BASE_SHA_SHORT>..<HEAD_SHA_SHORT>`

### Decisions
- **<decision title>**: <why this approach was chosen over alternatives>
  ```
  // minimal illustrative snippet (≤5 lines) if helpful
  ```
```

Keep each run's block under ~200 tokens. Snippets are optional.

## Step 4: Finalise

Once all codemap edits are complete, run:

```bash
bash ./scripts/finalize.sh
```

This writes `.reports/codemap-diff.txt` with the correct HEAD SHA and commit range. Only call this after analysis succeeds — if anything failed, skip it so the next run reprocesses the same commits.

## Tips

- Focus on **high-level structure**, not implementation details
- Prefer **file paths and function signatures** over full code blocks
- Keep each codemap under **1000 tokens** for efficient context loading
- Use ASCII diagrams for data flow instead of verbose descriptions
- Run after major feature additions or refactoring sessions

