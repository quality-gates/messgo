# Exploratory testing: naming rules, type-level size rules, and report context (2026-10-10)

An exploratory pass through the public `messgo` CLI. It covers the `naming`
ruleset and its properties, the type-level `codesize` counters, and the report
formats as CI consumers read them.

- **Build:** `go build -o messgo ./cmd/messgo` at `6810cdd` (`origin/main`, after v0.5.9).
- **Configuration:** built-in rulesets (`naming`, `codesize`, `controversial`,
  `opinionated`, `explicitness`), plus custom ruleset XML that sets the rule
  properties `docs/rules.md` and `rulesets/*.xml` describe. No instrumentation
  or source changes.
- **Starting state:** new scratch modules under the supplied `TMPDIR`. Each was
  checked with `go vet` before analysis.
- **Evidence:** [`replay.sh`](2026-10-10-naming-and-report-context/replay.sh)
  rebuilds every confirmed finding in an empty temp dir. Its output is in
  [`replay-output.txt`](2026-10-10-naming-and-report-context/replay-output.txt).
  Run it as `replay.sh /abs/path/messgo [python-with-jsonschema] [sarif-schema-2.1.0.json]`.
  It needs `go`, `jq` and `python3`. The SARIF check uses
  [`validate_sarif.py`](2026-10-10-naming-and-report-context/validate_sarif.py)
  (`jsonschema` Draft 4 with format checking) and the OASIS
  `sarif-schema-2.1.0.json`.

## Journeys

### J1: enforce a team naming policy

Goal: `messgo <path> text naming` reports short and long names for types,
variables and methods, plus misnamed boolean getters and constants. A team
ruleset can tune each rule through its documented properties.

- Ordinary path. The probe package reported 12 findings, with exit 2. Each one
  matched a hand-written expectation:
  - Classic `for` init variables and `range` keys and values are exempt from
    `ShortVariable`.
  - `GetFlag(x int) bool` is exempt from `BooleanGetMethodName` by default.
  - Named `(ok bool)` results are reported.
  - `MAX_SIZE` is reported and `a1` is not.
- Variation: a custom XML ruleset set these properties, and each one took effect:
  - `ShortVariable` `exceptions`
  - `ShortClassName` `exceptions`
  - `LongVariable` `subtract-prefixes` and `subtract-suffixes`, both of which
    are subtracted
  - `LongClassName` `subtract-prefixes`
  - `BooleanGetMethodName` `checkParameterizedMethods=true`
- Variation: generic types (`Box[T]`, `Pair[K, V]`) and non-ASCII identifiers.
  The `controversial` CamelCase rules accept `Größe`, `länge` and `fläche`
  correctly.
- Failed: the naming length rules count UTF-8 bytes, not characters (#256).
- Failed: in `json` and `xml`, `ShortClassName`, `LongClassName` and
  `CamelCaseClassName` give no class name (#257).

### J2: cap type and signature size with codesize

Goal: `ExcessiveParameterList`, `ExcessiveReturnCount`, `TooManyFields`,
`TooManyMethods` and `TooManyPublicMethods` count Go declarations correctly.

- Ordinary path. Each count was correct:
  - Grouped parameters `a, b, … j int` count as 10, and a variadic parameter
    counts as 1.
  - Grouped named results `(a, b, c, d int)` and unnamed results count as 4.
  - Grouped fields count as 16.
  - A function with exactly 3 results is not reported (`maxresults` 3).
- Variation: methods on a generic receiver (`func (b *Box[T])`). They are
  counted on `Box` (26).
- Variation: a type's 26 methods split 13/13 across two files. All 26 are
  counted, and the finding is on the type declaration.
- No failures.

### J3: consume reports in CI tooling

Goal: every format carries findings that its consumer can use, including file
paths with characters that need escaping, priority filters, and colour.

- Ordinary path: the file path was `a&b <x>, y%z/w.go`. Every format escaped it
  correctly:
  - `html`, `xml` and `checkstyle` entity-escape it.
  - `json` and `gitlab` use JSON escapes.
  - `github` writes `%2C` and `%25` in the `file=` property.
- Variation: `--minimumpriority`, `--maximumpriority`, both together, an
  out-of-range value (exit 0) and a non-numeric value (exit 1) all behaved as
  documented.
- Variation: `--suffixes txt` and `--suffixes .go,txt` select the expected files.
- Variation: `--color` and `ansi` output colour codes.
- Failed: when a `controversial` rule fires, the SARIF report fails the SARIF
  2.1.0 schema because `helpUri` is `"#"` (#258). A `naming`-only report
  validates with 0 errors. `opinionated` and `explicitness` rules leave out
  `helpUri`, which is valid.

## Confirmed bugs

Each bug recurred on 3 of 3 runs of `replay.sh`, with identical output.

| Issue | Finding | Impact | Root cause |
| :--- | :--- | :--- | :--- |
| [#256](https://github.com/quality-gates/messgo/issues/256) | Naming length rules count bytes, not characters | `überÄnderungsgrößen` (19 characters, 23 bytes) is reported as longer than 20. `名前` (2 characters, 6 bytes) passes `ShortVariable`, while `ab` is reported. | `len(name)` in `internal/rules/naming/naming.go` and `util.LengthWithoutPrefixesAndSuffixes` |
| [#257](https://github.com/quality-gates/messgo/issues/257) | Type-name rules leave out `class` in `json` and `xml` | Tools that group by class lose `ShortClassName`, `LongClassName` and `CamelCaseClassName` findings, while `TooManyFields` on the same type has the class. | These rules call `c.Report`, not `c.ReportClass` or `c.ReportInterface`. |
| [#258](https://github.com/quality-gates/messgo/issues/258) | SARIF `helpUri: "#"` for controversial rules | The SARIF report fails schema validation (`'#' is not a 'uri'`), so consumers that validate may reject it. | `externalInfoUrl="#"` on all six rules in `controversial.xml` |

Starting conditions, replay steps, and expected and actual output are in each
issue and in `replay.sh` sections "Bug 1" to "Bug 3".

## Rejected

- `ShortMethodName` ignores free functions such as `func ab()`. This is
  intentional and is pinned by `TestShortMethodNameIgnoresFreeFunctions`
  (`internal/rules/rules_test.go:2354`).
- `GetBoth() (bool, error)` is not reported by `BooleanGetMethodName`. The rule
  needs exactly one boolean result. This is consistent with phpmd, where the
  boolean is the return type.
- `LongVariable` with both `subtract-prefixes` and `subtract-suffixes`
  subtracts both (36 − 6 − 13 = 17). This matches phpmd's
  `Strings::lengthWithoutPrefixesAndSuffixes`.

## Unresolved

None.

## Usability observations

- Observation: the `ExcessiveReturnCount` message says "reduce the number of
  results to less than 3", but 3 results are allowed. Only 4 or more are
  reported.
  - Suggestion: say "to at most 3", or report from `maxresults` inclusive.
- Observation: in SARIF, `GlobalVariable`'s `helpUri` is the messgo repository
  root (`https://github.com/quality-gates/messgo`), not a rule-specific page.
  It is valid, but it does not help the user.

## Not explored

- The `design` ruleset metrics (`CouplingBetweenObjects`,
  `LackOfCohesionOfMethods`) on real modules.
- `--strict`. The docs mark it as reserved, and messgo has no suppression
  syntax to exercise.
- Uploading SARIF to GitHub code scanning. The validation here is offline and
  schema-only.

## Environment

- No Docker containers, images or volumes were created.
- The scratch modules and a Python venv (`jsonschema`) were in the supplied
  `TMPDIR` and were removed at the end of the pass.
