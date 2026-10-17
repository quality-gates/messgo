# messgo

Catch maintainability problems in Go before they calcify: oversized functions
and types, tangled dependencies, dead private code, muddy naming, and other
mess that reviews keep rediscovering.

`messgo` is a local CLI. It parses Go with the standard-library `go/ast`, never
builds or runs your project, and needs no project dependencies installed.
Go 1.26+.

## Quick start

```console
go install github.com/quality-gates/messgo/cmd/messgo@latest
messgo ./... text go --ignore-tests
```

That scans the module with the recommended low-noise policy and prints findings
on stdout. Exit `0` is clean, `2` means findings, `1` means the tool failed,
`3` means a source file failed to parse.

Common next steps:

```console
messgo ./... text go,opinionated --ignore-tests
messgo ./... sarif go --ignore-tests --reportfile reports/messgo.sarif
messgo ./... github go --ignore-tests
```

Directory inputs analyze only the files that build for the host platform,
honouring `//go:build` lines and `_GOOS`/`_GOARCH` filename suffixes. A file
named explicitly is always analyzed.

Full command syntax, options, and discovery: [docs/usage.md](docs/usage.md).
What each ruleset and rule checks: [docs/rules.md](docs/rules.md).

## Install

```console
go install github.com/quality-gates/messgo/cmd/messgo@latest
messgo --version
```

From a local checkout:

```console
go build -o messgo ./cmd/messgo
```

Stable macOS archives: [GitHub Releases](https://github.com/quality-gates/messgo/releases).
Homebrew: `brew install quality-gates/tap/messgo`.

## Tune the gate

Start with `go`. Add `opinionated` when you want the stricter checks the
recommended set leaves out. Add `explicitness` (or `explicitness-strict`) to
find functions with implicit inputs and outputs. Point at a custom XML ruleset
when thresholds or membership need to live in the repo:

`explicitness` classifies Go code within a documented native subset. Reads
through arguments are explicit inputs; writes to caller-visible storage are
implicit outputs. The analysis follows aliases, copies, escaped allocations,
and resolved same-package calls. Field and element writes update shared
allocation state, so existing aliases see later inserts and replacements.
Pointers to fields and elements address the same slots, including fields of
value structs and elements of value arrays.
Helper calls preserve that state in their stores and return values; separate
factory calls keep separate private allocations. Function literals have their
own `<lambda>` owner; invoking one transfers its effects to the caller,
including `defer` and `go` calls. Defining an uncalled literal leaves its
caller's storage unchanged. Reads of mutable captured storage are implicit
inputs. Literal-false branches and loops are pruned. Package initialization
uses a `<module>` owner. Injected callbacks,
opaque external calls, interface dispatch, and receiver effects excluded by
the non-strict policy produce `UnknownEffect`. A clean report means
"calculation within this analyzed subset."

```xml
<ruleset name="team policy">
  <exclude-pattern>*/gen/*</exclude-pattern>
  <rule ref="go">
    <exclude name="DevelopmentCodeFragment" />
  </rule>
  <rule ref="LongVariable">
    <priority>2</priority>
    <properties>
      <property name="maximum" value="50" />
    </properties>
  </rule>
</ruleset>
```

```console
messgo ./... text path/to/team-policy.xml --ignore-tests
```

Existing phpmd rulesets carry over: `rulesets/codesize.xml` and
`rulesets/naming.xml/LongVariable` resolve to the built-in sets unless a real
file exists at that path.

## Suppress one intentional exception

messgo has no per-line disable comment yet. Drop a rule for the whole run with
`--disable`, skip paths with `--exclude`, or encode the exception in a team
ruleset:

```xml
<rule ref="go">
  <exclude name="RuleName" />
</rule>
```

`--strict` is reserved for when source suppressions land; today it does not
change the report.

## Drop it into CI

```yaml
# GitHub Actions
- uses: actions/setup-go@v6
  with:
    go-version-file: go.mod
- run: go install github.com/quality-gates/messgo/cmd/messgo@latest
- run: messgo ./... github go --ignore-tests
```

```yaml
# GitLab Code Quality
script: messgo ./... gitlab go --reportfile gl-code-quality-report.json
artifacts:
  reports:
    codequality: gl-code-quality-report.json
```

This repository also self-checks after building the binary. A finding fails the
job with exit code `2`.

## Maintainers

Command reference: [docs/usage.md](docs/usage.md). Rulesets: [docs/rules.md](docs/rules.md).
Homebrew release path: [docs/homebrew-release.md](docs/homebrew-release.md).
Exploratory testing: [2026-09-19 CLI journeys](docs/exploratory-testing/2026-09-19-cli-journeys.md),
[2026-09-23 explicitness rulesets](docs/exploratory-testing/2026-09-23-explicitness.md),
[2026-09-26 rules and metrics](docs/exploratory-testing/2026-09-26-rules-and-metrics.md),
[2026-10-03 type semantics and metrics](docs/exploratory-testing/2026-10-03-type-semantics-and-metrics.md),
[2026-10-10 naming and report context](docs/exploratory-testing/2026-10-10-naming-and-report-context.md),
[2026-10-17 gate, explicitness, and Docker](docs/exploratory-testing/2026-10-17-gate-explicitness-docker.md).

Development checks:

```console
go test ./...
```
