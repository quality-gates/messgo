# Go mess-sign backlog: what messgo will learn to detect

messgo's charter is Go-native mess detection with PHPMD as its lineage, not a
faithful reimplementation of PHPMD's ruleset. A gap analysis
(`docs/research/go-mess-signs-gap-analysis.md`) found messgo essentially complete
against PHPMD's intended scope but missing Go-specific mess signs that Go linters
(staticcheck, revive, golangci-lint) catch and that fit messgo's AST-only, no-build
model. We decided which gaps to fill, in what order, and what to explicitly reject.

## Status

Accepted.

## Decision

Fill these gaps, in priority order:

1. **Interface-size defect fix** — extend `TooManyMethods` to `ApplyInterface`
   (default threshold 10, matching `interfacebloat`); `TooManyPublicMethods` and
   `ExcessivePublicCount` short-circuit on interfaces (their measures collapse to
   method count and would duplicate). The engine already dispatches
   `ApplyInterface`; the size rules just never wired it up.
2. **Nesting depth** — new `go` rule, threshold 5 (`nestif` convention).
3. **Function-result count** — new `go` rule, threshold 3 (revive convention).
4. **Naked return in long/complex functions** — new `go` rule; fires only when
   gated by LOC ≥ 50 OR CCN ≥ 10, so idiomatic naked returns in short funcs don't
   fire.
5. **Cognitive complexity** — new metric (SonarSource algorithm, `gocognit`
   convention) + threshold rule in `codesize`, default 20. Same shape as
   `CyclomaticComplexity`: a computation in `internal/metrics` plus a rule. No
   report-schema change (reports emit violations, not metric values).
6. **Unchecked type assertions** — new `opinionated` rule; promote to `go` only
   if empirically quiet.
7. **Identical branches** — new `opinionated` rule.
8. **Struct embedding depth** — new `opinionated` rule, threshold 3;
   cross-package embedding is approximate (within-package transitive only).

## Considered Options (rejected)

- **The 9 unported PHPMD rules** (StaticAccess, ErrorControlOperator,
  MissingImport, UndefinedVariable, Superglobals, CamelCaseNamespace,
  EvalExpression, NumberOfChildren, DepthOfInheritance). Rejected: 4 are already
  enforced by `go build`; the rest are PHP-specific with no Go analog, except
  DepthOfInheritance whose Go analog (embedding depth) we adopt as #8.
- **Unreachable code.** Rejected: staticcheck's SA4 family already covers it, and
  the marginal mess signal over messgo's existing `unusedcode` ruleset is thin.
- **Duplicate-code detection** (`dupl`). Rejected: needs a tokenized similarity
  engine, not `go/ast` — wrong fit for messgo's architecture.
- **Goroutine leaks, data races, deferred-in-loop.** Out of scope: require
  flow/concurrency analysis, not structural AST inspection.

## Inclusion bar

A Go-specific gap clears the bar when: (i) it is detectable from `go/ast` alone,
no build; (ii) its primary signal is maintainability — a reviewer would flag it in
a PR — even if it also has a correctness edge (e.g. unchecked type assertions);
(iii) it has a principled threshold, a false-positive pattern bounded by the AST,
and minimal overlap with existing rules. Noisy-but-defensible rules go to the
`opinionated` ruleset, not the default `go` ruleset; promotion to `go` requires
empirical evidence of low noise.