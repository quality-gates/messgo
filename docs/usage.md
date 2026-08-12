# Usage

Command shape:

```console
messgo <paths> <format> <ruleset[,ruleset...]> [options]
```

- **paths** — comma-separated files or directories. Directories are walked;
  `vendor/`, `node_modules/`, and `.git/` are skipped.
- **format** — `text`, `xml`, `json`, `html`, `ansi`, `github`, `gitlab`,
  `checkstyle`, or `sarif`.
- **ruleset** — one or more built-in names or paths to phpmd-format ruleset XML.

`text` format prints one finding per line as `file:line  Rule  message`.

## Examples

```console
messgo ./... text codesize
messgo ./internal,./cmd json naming,unusedcode
messgo main.go xml codesize,design,cleancode --minimumpriority 2
messgo ./... text codesize,design --only CyclomaticComplexity,GlobalVariable
messgo ./... text go --disable LongVariable
messgo ./... sarif go --ignore-tests --reportfile reports/messgo.sarif
```

`--only` (alias `--enable`) and `--disable` filter by **rule name** within the
ruleset(s) you load. `--only` keeps the listed rules; `--disable` drops them;
combine them to whitelist then subtract. Names match report output
(e.g. `CyclomaticComplexity`, `ElseExpression`). `--only` cannot pull in a rule
the loaded ruleset does not already include.

## Options

| Option | Effect |
| :--- | :--- |
| `--minimumpriority <n>` | Only run rules with priority ≤ n. |
| `--maximumpriority <n>` | Only run rules with priority ≥ n. |
| `--reportfile <file>` | Write the report to a file instead of stdout. |
| `--suffixes <list>` | File extensions to scan (default: `go`). |
| `--exclude <list>` | Path substrings to exclude. |
| `--enable`, `--only <list>` | Run only these rules (comma-separated names) from the loaded ruleset(s). |
| `--disable <list>` | Skip these rules (comma-separated names). |
| `--ignore-tests` | Skip `*_test.go` files. |
| `--strict` | Reserved; also report suppressed violations when suppressions exist. |
| `--color` | Colorize text output. |
| `--verbose`, `-v` | Verbose diagnostics. |
| `--ignore-errors-on-exit` | Exit `0` even if parse errors occurred. |
| `--ignore-violations-on-exit` | Exit `0` even if violations were found. |
| `--version` | Print version. |
| `--help`, `-h` | Show help. |

## Exit codes

| Code | Meaning |
| :--: | :--- |
| **0** | Clean — no violations |
| **1** | Error (bad arguments, parse failure, …) |
| **2** | Violations found |

Exit codes match phpmd so a non-zero exit fails CI without extra scripting.

## Install variants

```console
go install github.com/quality-gates/messgo/cmd/messgo@latest
```

Prebuilt macOS archives for stable versions:
[GitHub Releases](https://github.com/quality-gates/messgo/releases).

Homebrew (stable formula from the org tap):

```console
brew install quality-gates/tap/messgo
```

From a checkout:

```console
go build -o messgo ./cmd/messgo
```

## Reports

Formats: `text`, `xml`, `json`, `html`, `ansi`, `github`, `gitlab`,
`checkstyle`, `sarif`. Use `--reportfile` to write the full report to disk.
