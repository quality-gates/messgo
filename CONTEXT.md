# messgo

A Go-native mess detector with PHPMD as its lineage: parses Go source with
`go/ast`, applies rules faithful to Go semantics, and distributes stable
command-line releases through Go tooling and a project-owned Homebrew tap.

## Language

**Mess sign**:
A structural maintainability problem detectable from the AST alone — oversized
functions or types, tangled coupling, dead private code, muddy naming, deeply
nested control flow, unchecked type assertions, and the like. The unit messgo
reports. PHPMD is the rule taxonomy and inspiration, not the scope ceiling:
Go-specific mess signs are in scope on their own merits.
_Avoid_: bug, defect, correctness issue (the Go toolchain's job, not messgo's);
"port" used to mean "faithful reimplementation of PHPMD's ruleset only".

**Gap, filled**:
A mess sign a Go linter or PHPMD catches that messgo does not, and that messgo
subsequently gains a rule or metric for. "Filling" a gap is adding detection,
not changing the AST-only, no-build contract.

**Git hooks**:
The mechanism delivering local quality checks — committed shell scripts in `githooks/`, activated per-clone via `git config core.hooksPath githooks`.
_Avoid_: Pre-commit (as a mechanism name — see below), hooks framework.

**Pre-commit** / **pre-push**:
Git hook *stages* only — the point in the git workflow a script runs at (`git commit`, `git push`). Not the name of a tool or framework.
_Avoid_: Using "pre-commit" to mean the Python `pre-commit` framework; this repo does not use it.

**Stable release**:
A messgo release identified by an exact `vMAJOR.MINOR.PATCH` tag, with no
prerelease or build suffix.
_Avoid_: Production release, final build

**Release commit point**:
The moment a draft becomes an immutable GitHub release; failures after this
point cannot invalidate or replace that release.
_Avoid_: Homebrew publication, formula merge

**Tap publication**:
The process that makes an existing stable release available through
`quality-gates/tap`.
_Avoid_: Release, deployment

**Formula candidate**:
The complete generated `messgo` formula proposed from one immutable stable
release and awaiting the tap's merge policy.
_Avoid_: Formula draft, manual formula
