# State in messgo

Messgo's stateful algorithms use the structures below. The table maps the earlier inventory on commit `139ace801e0368b63146d709dfc51cc91e7566f6` to the current implementation. It describes state that affects a later decision, including traversals and caches. Ordinary loop counters are outside this inventory.

| Inventory entry | State and transitions | Implementation and checks |
| --- | --- | --- |
| Ownership interpretation | `state` holds bindings, heap, calls, and addresses. Statements update it, branches join it, and the flow records snapshots. | `internal/rules/explicitness/ownership.go`, `ownership_flow.go`; `/verify-explicitness-aliases` |
| Loop convergence | `walkLoop` joins the entry, body, and jumps until consecutive states match. | `internal/rules/explicitness/ownership_flow.go`; `/verify-explicitness-calls` covers a bounded loop grammar |
| Label and goto convergence | `ownershipFlow.labels` accumulates incoming states. `trackOwnership` repeats the function until the label map stops changing. | `internal/rules/explicitness/ownership_flow.go`; CLI explicitness tests |
| Known-call ownership | A summary is new, evaluating, ready, or re-evaluating. Active summaries return provisional results; dependencies and a deduplicated queue propagate changed results to callers. The cache resets when its file changes. | `internal/rules/explicitness/storage_summaries.go`, `storage_summary_solver.go`; solver tests and `/verify-explicitness-calls` |
| Effect summary propagation | `effectGraph` discovers calls and repeatedly propagates effects until none change. | `internal/rules/explicitness/summaries.go`; `/verify-explicitness-calls` |
| Member type environment | A collector passes a type environment through declarations, assignments, and scoped bodies. Later selections use the resulting bindings. | `internal/model/member_selection.go`; `/verify-promoted-selectors` covers a bounded selector grammar |
| Ruleset reference expansion | `refExpander` owns the active reference stack and expansion cache. Entry rejects cycles; return removes the active reference. | `internal/ruleset/refs.go`; `/verify-ruleset-transitions` |
| Rule candidate selection | Candidate provenance and existing definitions determine whether to retain, replace, upgrade, or reject a candidate. | `internal/ruleset/ruleset.go`; `/verify-ruleset-transitions` |
| Promoted member traversal | `promotedMemberCollector` owns the visited set and queues. Direct members shadow promoted names. | `internal/model/promoted.go`; model tests and `/verify-promoted-selectors` for selector resolution |
| Type and global resolution | Per-path visited sets stop recursive type and global alias resolution. | `internal/util/sharing.go`, `internal/model/member_selection.go`; util and model tests |
| Cohesion graph | `cohesionGraph` tracks active methods and union-find components. Field uses and calls activate and join methods. | `internal/rules/design/design.go`; design tests |
| Duplicate literal keys | A map retains the first line for each canonical key; later occurrences emit duplicates. | `internal/model/query.go`; model tests |
| Lazy analysis caches | `sync.Once` moves per-file caches and shared package indexes from uncomputed to computed. Go owns synchronization. | `internal/model/model.go`, `internal/model/promoted.go`; model tests |
| CLI argument consumption | `argumentCursor` owns the next token. Flags consume values or return an error; informational flags short-circuit before parsing. | `internal/cli/cli.go`; CLI tests |
| Command and result lifecycle | `Main` parses and validates. `run` selects a renderer, loads rules, runs analysis, and writes output. `exitCodeFor` selects the final status. | `internal/cli/cli.go`; CLI tests |
| File discovery | `discoveredSources` owns seen identities and selected paths. A new identity adds one path; results are sorted after discovery. | `internal/runner/discovery.go`; runner tests |
| Grouped report rendering | `fileRunCursor` owns the position and pending violation between adjacent file runs. Renderers consume each run in report order. | `internal/report/view.go`, `renderers.go`; report tests |
| Writer error latch | `checkedWriter` retains the first error and returns it on subsequent writes. | `internal/report/renderers.go`; report tests |
| Report identity registries | The report view indexes files on first encounter; SARIF records rule identities for later results. | `internal/report/view.go`, `renderers.go`; report tests |
| Release orchestration | Workflow `needs`, `if`, and concurrency express validation, build, smoke, immutable-release retry, and publish transitions. Reusable actions own their own behavior. | `.github/workflows/release.yml`; workflow checks |
| Git hook resource and gate lifecycle | Pre-commit owns temporary directories and an exit cleanup trap. Pre-push gates on its prerequisites and mutation check. | `githooks/pre-commit`, `githooks/pre-push`; `githooks` tests |

The inventory is a source review, not a proof of completeness. Bounded verification skills state their input spaces and exclusions. The two existing alias and selector skills remain the checks for their covered paths. The ruleset and call checks cover additional bounded input spaces and run the real implementation.
