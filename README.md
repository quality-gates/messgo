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
on stdout. Exit `0` is clean, `2` means findings, `1` means the tool or a
source file failed.

Common next steps:

```console
messgo ./... text go,opinionated --ignore-tests
messgo ./... sarif go --ignore-tests --reportfile reports/messgo.sarif
messgo ./... github go --ignore-tests
```

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
recommended set leaves out. Point at a custom XML ruleset when thresholds or
membership need to live in the repo:

```xml
<ruleset name="team policy">
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

Development checks:

```console
go test ./...
```
