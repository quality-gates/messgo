# messgo

A PHP Mess Detector (phpmd) port for Go: parses Go source with `go/ast` and applies rules faithful to Go semantics.

## Language

**Git hooks**:
The mechanism delivering local quality checks — committed shell scripts in `githooks/`, activated per-clone via `git config core.hooksPath githooks`.
_Avoid_: Pre-commit (as a mechanism name — see below), hooks framework.

**Pre-commit** / **pre-push**:
Git hook *stages* only — the point in the git workflow a script runs at (`git commit`, `git push`). Not the name of a tool or framework.
_Avoid_: Using "pre-commit" to mean the Python `pre-commit` framework; this repo does not use it.
