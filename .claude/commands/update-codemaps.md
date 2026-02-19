# Update Codemaps

Incrementally update architecture codemaps using git diffs since the last processed commit.

## Step 1: Determine Change Range

Run the following to determine the change range:

```bash
# Extract last processed SHA (returns empty string if file missing or field absent)
grep '^last-sha:' .reports/codemap-diff.txt 2>/dev/null | awk '{print $2}'

git rev-parse HEAD
```

- If a SHA was returned: set `BASE_SHA=<that value>`. Run `git log <BASE_SHA>..HEAD --oneline` to confirm new commits exist. If none, print "Codemaps are up to date." and stop.
- If empty (file missing or first run): set `BASE_SHA=""` and proceed with a full scan of source files instead of a git diff.

## Step 2: Collect the Diff

```bash
git diff <BASE_SHA>..HEAD --name-status
git log  <BASE_SHA>..HEAD --oneline
```

Focus only on these diff outputs. Do not scan the entire project unless `BASE_SHA` is empty.

## Step 3: Analyse What Changed

From the diff, identify:
- Files added / removed / renamed
- Interface, type, class, or function signature changes
- New packages, modules, or entry points
- Configuration or environment variable changes
- Container / build tooling topology changes
- New external dependencies

## Step 4: Update Codemaps

Read only the codemaps affected by the identified changes and apply targeted edits — change only the sections that reflect the diff. Do not rewrite unaffected sections. Keep each codemap under 1000 tokens.

| Codemap | Trigger |
|---------|---------|
| `docs/CODEMAPS/architecture.md` | interface/contract changes, new components, data-flow changes |
| `docs/CODEMAPS/backend.md` | new/removed source files or packages, API route changes |
| `docs/CODEMAPS/frontend.md` | changes in UI templates, client-side logic, or component structure |
| `docs/CODEMAPS/dependencies.md` | dependency manifest changes, new env vars, container/build changes |

Update the freshness header in every file you touch:
```
<!-- Commit: <HEAD_SHA> | Files scanned: N | Token estimate: ~T -->
```

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

## Step 5: Update codemap-diff.txt

Overwrite `.reports/codemap-diff.txt` with:

```
last-sha: <HEAD_SHA>
updated: <YYYY-MM-DD>
commit-range: <BASE_SHA_SHORT>..<HEAD_SHA_SHORT>
codemaps-touched: <space-separated list of files updated>
```

No narrative prose — just these structured fields for machine readability.

## Tips

- Focus on **high-level structure**, not implementation details
- Prefer **file paths and function signatures** over full code blocks
- Keep each codemap under **1000 tokens** for efficient context loading
- Use ASCII diagrams for data flow instead of verbose descriptions
- Run after major feature additions or refactoring sessions