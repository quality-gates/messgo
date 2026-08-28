# Performance audit

Audit date: 2026-08-27
Audited revision: `aa0f132` (`Fix silent gates, false findings, and broken report contracts (#37)`)

## Scope and seams

The audit follows the caller-visible `messgo <paths> <format> <ruleset>` path. It
uses these seams because each has a distinct scaling input and can be measured
without conflating setup, analysis, and output costs:

1. CLI parsing and ruleset loading.
2. File discovery, parsing, model construction, and package annotation.
3. Per-file rule dispatch, AST queries, and metrics.
4. Violation sorting and report rendering.

The ranked findings compare inputs only where their growth is defined. A custom
ruleset's reference depth does not scale with source AST size, so finding 1 is
ranked by its faster asymptotic class, not by its impact on the current
repository. Findings 2–4 are all quadratic when their two variables grow
together; their order is decided by default reachability and measured impact.

## Findings

### 1. Critical — exponential ruleset reference expansion, `O(b^d)`

**Location and path:** CLI `run` → `loadRuleSets` → `ruleset.Loader.Load` →
`parse` → `addRef` → `expandRef` → `importSourceRules` → `expandSourceRule` →
`expandRef` (`internal/cli/cli.go:64-94`, `internal/ruleset/ruleset.go:96-116`,
`internal/ruleset/refs.go:17-95`).

**Inputs:** `b` is the number of references from one ruleset to the next shared
ruleset, `d` is reference depth, `x` is the bytes parsed per referenced XML
file, and `r` is the number of rules at the leaf. In the repeated-reference
case, current time is `O(b^d * (x + r))` and retained construction space before
deduplication is `O(b^d * r + d)`. With fixed `b = 2`, this is `O(2^d)`.

**Cause and evidence:** `expandRef` reads and unmarshals the referenced file on
every edge. It has no cache for already parsed locations and no active-reference
set. `dedupeRules` runs only after the complete expansion. A compact input can
therefore make the same subtree expand repeatedly. A reference cycle has no
termination check and can recurse until stack or process exhaustion.

A benchmark used one leaf containing one implemented rule. Every higher level
contained two references to the preceding file. The loader returned only the
same logical rule after final deduplication, while cost quadrupled for every two
levels:

| Depth | Time/op | Bytes/op | Allocations/op |
| ---: | ---: | ---: | ---: |
| 4 | 2.943 ms | 127,328 | 1,814 |
| 6 | 10.763 ms | 521,056 | 7,384 |
| 8 | 45.547 ms | 2,096,480 | 29,658 |
| 10 | 180.448 ms | 8,419,940 | 118,749 |

Built-in rulesets have shallow, acyclic references, so normal `go` ruleset
loading does not trigger the growth. Nested custom rulesets are a supported and
tested input, making repeated shared references and accidental cycles reachable
configuration workloads.

**Remediation direction:** canonicalize reference locations, cache each parsed
XML ruleset, track the active expansion stack to reject cycles, and suppress a
duplicate logical rule before constructing it. Expansion should become
`O(X + E + R)` time and `O(X + E + R + d)` space, where `X` is total bytes in
unique ruleset files, `E` unique reference edges, and `R` unique imported rules.

**Confidence:** high for the repeated-reference complexity and measurements.
High for non-termination on a cycle from source inspection; the audit did not
crash a process merely to measure stack exhaustion.

### 2. High — selected-member AST scan repeats for every class, `O(cn)` / `O(n^2)`

**Location and path:** `runner.Run` → `rule.Analyze` → `applyRule` →
`UnusedPrivateField.ApplyClass` or `UnusedPrivateMethod.ApplyClass` →
`selectedNames` → `File.SelectedMemberNames` (`internal/runner/runner.go:27-44`,
`internal/rule/engine.go:15-49`, `internal/rules/unusedcode/unusedcode.go:19-55`,
`internal/model/query.go:260-274`).

**Inputs:** `n` is AST nodes in a file, `c` is classes in that file, `u` is
unique selected member names, and `v` is emitted violations. Current time is
`O(cn + fields + methods)` for each of the two rules. If classes and file size
grow together, this is `O(n^2)`. Peak auxiliary space is `O(u + v)`; total
temporary-map creation is `O(c)` per rule.

**Cause and evidence:** `applyRule` invokes each class-aware rule once per class.
Both unused-member rules call `SelectedMemberNames`, which allocates a new map
and walks the complete file AST on every invocation even though the answer is
file-invariant.

A benchmark analyzed files containing one unexported field in each of `c`
classes with only these two default rules enabled:

| Classes | Time/op | Bytes/op | Allocations/op |
| ---: | ---: | ---: | ---: |
| 100 | 21.017 ms | 47,091 | 1,011 |
| 200 | 70.749 ms | 94,201 | 2,012 |
| 400 | 206.612 ms | 188,671 | 4,013 |
| 800 | 1.252 s | 376,116 | 8,015 |

The default `go` ruleset includes both rules. In a profile of messgo analyzing
its own `internal` tree, `SelectedMemberNames` accounted for 12.55% cumulative
sampled CPU and 71.12 MB (15.89%) cumulative allocated space across 100 runs.
This was the largest active quadratic path in that workload.

**Remediation direction:** compute selected member names once per file and make
the immutable result available to both rules, either as a lazy `model.File`
fact or as a run-scoped analysis fact. This changes the pair of rules to
`O(n + c + fields + methods)` time and `O(u + v)` space.

**Confidence:** high. The caller multiplication, default reachability,
benchmark scaling, and production profile all agree. Real Go files usually
contain far fewer than hundreds of classes, so the current repository impact is
material but not catastrophic.

### 3. High — binding-aware read detection rescans a function per local, `O(qn)` / `O(n^2)`

**Location and path:** `runner.Run` → `rule.Analyze` →
`UnusedLocalVariable.check` → `Function.LocalVariables` →, for every local,
`Function.IdentifierRead` → `collectWriteIdents` plus
`containsIdentifierRead` (`internal/rules/unusedcode/unusedcode.go:77-104`,
`internal/model/query.go:293-358`, `internal/model/query.go:495-531`,
`internal/util/astutil.go:16-98`). `UnusedFormalParameter` has the same path per
parameter when that opt-in rule is enabled (`internal/rules/unusedcode/unusedcode.go:58-75`).

**Inputs:** `n` is AST nodes in a function body and `q = l + p` is candidate
locals plus parameters. Current worst-case time is `O(qn)` because each query
first walks the body to rebuild all write identifiers and then may walk it again
to search for a read. When declarations scale with body size, this is
`O(n^2)`. Peak auxiliary space is `O(n + v)` for the write map and violations;
total allocation over the analysis is `O(qn)`.

**Cause and evidence:** object identity correctly handles shadowing, but the
binding-aware result is asked as a one-target-at-a-time query. Unread or
late-read identifiers force the full second walk. The local collector itself
also performs two linear AST walks, but that is not the asymptotic problem.

A benchmark analyzed one function containing `q` unread local declarations
with the default `UnusedLocalVariable` rule:

| Locals | Time/op | Bytes/op | Allocations/op |
| ---: | ---: | ---: | ---: |
| 100 | 13.474 ms | 532,272 | 2,143 |
| 200 | 50.662 ms | 2,044,201 | 4,650 |
| 400 | 225.170 ms | 7,891,053 | 10,061 |
| 800 | 1.151 s | 30,544,984 | 21,687 |

The default `go` ruleset includes `UnusedLocalVariable` and excludes
`UnusedFormalParameter`. In the self-analysis profile, the unused-local rule
accounted for 10.33% cumulative sampled CPU and `IdentifierRead` for 7.75%.

**Remediation direction:** make one binding-aware function pass produce a set
of read `*ast.Object` identities (with an explicit fallback for unresolved
identifiers), alongside the write-target set. Test all locals and parameters by
map lookup. Shared or lazy per-function facts would reduce the path to
`O(n + q)` time and `O(n + q + v)` space while preserving shadowing semantics.

**Confidence:** high. The worst case is reachable in generated code and large
handwritten setup functions, and the active rule appears in the production
profile. Functions with hundreds of locals are uncommon, which limits typical
severity relative to finding 2.

### 4. Medium — whitespace-aware LOC scans the whole file per declaration, `O(en)` / `O(n^2)`

**Location and path:** `LongMethod.measure` or `LongClass.measure` → `funcLOC` /
`classLOC` → `metrics.EffectiveLinesOfCode` (`internal/rules/codesize/codesize.go:27-47`,
`internal/rules/codesize/codesize.go:108-170`, `internal/metrics/metrics.go:232-269`).

**Inputs:** `n` is source bytes/lines in a file and `e` is the number of
declaration ranges measured. Current time is `O(en)` and total temporary
allocation is `O(eL)`, where `L` is file lines, because every call splits the
entire source. Peak auxiliary space is `O(L)`. When declarations and file size
grow together, time and total allocation are `O(n^2)`.

**Cause and evidence:** every `EffectiveLinesOfCode` call runs `splitLines` over
the complete source and then scans from line 1 through the declaration end to
reconstruct block-comment state. `LongClass` repeats `funcLOC` for every method,
so enabling both long-method and long-class whitespace handling can measure a
method twice.

A benchmark parsed one file containing `e` one-line functions and invoked
`EffectiveLinesOfCode` once per function:

| Functions | Time/op | Bytes/op | Allocations/op |
| ---: | ---: | ---: | ---: |
| 100 | 6.748 ms | 750,400 | 700 |
| 200 | 17.592 ms | 3,139,200 | 1,600 |
| 400 | 63.816 ms | 12,832,000 | 3,600 |
| 800 | 342.591 ms | 47,475,200 | 8,000 |

The built-in codesize XML does not give `ignore-whitespace` a true value, so
this path is disabled by default. A custom ruleset property override makes it
reachable; that lower call frequency breaks the quadratic tie with findings 2
and 3.

**Remediation direction:** scan source once per file to build a prefix count of
effective code lines (and, if needed, line-start comment state). Each declaration
then becomes an `O(1)` range query. Total cost becomes `O(n + e)` time and
`O(L)` cached space.

**Confidence:** high for source complexity and the direct metric benchmark.
Medium for user impact because no workload data shows how often custom
rulesets enable the property.

## Runtime evidence

Benchmarks ran with:

- Command: `go test -run '^$' -bench 'Benchmark(UnusedLocalVariableScaling|UnusedMemberScaling|EffectiveLOCScaling|RulesetReferenceFanout)$' -benchmem -benchtime=1s -count=1 -cpu=1 .`
- Profile command: `go test -run '^$' -bench '^BenchmarkSelfAnalysis$' -benchmem -benchtime=5s -count=1 -cpuprofile=/tmp/messgo-performance-cpu.out -memprofile=/tmp/messgo-performance-mem.out .`
- Self-analysis workload: `runner.Run` on `internal`, built-in `go` ruleset,
  tests ignored. Result: 89.911 ms/op, 4,653,142 bytes/op, 97,406 allocations/op.
- Go: `go1.26.6 darwin/arm64`.
- OS: macOS Darwin 25.5.0, arm64.
- Machine: Apple M3, 16 GiB RAM.

The benchmark harness generated source in memory, parsed it once for analysis
microbenchmarks, and kept parsing outside the timed region. Ruleset expansion
used temporary XML files. The self-analysis benchmark included discovery,
reads, parsing, package annotation, rules, sorting, and excluded rendering. The
temporary harness was removed after measurement so the audit changes no Go
code.

## Coverage summary

| Subsystem | Inspected paths and derived bound | Result |
| --- | --- | --- |
| `cmd/messgo`, `internal/cli`, `internal/version` | Argument parsing `O(a)`, renderer lookup `O(1)`, orchestration | No meaningful bottleneck; ruleset setup leads to finding 1. |
| `internal/ruleset` | XML reads/unmarshal, reference recursion, filters, registry lookup, final deduplication | Finding 1. Ordinary flat loading is `O(x + r)`. |
| `internal/runner` | Walk `O(d(s + e))`, deduplication `O(f)`, file sort `O(f log f)`, sequential parse/analyze, package grouping | Expected input/output bounds; no additional bottleneck. Here `d` is directory entries, `s` suffix count, `e` excludes, and `f` files. |
| `internal/model` | Parse/build `O(n)`; function, literal, identifier, receiver, and file queries traced through callers | Findings 2 and 3. Other individual AST queries are linear in their root. |
| `internal/metrics` | Cyclomatic and NPath walks `O(n)`; LOC `O(1)`; effective LOC and callers | Finding 4. NPath's numeric value can grow exponentially, but the saturating computation itself is linear in AST size. |
| `internal/rule` | Dispatch `O(r(a + w))`, violation creation `O(v)`, stable sort `O(v log v)` | Sort is appropriate for ordered output; repeated rule walks lead to findings 2–4, not a separate engine defect. |
| `internal/rules/codesize` | All ten rule implementations, including WMC and LOC callers | Finding 4; remaining counts are linear in their artifacts or method bodies. |
| `internal/rules/unusedcode` | All four rules and model/util query paths | Findings 2 and 3. |
| `internal/rules/design` | Call/goto/loop/nil scans, globals, coupling, LCOM4 union-find | Linear AST work per enabled rule; LCOM4 is `O(n + m alpha(m))` for method-body nodes `n` and methods `m`. |
| `internal/rules/cleancode` | Per-file and per-function duplicate-key and statement queries | Linear in inspected AST and literal elements. Nested literals do not multiply ancestor work. |
| `internal/rules/naming`, `controversial` | Artifact checks, constant scan, local collectors, configured prefix/suffix lists | Linear for fixed rules/properties; repeated local walks are constant-factor work, not an additional asymptotic finding. |
| `internal/util` | Local collection, package mutation analysis, call/callee helpers, strings | Linear per call; caller multiplication is captured in finding 3. |
| `internal/report` | Text, XML, JSON, GitHub, GitLab, Checkstyle, SARIF, and HTML renderers | `O(v + z)` time where `z` is emitted bytes; structured renderers use `O(v)` materialization. No meaningful bottleneck beyond required output work. |

No factorial or higher-degree polynomial analysis path was found. The four
findings above account for every repeated traversal whose caller can make its
input grow independently; other repeated rule walks are bounded by the fixed
enabled-rule count and remain linear in source size.
