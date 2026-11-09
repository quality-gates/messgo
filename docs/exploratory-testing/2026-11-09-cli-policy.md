# Exploratory testing: CLI policy and reports (2026-11-09)

This pass exercised the public CLI on Linux amd64 at `0ed79d06ce1d0350d9b025cccc9cf551a39db565`, the tip of `origin/main`. Go 1.26.8 built the binary with `GOFLAGS=-mod=readonly`. The module has no external dependencies. The worktree held the binary, caches, and isolated fixtures. No product code changed.

[The replay script](2026-11-09-cli-policy/replay.sh) creates the fixtures and runs the journeys. [The captured output](2026-11-09-cli-policy/output.txt) records commands, reports, warnings, and exit codes. Pass the script an absolute binary path and a new empty scratch directory. It runs commands in the foreground.

```console
bash docs/exploratory-testing/2026-11-09-cli-policy/replay.sh /absolute/path/to/messgo /absolute/path/to/empty-directory
```

Two runs in separate empty directories produced identical output after JSON timestamp normalization. The first attempt used `fmt.Println`, which the default rule does not identify as a debug call. The captured replay uses the supported `println` example instead.

## Discover files for a gate

The goal was to run the recommended `go` policy on a module while excluding tests and platform-specific files, as [usage](../usage.md) describes. The fixture contained an ordinary source file, a test, a Windows-only source file, and a generated directory without a generated-code marker.

- The recursive scan with `--ignore-tests` reported the ordinary file and the generated directory and exited 2. It skipped the test and Windows file.
- Removing `--ignore-tests` added the test finding. Naming the Windows file explicitly reported its finding.
- A team ruleset with `<exclude-pattern>*/generated/*</exclude-pattern>` removed the generated-directory finding and retained the ordinary finding. This pass did not reproduce [#152](https://github.com/quality-gates/messgo/issues/152).

## Save and replace a CI report

The goal was to write a JSON report and use its exit code as a gate. `--reportfile reports/result.json` created the missing parent directory, left the command output empty, saved both findings, and exited 2. After the ordinary source was corrected, a second run replaced the report with only the generated-directory finding. This pass did not reproduce [#150](https://github.com/quality-gates/messgo/issues/150).

A malformed source file produced a processing error and exit 3 even though a separate finding remained. With `--ignore-errors-on-exit`, the error remained visible and the finding caused exit 2. The [previous pass](2026-11-01-cli-gate-and-effects.md) noted that the options table does not make that combined case explicit. This remains a documentation ambiguity, not a confirmed product bug.

## Apply a team threshold

The goal was to apply a custom `LongVariable` threshold and disable the rule when needed. A single-rule XML policy with `maximum` set to 8 reported the long local variable and exited 2. Adding `--disable LongVariable` removed the finding, warned that no rules were selected, and exited 0. This did not exercise inherited-rule conflict cases in [#312](https://github.com/quality-gates/messgo/issues/312).

## Findings and limits

No product bugs were confirmed and no issues were filed. The open and closed issue list, comments, linked pull requests, and `wontfix` label were checked. The warning when all rules are disabled makes an otherwise clean exit visible to the user.

`go build ./...` and self-analysis with `./internal text go --ignore-tests` passed. The first `go test ./...` run failed `TestPreCommitClearsGitEnvironment` because its temporary directory was inside the worktree and Git found the enclosing repository. All other packages passed. A full uncached retry passed with `GIT_CEILING_DIRECTORIES` set to the worktree's scratch directory. This is a test-environment observation also noted in the previous pass, not a CLI finding. [Verification output](2026-11-09-cli-policy/verification.txt) records both runs.

The root filesystem ran out of space during the first build. The worktree was recreated on `/tmp`, with a link under the repository's `.worktrees` directory. Build outputs and test data remained in the worktree. The initial incomplete build was removed. Local hooks require tools not installed as locked project dependencies. CI is the remote gate.

The pass did not exercise other output formats, remote annotation upload, Homebrew, Docker, or explicitness analysis. No servers, browsers, containers, images, volumes, or listening ports were started.
