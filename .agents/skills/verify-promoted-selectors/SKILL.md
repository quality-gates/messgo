---
name: verify-promoted-selectors
description: Check Go promoted-selector resolution when changing messgo's member-use tracking or concrete call resolution.
---

# Verify promoted selectors

For a valid selector, messgo must resolve the method Go selects and credit the selected member and embedded fields as used. If Go selects a function-valued field, concrete-method resolution must not invent a method target. Wrong choices can hide effects or produce incorrect unused-member findings.

## Run the check

From the repository root, run:

```sh
GOTOOLCHAIN=go1.26.6 go run ./.agents/skills/verify-promoted-selectors/scripts/check.go
```

The checker generates 9,126 single-file Go programs. Each has a `Root` struct with two disjoint embedded branches, each one to three named structs deep. Every struct can declare no `m`, a value-receiver method `m() int`, or a function-valued field `m func() int`. It covers both orders of the two root fields and calls `x.m()` on a `Root` parameter. Each declaration set is type-checked, and Go's `types.LookupFieldOrMethod` excludes 18 missing and 1,488 ambiguous selectors. Unexpected parse or type-check errors fail the run.

For the remaining 7,620 programs, the checker parses the source with messgo and type-checks that same AST with Go's `go/types`. It compares `model.ResolveCall` with the selected method, or requires no concrete method when Go selects a field. It also compares `model.SelectedMemberUses` with Go's selected declaration and embedding path. When `Root` has no direct `m`, the check ignores the model's synthetic `(Root, m)` lookup key, which does not name a declared member. It prints two examples per property and a fingerprint of all case IDs and properties that failed. To inspect one printed case, append `-case <case-id>` to the command; an unknown or excluded ID fails.

At the sync commit below, the check passes. Before the fixes for [#308](https://github.com/quality-gates/messgo/issues/308) and [#310](https://github.com/quality-gates/messgo/issues/310), it exposed competing promotion depths and function fields shadowing promoted methods.

A pass covers only this finite grammar, not all Go programs. It checks the real model directly, not the CLI or effect propagation. It does not cover pointer receivers or embeddings, interfaces, aliases, generics, diamonds, cross-file packages, method expressions, other selectors, or callback execution. The Go type checker supplies the selection and path rather than a copy of messgo's traversal. Gobra verifies annotated Go, but this check exercises the existing unannotated analyzer.

## Investigate a failure

Run the printed case alone and compare its complete source with Go's selection and messgo's output. A different method, invented method for a field, or incorrect selected-member path is a product counterexample if the source remains inside the grammar. For a parse, type-check, missing-selection, or coverage-count error, inspect the generator and oracle before drawing a conclusion. Do not discard a valid case or weaken the comparison to make the check pass.

## Keep coverage in step with the code

Last confirmed against commit `320beba74f06c7995b9042f67e751a17a757ec89`.

The checked path includes:

- `internal/model/build.go`: `ParseSource` and the file builder construct the real model.
- `internal/model/member_selection.go`: `recordSelector`, `promotedMemberPath`, `lookupMember`, and `lookupMethod` record uses and resolve members. `selectMember`, `selectMemberLevel`, `directSelection`, `memberPath`, and `appendEmbeddedSearches` choose a member and its path.
- `internal/model/call_resolution.go`: `ResolveCall` resolves the concrete call target.
- `internal/model/model.go`: `SelectedMemberUses` returns the recorded selection index.

Inspect changes since the sync commit, including uncommitted changes:

```sh
git diff 320beba74f06c7995b9042f67e751a17a757ec89 -- internal/model go.mod
```

When a covered path changes, trace the generated syntax through the changed code. Update the grammar or checker if the intended behavior changes, then rerun the check. Move the sync commit forward only after confirming that the coverage record and checker describe the code at the new commit. Use `/maintain-formal-verification-skill` when changes have accumulated without that review.

## Check that the checker can fail

Use a separate scratch worktree at the same commit, copying this skill there if necessary. Record the full check's counts and mismatch fingerprint. These two cases must pass before either mutation:

```sh
GOTOOLCHAIN=go1.26.6 go run ./.agents/skills/verify-promoted-selectors/scripts/check.go -case rootM-L0-R0-LR
GOTOOLCHAIN=go1.26.6 go run ./.agents/skills/verify-promoted-selectors/scripts/check.go -case root0-LM-R0-LR
```

1. In `internal/model/member_selection.go`, remove `method: method,` from the return in the `class.Methods` loop of `directSelection`. Rerun the first case. It must fail the call-target property. Restore the file and confirm that case passes again.
2. In the same file, replace `uses[key] = true` inside `recordSelector`'s `promotedMemberPath` loop with `_ = key`. Rerun the second case. It must fail the member-use property. Restore the file and confirm that case passes again.
3. Rerun the full check and confirm that its counts and mismatch fingerprint match the original run, whether that run passed or reported known product findings. If the code has changed since the sync commit, choose equivalent scratch breaks and passing witness cases.
