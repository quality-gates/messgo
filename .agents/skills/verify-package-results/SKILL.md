---
name: verify-package-results
description: Check exact findings, clean results, and parse errors when changing messgo package discovery, rules, analysis, or JSON reporting.
---

# Verify package results

A scan must report each expected violation at the right file and line, leave clean examples unreported, and return the right exit status. Missing findings let a problem pass a quality gate; extra findings reject clean code.

## Run

From the repository root:

```sh
GOTOOLCHAIN=go1.26.6 python3 .agents/skills/verify-package-results/scripts/check.py
```

The checker builds the actual CLI and writes a temporary Go module. It exhausts 216 combinations of three source files: two files in the root package, including a `_test.go` file, and one in a child package. Each file has a local variable of length 19, 20, or 21 and either writes to its slice parameter or leaves it untouched. The independently specified expected result is `LongVariable` only above 20 and `ImplicitOutput` only for the write. It checks exact file, rule, line, priority, multiplicity, exit code, and `ImplicitOutput` function. It ignores the function field for `LongVariable`, including runs with no findings.

It also compiles the all-violating configuration with `go test ./...`. For each combination, it scans the module directory, `./...`, the module with `--ignore-tests`, and the child directory: 864 scans. It selects the two rules explicitly from the `naming` and `explicitness` rulesets. It also scans three configurations with the normal `go` ruleset, checking that a 34- or 35-character name passes and a 36-character name produces only the expected `LongVariable` finding. It then adds an unparseable file to both a clean configuration and one with findings. Both scans must report exactly that error and exit 3; the latter must retain its findings. A pass prints all totals. Build failure, timeout, invalid JSON, unexpected parse errors, or any mismatch fails the check.

Messgo accepts filesystem paths, not Go import patterns. Both directory and `./...` scans recursively walk directories, including child packages; a `go.mod` file does not define the discovery boundary. JSON has no list of passed rules or clean files. A clean example here means the expected report has no finding for that example; toggling each file to a violating example also checks that discovery can report it.

This is exhaustive only over the stated generator and four scan modes, not over Go packages or all messgo rules. The three default `go` scans cover just the stated naming boundary and clean examples, not every rule in that ruleset. It does not cover build tags, cross-file call resolution, imports, custom rulesets, module boundaries, other output formats, ignored paths other than test files, diagnostic text, path spelling, or arbitrary source programs. The rule contracts and the generated source shapes supply the expected findings; the generator compiles one configuration but does not execute the generated functions or reimplement messgo's analyzer. No verifier for unannotated Go is used, so this is bounded executable checking rather than a proof of the implementation for all inputs.

## Investigate a failure

The checker prints the per-file `(name length, writes)` states, path, options, and expected and actual findings. Recreate those three files and run the printed scan. Check the rule contract and the generated program before calling a mismatch a product bug. A zero exit with an expected violation is a missed finding; a finding for a 20-character name or a non-writing function is an unexpected violation. Do not weaken an expected result to hide a confirmed bug. For a malformed or missing report, inspect stderr, JSON, and exit status first.

## Keep coverage in step

Last confirmed against commit `afeee5875e4e3a34578a1cb4faf43c20c11e074e`.

The checked path includes `internal/cli/cli.go`, `internal/runner/discovery.go` and `runner.go`, `internal/model`, `internal/rule`, `internal/rules/naming/naming.go`, `internal/rules/explicitness`, `internal/ruleset`, and `internal/report`. Changes to a dependency, CLI invocation, rule default, or JSON fields can affect this check. Inspect changes since the sync commit, including uncommitted changes:

```sh
git diff afeee5875e4e3a34578a1cb4faf43c20c11e074e -- cmd/messgo internal/cli internal/runner internal/model internal/rule internal/rules/naming internal/rules/explicitness internal/ruleset internal/report go.mod
```

Trace the generated inputs through changed code. If intended behavior changes, update the oracle with evidence for the new contract. Run the check and mutation trials, then move the sync commit forward only after confirming the check still describes the code. Use `/maintain-formal-verification-skill` when covered changes accumulate.

## Check that the check can fail

Use a separate scratch worktree at the code under test and copy this skill into it if needed. Run the command above before the trials. Restore each file after its trial.

1. In `internal/rules/naming/naming.go`, change the `LongVariable.checkName` comparison `<= r.maximum` to `< r.maximum`. The check must reject a finding for a 20-character name.
2. In the same comparison, add `true ||` before `util.LengthWithoutPrefixesAndSuffixes(...) <= r.maximum`. The check must reject the missing `LongVariable` finding.
3. In `internal/rules/explicitness/explicitness.go`, replace the body of `ImplicitOutput.ApplyFunc` with `return`. The check must reject a missing write finding.
4. In `internal/runner/discovery.go`, change `f.options.IgnoreTests && strings.HasSuffix(path, "_test.go")` to `false && strings.HasSuffix(path, "_test.go")`. The `--ignore-tests` scan must reject an unexpected test-file finding.
5. In `internal/cli/cli.go`, change `return ExitProcessingError` inside `exitCodeFor` to `return ExitSuccess`. The parse-error scan must reject exit 0.
6. In `internal/report/renderers.go`, insert `if len(r.Errors) > 0 { break }` at the start of the `view.indexedViolations()` loop in `JSONRenderer.Render`. The parse-error scan must reject the missing findings.
7. In `internal/cli/cli.go`, add `len(rep.Violations) > 0 &&` to the processing-error guard in `exitCodeFor`. The clean parse-error scan must reject the wrong exit status.
8. In `internal/ruleset/builtin/go.xml`, change the `LongVariable` maximum from 35 to 36. The default-rule scan must reject the missed finding at 36 characters.
9. Restore all changes and run the full check again. Remove the scratch worktree.

If code has moved, choose equivalent real-code breaks and record why each still exercises the property. A mutation that fails to build or times out is not evidence that the checker detected a wrong report.
