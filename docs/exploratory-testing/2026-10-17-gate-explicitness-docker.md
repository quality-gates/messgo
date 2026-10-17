# Exploratory testing: gate, explicitness, and Docker (2026-10-17)

An exploratory pass through the public `messgo` CLI. It covers the recommended
`go` gate, the `explicitness` ruleset after the ownership rewrite, and the
Docker entrypoint documented in the `Dockerfile`.

- **Build:** `go build -o messgo ./cmd/messgo` at `74a4acf`
  (`origin/main`, after the ownership rewrite). `./messgo --version` prints
  `messgo dev`.
- **Configuration:** built-in rulesets (`go`, `explicitness`,
  `explicitness-strict`) and a team ruleset XML in the shape `docs/rules.md`
  shows. No instrumentation or source changes.
- **Starting state:** scratch modules under the supplied `TMPDIR`. The host
  binary and a Docker image built from this checkout's `Dockerfile` were both
  driven. `go vet` was clean on the first gate module.
- **Evidence:** [`replay.sh`](2026-10-17-gate-explicitness-docker/replay.sh)
  rebuilds every confirmed finding in an empty temp dir. Its output is in
  [`replay-output.txt`](2026-10-17-gate-explicitness-docker/replay-output.txt).
  Run it as `replay.sh /abs/path/messgo`. It needs `go` and `python3`. The
  stack-overflow frames are in
  [`stack-overflow-frames.txt`](2026-10-17-gate-explicitness-docker/stack-overflow-frames.txt).
  A trailing `exit=0` in the replay log is the harness, not messgo. The tool's
  own exit code is the `exit=` line printed inside each run.

## Journeys

### J1: gate a module before opening a PR

Goal: `messgo ./... text go --ignore-tests` reports maintainability findings,
honours build constraints, and writes a CI report. A team ruleset can drop a
rule, skip generated files, and raise `LongVariable`'s maximum, as
`README.md` and `docs/rules.md` describe.

- Ordinary path. A small module with a parenthesized type block, a
  parenthesized receiver, a `//line` directive, a `//go:build windows` file,
  a `_windows.go` file, a `*_test.go` file, and a `gen/` file reported six
  findings, exit 2. Tests and both Windows files were absent. The `//line`
  directive did not move the `NestingDepth` finding off `score.go`.
  `UnusedPrivateMethod` attached `hide` to its parenthesized receiver.
- Self-analysis of `./internal` with `go --ignore-tests` was exit 0.
- Variation: a broken team ruleset XML exited 1 with the syntax error on
  stderr. Correcting the close tag and retrying excluded `gen/` and
  `DevelopmentCodeFragment`. The `LongVariable` maximum override changed the
  message from "under 35" to "under 50". Naming the generated file explicitly
  still skipped it.
- Variation: `--reportfile reports/messgo.sarif` created the missing
  directory, left stdout empty, and wrote JSON. Editing the source and
  rerunning replaced the file with the new finding. A directory path and a
  parent that is a file both exited 1 with the OS error on stderr.
- Variation: a named `//go:build windows` file and a named `_windows.go` file
  were analyzed. A directory scan skipped both. `--ignore-tests` skipped the
  test file; without it, the test's `println` was reported.
- Variation: an unreadable path exited 1 on stderr. A parse error exited 3
  and took precedence over a finding in another file. `--strict` did not
  change the report.
- Variation: methods on parenthesized struct, generic, and alias types in a
  second file were attached. JSON `class` was `Box`, `Pair`, and `Alias`.
- No confirmed failures on this journey.

### J2: classify effects with explicitness

Goal: `messgo <path> text explicitness` reports implicit inputs, implicit
outputs, and unknown effects as `docs/rules.md` and
`internal/ruleset/builtin/explicitness.xml` describe. A clean report certifies
a calculation only inside that subset. `explicitness-strict` classifies
receiver effects instead of leaving them unknown.

- Ordinary path. The ruleset examples behaved as written: a write and a read
  of a changed package variable were reported, `fmt.Println` was an implicit
  output, and `Add` was silent. Exit 2.
- Variation: ownership. Alias writes, nested pointer writes, conditional and
  loop-carried origins, slice assignment, `append` then mutate, variadic
  writes, `delete`, and `slices.Sort` were reported. Rebinding a parameter to
  a fresh local was silent. `slices.Clone` of `[]int` was silent.
  `slices.Clone` of `[]*int` was reported. A copied array, a `copy` into a
  new slice, a private channel, a fixed package variable, a literal `if false`
  branch, and a value-receiver field assignment were silent.
- Variation: calls. Same-package calls, recursion, and a function value
  propagated a parameter write. An injected callback, including one stored in
  a local, and `io.Writer.Write` were `UnknownEffect`. `defer` and `go`
  transferred the effect and still reported the lambda. `init` belonged to
  `<module>`. Defining a function literal did not report the definer.
  `os.Getenv`, `os.Setenv`, `time.Now`, `panic`, and `recover` matched the
  modeled lists. `os.File.Write` and `bytes.Buffer.WriteString` were
  `UnknownEffect`.
- Variation: `explicitness-strict` reported receiver reads as implicit inputs
  and caller-visible receiver writes as implicit outputs. A method that does
  not use its receiver stayed silent. SARIF omitted `helpUri` when the rule
  URL was empty.
- Failed: taking the address of a local and then reading a field crashes the
  process (#275). The same crash happens for `explicitness-strict` and
  `go,explicitness`, and inside the Docker image.
- Failed: four functions from `internal/model/build.go`, analyzed together,
  do not finish (#276). Each function alone finishes. The `go` ruleset on the
  extract exits 0.
- Failed: non-strict mode reports a pointer-receiver write as `ImplicitOutput`
  and omits a receiver read, so the read looks pure (#277).
- Failed: `return ctx.Done()` is absent, while `ctx.Err()` is `UnknownEffect`
  (#278).
- Failed: a write through local `q := p` is described as "write through
  parameter q" (#279).

### J3: run the documented Docker image

Goal: `docker build -t messgo .` then
`docker run --rm -v "$PWD":/code messgo /code text go --ignore-tests`, as the
`Dockerfile` comment says, produces the same findings as the host binary.
A bad path is an error. `--reportfile` writes back to the mount.

- Ordinary path. The image's default command is `--help`, exit 0.
  `--version` prints `messgo dev`. The mounted module reported the same
  `DevelopmentCodeFragment` as the host binary, exit 2. The test file and the
  `_windows.go` file were skipped.
- Variation: `/missing` exited 1 with `stat /missing: no such file or directory`
  on stderr.
- Variation: `--reportfile /code/reports/messgo.txt` created the file on the
  host mount and left stdout empty. Appending a `println` and rerunning added
  that finding to the file.
- Failed: the #275 program also stack-overflows in the container, exit 2,
  empty stdout.

## Confirmed bugs

Each bug recurred on 3 of 3 runs of `replay.sh`.

| Issue | Finding | Impact |
| :--- | :--- | :--- |
| [#275](https://github.com/quality-gates/messgo/issues/275) | Stack overflow after `&s` and then `s.f` | `explicitness` dies before a report. Exit 2, which the docs use for findings. Stdout empty. Same crash in Docker and on `internal/model/call_resolution.go`. |
| [#276](https://github.com/quality-gates/messgo/issues/276) | Cross-function `explicitness` does not finish | `internal/model/build.go` produced no report in 90 seconds. A 100-line extract of four functions did not finish in 8 seconds. The `go` ruleset on that extract exits 0. |
| [#277](https://github.com/quality-gates/messgo/issues/277) | Non-strict mode classifies receiver writes and drops receiver reads | `Inc` is an implicit output. `N` and `Read` are missing, so they look pure. The ruleset says those effects stay `UnknownEffect`. |
| [#278](https://github.com/quality-gates/messgo/issues/278) | `return ctx.Done()` is certified pure | A custom `Context.Done` is an unresolved interface call. `ctx.Err()` on the same interface is `UnknownEffect`. |
| [#279](https://github.com/quality-gates/messgo/issues/279) | Local alias reported as a parameter | `q := p; *q = 1` says "write through parameter q". `q` is not a parameter. The direct write names `p`. |

Starting conditions, replay steps, and expected and actual output are in each
issue and in `replay.sh`.

## Rejected

- `slices.Clone` of `[]int` followed by `c[0] = 1` is silent. The clone does
  not share elements. `[]*int` is reported. This matches "a shallow clone has
  private outer storage but can still reach shared elements."
- `p = &local; *p = 1` is silent. This matches "ownership follows rebinding."
- A function literal that is defined and not called does not report the
  definer. The lambda itself is reported. This matches "defining one does not
  execute its body."
- A write to a field of a value receiver is silent in both modes. The ruleset
  XML says that write changes only a copy.
- An empty result on a named `//go:build windows` file was `fmt.Println`,
  which is not in `DevelopmentCodeFragment`'s default list (`println,print`).
  The same file with `println` was reported. The directory scan still skipped it.
- `func parenthesized(c *(Counter))` is a function with a pointer parameter,
  not a method. Reporting "write through parameter c" is correct.

## Unresolved

- A parse error in `sarif` is a result with an empty `ruleId` and no `region`.
  This pass did not validate that result against the SARIF schema.
- #276 is filed because the command does not return in the times above. This
  pass did not prove it can never finish. A `SIGQUIT` during `build.go` showed
  `joinCells` / `joinHeaps` / `joinStates` under a type switch, not the
  #275 stack overflow.
- Text-format parse errors are tab-separated lines in the report, not
  `file:line  Rule  message`. `usage.md` describes findings, not error lines.
  JSON keeps them in a separate `errors` array. Not filed.

## Usability observations

- Observation: the recommended `go` ruleset flags `Order.ID()` with
  `ShortMethodName` (minimum 3). The ruleset drops `ShortVariable`, not
  `ShortMethodName`.
  - Suggestion: exempt common short exported names, or say in the quick start
    that `ID` is reported.
- Observation: `DevelopmentCodeFragment` does not flag `fmt.Println` unless
  `unwanted-functions` is set. The default `println,print` is not in
  `docs/rules.md`.
  - Suggestion: document the default list.
- Observation: the documented Docker command reports paths under `/code`, the
  container path. A `github` or `sarif` report from that command does not match
  host paths.
  - Suggestion: show a relative path, or say that CI annotations need a path
    rewrite.
- Observation: `github` annotations use `col=1`. JSON findings have no column.
- Observation: `close(ch)` is reported as "write through parameter ch", while
  `ch <- 1` is "send on ch". The close is detected. The message does not say
  close.
- Observation: the implicit-output template says "Return it instead" for
  `fmt.Println` and `panic`, where a return is not the remedy.

## Not explored

- Uploading SARIF to GitHub code scanning.
- The Homebrew formula.
- `explicitness` on a large third-party module. `internal/model` did not
  finish, so that comparison was blocked.
- `--color` and `ansi`. A previous pass covered them.

## Environment

- darwin/arm64, Go 1.26.6. `GOMAXPROCS=1` for the timing runs.
- Docker image `messgo-et-43e0a389` was built from this checkout and removed
  at the end of the pass, along with the containers from `docker run --rm`.
  No volume was created.
