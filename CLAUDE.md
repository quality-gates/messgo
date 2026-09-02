# messgo

A PHP Mess Detector (phpmd) port for Go. Parses Go source code using `go/ast` and applies rules faithful to Go semantics.

## Definition of Ready

Before starting work in this repo:

1. Activate git hooks: `git config core.hooksPath githooks`

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

## Releasing: Homebrew tap approval gate

The release workflow dispatches to `quality-gates/homebrew-tap` to publish a formula. The tap's **Test tap** workflow requires manual approval for automation branches. The publish action polls for checks for ~5 minutes then fails if none are registered.

Approve the pending run on the tap repo early:

```bash
gh run list --repo quality-gates/homebrew-tap --limit 3
gh api repos/quality-gates/homebrew-tap/actions/runs/<RUN_ID>/approve --method POST
```

If the window already expired, re-run the failed messgo release job after approving, then merge the tap PR once checks pass:

```bash
gh run rerun <MESSGO_RELEASE_RUN_ID> --failed
gh pr merge <PR_NUMBER> --repo quality-gates/homebrew-tap --squash
```
