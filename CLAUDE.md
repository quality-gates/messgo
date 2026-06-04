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
5. **Commit and push** — land changes via PR.
6. **Watch CI** — wait for Actions to go green.
7. **Merge to main** — then push.
8. **Tag and release** — tag the release and publish.

## Conventions

- Exit codes match phpmd exactly (0 success, 1 error, 2 violations).
- **Edit files one at a time using Read then Edit.** Avoid bulk string-replacement tools across multiple directories.
- Keep complexity metrics (Cyclomatic Complexity, NPath) of messgo's own functions below their configured limits.

## Testing posture

Rules are verified using crafted Go fixture sources in `internal/rules/rules_test.go`.

**Assert on behavior:**
- Assert on which rules fire (using `mustHave` and `mustNotHave`).
- Ensure metrics values correspond to expected outputs of reference tools (like real phpmd).
