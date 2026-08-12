# Issue Tracker

**Tracker:** GitHub Issues (repo: `quality-gates/messgo`)
**CLI:** `gh` (GitHub CLI)

This file tells agent skills (`to-tickets`, `triage`, `to-spec`, `qa`, etc.) where
to read and write issues for this repo.

## Conventions

- Create issues with `gh issue create --repo quality-gates/messgo --title "..." --body "..."`
- List/search with `gh issue list` / `gh issue view <number>`
- Reference issues in commits/PRs as `#<number>`

## Flags

- **PRs as a request surface:** off. Skills should treat only GitHub Issues as
  the request queue, not open pull requests.
