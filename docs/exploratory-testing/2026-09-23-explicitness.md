# Exploratory testing: explicitness rulesets (2026-09-23)

An exploratory pass through the public `messgo` CLI for the new `explicitness`
and `explicitness-strict` rulesets (the diff `origin/main..5b1d118`).

- **Builds:** `go build -o messgo ./cmd/messgo` at `5b1d118` for the pass. I
  replayed the fixes on `2e0d32e`.
- **Configuration:** built-in rulesets, and ruleset XML written as `docs/rules.md`
  describes. Debug probes (`[DEBUG-e7x1]`) were added to `visitors.go` only during
  diagnosis and then removed. Every observation below comes from an
  uninstrumented binary.
- **Starting state:** a Go port of the "Grokking Simplicity" MegaMart cart,
  writable copies of `golang.org/x/sync@v0.23.0` and `golang.org/x/mod@v0.41.0`
  from the module cache, and small scratch modules.
- **Evidence:** [`replay.sh`](2026-09-23-explicitness/replay.sh) rebuilds every
  confirmed finding in an empty temp dir. Its output on both builds is in
  [`replay-output.txt`](2026-09-23-explicitness/replay-output.txt). Run it as
  `replay.sh ./messgo`.

## Journeys

### J1: find the actions in a realistic package

Goal: `messgo <dir> text explicitness` lists each implicit input and output that
the book identifies. There must be no findings for calculations. The exit code
must be 2 when there are findings.

- Ordinary path: on the MegaMart port, all 11 hand-written expectations matched,
  with exit 2. The calculations (`calc_total`, `add_item`, ...) were silent.
- Variation: all nine formats carried the same 11 findings. XML and checkstyle
  parse, and the GitLab fingerprints are unique.
- Variation: on `x/sync` with `--ignore-tests`, there were 9 findings, all
  correct on inspection.
- Failed: a probe package of 13 patterns found three misses (#179, #180, #181).

### J2: turn on strict mode and tune the rules

Goal: `explicitness-strict`, the `include-receiver` property, and the usual
ruleset controls behave as documented.

- `explicitness-strict`, per-rule `<property name="include-receiver"
  value="true"/>`, `--only`, `--disable`, `--minimumpriority 2`, the
  `rulesets/explicitness-strict.xml` ref, and `<exclude name>` all worked.
- A misspelt property name gives a warning.
- A bare `<rule ref="ImplicitOutput"/>` gave the same 6 outputs as the ruleset
  on MegaMart, with exit 2. Rejected: this is correct.

### J3: add the ruleset to an existing CI gate

Goal: run it next to `go` on a real module, with a usable signal-to-noise ratio
and speed.

- `x/mod`, `--ignore-tests`:
  - `explicitness`: 92 findings in 0.06 s.
  - `explicitness-strict`: 515 findings in 0.17 s.
  - `go,explicitness`: 230 findings, which is exactly 138 + 92.
- Of the 20 "write through parameter" findings, 19 are real writes to the
  caller's data. The exception is under Usability observations.

## Confirmed bugs

Each bug recurred on 3 of 3 runs of the regression tests and on the CLI replay.
All three are fixed in `2e0d32e`. Each fix has a test that failed before the
fix and passes after it (`internal/cli/explicitness_test.go`,
`internal/rules/rules_test.go`).

| Issue | Finding | Impact | Root cause |
| :--- | :--- | :--- | :--- |
| [#179](https://github.com/quality-gates/messgo/issues/179) | A global that only `copy` or `sort.Ints`/`slices.Sort` changes counts as constant | `ImplicitInput` misses reads of it. The released `GlobalVariable` rule also misses the global | `util.MutatedGlobalNames` did not count `copy` or in-place sorts |
| [#180](https://github.com/quality-gates/messgo/issues/180) | `<-ctx.Done()` is not an input | The most common implicit input in context-aware code is missed | `flow` used `RootIdent`, which does not peel a call |
| [#181](https://github.com/quality-gates/messgo/issues/181) | A global in a key of `names{n: "x"}` is not read | A read is missed for named map types | `visitComposite` checked only for a literal `*ast.MapType` |

The replay steps, expected results and actual results are in each issue and in
`replay.sh` sections B1–B3.

The fix for #179 changes shared code. On `x/sync`, `x/mod` and messgo's own
`internal/`, the `design`, `go` and `explicitness` outputs did not change.

## Rejected

- A null `helpUri` in SARIF output for the explicitness rules. Other messgo-only
  rules have the same null, so this is the project convention.
- A bare `<rule ref="ImplicitOutput"/>` exits 2 with the default properties.
  This is correct (see J2).

## Unresolved

None.

## Usability observations

- Observation: a function that copies a parameter before it sorts it is still
  reported, although the caller's slice does not change.
  `sumdb/dirhash/hash.go:47` (`files = append([]string(nil), files...);
  slices.Sort(files)`) is the one false positive among the 20 x/mod findings.
  This is the book's own copy-on-write technique.
  - Suggestion: stop tracking a parameter after the function assigns a new value
    to it.
  - Status: now documented as a limit (`replay.sh` O1).
- Observation: a receive through a local alias (`done := ctx.Done(); <-done`)
  and a send on a range variable are not reported. These are the documented
  alias limit, seen once in `x/sync`.
- Observation (pre-existing, not in this diff): `include-receiver="TRUE"`
  silently means false, because `Properties.Bool` is case-sensitive. A warning
  for a value that is not a boolean would catch this.

## Not explored

- `html` and `ansi` output were checked only for exit code and finding count.
  I did not look at the rendered output.
- Cross-package effects. By design, the analysis is AST-only and per function.
- Performance on large monorepos. The largest corpus was `x/mod`.
