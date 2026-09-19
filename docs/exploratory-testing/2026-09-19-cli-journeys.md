# Exploratory testing: CLI journeys (2026-09-19)

An exploratory pass through the public `messgo` CLI, run as a user would run it.

- **Builds:** `go build -o messgo ./cmd/messgo` at `74f311e`. All confirmed
  findings were replayed again on `7dc147e` (then `origin/main`) with identical
  output.
- **Configuration:** built-in rulesets, and custom ruleset XML written the way
  the README and phpmd docs describe. No instrumentation or source changes.
- **Starting state:** writable copies of `golang.org/x/sync@v0.23.0` and
  `golang.org/x/mod@v0.41.0` from the module cache, plus small scratch modules
  under a temp directory.
- **Evidence:** [`replay.sh`](2026-09-19-cli-journeys/replay.sh) rebuilds every
  confirmed finding from an empty temp dir. Its output is in
  [`replay-output.txt`](2026-09-19-cli-journeys/replay-output.txt). Run it as
  `replay.sh ./messgo`; it needs `jq` and `go`.

## Journeys

### J1: gate a real module in CI

Goal: `messgo ./... <format> go --ignore-tests` reports findings, exits `2` when
there are findings, and writes `--reportfile` where the README says.

- Ordinary path: on `x/sync` it reported 5 findings with exit 2. I checked the
  `Acquire()` CCN of 12 by hand-counting decision points. On `x/mod` it reported
  137 findings with exit 2.
- Variations: path forms `mod/...`, absolute `/…/mod/...`, comma lists, a single
  file, and a trailing `/`. The options `--ignore-violations-on-exit` (exit 0),
  `--reportfile out.txt` (file written, exit 2), and a parse error (exit 3,
  error included in every format) all behaved as documented.
- Failed: the README's `--reportfile reports/messgo.sarif` exits 1 unless
  `reports/` already exists (#150).
- Failed: `./...` walks `testdata/`, although Go skips it (#154).

### J2: tune a team ruleset

Goal: a ruleset XML that excludes rules, overrides priorities and properties,
and filters on the CLI. It should behave like phpmd.

- Ordinary path: the README team policy worked. `LongVariable` took
  `maximum=50` and priority 2, and `DevelopmentCodeFragment` was excluded.
  `--minimumpriority 1/2`, `--only`, `--disable`, `--exclude`, and an `--only`
  that matches nothing (warning, exit 0) all worked.
- Failed: phpmd's canonical refs `rulesets/codesize.xml` and
  `rulesets/naming.xml/LongVariable` exit 1 (#151).
- Failed: `<exclude-pattern>` is silently ignored, even with `-v` (#152).

### J3: consume reports in CI tooling

Goal: every format carries the same findings, in a shape its consumer accepts.

- On `x/mod`, all nine formats ran. `json`, `xml`, `checkstyle`, `gitlab`,
  `sarif`, and `github` each held 137 findings. XML and checkstyle parse with
  `xmllint`. SARIF validates against the OASIS SARIF 2.1.0 schema with 0 errors.
- Failed: `gitlab` fingerprints collide when one rule fires twice on the same
  line (#153).

## Confirmed bugs

All five recurred in each of three runs: the original exploration and two clean
replays of `replay.sh`. They also recurred on `7dc147e`.

| Issue | Finding | Impact |
| :--- | :--- | :--- |
| [#150](https://github.com/quality-gates/messgo/issues/150) | `--reportfile` does not create missing parent directories; phpmd does | The README's SARIF quick start fails in a fresh CI checkout |
| [#151](https://github.com/quality-gates/messgo/issues/151) | `rulesets/<name>.xml[/<Rule>]` refs are rejected | phpmd rulesets cannot be reused as they are |
| [#152](https://github.com/quality-gates/messgo/issues/152) | `<exclude-pattern>` is ignored without a warning | Excluded files still fail the gate |
| [#153](https://github.com/quality-gates/messgo/issues/153) | GitLab fingerprint is `path:line:rule`, so it is not unique | GitLab merges or hides findings that collide (phpmd uses the same recipe) |
| [#154](https://github.com/quality-gates/messgo/issues/154) | `./...` scans `testdata/` | Fixture code fails the gate |

The replay steps, expected results, and actual results are in each issue and in
`replay.sh` sections B1–B5.

## Unresolved

- `--minimumpriority 0` applies no filter; every rule runs. The help text says
  "priority <= n", which would select nothing. Internally, 0 means "unset"
  (`internal/ruleset/ruleset.go`). No user would plausibly pass 0, and I did not
  check what phpmd does with 0. Not filed.

## Usability observations

- Observation: in the JSON report, a `LongVariable` finding on a local variable
  inside `Compute()` has `"function": ""`. `ExcessiveParameterList` fills in
  `function`. Suggestion: fill the enclosing function for local-variable rules
  too. I did not check whether phpmd does this.
- Observation: `github` annotations use whatever path form the user passed.
  `messgo /abs/path/... github` emits absolute `file=` paths. Suggestion: tell
  users to run from the repository root with relative paths.

## Not explored

- `html` and `ansi` output were only checked for exit code and size, not rendered.
- Rules were only spot-checked (CCN on one function, `ExcessiveParameterList`
  threshold parity with phpmd). This pass did not verify metric accuracy.
- Homebrew and `go install` distribution.
