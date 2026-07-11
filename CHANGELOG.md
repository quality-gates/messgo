# Changelog

All notable changes to this project will be documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/). This project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Committed git hooks in `githooks/` mirroring the CI workflows locally: `pre-commit` runs `gofmt -s`, `go vet`, `gocyclo -over 15`, `ineffassign`, `go build`, `go test`, and messgo self-analysis (whole-tree, hard-fails on any finding or missing tool); `pre-push` runs diff-scoped mutation testing (`mutago --git-diff-lines` against `origin/main`, min covered-MSI 80%). `govulncheck` stays CI-only since it depends on external advisories, not the diff. Activation is a manual `git config core.hooksPath githooks`, now listed as a new "Definition of Ready" step in `CLAUDE.md`; the hooks are inert until a clone opts in. Added `CONTEXT.md` distinguishing "git hooks" (the mechanism) from "pre-commit"/"pre-push" (the git stages) and from the unrelated Python `pre-commit` framework.
- Agent skills configuration: `docs/agents/issue-tracker.md` (GitHub Issues) and `docs/agents/domain.md` (single-context `CONTEXT.md` + `docs/adr/` layout), linked from a new `## Agent skills` section in `CLAUDE.md`. No behavioural change.
- Installed the 21 [mattpocock/skills](https://github.com/mattpocock/skills) engineering/productivity skills under `.claude/skills/`, with MIT attribution recorded in `.claude/skills/ATTRIBUTION.md`. No behavioural change to messgo itself.
- `docs/agents/triage-labels.md`, mapping the `triage` skill's five canonical roles to this repo's label vocabulary (kept at defaults), linked from `CLAUDE.md`.

### Fixed
- Split `TestParseFunctionsAndMethods` (`internal/model/build_test.go`) into three focused tests (`TestParseFreeFunction`, `TestParseGreeterMethods`, `TestParseAllFuncsIncludesFunctionsAndMethods`) to bring cyclomatic complexity under the new pre-commit hook's `gocyclo -over 15` gate. Same assertions, same coverage.

### Changed
- Created the five triage labels on GitHub Issues (via seed issue #19, since closed); triage docs now state the labels are live rather than pending creation.
- Moved AST walking for `design`, `cleancode`, and `unusedcode` rules behind model-level query primitives. Rules now ask `model.File` / `model.Function` for package variables, statement patterns, duplicate literal keys, identifier reads, and receiver uses instead of importing `go/ast`; added direct model query tests for those behaviours.
- Moved threshold-style rules onto a shared declarative `ThresholdRule` skeleton with typed load-time configuration. Codesize/design threshold rules now declare property/default/boundary plus a metric function; property parsing, regex compilation, and list splitting happen when rulesets load instead of during artifact walks. Built-in XML keeps property names/descriptions but no longer duplicates defaults owned by rule declarations.
- Collapsed the rule engine's separate `MethodRule` and `FunctionRule` interfaces (and the `applyMethodRule` helper) into a single `FuncRule` with one `ApplyFunc(ctx, fn)` seam. The model already unifies free functions and methods as `*model.Function`, so the engine now iterates the unified function list once and rules that only care about methods guard on `fn.IsMethod()` inline. This removes 20+ identical `ApplyMethod`/`ApplyFunction` pass-through pairs across the rule packages. No behavioural change.

## [0.1.9] - 2026-06-10

### Added
- New `Design/LackOfCohesionOfMethods` rule (messgo-native, no PHPMD analog) computing the **LCOM4** cohesion metric per struct type: methods are linked when they use a common field or call one another through the receiver, and the metric is the number of disconnected method groups. A value above the `maximum` property (default 1) is reported — the type bundles unrelated responsibilities and could be split, one type per group. Methods that touch no state (pure helpers, interface stubs) and trivial getters/setters are excluded so plain data carriers and idiomatic Go helpers don't false-positive; a call to a getter counts as a use of the wrapped field. Included in the default `go` ruleset.

## [0.1.8] - 2026-06-05

### Added
- Unit tests for the previously untested `internal/model` parser, covering package/class/interface/field/embed extraction, free-function vs. method classification (receiver, receiver name, owning class, `AllFuncs`), interface method signatures, parse-error handling, and `exprString` type rendering. Raises the package from 0% to ~78% statement coverage. No production code changes.

## [0.1.7] - 2026-06-05

### Changed
- `RenderMessage` and `CompileRegex` now reuse package-level compiled regexps instead of recompiling on every call. `RenderMessage` runs once per reported violation, so this removes a per-violation regex compilation from the hot path. Behaviour is unchanged.

### Added
- Unit tests for the previously untested `internal/rule` package, covering `RenderMessage` placeholder substitution and number formatting, the typed `Properties` accessors (`Int`/`Float`/`Bool`/`String`), `CompileRegex`, and `SortViolations`.

## [0.1.6] - 2026-06-05

### Added
- `--enable` / `--only` and `--disable` CLI flags to run a subset of individual rules by name (comma-separated), filtered within the loaded ruleset(s). `--only` keeps just the listed rules; `--disable` drops them; the two can be combined (whitelist, then subtract). Example: `messgo ./... text codesize,design --only CyclomaticComplexity,GlobalVariable`.

## [0.1.5] - 2026-06-05

### Changed
- `Design/GlobalVariable` is now **mutation-aware** and reports far less noise. By default it flags only package-level variables that are actually mutated (reassigned, incremented/decremented, written through via `g.f`/`g[k]`, or address-taken); effectively-constant globals such as sentinel errors, compiled regexps, and lookup tables are no longer reported. The new `report-immutable` property (default `false`) re-enables flagging read-only package-level variables.

### Added
- Cross-file (package-wide) analysis: messgo now groups a package's files before running rules, so `Design/GlobalVariable` correctly classifies a variable declared in one file but mutated in another. Added `util.MutatedGlobalNames` and a `MutatedGlobals` field on `model.File` populated by the runner.

## [0.1.4] - 2026-06-05

### Added
- New `Design/GlobalVariable` rule (messgo-native, no PHPMD analog) that detects mutable package-level variables — global shared state that hurts testability and concurrency safety. It inspects only top-level declarations, so local variables are never flagged; constants and the blank identifier (`var _ = ...`) are ignored. Available via the `design` and `opinionated` rulesets; it is excluded from the default `go` ruleset because some package-level variables are idiomatic in Go (sentinel errors, compiled regexps, registries).

## [0.1.3] - 2026-06-05

### Added
- New opt-in `opinionated` ruleset that bundles the checks deliberately excluded from the default `go` ruleset for conflicting with idiomatic Go: `ElseExpression`, `BooleanArgumentFlag`, and `UnusedFormalParameter`. Run it explicitly (`messgo ./... text opinionated`, or `go,opinionated` to combine) for a stricter, more PHP-flavoured style.

### Changed
- The default `go` ruleset no longer enables `CleanCode/ElseExpression` (`else` is idiomatic in Go), `CleanCode/BooleanArgumentFlag` (boolean parameters are common in Go's standard library), or `UnusedCode/UnusedFormalParameter` (unused parameters are routinely required to satisfy interfaces and standard signatures such as `http.HandlerFunc`). These rules remain available via their original rulesets and the new `opinionated` ruleset.

## [0.1.2] - 2026-06-05

### Fixed
- Fixed duplicate violations when overlapping rulesets were requested (e.g. `go,codesize`, where the `go` ruleset already imports `codesize`). Rules are now deduplicated by name across rulesets, keeping the first occurrence, so each violation is reported once.

## [0.1.1] - 2026-06-05

### Fixed
- Corrected the embedded version string (reported by `--version` and in machine-readable output) from `1.0.0` to match the released tag.

## [0.1.0] - 2026-06-05

### Fixed
- Fixed loop variable detection for `RangeStmt` where range variables were not marked as loop counters (`IsLoop = false`), causing the `ShortVariable` rule to mistakenly flag short variables like `i` or `v` in range loops.
- Fixed unused local variable detection where range loop variables were never marked as writes inside `identReads`, causing them to be treated as reads and preventing the `UnusedLocalVariable` rule from flagging unused range loop variables.
- Refactored 9 code hotspots (including `CyclomaticComplexity`, `npathStmt`, `lineHasCode`, `build`, `exprString`, `For`, `identReads`, `addRef`, and `discover`) to reduce their Cyclomatic Complexity below the configured threshold of 10.

### Changed
- Restructured `README.md` into a clear, linear getting-started flow (build → run → read output → exit codes), using messgo's own CI self-analysis command as the introductory example.

### Added
- Documented `go install github.com/quality-gates/messgo/cmd/messgo@latest` as the recommended install method in `README.md`.
- Documented `.worktrees/` as the location for git worktrees (gitignored) in `CLAUDE.md`.
- GitHub Actions CI workflows for security checking (govulncheck), report card grade enforcement, mutation testing with mutago, and messgo self-analysis & testing.
- Updated self-analysis step in CI to explicitly run `codesize` ruleset, and updated mutation testing to enforce `min-covered-msi` of 80% on the `internal/metrics` package.
- Coverage-guided fuzz test (`FuzzAnalyze`) in `internal/rules/fuzz_test.go` to verify ruleset analysis stability under mutated source bytes.
- Symlinks `GEMINI.md` and `AGENTS.md` pointing to developer guide `CLAUDE.md`.

### Removed
- Removed the `PARITY.md` file as part of code quality cleanup.
