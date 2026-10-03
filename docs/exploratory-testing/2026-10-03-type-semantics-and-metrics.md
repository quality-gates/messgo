# Exploratory testing: type semantics and metrics (2026-10-03)

An exploratory pass through the public `messgo` CLI targeting control-flow nesting semantics, sealed interface satisfaction with type aliases, composite literal key handling, and method analysis across all named Go types across the built-in rulesets (`codesize`, `unusedcode`, `cleancode`, `opinionated`, `naming`, `design`, and `controversial`).

- **Build:** `go build -o messgo ./cmd/messgo` at `becb5a0` (v0.5.6).
- **Configuration:** built-in rulesets (`go`, `codesize`, `unusedcode`, `cleancode`, `opinionated`, `design`, `naming`, `controversial`), plus custom XML rulesets adjusting thresholds as described in `docs/rules.md`. Every observation below comes from an uninstrumented binary.
- **Starting state:** clean scratch modules created under temporary directories, testing idiomatic Go language idioms (sealed interfaces, type aliases, type conversions, non-struct named types, composite literals, and control-flow nesting).
- **Evidence:** [`replay.sh`](2026-10-03-type-semantics-and-metrics/replay.sh) rebuilds every confirmed finding from an empty temp dir. Its output is recorded in [`replay-output.txt`](2026-10-03-type-semantics-and-metrics/replay-output.txt). Run it as `replay.sh ./messgo`.

## Journeys

### J1: measure control-flow nesting and cognitive complexity in codesize

Goal: `messgo <path> text codesize` flags functions whose cognitive complexity exceeds the configured threshold, properly penalizing control-flow nesting in all supported branching constructs (`if`, `else`, `switch`, `for`, `select`) per the SonarSource specification and `docs/rules.md`.

- Ordinary path:
  - `CognitiveComplexity`: correctly scores nesting penalties in `if` bodies, `switch` cases, `for` loops, closures, and binary operator chains (`&&` / `||`).
  - `NestingDepth`: correctly handles `else` blocks (`n.Else.(*ast.BlockStmt)`) at `depth+1`, whilst keeping `else if` chains at the parent depth.
- Failed:
  - `CognitiveComplexity` fails to increment the nesting depth for statements inside `else` blocks (`n.Else.(*ast.BlockStmt)`). While `v.inc()` scores +1 for the `else` keyword, `ast.Walk(v, blk)` is invoked without `v.incNesting()` / `v.decNesting()`. Statements inside `else` (`if`, `for`, `switch`, `select`) are scored at nesting level 0 instead of nesting level 1 (#234).

### J2: verify sealed-interface satisfaction with Go type aliases in unused code

Goal: `messgo <path> text unusedcode` identifies unexported methods that satisfy package interfaces (the sealed-interface / marker-method idiom) as used, recognizing Go's predeclared type aliases (`any` and `interface{}`, `byte` and `uint8`, `rune` and `int32`).

- Ordinary path:
  - `UnusedPrivateMethod`: correctly marks concrete methods as used when their parameter and return types textually match a same-named interface method declared in the same package.
- Failed:
  - `sameSignature` in `internal/model/model.go` compares parameter and return types using literal string equality (`a.Params[i].Type != b.Params[i].Type`).
  - When an interface method specifies `any` (`handle(any) bool`) and a concrete struct implements it using `interface{}` (`handle(interface{}) bool`) — or vice-versa, or using `byte` vs `uint8`, `rune` vs `int32` — `sameSignature` returns `false`.
  - Consequently, `InterfaceMethodSatisfied` returns `false`, and `UnusedPrivateMethod` fires a false positive violation with exit code 2: `Avoid unused private methods such as 'handle'` (#235).

### J3: detect duplicate keys with type conversions in clean code

Goal: `messgo <path> text cleancode` flags duplicate keys in maps and composite literals (`DuplicatedArrayKey`), catching duplicate keys across constant literals, unary expressions, identifiers, selector expressions, and explicit type conversions.

- Ordinary path:
  - `DuplicatedArrayKey`: correctly flags duplicate string literals, integer constants (including matching hex and decimal representations), negative numbers, identifiers, and package-qualified constants (`http.StatusOK`).
- Failed:
  - `literalKey` in `internal/model/query.go` only recognizes `*ast.ParenExpr`, `*ast.UnaryExpr`, `*ast.BasicLit`, `*ast.Ident`, and `*ast.SelectorExpr`.
  - For `*ast.CallExpr` (`Type(value)`), `literalKey` returns `"", false`.
  - In Go, map literals are commonly keyed by typed constants or conversions (e.g. `time.Duration(1)`, `uint32(10)`, or custom enum types like `StatusCode(200)`). Duplicate keys using explicit type conversions are completely ignored by `DuplicatedArrayKey`, which exits 0 clean (#236).

### J4: audit unexported methods across all named Go types in unused code

Goal: `messgo <path> text unusedcode` detects unused private methods on all named types declared in the package, including non-struct types (`type StatusCode int`, `type HandlerFunc func() error`, `type StringSet map[string]struct{}`, `type IDList []string`).

- Ordinary path:
  - `UnusedPrivateMethod`: flags uncalled unexported methods on struct types (`type MyStruct struct`).
- Failed:
  - `build.go`'s `collectTypes` only creates `*model.Class` instances for `*ast.StructType`. Non-struct named types are omitted from `file.Classes`, leaving methods on non-struct types with `fn.Class == nil`.
  - Because `UnusedPrivateMethod` is implemented as a `ClassRule` (`ApplyClass`) and only iterates over `file.Classes`, methods on non-struct types are never inspected.
  - Consequently, dead unexported methods on non-struct types are silently ignored, and messgo exits 0 clean (#237).

## Confirmed bugs

Each bug recurred on 3 of 3 runs and is verified in the clean replay harness.

| Issue | Finding | Impact | Root cause |
| :--- | :--- | :--- | :--- |
| [#234](https://github.com/quality-gates/messgo/issues/234) | `CognitiveComplexity` ignores nesting inside `else` blocks (`*ast.BlockStmt`) | Control flow nested in `else` blocks (`if`, `for`, `switch`, `select`) misses the +1 nesting penalty, undercounting cognitive complexity. | `visitIf` (`internal/metrics/metrics.go:317`) calls `v.inc()` but omits `v.incNesting()` and `v.decNesting()` around `ast.Walk(v, blk)`. |
| [#235](https://github.com/quality-gates/messgo/issues/235) | `InterfaceMethodSatisfied` misses type aliases (`any` vs `interface{}`, `byte` vs `uint8`, `rune` vs `int32`) | Concrete methods satisfying package interfaces with equivalent aliased types are falsely flagged by `UnusedPrivateMethod` with exit code 2. | `sameSignature` (`internal/model/model.go:225`) uses literal string comparison (`a.Params[i].Type != b.Params[i].Type`). |
| [#236](https://github.com/quality-gates/messgo/issues/236) | `DuplicatedArrayKey` ignores duplicate keys with type conversions (`*ast.CallExpr`) | Duplicate map keys using typed conversions (e.g. `time.Duration(1)`, `uint32(10)`, `StatusCode(200)`) are silently ignored. | `literalKey` (`internal/model/query.go:332`) only checks `ParenExpr`, `UnaryExpr`, `BasicLit`, `Ident`, and `SelectorExpr`, returning `"", false` for `CallExpr`. |
| [#237](https://github.com/quality-gates/messgo/issues/237) | `UnusedPrivateMethod` ignores unexported methods on non-struct named types | Dead unexported methods on primitive aliases, function types, slice types, or map types are never reported. | `build.go:80` only creates `*model.Class` for structs; `UnusedPrivateMethod` only implements `ClassRule` and visits `file.Classes`. |

Replay steps, expected results, and actual outputs are in each filed issue and in `replay.sh` sections B1–B4.

## Rejected

- Type assertions in `for` loops and `if` init: comma-ok assertions in loop headers (`for v, ok := x.(T); ok;`) are properly marked safe by `collectSafeTypeAssertions`.
- Type assertions ignoring the boolean flag (`v, _ := x.(T)`): correctly treated as safe because the 2-value assignment form never panics in Go.
- Variadic boolean parameters (`b ...bool`): `BooleanArgumentFlag` correctly skips `...bool` because its type string is `"...bool"`, not `"bool"`.

## Unresolved

None.

## Usability observations

- Observation: `ConstantNamingConventions` uses PHPMD's message template `"Constant {0} should be defined in uppercase"`. In Go, constants follow MixedCaps conventions, and the rule flags identifiers containing underscores (such as `ALL_CAPS` or `snake_case`). Reporting `"Constant MY_CONSTANT should be defined in uppercase"` when `MY_CONSTANT` is already uppercase is confusing to Go developers.
  - Suggestion: Customize the message template in `rulesets/naming.xml` or Go adaptations to say `"Constant {0} should be defined in MixedCaps without underscores"`.
- Observation: `ShortMethodName` and `CamelCaseMethodName` report violations against top-level free functions using method terminology (`"The method '...' ..."`).
  - Suggestion: Distinguish free functions from methods in report message formatting when `!fn.IsMethod()`.

## Not explored

- Complex interactions between generic type parameters and cross-package coupling analysis (tracked in #222).
- Performance on repositories with deep directory structures exceeding 10,000 files.
