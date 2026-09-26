# Exploratory testing: rules and metrics (2026-09-26)

An exploratory pass through the public `messgo` CLI targeting rule semantics,
language construct coverage, and metrics across the built-in rulesets
(`opinionated`, `cleancode`, `codesize`, `design`, `naming`, and `explicitness`).

- **Build:** `go build -o messgo ./cmd/messgo` at `8a0867a` (v0.5.1).
- **Configuration:** built-in rulesets (`go`, `opinionated`, `cleancode`,
  `codesize`, `design`, `naming`, `explicitness`), plus custom XML rulesets
  adjusting thresholds as described in `docs/rules.md`. Every observation below
  comes from an uninstrumented binary.
- **Starting state:** clean scratch modules created under temporary directories,
  testing both idiomatic Go language idioms (methods, generics, type switches,
  slice operations, package-qualified constants) and error states.
- **Evidence:** [`replay.sh`](2026-09-26-rules-and-metrics/replay.sh) rebuilds
  every confirmed finding from an empty temp dir. Its output is recorded in
  [`replay-output.txt`](2026-09-26-rules-and-metrics/replay-output.txt). Run it
  as `replay.sh ./messgo`.

## Journeys

### J1: enforce Go-specific code smells with the opinionated ruleset

Goal: `messgo <path> text opinionated` flags unchecked type assertions,
identical branches across branching constructs, and excessive struct embedding
depth, while remaining silent on safe constructs.

- Ordinary path:
  - `UncheckedTypeAssertion`: correctly reports bare type assertions
    (`_ = x.(T)`, `return x.(T)`, `fn(x.(T))`, assertions in closures) with exit
    code 2. Correctly stays silent on comma-ok forms (`v, ok := x.(T)`,
    `var v, ok = x.(T)`, `if v, ok := x.(T); ok`, `for v, ok := x.(T); ok;`)
    and type assertions inside type switch headers (`switch x.(type)`).
  - `StructEmbeddingDepth`: correctly calculates transitive embedding chains
    across within-package structs, follows pointer embeddings (`*Base`),
    traverses parenthesized embeddings, avoids infinite loops on recursive or
    mutually-embedded structs, and treats cross-package embeddings as depth-0
    leaves. Correctly flags depth > 3.
  - `IdenticalBranches`: correctly flags single `if/else` pairs and `switch`
    cases with identical statement lists.
- Failed:
  - `IdenticalBranches` completely ignores `else if` chains when comparing
    branches (#195).
  - `IdenticalBranches` completely ignores type switches (`switch x.(type)`)
    containing duplicate case bodies (#195).

### J2: track mutations and data flow through slices in design and explicitness

Goal: `GlobalVariable` (in `design`) reports package-level state that is
mutated anywhere in the package, and `ImplicitOutput` (in `explicitness`) reports
functions that mutate caller data through slice parameters or receiver fields.

- Ordinary path:
  - `GlobalVariable`: flags package-level variables modified by assignment,
    reassignment, increment/decrement, field/map writes (`g.f = x`, `g[k] = v`),
    address-of (`&g`), channel sends (`g <- x`), `close(g)`, and direct calls
    to `copy(g, src)` or `sort.Ints(g)`.
  - `ImplicitOutput`: flags writes to data that a parameter shares with the
    caller (`delete(m, k)`, `p.m[k] = v`, `copy(p, src)`).
- Failed:
  - Slices or arrays mutated via subslice or full-slice expressions
    (`*ast.SliceExpr`) — including `copy(arr[:], src)` (the only valid way to copy
    into an array in Go!), `copy(slice[1:], src)`, `sort.Ints(slice[1:])`,
    `slices.Sort(slice[:])`, and `clear(slice[:])` — are not recognized by
    `util.RootIdent`.
  - As a result, `GlobalVariable` treats package variables mutated via slice
    expressions as immutable constants and exits 0 (#194).
  - Similarly, `ImplicitOutput` misses functions that mutate slice parameters
    via slice expressions (`sort.Ints(s[1:])`, `copy(s[:], src)`) and exits 0
    (#194).

### J3: audit clean code, naming conventions, and cognitive complexity

Goal: `DuplicatedArrayKey` flags duplicate literal keys in maps and composite
literals, `BooleanGetMethodName` checks boolean getters, and
`CognitiveComplexity` measures control-flow and recursion complexity.

- Ordinary path:
  - `DuplicatedArrayKey`: correctly flags duplicate string literals, integer
    constants (including matching `0x10` with `16`), negative numbers, and local
    constant identifiers.
  - `CognitiveComplexity`: correctly measures control-flow nesting penalties,
    binary operator sequences, and direct recursion in free functions.
- Failed:
  - `DuplicatedArrayKey` fails to detect duplicate map keys when the keys are
    package-qualified constants (e.g. `http.StatusOK`, `time.Second`), which are
    `*ast.SelectorExpr` nodes (#196).
  - `CognitiveComplexity` fails to count direct recursion in methods
    (`w.Walk()`), scoring them +0 instead of +1, because `visitCall` only inspects
    `*ast.Ident` callee expressions (#197).

## Confirmed bugs

Each bug recurred on 3 of 3 runs and is verified in the clean replay harness.

| Issue | Finding | Impact | Root cause |
| :--- | :--- | :--- | :--- |
| [#194](https://github.com/quality-gates/messgo/issues/194) | `RootIdent` does not peel slice expressions (`*ast.SliceExpr`) | `GlobalVariable` misses globals mutated via `copy(arr[:], src)`, `sort.Ints(s[1:])`, or `slices.Sort(s[:])`. `ImplicitOutput` misses parameter mutations via slice expressions. | `util.RootIdent` (`internal/util/astutil.go:213`) lacks a `*ast.SliceExpr` case. |
| [#195](https://github.com/quality-gates/messgo/issues/195) | `IdenticalBranches` misses `else if` chains and type switches (`*ast.TypeSwitchStmt`) | Copy-pasted logic in multi-branch conditionals (`if / else if`) and type switches is not reported. | `checkIfElse` requires `n.Else` to be `*ast.BlockStmt` (which fails on `*ast.IfStmt`); `ApplyFunc` only checks `*ast.SwitchStmt` and ignores `*ast.TypeSwitchStmt`. |
| [#196](https://github.com/quality-gates/messgo/issues/196) | `DuplicatedArrayKey` ignores package-qualified identifier keys (`*ast.SelectorExpr`) | Duplicate keys using imported package constants (e.g. `http.StatusOK`, `time.Second`) are silently ignored in maps and composite literals. | `literalKey` (`internal/model/query.go:322`) only checks `ParenExpr`, `UnaryExpr`, `BasicLit`, and `Ident`, returning `"", false` for `SelectorExpr`. |
| [#197](https://github.com/quality-gates/messgo/issues/197) | `CognitiveComplexity` ignores direct recursion in methods | Recursive method calls (`w.Walk()`) receive +0 instead of the +1 direct recursion increment, undercounting complexity for recursive types. | `visitCall` (`internal/metrics/metrics.go:341`) only checks `n.Fun.(*ast.Ident)`, which fails on method selectors (`recv.Method()`). |

Replay steps, expected results, and actual outputs are in each filed issue and in
`replay.sh` sections B1–B4.

## Rejected

- Generic type instantiations in struct embeds (`type B struct { A[int] }`):
  `embeddedTypeName` correctly strips index expressions to `"A"` and resolves
  within-package embedding chains.
- Nested type assertions in comma-ok assignments (`v, ok := x.(any).(int)`):
  the outer assertion is properly marked safe, while the unchecked inner
  assertion `x.(any)` is flagged. This is correct since the inner assertion can
  still panic.

## Unresolved

None.

## Usability observations

- Observation: `BooleanGetMethodName` checks top-level free functions in
  addition to struct methods, because `check(c, fn)` does not guard on
  `fn.IsMethod()`. When reporting a free function `getStatus() bool`, it formats
  the message as `"The 'getStatus()' method which returns a boolean should be
  named 'is...()' or 'has...()'"` (referring to a free function as a method).
  - Suggestion: Guard `BooleanGetMethodName` on `fn.IsMethod()` if the rule is
    intended only for methods, or adapt the message template to distinguish
    functions from methods.
- Observation: When `DuplicatedArrayKey` flags a duplicate key, the message
  refers to "The array key '...'". In Go terminology, these are map keys or
  composite literal keys.
  - Suggestion: Align the message template with Go nomenclature while preserving
    phpmd compatibility where expected.

## Not explored

- Complex interactions between generic type parameters and cross-package
  coupling analysis.
- Performance on repositories with deep directory structures exceeding 10,000 files.
