# messgo

**messgo** is a [PHP Mess Detector](https://phpmd.org) (phpmd) port for Go: it
is written in Go *and* analyzes Go source code, applying phpmd's rule catalog,
ruleset format, message templates, CLI surface, and report renderers — adapted
faithfully to Go semantics.

Where phpmd parses PHP via pdepend, messgo parses Go via the standard-library
`go/ast`. By default it uses idiomatic Go principles (the bundled `go`
ruleset), but a fuller set of checks that more closely emulates standard phpmd
rules can be optionally enabled.

## Getting started

### 1. Install the binary

Install with `go install` to put `messgo` on your `PATH`:

```bash
go install github.com/quality-gates/messgo/cmd/messgo@latest
```

Or build it from a checkout of this repo:

```bash
go build -o messgo ./cmd/messgo
```

### 2. Run it on your code

The simplest way to start is the same command messgo uses to check *itself* in
CI — point it at a directory using the bundled `go` ruleset, with plain `text`
output, skipping test files:

```bash
./messgo ./internal text go --ignore-tests
```

That's the whole pattern. The command is always:

```bash
messgo <paths> <format> <ruleset[,...]> [options]
```

* **paths** — comma-separated files or directories. Directories are walked;
  `vendor/`, `node_modules/`, and `.git/` are skipped.
* **format** — `text`, `xml`, `json`, `html`, `ansi`, `github`, `gitlab`,
  `checkstyle`, or `sarif`.
* **ruleset** — one or more rulesets (see [Rulesets](#rulesets)). Start with
  `go`.

### 3. Read the output

`text` format prints one violation per line as `file:line  Rule  message`:

```
internal/cli/cli.go:131  ShortVariable  Avoid variables with short names like a. Configured minimum length is 3.
internal/cli/cli.go:157  ShortVariable  Avoid variables with short names like ok. Configured minimum length is 3.
```

### 4. Check the exit code

Exit codes match phpmd exactly:

| Code | Meaning |
| :--: | :--- |
| **0** | Clean — no violations |
| **1** | Error (e.g. bad arguments, parse failure) |
| **2** | Violations found |

This makes messgo drop straight into a build script or CI step: a non-zero
exit fails the job.

## Use it in CI (GitHub Actions)

messgo runs on itself in CI. Here is the exact job from this repo's
`.github/workflows/ci.yml` — copy it as a starting point:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  messgo:
    name: Run messgo
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod

      - name: Build messgo
        run: go build -o messgo ./cmd/messgo

      - name: Run self-analysis
        run: ./messgo ./internal text go,codesize --ignore-tests
```

Because messgo exits `2` when it finds violations, the `Run self-analysis`
step fails the job automatically — no extra scripting needed.

## More usage examples

```bash
messgo ./... text codesize                                   # one ruleset, all packages
messgo ./internal,./cmd json naming,unusedcode               # multiple paths and rulesets
messgo main.go xml codesize,design,cleancode --minimumpriority 2
```

### Options

| Option | Effect |
| :--- | :--- |
| `--minimumpriority <n>` | Only run rules with priority ≤ n. |
| `--maximumpriority <n>` | Only run rules with priority ≥ n. |
| `--reportfile <file>` | Write the report to a file instead of stdout. |
| `--suffixes <list>` | File extensions to scan (default: `go`). |
| `--exclude <list>` | Path substrings to exclude. |
| `--ignore-tests` | Skip `*_test.go` files. |
| `--strict` | Also report suppressed violations. |
| `--color` | Colorize text output. |
| `--verbose`, `-v` | Verbose diagnostics. |
| `--ignore-errors-on-exit` | Exit `0` even if parse errors occurred. |
| `--ignore-violations-on-exit` | Exit `0` even if violations were found. |
| `--version` | Print version. |
| `--help`, `-h` | Show help. |

## Rulesets

Pass rulesets by name (comma-separated), or pass a path to your own
phpmd-format ruleset XML file.

| Ruleset | What it checks |
| :--- | :--- |
| **`go`** | **Recommended default.** Pulls in all rulesets below, but tunes the rules whose PHP defaults misfire on idiomatic Go: drops `ShortVariable`, `Design/ExitExpression`, `Design/CountInLoopExpression`, `Design/GlobalVariable`, `CleanCode/ElseExpression`, `CleanCode/BooleanArgumentFlag`, and `UnusedCode/UnusedFormalParameter`, and raises `LongVariable`'s maximum. On this codebase `go` reports ~19 findings versus ~441 at raw PHP defaults. |
| `codesize` | CyclomaticComplexity, NPathComplexity, ExcessiveMethodLength, ExcessiveClassLength, ExcessiveParameterList, ExcessivePublicCount, TooManyFields, TooManyMethods, TooManyPublicMethods, ExcessiveClassComplexity |
| `naming` | ShortClassName, LongClassName, ShortVariable, LongVariable, ShortMethodName, ConstantNamingConventions, BooleanGetMethodName, ConstructorWithNameAsEnclosingClass |
| `unusedcode` | UnusedPrivateField, UnusedLocalVariable, UnusedPrivateMethod, UnusedFormalParameter |
| `cleancode` | BooleanArgumentFlag, ElseExpression, IfStatementAssignment, DuplicatedArrayKey |
| `design` | ExitExpression, GotoStatement, CountInLoopExpression, DevelopmentCodeFragment, EmptyCatchBlock, CouplingBetweenObjects, GlobalVariable |
| `controversial` | CamelCaseClassName, CamelCaseMethodName, CamelCasePropertyName, CamelCaseParameterName, CamelCaseVariableName |
| `opinionated` | **Opt-in, not part of idiomatic Go.** Bundles the rules the `go` ruleset deliberately drops because they fight Go conventions: `ElseExpression` (`else` is idiomatic), `BooleanArgumentFlag` (bool params fill Go's stdlib), `UnusedFormalParameter` (unused params are required to satisfy interfaces and handler signatures), and `GlobalVariable` (mutable package-level variables — though sentinel errors, compiled regexps, and registries are idiomatic). Run them if you want a stricter, more PHP-flavoured style. |

Rules with a direct Go analog reproduce phpmd's behavior and message templates
exactly; rules that are intrinsically PHP-specific are either adapted to the
nearest Go idiom or omitted (Go's compiler already enforces several of them).

The `opinionated` ruleset is the home for checks that come from phpmd's
PHP/OO heritage but are *not* idiomatic Go. They stay available — run
`messgo ./... text opinionated`, or `go,opinionated` to combine — but the
default `go` ruleset leaves them off so a clean run reflects idiomatic Go.

### Custom rulesets

Ruleset XML supports phpmd's `<rule ref="...">` form, `<exclude name="..."/>`
children, and single-rule property/priority overrides — so you can compose your
own tuned ruleset the same way phpmd does, then pass its path as the ruleset
argument.

## Running the tests

```bash
go test ./...
```

The suite includes metric tests pinned to numbers produced by **real phpmd
2.15.0** (cyclomatic complexity 12, NPath 324 on a reference function), plus
per-ruleset behavioral tests and CLI/exit-code tests.
