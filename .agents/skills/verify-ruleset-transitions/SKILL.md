---
name: verify-ruleset-transitions
description: Check ruleset reference cycles and explicit override precedence when changing messgo's ruleset loader or reference expansion.
---

# Verify ruleset transitions

An explicit override must win over an inherited rule, and two different explicit overrides for the same rule must fail regardless of order. Otherwise configuration order can silently change which rule runs. A reachable reference cycle must fail, while an acyclic reference graph must load. A false cycle error rejects a valid configuration.

## Run the check

From the repository root, run:

```sh
GOTOOLCHAIN=go1.26.6 python3 .agents/skills/verify-ruleset-transitions/scripts/run.py
```

The check creates XML files in a temporary directory and calls the real `ruleset.Loader.Load`. It tests all 363 sequences of one to five occurrences of an inherited `naming/LongVariable` rule and explicit priority 2 or 3 overrides. It requires one rule with the selected priority when overrides agree and an error when they conflict. Its expected result depends on the set of override values, not the loader's merge implementation.

The check also tests all 530 directed reference graphs with one to three XML files. Each node imports a distinct built-in rule. The first file is the root. An independent path-based graph search decides whether a cycle is reachable from the root. A reachable cycle must produce a cycle error. Otherwise the loader must return exactly the rules belonging to reachable nodes. Unreachable cycles do not make the root invalid.

These are exhaustive checks of the stated finite input spaces, not proofs for arbitrary rulesets. They do not cover property overrides, direct rule definitions, exclusions, multiple top-level ruleset arguments, malformed XML, larger graphs, or alternate path spellings. The wrapper terminates the check after 120 seconds, including its child processes, so a nonterminating expansion fails. The Go check exercises unannotated production code through the loader API. A separate model of the algorithm would not establish that the Go implementation follows the model.

## Investigate a failure

The check prints the rule sequence or graph size and edge bitmask. For an override sequence, create the printed XML and call `Loader.Load`, then compare the output or conflict with the two stated rules. For a graph, bit `from * nodes + to` in the edge mask denotes a reference from one file to another. Start at node zero and follow those references. A cycle is relevant only if the root can reach it. Confirm that the generated input is in scope before treating a mismatch as a product bug. Do not weaken either property to remove a real failure.

## Keep coverage in step with the code

Last confirmed against commit `618f7da`.

The checked path includes `Loader.Load`, `loadSession.parse`, `selectCandidates`, `selectCandidate`, `mergeCandidates`, and `sameRuleDefinition` in `internal/ruleset/ruleset.go`; `refExpander.enter`, `leave`, `expandRef`, and `importSourceRules` in `internal/ruleset/refs.go`; and the `LongVariable` definition in `internal/ruleset/builtin/naming.xml`. Go's `encoding/xml` decodes the input. Review changes to these paths and any moved code that affects reference identity, expansion, candidate provenance, or priority.

```sh
git diff 618f7da -- internal/ruleset internal/rule
```

When the covered code changes, trace both generated input families through it. Update the check if the intended contract changes, with evidence for that change. Run the check and the trials below. Move the sync commit forward only after confirming that the check describes the code at the new commit. Use `/maintain-formal-verification-skill` if covered changes accumulated without this review.

## Check that the check can fail

Use a separate scratch worktree at the same commit. Copy this skill to it if necessary. Run the full check once before either trial.

1. In `internal/ruleset/ruleset.go`, change `if incoming.kind == candidateOverride && existing.kind != candidateOverride` to `if false && incoming.kind == candidateOverride && existing.kind != candidateOverride`. Run the full command. It must report that inherited, priority 3, then priority 2 did not produce the expected conflict. Restore the file.
2. In `internal/ruleset/refs.go`, change `if pos, active := e.activePos[key]; active` to `if pos, active := e.activePos[key]; active || len(e.active) > 0`. Run the command with `graphs` appended. It must report that a graph with one node and no edges unexpectedly failed with a cycle error. Restore the file.
3. In `internal/ruleset/refs.go`, immediately before `e.enter(key)`, add `if _, active := e.activePos[key]; active { return false, nil }`. Run with `graphs` appended. It must reject a reachable cycle that the mutant silently accepts. Restore the file.
4. In the same file, make `expandRef` return `(false, nil)` when `xr.Ref == "node1.xml"`. Run with `graphs` appended. It must detect a missing reachable rule. Restore the file.
5. Run the full command again and confirm both original totals return. Remove the scratch worktree. If the code has changed, choose equivalent scratch breaks and explain why they still exercise these properties.
