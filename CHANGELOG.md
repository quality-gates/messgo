# Changelog

All notable changes to this project will be documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/). This project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed
- Fixed loop variable detection for `RangeStmt` where range variables were not marked as loop counters (`IsLoop = false`), causing the `ShortVariable` rule to mistakenly flag short variables like `i` or `v` in range loops.
- Fixed unused local variable detection where range loop variables were never marked as writes inside `identReads`, causing them to be treated as reads and preventing the `UnusedLocalVariable` rule from flagging unused range loop variables.
- Refactored 9 code hotspots (including `CyclomaticComplexity`, `npathStmt`, `lineHasCode`, `build`, `exprString`, `For`, `identReads`, `addRef`, and `discover`) to reduce their Cyclomatic Complexity below the configured threshold of 10.

### Added
- GitHub Actions CI workflows for security checking (govulncheck), report card grade enforcement, mutation testing with mutago, and messgo self-analysis & testing.
- Updated self-analysis step in CI to explicitly run `codesize` ruleset, and updated mutation testing to enforce `min-covered-msi` of 80% on the `internal/metrics` package.
- Coverage-guided fuzz test (`FuzzAnalyze`) in `internal/rules/fuzz_test.go` to verify ruleset analysis stability under mutated source bytes.
- Symlinks `GEMINI.md` and `AGENTS.md` pointing to developer guide `CLAUDE.md`.

### Removed
- Removed the `PARITY.md` file as part of code quality cleanup.
