# Exploratory testing: CLI gate, reports, and effects (2026-11-01)

This pass exercised the public CLI on Linux at `f929dc84effb`, the tip of
`origin/main`. The binary was built with Go 1.26.8 and `-mod=readonly`.
The module has no external dependencies. Fixtures were isolated in scratch
directories. No product code was changed.

[The replay script](evidence-2026-11-01/replay.sh) creates the fixtures and
runs all three journeys. [The captured output](evidence-2026-11-01/output.txt)
includes commands, reports, and exit codes. To replay, pass an absolute path to
a built binary and a new empty scratch directory:

```console
bash docs/exploratory-testing/evidence-2026-11-01/replay.sh /absolute/path/to/messgo /absolute/path/to/scratch
```

Two runs in separate empty directories produced identical output after
normalizing the JSON timestamps. The script runs commands in the foreground
and removes any jobs it starts on exit.

## Gate a Go module

The goal was to run the recommended `go` policy before a PR, as described in
[the README](../../README.md). The fixture included a normal file, a test, a
Windows-only file, and a generated subdirectory.

- `./... text go --ignore-tests` reported the ordinary and generated files,
  skipped the test and Windows file, and exited 2.
- Removing `--ignore-tests` added the test finding. Naming the Windows file
  explicitly reported its finding despite the host build constraint.
- A team ruleset excluded `DevelopmentCodeFragment` and returned exit 0.
  A second ruleset kept that rule but excluded `*/gen/*`. The directory scan
  reported only the ordinary file, and the explicitly named generated file was
  also excluded.

## Write and replace a CI report

The goal was to save a machine-readable report with `--reportfile`, as
[usage](../usage.md) describes. JSON output created a missing `reports`
directory, left stdout empty, contained both findings, and exited 2. After the
ordinary file was corrected, a second invocation replaced the report. The new
file contained only the generated-file finding.

A malformed Go file produced a JSON `errors` entry and exit 3, even with a
separate finding. Adding `--ignore-errors-on-exit` retained the error in the
report and returned exit 2 for the remaining violation. The observed result
matches the documented exit precedence.

## Classify implicit effects

The goal was to distinguish calculations, shared-state effects, and unknown
calls under the [explicitness contract](../rules.md). The fixture includes a
package variable, a parameter alias, private allocation, a callback, and
receiver methods.

- `explicitness` reported the package write and read, attributed the alias
  write to parameter `p`, and marked the callback and receiver effects unknown.
  The calculation and private allocation were absent from the report.
- `explicitness-strict` classified the receiver read and write instead of
  marking them unknown. The increment also read the receiver field.
- A JSON report filtered with `--only ImplicitOutput` contained the package
  write and the parameter write, with no input or unknown-effect findings.

These observations also exercise the previously reported receiver and alias
cases in [#277](https://github.com/quality-gates/messgo/issues/277) and
[#279](https://github.com/quality-gates/messgo/issues/279). They do not
reproduce those closed issues on this build.

## Findings and limits

No bugs were confirmed, and no new issues were filed. The open and closed issue
list and the `wontfix` label were checked before reporting. There were no
unresolved product observations in the exercised cases.

The initial `go mod download -mod=readonly` attempt failed because `download`
does not accept that flag. Retrying with `GOFLAGS=-mod=readonly` succeeded.
The initial `go test ./...` used a Go temporary directory inside the worktree.
The hook environment test failed because Git discovered the enclosing
repository above its temporary checkout. Running the compiled hook tests with
a separate temporary directory passed. Every other package passed in the
original run. An attempt to use the host `/tmp` for Go build files hit its
user quota. The compiled hook tests instead used a temporary directory in
`/dev/shm`, which was removed after the run. Build and module caches remained
inside the worktree. `go build ./...` and self-analysis with
`go,codesize --ignore-tests` passed.

The Homebrew release, Docker image, remote CI annotation upload, other report
formats, and broad third-party codebases were not exercised. No servers,
browsers, containers, images, volumes, or listening ports were started.
