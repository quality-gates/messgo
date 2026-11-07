---
name: verify-explicitness-aliases
description: Check caller-visible slice writes through aliases when changing messgo's explicitness analysis, ownership tracking, or its reporting pipeline.
---

# Verify explicitness aliases

A function that changes caller-owned slice storage must receive an `ImplicitOutput` finding. A missed finding can make an effectful function appear to be a calculation.

## Run the check

From the repository root, run:

```sh
GOTOOLCHAIN=go1.26.6 go run ./.agents/skills/verify-explicitness-aliases/scripts/check.go
```

The checker builds `messgo` from the current checkout. It generates and executes Go functions, then scans the same functions with the `explicitness` ruleset and compares the runtime observations with the JSON report. It fails if a function changes its caller's slice without an `ImplicitOutput` finding. A timeout, build or parse error, incomplete output, unexpected finding, or `UnknownEffect` also fails the check.

The finite grammar contains 2,016 functions. Each starts with two aliases bound to the caller's one-element slice or private one-element slices. It has one `if` with optional assignment in either arm, an optional alias assignment after the branch, and one write through an alias. The checker executes each function with `flag=false` and `flag=true`, for 4,032 executions. The caller's element starts at zero and the sole write sets it to one.

A pass proves the stated implication only for this grammar and these inputs. It does not prove the absence of bugs in arbitrary Go programs. The check permits conservative extra `ImplicitOutput` findings. It does not cover loops, calls, maps, structs, pointers, multiple writes, receivers, or concurrency. The Go runtime supplies the expected mutation, so the checker does not duplicate the analyzer's ownership algorithm. Gobra verifies annotated Go, but this check exercises the unmodified analyzer through its CLI.

## Investigate a failure

For a missing finding, the checker prints the function, the Boolean input, and the observed change. Run that function with the printed input and analyze it with `messgo <file> json explicitness`. If the caller changes and the finding is absent, report the function and input as a product counterexample. If the function or input falls outside the grammar above, fix the generator or its stated scope before treating the result as a product defect.

For any other failure, inspect the error and reproduce the failed build, runtime execution, or CLI report. Do not treat an incomplete run as a pass. Keep a confirmed product finding visible; do not remove its case to make the check pass.

## Keep coverage in step with the code

Last confirmed against commit `ef7bfde28901f163d8ae6fb16ce7e15aafc3d114`.

The checked path includes these functions and files:

- `internal/rules/explicitness/ownership_flow.go` owns `trackOwnership`, `walkStatement`, `walkIf`, and `recordSnapshot`.
- `internal/rules/explicitness/ownership.go` owns `joinStates` and `storageOf`. `ownership_changes.go` owns `assignBindings` and `applyBindingChanges`.
- `internal/rules/explicitness/visitors.go` owns `writeVisitor.visit` and `writeVisitor.write`. `explicitness.go` owns `ImplicitOutput.ApplyFunc`, `reportFunctions`, and `scanScope`.
- `internal/rules/explicitness/summaries.go` owns `analyze`. The generated programs have no calls, so this check does not establish call-summary correctness.
- `internal/model/build.go`, `internal/runner/runner.go`, `internal/rule/engine.go`, and `internal/report/renderers.go` parse the input, dispatch the rule, and render its result. `internal/ruleset/builtin/explicitness.xml` states the rule's contract.

Inspect changes since the sync commit, including uncommitted changes:

```sh
git diff ef7bfde28901f163d8ae6fb16ce7e15aafc3d114 -- internal/rules/explicitness internal/model internal/runner internal/rule internal/report internal/ruleset internal/cli/cli.go cmd/messgo go.mod
```

When a covered path changes, trace the generated syntax through the changed code. Update the grammar or checker if the intended behavior changes, then rerun the check. Move the sync commit forward only after confirming that the coverage record and checker describe the code at the new commit. Use `/maintain-formal-verification-skill` when changes have accumulated without that review.

## Check that the checker can fail

When changing the checker, repeat this trial in a separate scratch worktree. Copy the skill into the scratch worktree if it is not committed there. In `internal/rules/explicitness/ownership_flow.go`, replace `then := w.current` in `walkIf` with `then := cloneState(pre)`. Run the command above. The checker must print a function where `flag=true` changes the caller's slice without an `ImplicitOutput` finding. Discard the scratch edit, rerun the command, and confirm the original result returns. If the code has changed since the sync commit, find an equivalent break in the branch join before repeating the trial.
