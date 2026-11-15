---
name: verify-explicitness-calls
description: Check caller-visible slice writes through call chains, recursion, and loops when changing explicitness effect propagation or ownership flow.
---

# Verify explicitness calls

If a function changes a caller-owned slice through a call, messgo must report `ImplicitOutput` for that function. A missed finding can classify an effectful function as a calculation.

## Run the check

From the repository root, run:

```sh
GOTOOLCHAIN=go1.26.6 python3 .agents/skills/verify-explicitness-calls/scripts/check.py
```

The checker generates 48 call graphs. Each graph has one to three forwarding functions, a direct or recursive writer, and a caller that chooses caller-owned or private storage. The caller invokes the chain unconditionally or under a Boolean condition, with or without a two-iteration loop. Go executes each caller for both Boolean inputs, for 96 observations. The checker builds messgo and compares the runtime mutations with its JSON findings. A caller mutation without `ImplicitOutput`, a build failure, timeout, or incomplete output fails the check. Only observations where the caller slice changes assert a finding: 36 executions across 24 caller-owned-storage graphs. The 24 private-storage graphs do not assert precision; extra conservative findings are allowed.

The check covers only this finite grammar. It does not cover arbitrary Go, other storage shapes, goroutines, callbacks, goto, or all summary-worklist states. In particular, it does not establish summary-worklist propagation, recursive re-evaluation, or loop fixed-point convergence. Direct effect propagation can mask a broken storage summary solver. The runtime supplies the expected mutations, so the check exercises the unmodified analyzer rather than a hand model of it. No verifier for the unannotated Go analyzer is used.

## Investigate a failure

Reproduce the named generated case and Boolean input. If Go changes the caller slice and messgo reports no `ImplicitOutput` for the caller, it is a product counterexample. For malformed reports, a failed command, or an incomplete observation set, inspect the checker before drawing that conclusion. Do not weaken the property to hide a product failure.

## Keep coverage in step

Last confirmed against commit `618f7da`, including the explicit lifecycle refactor.

The checked path includes `internal/rules/explicitness/summaries.go` (`analyze`, `propagate`), `ownership_flow.go` (the generated branch and loop syntax), the CLI and its rule loading, model, runner, and JSON reporting path. Trace changes in these files through the generated grammar. Run the command again after a relevant change and update this record only after confirming that the check describes the new code. Use `/maintain-formal-verification-skill` for accumulated changes.

## Check that the checker can fail

In a separate scratch worktree with this skill present, replace `if graph.propagate(fn, edge) {` with `if false && graph.propagate(fn, edge) {` in `internal/rules/explicitness/summaries.go`. Run the command above from that scratch worktree. It must fail with a case that changes its caller's slice without an `ImplicitOutput` finding. Restore the production file and rerun the check. It must pass again. If the source shape changed, choose an equivalent break in effect propagation and record it.
