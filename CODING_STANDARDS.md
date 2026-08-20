# Coding standards

## Tests

- Strongly prefer integration tests and end-to-end tests over unit tests.
- Strongly prefer exercising real system behaviour over "the tests pass so it must work."
- Only mock third-party services we cannot control. Do not mock code we own.
- For this codebase, the default proof is: run the real CLI/analyzer on real (or fixture) Go source and assert findings, exit codes, and report output.

## Comments and docs

- Code comments use ASD-STE100 Simplified Technical English.
- Ground terms in `CONTEXT.md` domain language when that file exists. Do not invent synonyms for glossary terms (git hooks vs pre-commit stage, stable release, tap publication, formula candidate).
- Do not write comments that only repeat what the code already makes clear.
- Do not put brittle references in README or comments (versions, line numbers, temporary paths, "as of today" claims) when those details are allowed to change.

## Common footguns

- Tautological tests (asserting the mock was called the way the test just configured it).
- Mocks of modules/services we own.
- "Green suite" treated as proof the product works for a user.
- Narrating comments and README drift magnets.
- Cheating complexity or quality gates with denser syntax, hidden branching, or indirection that does not reduce real complexity.

## Go

- Format with `gofmt`; keep `go vet` clean. Prefer packages under `cmd/messgo` and `internal/{cli,metrics,model,report,rule,rules,ruleset,runner,util}`.
- Parse Go with `go/ast` (and existing model builders). Do not add a second parser.
- Keep the phpmd-faithful shape: ruleset XML, exit codes `0` clean / `1` error / `2` violations.
- Prefer composition and small helpers in `internal/util` over growing god-functions. Do not cheat Cyclomatic Complexity or NPath limits on messgo's own code.
- Self-analysis must stay clean on production paths: `./messgo ./internal text go --ignore-tests` (or the current equivalent) before merge when you touch analyzer code.
- Tests assert behaviour with `mustHave` / `mustNotHave` style fixture checks and metric values against reference expectations — not incidental string snapshots of entire reports unless the report format is the behaviour under test.
- Keep module path `github.com/quality-gates/messgo` and the Go version in `go.mod` honest; do not use newer language features without bumping the module Go version deliberately.
- Git worktrees belong in `.worktrees/` (gitignored).
- Local quality hooks live in `githooks/` and are activated with `git config core.hooksPath githooks`. "Pre-commit" / "pre-push" name hook *stages*, not the Python pre-commit framework.
