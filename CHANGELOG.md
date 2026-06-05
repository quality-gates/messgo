# Changelog

All notable changes to this project will be documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/). This project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
