# Domain Docs

**Layout:** single-context

- `CONTEXT.md` at the repo root — high-level domain/architecture context for this codebase.
- `docs/adr/` at the repo root — Architecture Decision Records, one file per decision.

## Consumer rules

- Skills that need domain context (e.g. `to-spec`, `qa`) read root `CONTEXT.md`
  first, then relevant ADRs under `docs/adr/`.
- Neither exists yet in this repo. Create `CONTEXT.md` when a skill needs to
  record cross-cutting context, and add ADRs under `docs/adr/` as significant
  decisions are made — don't scaffold empty placeholders speculatively.
