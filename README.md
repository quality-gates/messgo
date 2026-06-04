# messgo

**messgo** is a [PHP Mess Detector](https://phpmd.org) (phpmd) port for Go: it
is written in Go *and* analyzes Go source code, applying phpmd's rule catalog,
ruleset format, message templates, CLI surface, and report renderers — adapted
faithfully to Go semantics.

Where phpmd parses PHP via pdepend, messgo parses Go via the standard-library
`go/ast`. Every phpmd concept maps onto a Go analog: a PHP *class* → a Go named
struct type (plus its methods), a *property* → a struct field, a *method* /
*function* → a Go method / function, *private* → *unexported*.

## Install / build

```
go build -o messgo ./cmd/messgo
```

## Usage

The command line mirrors phpmd's `phpmd <paths> <format> <ruleset>`:

```
messgo <paths> <format> <ruleset[,...]> [options]
```

Examples:

```
messgo ./... text codesize
messgo ./internal,./cmd json naming,unusedcode
messgo main.go xml codesize,design,cleancode --minimumpriority 2
```

* **paths** — comma-separated files or directories (directories are walked;
  `vendor/`, `node_modules/`, `.git/` are skipped).
* **format** — `text`, `xml`, `json`, `html`, `ansi`, `github`, `gitlab`,
  `checkstyle`, `sarif`.
* **ruleset** — comma-separated built-in rulesets (`cleancode`, `codesize`,
  `controversial`, `design`, `naming`, `unusedcode`) or paths to phpmd-format
  ruleset XML files. The bundled **`go`** ruleset pulls in all of the above but
  tunes a few rules whose PHP defaults misfire on idiomatic Go (drops
  `ShortVariable`, `Design/ExitExpression`, `Design/CountInLoopExpression`, and
  raises `LongVariable`'s maximum). On this codebase `go` reports ~19 findings
  versus ~441 at raw PHP defaults.

Ruleset XML supports phpmd's `<rule ref="...">` form, `<exclude name="..."/>`
children, and single-rule property/priority overrides — so you can compose
your own tuned ruleset the same way phpmd does.

Options: `--minimumpriority`, `--maximumpriority`, `--reportfile`,
`--suffixes`, `--exclude`, `--ignore-tests`, `--strict`, `--color`,
`--verbose`, `--ignore-errors-on-exit`, `--ignore-violations-on-exit`,
`--version`, `--help`.

Exit codes match phpmd: **0** clean · **1** error · **2** violations found.

## Rulesets

All six phpmd rulesets are implemented. Rules with a direct Go analog reproduce
phpmd's behavior and message templates exactly; rules that are intrinsically
PHP-specific are either adapted to the nearest Go idiom or omitted (Go's
compiler already enforces several of them). See **[PARITY.md](PARITY.md)** for
the complete rule-by-rule mapping and the manual phpmd comparison evidence.

| Ruleset | Implemented |
| --- | --- |
| Code Size | CyclomaticComplexity, NPathComplexity, ExcessiveMethodLength, ExcessiveClassLength, ExcessiveParameterList, ExcessivePublicCount, TooManyFields, TooManyMethods, TooManyPublicMethods, ExcessiveClassComplexity |
| Naming | ShortClassName, LongClassName, ShortVariable, LongVariable, ShortMethodName, ConstantNamingConventions, BooleanGetMethodName, ConstructorWithNameAsEnclosingClass |
| Unused Code | UnusedPrivateField, UnusedLocalVariable, UnusedPrivateMethod, UnusedFormalParameter |
| Clean Code | BooleanArgumentFlag, ElseExpression, IfStatementAssignment, DuplicatedArrayKey |
| Design | ExitExpression, GotoStatement, CountInLoopExpression, DevelopmentCodeFragment, EmptyCatchBlock, CouplingBetweenObjects |
| Controversial | CamelCaseClassName, CamelCaseMethodName, CamelCasePropertyName, CamelCaseParameterName, CamelCaseVariableName |

## Architecture

```
cmd/messgo            entry point
internal/model        go/ast → phpmd-style artifacts (Class/Interface/Method/Function/Field/Parameter)
internal/metrics      CCN, NPath, LOC (pdepend-equivalent algorithms)
internal/rule         Rule interface, Violation, RuleSet, dispatch engine
internal/rules/*      rule implementations, grouped by ruleset
internal/ruleset      phpmd-format ruleset XML loader (+ embedded built-ins)
internal/report       renderers (text/xml/json/html/ansi/github/gitlab/checkstyle/sarif)
internal/runner       file discovery + orchestration
internal/cli          command-line interface
```

Rules register themselves under their phpmd class name (e.g.
`PHPMD\Rule\CyclomaticComplexity`); the ruleset XML — byte-for-byte phpmd's own
`codesize.xml` etc. — wires class → metadata (message, priority, properties).

## Tests

```
go test ./...
```

The test suite includes metric tests pinned to numbers produced by **real
phpmd 2.15.0** (cyclomatic complexity 12, NPath 324 on a reference function),
plus per-ruleset behavioral tests and CLI/exit-code tests.
