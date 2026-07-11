# messgo

A PHP Mess Detector (phpmd) port for Go. Parses Go source code using `go/ast` and applies rules faithful to Go semantics.

## Build & test

```bash
go build -o messgo ./cmd/messgo
go test ./...
```

All packages pass.

## Key packages

| Package | What it does |
| :--- | :--- |
| `cmd/messgo/` | Binary entrypoint; CLI surface |
| `internal/cli/` | Command-line flag parsing, validation, and execution orchestration |
| `internal/metrics/` | Cyclomatic complexity, NPath complexity, and lines-of-code calculation |
| `internal/model/` | Parser and models representing struct (Class), Interface, Method, Function, Field, Parameter |
| `internal/report/` | Renderers (text, xml, json, html, ansi, github, gitlab, checkstyle, sarif) |
| `internal/rule/` | Base structures, context, violation storage, and execution dispatch engine |
| `internal/rules/` | Rule implementations grouped by rulesets (cleancode, codesize, controversial, design, naming, unusedcode) |
| `internal/ruleset/` | Ruleset XML loader, priority filters, and overrides |
| `internal/runner/` | File discovery and full pipeline orchestration |
| `internal/util/` | AST helper functions and string utilities |

## Running messgo on itself (Self-Analysis)

To run messgo on itself locally to check for design or quality violations:

```bash
./messgo ./internal text go --ignore-tests
```

Exit code matches phpmd: **0** clean · **1** error · **2** violations found.

## Shipping workflow

Follow these steps in order when landing a change:

1. **Build and test locally** — `go build ./...` and `go test ./...`.
2. **Run self-analysis** — run the compiled binary on `./internal` using the `go` ruleset.
3. **Manual smoke test** — build the binary and run it against a real package or testdata file. Confirm stdout looks right.
4. **Update docs if needed** — if a rule is added, removed, or properties change, update `README.md`.
5. **Update CHANGELOG.md** — add an entry under `[Unreleased]` describing what changed (Added / Fixed / Changed).
6. **Commit and push** — land changes via PR.
7. **Watch CI** — wait for Actions to go green.
8. **Merge to main** — then push.
9. **Tag and release** — tag the release and publish.

## Conventions

- Exit codes match phpmd exactly (0 success, 1 error, 2 violations).
- **Edit files one at a time using Read then Edit.** Avoid bulk string-replacement tools across multiple directories.
- Keep complexity metrics (Cyclomatic Complexity, NPath) of messgo's own functions below their configured limits.
- **Git worktrees go in `.worktrees/`** (gitignored). Create new worktrees there, e.g. `git worktree add .worktrees/my-feature`.

## Testing posture

Rules are verified using crafted Go fixture sources in `internal/rules/rules_test.go`.

**Assert on behavior:**
- Assert on which rules fire (using `mustHave` and `mustNotHave`).
- Ensure metrics values correspond to expected outputs of reference tools (like real phpmd).

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues (`quality-gates/messgo`, via `gh`). See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`), created on GitHub and ready for `/triage`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout — `CONTEXT.md` + `docs/adr/` at the repo root (not yet created). See `docs/agents/domain.md`.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:6cd5cc61 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->
