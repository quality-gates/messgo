# Go mess signs messgo could detect but doesn't

## Question

What Go "mess signs" (code smells / quality violations) could messgo be detecting
that it currently isn't? Is the answer "we're already good", or are there real gaps?

## Method

Primary sources only:

- messgo's own rule implementations in `internal/rules/` and metrics in
  `internal/metrics/metrics.go`.
- PHPMD's shipped rulesets, taken from the canonical source on GitHub
  (`github.com/phpmd/phpmd`, `src/main/resources/rulesets/*.xml`) cross-checked
  against the PHP rule classes PHPMD actually implements
  (`src/main/php/PHPMD/Rule/**/*.php`). The phpmd.org rules pages under-document
  the shipped set (e.g. `CamelCaseNamespace` has a PHP class but is missing from
  the index); the GitHub source is authoritative. Some entries in PHPMD's XMLs
  (`AbstractNaming`, `NoPackage`, `Ncss*`, `MisleadingVariableName`, the
  `*NamingConventions` / `AvoidFieldNameMatching*` naming rules) are PMD-inherited
  XPath rules with no PHP implementation — PHPMD and messgo both skip them with a
  warning, so they are not counted as part of PHPMD's implemented ruleset.
- Go linters: staticcheck (`staticcheck.dev/docs/checks`), revive
  (`mgechev/revive`), golangci-lint (`golangci-lint.run/usage/linters`).

## Current coverage

messgo implements 37 of PHPMD's 46 implemented rules, plus 2 Go-specific
extensions (`GlobalVariable`, `LackOfCohesionOfMethods`). Metrics computed
(`internal/metrics/metrics.go`): cyclomatic complexity (CCN), NPath complexity,
lines-of-code (physical and effective). No cognitive complexity, no nesting
depth.

Rules are registered in `internal/rules/*/*.go` (via `rule.Register`) and surfaced
through the builtin rulesets in `internal/ruleset/builtin/*.xml`. The `go`
ruleset is the recommended default; `opinionated` holds the PHP-flavoured rules
the `go` ruleset deliberately drops.

| Ruleset | Rule | Fires on | Source |
| :--- | :--- | :--- | :--- |
| cleancode | BooleanArgumentFlag | `bool` parameters (SRP smell); opinionated-only | `internal/rules/cleancode/cleancode.go:25` |
| cleancode | ElseExpression | `else` branches; opinionated-only | `internal/rules/cleancode/cleancode.go:57` |
| cleancode | IfStatementAssignment | plain `=` in an `if` initializer (not `:=`) | `internal/rules/cleancode/cleancode.go:71` |
| cleancode | DuplicatedArrayKey | duplicate keys in a composite literal | `internal/rules/cleancode/cleancode.go:85` |
| codesize | CyclomaticComplexity | CCN ≥ threshold (default 10) | `internal/rules/codesize/codesize.go:64` |
| codesize | NPathComplexity | NPath ≥ threshold (default 200) | `internal/rules/codesize/codesize.go:87` |
| codesize | ExcessiveMethodLength (LongMethod) | function LOC ≥ threshold (default 100) | `internal/rules/codesize/codesize.go:110` |
| codesize | ExcessiveClassLength (LongClass) | struct LOC ≥ threshold (default 1000) | `internal/rules/codesize/codesize.go:142` |
| codesize | ExcessiveParameterList (LongParameterList) | param count ≥ threshold (default 10) | `internal/rules/codesize/codesize.go:174` |
| codesize | ExcessivePublicCount | exported members ≥ threshold (default 45) | `internal/rules/codesize/codesize.go:197` |
| codesize | TooManyFields | struct field count > threshold (default 15) | `internal/rules/codesize/codesize.go:231` |
| codesize | TooManyMethods | method count > threshold (default 25) | `internal/rules/codesize/codesize.go:254` |
| codesize | TooManyPublicMethods | exported method count > threshold (default 10) | `internal/rules/codesize/codesize.go:293` |
| codesize | ExcessiveClassComplexity (WMC) | sum of method CCN ≥ threshold (default 50) | `internal/rules/codesize/codesize.go:335` |
| controversial | CamelCaseClassName | struct/interface name contains `_` | `internal/rules/controversial/controversial.go:30` |
| controversial | CamelCaseMethodName | method name contains `_` | `internal/rules/controversial/controversial.go:45` |
| controversial | CamelCasePropertyName | struct field name contains `_` | `internal/rules/controversial/controversial.go:56` |
| controversial | CamelCaseParameterName | parameter name contains `_` | `internal/rules/controversial/controversial.go:68` |
| controversial | CamelCaseVariableName | local variable name contains `_` | `internal/rules/controversial/controversial.go:81` |
| design | ExitExpression | calls to `os.Exit` / `syscall.Exit`; opinionated-only via `go` exclusion | `internal/rules/design/design.go:69` |
| design | GotoStatement | any `goto` | `internal/rules/design/design.go:83` |
| design | CountInLoopExpression | `len`/`cap` in a loop condition; `go` ruleset excludes | `internal/rules/design/design.go:97` |
| design | DevelopmentCodeFragment | calls to `println`/`print` (configurable) | `internal/rules/design/design.go:110` |
| design | EmptyCatchBlock | empty `if err != nil { }` | `internal/rules/design/design.go:145` |
| design | CouplingBetweenObjects | distinct coupled types via fields/signatures ≥ 13 | `internal/rules/design/design.go:164` |
| design | GlobalVariable | mutable package-level `var`; `go` ruleset excludes | `internal/rules/design/design.go:42` |
| design | LackOfCohesionOfMethods | LCOM4 > 1 (disjoint method groups in a struct) | `internal/rules/design/design.go:309` |
| naming | ShortClassName | struct/interface name < 3 chars | `internal/rules/naming/naming.go:26` |
| naming | LongClassName | struct/interface name > 40 chars | `internal/rules/naming/naming.go:60` |
| naming | ShortVariable | field/param/local name < 3 chars; `go` ruleset excludes | `internal/rules/naming/naming.go:93` |
| naming | LongVariable | field/param/local name > 20 (35 in `go`) | `internal/rules/naming/naming.go:144` |
| naming | ShortMethodName | method name < 3 chars | `internal/rules/naming/naming.go:190` |
| naming | BooleanGetMethodName | `GetX()` returning a single `bool` | `internal/rules/naming/naming.go:219` |
| naming | ConstantNamingConventions | constant name contains `_` | `internal/rules/naming/naming.go:262` |
| naming | ConstructorWithNameAsEnclosingClass | method named after its receiver type | `internal/rules/naming/naming.go:277` |
| unusedcode | UnusedPrivateField | unexported field never selected | `internal/rules/unusedcode/unusedcode.go:21` |
| unusedcode | UnusedLocalVariable | local never read after assignment | `internal/rules/unusedcode/unusedcode.go:70` |
| unusedcode | UnusedPrivateMethod | unexported method never called | `internal/rules/unusedcode/unusedcode.go:36` |
| unusedcode | UnusedFormalParameter | unread param; opinionated-only via `go` exclusion | `internal/rules/unusedcode/unusedcode.go:51` |

Note on scope: only the naming and controversial "class-name" rules implement
`ApplyInterface` (`internal/rule/rule.go:161`). Every codesize and design
class-level rule (`TooManyMethods`, `TooManyPublicMethods`, `TooManyFields`,
`CouplingBetweenObjects`, `LackOfCohesionOfMethods`, `ExcessivePublicCount`,
`ExcessiveClassLength`, `ExcessiveClassComplexity`) is `ApplyClass` only and
never runs against `*model.Interface`. Interfaces are unchecked for size/coupling.

## Gaps vs PHPMD

Authoritative PHPMD rule set: 46 implemented rules (8 CleanCode, 10 CodeSize,
7 Controversial, 9 Design, 8 Naming, 4 UnusedCode) — derived from
`src/main/resources/rulesets/*.xml` intersected with the PHP classes in
`src/main/php/PHPMD/Rule/`. messgo ports 37 and adds 2 Go-specific rules.

PHPMD rules messgo does not port, with a Go-mapping judgement:

| PHPMD rule | Ruleset | Maps to Go? | Judgement |
| :--- | :--- | :--- | :--- |
| StaticAccess | CleanCode | Weak | Go has no static methods; package-level function calls are idiomatic and not "unexchangeable dependencies" in the OO sense. Omit. |
| ErrorControlOperator | CleanCode | No | Go has no `@` suppression operator. No analog. |
| MissingImport | CleanCode | No | Go compiler rejects unresolved identifiers and enforces unused imports. Already enforced by the toolchain. |
| UndefinedVariable | CleanCode | No | Go compiler rejects use of undeclared variables. Already enforced. |
| Superglobals | Controversial | No | Go has no superglobals. No analog. |
| CamelCaseNamespace | Controversial | Weak | Go has no namespaces; packages are named lowercase. Could flag package names containing underscores, but `go` already forbids them in import paths. Low value. |
| EvalExpression | Design | No | Go has no `eval`. `reflect` is a different concern (not arbitrary code execution). No analog. |
| NumberOfChildren | Design | Weak | Go has no inheritance. The nearest analog is "number of embedders of a struct", which is cross-package and not resolvable from a single package's AST. Low value as a literal port. |
| DepthOfInheritance | Design | Partial | Could map to **struct embedding depth** (a struct embedding a struct embedding a struct…). This is a genuine Go mess sign and AST-trivial within a package. See "Go-specific gaps". |

Bottom line on the PHPMD gap: the unported rules are either PHP-specific with no
Go analog (4 of 9 are already enforced by `go build`), or weak fits. The only one
with a real Go-flavoured counterpart is `DepthOfInheritance` → embedding depth.
messgo is essentially complete against PHPMD's *intended* scope; the omissions are
documented and defensible (see the package doc comments in
`internal/rules/cleancode/cleancode.go:1` and `internal/rules/design/design.go:1`).

## Gaps vs Go linters

Surveyed staticcheck (SA/S/ST/QF families), revive's full rule set, and
golangci-lint's enabled-by-default and complexity/design linters. Filtered to
*mess signs* (complexity, coupling, size, design, naming, unused code) and to
what is detectable via `go/ast` (messgo's only input). Correctness bugs (most SA,
errcheck, govet checks) are out of scope for a mess detector and ignored.

Categories linters catch that messgo does not:

| Category | Linter source | messgo has it? | Notes |
| :--- | :--- | :--- | :--- |
| Cognitive complexity | `gocognit`, `cyclop` (golangci-lint); revive `cognitive-complexity` | No | Complements CCN: penalises linear accumulation of `if`/`return` sequences that CCN scores as low. Well-defined metric, AST-feasible. |
| Nesting depth | `nestif` (golangci-lint); revive `max-control-nesting` | No | Deeply nested `if`/`for`/`switch`. Trivial on AST; not captured by CCN or NPath. |
| Function result count | revive `function-result-limit` | No | Go-specific smell (functions returning many values). AST-trivial. |
| Interface method count | `interfacebloat` (golangci-lint) | No | messgo's `TooManyMethods`/`TooManyPublicMethods` are `ApplyClass` only — interfaces are never checked (see `internal/rule/engine.go:44`). "Interface pollution" is a recognized Go smell. |
| Bare (naked) returns | revive `bare-return` | No | `return` with no values in a named-result function. Idiomatic smell, AST-trivial. |
| Unchecked type assertions | revive `unchecked-type-assertion` | No | `x.(T)` without comma-ok. Go-specific smell; AST-trivial. |
| Unreachable code | revive `unreachable-code`; staticcheck SA4 family | No | Dead code after `return`/`panic`/`break`. Complements `unusedcode` (which finds unused *declarations*, not unreachable *statements*). |
| Identical branches | revive `identical-branches` / `identical-switch-branches` | No | `if`/`else` or `switch` cases with identical bodies. Smell, AST-feasible. |
| Confusing naming | revive `confusing-naming` | No | Methods/types differing only by capitalization. Smell, AST-feasible. |
| Modifies parameter / value receiver | revive `modifies-parameter`, `modifies-value-receiver` | No | Reassigning params or value receivers. Smell, AST-feasible. |
| Nested structs | revive `nested-structs` | No | Structs deeply embedding/nesting structs. Overlaps with the embedding-depth gap. |
| `init()` abuse | `gochecknoinits` (golangci-lint) | No | Blanket ban is too blunt, but counting `init()` functions per package or flagging `init()` with non-trivial side effects is a real mess signal. |
| File length | revive `file-length-limit` | No | messgo has `ExcessiveClassLength` (per struct) but nothing caps a whole file. |
| Duplicate code | `dupl` (golangci-lint) | No | Needs tokenized similarity, not pure AST. Lower value, higher cost. |

## Go-specific gaps

Mess signs that are native to Go and that neither PHPMD (a PHP tool) nor messgo
naturally catches, but a Go mess detector should, and which are structurally
detectable via `go/ast`:

1. **Struct embedding depth** (Go analog of `DepthOfInheritance`). A struct that
   embeds a struct that embeds a struct… bundles layers of behaviour and is hard
   to reason about. Single-package AST can count embedded fields transitively;
   cross-package embedding depth is approximate but still useful.
2. **Interface pollution** — interfaces with many methods, or `any`/`interface{}`
   used as a parameter where a concrete narrow type would do. The first half is
   the `interfacebloat` gap above; the second is a distinct "loss of type safety"
   smell.
3. **Naked returns in long/complex functions.** `bare-return` plus a
   complexity/length gate: a naked `return` in a 200-line function is a real
   readability hazard; in a 3-line helper it is idiomatic. The combination is the
   smell, and neither `bare-return` alone nor messgo's `LongMethod` captures it.
4. **`init()` side effects and ordering.** Multiple `init()` functions in one
   file, or `init()` functions that perform I/O / mutate package globals, create
   hidden ordering dependencies. AST-detectable (count `init` decls; inspect
   bodies for calls/assignments).
5. **Excessive return values.** Go allows multiple returns; functions returning
   5+ values are a smell that begs a struct result. AST-trivial.
6. **Unchecked type assertions.** `x.(T)` ignoring the comma-ok form panics on
   mismatch; a mess detector can flag every type assertion that does not bind
   two values. AST-trivial.

Out of scope for an AST-only mess detector (need flow/concurrency analysis, not
in messgo's remit): goroutine leaks, data races, deferred-in-loop bugs, channel
misuse. staticcheck/govet/`deadcode` cover those as correctness; messgo should
not chase them.

## Recommendation

messgo is **not** "already good". It is essentially complete against PHPMD's
intended scope (the unported PHPMD rules genuinely don't map to Go, or are
already enforced by `go build`), but it is missing several high-value Go mess
signs that Go linters catch and that fit messgo's structural, AST-based model
cleanly. Ranked by value-to-effort, with the skeptical filter applied (style nits
excluded):

1. **Interface size rules** (extend `TooManyMethods`/`TooManyPublicMethods` to
   `ApplyInterface`). Highest value, smallest change: the rules and thresholds
   already exist, they just never run on interfaces because they don't implement
   `ApplyInterface`. Closes the most obvious Go-specific gap. Source:
   `internal/rule/engine.go:44`, `internal/rules/codesize/codesize.go:254,293`.
2. **Nesting depth** (new rule, `nestif`-equivalent). Trivial on `go/ast`,
   independent of CCN/NPath, high signal for the "arrow code" smell. Maps to
   revive `max-control-nesting`.
3. **Cognitive complexity** (new metric + threshold rule). More work than the
   above, but the metric is well-specified (SonarSource algorithm, implemented in
   `gocognit`) and catches exactly the linear-accumulation functions CCN
   under-reports. Would be the first new metric since NPath.
4. **Naked return in long/complex functions** (new rule combining `bare-return`
   with the existing LOC/CCN measurements). Go-specific, idiomatic-but-abused
   smell, AST-trivial. Pure `bare-return` without the length gate is too noisy.
5. **Unchecked type assertions** (new rule). AST-trivial, real Go smell, no
   overlap with anything messgo currently does.

Worth doing but lower priority: function-result-count limit, struct embedding
depth (Go `DepthOfInheritance` analog), unreachable-code (complements
`unusedcode`), identical-branches. These are sound but either overlap existing
rules or are narrower signals.

Explicitly **not** worth doing: chasing the unported PHPMD CleanCode rules
(StaticAccess, ErrorControlOperator, MissingImport, UndefinedVariable), Superglobals,
CamelCaseNamespace, EvalExpression, NumberOfChildren. These are PHP-specific or
already enforced by the Go toolchain; porting them would be padding. Duplicate
code (`dupl`) is a real mess sign but needs a tokenized similarity engine, not
`go/ast` — wrong fit for messgo's current architecture.

## Agreed backlog (ADR-0001)

This section records the decisions taken in the grilling session that adopted
this analysis. See `docs/adr/0001-go-mess-sign-backlog.md`.

**Charter:** Go-native mess detection, PHPMD as lineage not ceiling. "Port" in
`CONTEXT.md` sharpened to "Go-native mess detector with PHPMD as its lineage."

**Inclusion bar:** (i) AST-only, no build; (ii) primary signal is maintainability
(would a reviewer flag it in a PR?), correctness edges allowed; (iii) principled
threshold + AST-bounded false positives + minimal overlap; noisy-but-defensible
rules go to `opinionated`, promotion to `go` requires empirical evidence.

**One new metric allowed:** cognitive complexity (SonarSource algorithm). Same
shape as `CyclomaticComplexity` — a computation in `internal/metrics` plus a
threshold rule; no report-schema change (reports emit violations, not metric
values).

**Interface-size is a defect, not a feature:** the engine already dispatches
`ApplyInterface`; the size rules never wired it up. Fix: only `TooManyMethods`
fires on interfaces (threshold 10, `interfacebloat` convention);
`TooManyPublicMethods` and `ExcessivePublicCount` short-circuit on interfaces —
their measures collapse to method count and would duplicate.

**Ranked backlog:**

| Rank | Item | Ruleset | Default threshold | Effort |
| :--- | :--- | :--- | :--- | :--- |
| 1 | Interface-size defect fix (extend `TooManyMethods` to `ApplyInterface`) | `go` | 10 (interface) | tiny |
| 2 | Nesting depth | `go` | 5 (`nestif`) | small |
| 3 | Function-result count | `go` | 3 (revive) | small |
| 4 | Naked return gated by LOC/CCN | `go` | LOC ≥ 50 OR CCN ≥ 10 | small |
| 5 | Cognitive complexity (new metric + rule) | `go` | 20 (`gocognit`) | medium |
| 6 | Unchecked type assertions | `opinionated` | none | small |
| 7 | Identical branches | `opinionated` | none | small |
| 8 | Struct embedding depth | `opinionated` | 3 | medium |

**Rejected (with reason):** unported PHPMD rules (PHP-specific / toolchain-enforced);
unreachable code (staticcheck SA4 covers it, thin gain over `unusedcode`); duplicate
code (needs tokenized similarity, wrong architecture); goroutine leaks / data races /
deferred-in-loop (need flow/concurrency analysis, out of remit).