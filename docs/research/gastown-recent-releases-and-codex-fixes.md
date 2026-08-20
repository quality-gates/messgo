# Recent Gas Town releases and Codex-specific fixes

## Question

As of 2026-07-16, does the official Gas Town GitHub repository have recent
releases, particularly releases containing Codex-specific fixes?

This note uses only first-party GitHub repository, release, commit, and pull
request records. Dates and times are UTC.

## Short answer

**Yes, with a timing caveat.** The official repository is
[`gastownhall/gastown`](https://github.com/gastownhall/gastown), described by
its GitHub repository record as “Gas Town - multi-agent workspace manager.” It
published seven releases between 2026-03-16 and 2026-06-06. Releases
[`v0.12.1`](https://github.com/gastownhall/gastown/releases/tag/v0.12.1) and
[`v0.13.0`](https://github.com/gastownhall/gastown/releases/tag/v0.13.0)
explicitly name Codex fixes in their release notes.

However, the newest clearly Codex-specific change is newer than the newest
release: [PR #4221, “fix: harden codex agent
startup”](https://github.com/gastownhall/gastown/pull/4221), merged on
2026-06-12, while the latest release, `v1.2.1`, was published on 2026-06-06.
Therefore that June startup hardening was still **unreleased** as of
2026-07-16.

## Recent release chronology

GitHub's published release records show:

| Tag | Published at (UTC) | Codex in the release-note text? |
| --- | --- | --- |
| [`v1.2.1`](https://github.com/gastownhall/gastown/releases/tag/v1.2.1) | 2026-06-06 17:17:53 | No |
| [`v1.2.0`](https://github.com/gastownhall/gastown/releases/tag/v1.2.0) | 2026-05-30 23:17:56 | No |
| [`v1.1.0`](https://github.com/gastownhall/gastown/releases/tag/v1.1.0) | 2026-05-07 00:10:12 | No |
| [`v1.0.1`](https://github.com/gastownhall/gastown/releases/tag/v1.0.1) | 2026-04-25 20:49:31 | No (but see the Codex fallback note below) |
| [`v1.0.0`](https://github.com/gastownhall/gastown/releases/tag/v1.0.0) | 2026-04-03 05:46:01 | No |
| [`v0.13.0`](https://github.com/gastownhall/gastown/releases/tag/v0.13.0) | 2026-03-29 22:15:00 | **Yes** |
| [`v0.12.1`](https://github.com/gastownhall/gastown/releases/tag/v0.12.1) | 2026-03-16 02:10:42 | **Yes** |

This means Gas Town has had recent releases, but the most recent release whose
notes literally name Codex is `v0.13.0`, not the current latest release.

## Explicit Codex release-note entries

### `v0.13.0` — published 2026-03-29

The release notes contain these exact Codex bug-fix lines:

- “`Merge PR #2923: fix: start Codex nudge poller without cancel hook`”
  ([merge commit `765a474`](https://github.com/gastownhall/gastown/commit/765a4743090deade4d72f2a52e5fcba20314d43e),
  committed 2026-03-16 18:01:30 UTC).
- “`fix(codex): wait for idle before draining queued nudges`”
  ([commit `9010b0a`](https://github.com/gastownhall/gastown/commit/9010b0a5bc8b7e70ab3f85164930f38f656321a5),
  committed 2026-03-20 16:50:47 UTC). Its commit message says it gives the
  Codex preset a prompt prefix for idle detection and prevents queued nudges
  from draining while prompt-aware agents are busy.
- “`fix: start Codex nudge poller without cancel hook`”
  ([underlying commit `e6e0e87`](https://github.com/gastownhall/gastown/commit/e6e0e87baae89f1bfb52a83e572bf55c58892c8c),
  committed 2026-03-16 13:21:13 UTC).

The first and third lines are the merge and underlying commit for the same
poller change, so the three release-note lines represent two substantive
Codex fixes.

### `v0.12.1` — published 2026-03-16

Its changelog explicitly includes:

- “`Merge PR #2724: fix: handle Codex trust dialogs on startup`”
  ([merge commit `609b7bd`](https://github.com/gastownhall/gastown/commit/609b7bdde5bce8f82a058a4658e5b3b3c7dc45f4),
  committed 2026-03-13 21:22:14 UTC). The commit says detection was extended
  to Codex's “Do you trust the contents of this directory?” prompt.
- “`fix: address codex-hooks review findings`”
  ([commit `92a0582`](https://github.com/gastownhall/gastown/commit/92a0582dc7a59522d6007810d331d5a21acba6ab)).
- “`fix: avoid codex session-start json parse errors`”
  ([commit `b03c4bb`](https://github.com/gastownhall/gastown/commit/b03c4bb93dd120cd9503415fb91973d08dca3e02),
  committed 2026-03-12 09:37:12 UTC).
- “`fix: background codex stop hook cost recording`”
  ([commit `39812ad`](https://github.com/gastownhall/gastown/commit/39812adc39829c2e68a61cd506a38f6922bf34f9),
  committed 2026-03-12 09:35:07 UTC).
- “`fix: clean up Codex hooks dead code and add missing test coverage`”
  ([commit `7e5dbf5`](https://github.com/gastownhall/gastown/commit/7e5dbf5927fb71d8245f5824725479138fdbe5e8),
  committed 2026-03-12 21:37:14 UTC).
- “`fix: silence codex stop hook output`”
  ([commit `3430fc4`](https://github.com/gastownhall/gastown/commit/3430fc42d140a0cdd6d201b6cf0de9a45595bbcd),
  committed 2026-03-12 09:37:12 UTC).
- “`refactor: support codex hooks via custom profiles`”
  ([commit `3998fee`](https://github.com/gastownhall/gastown/commit/3998fee1166abed39203dc9d198c8295fa686432)).

The release also repeats the trust-dialog work as the underlying line
“`handle codex trust dialogs on startup`” and includes planning and cleanup
entries around the Codex hooks work. The items above are the operationally
relevant fix/refactor entries rather than every planning commit.

## Later Codex-related work

`v1.0.1` does not contain the literal word `Codex` in its release-note text,
but it lists [commit `93ae201`, “Fix promptless polecat startup and nudge
targeting (#3574)”](https://github.com/gastownhall/gastown/commit/93ae201e2669d30093f4a527c3d1bd8718f80b26),
committed 2026-04-12 20:23:45 UTC. The official merged commit details include
the component change “`fix(polecat): deliver full startup prompt for codex
fallback`.” It is therefore Codex-related behavior shipped in a later release
even though the generated release-note summary does not say so explicitly.

The newest unambiguous Codex-specific work is
[PR #4221](https://github.com/gastownhall/gastown/pull/4221):

- Created 2026-06-10 10:18:42 UTC and merged 2026-06-12 18:36:04 UTC.
- Its stated changes pass bootstrap prompts to Codex instead of dropping them
  under `prompt_mode none`, suppress Codex startup update checks, and fail
  fast when known startup modals remain visible.
- The merge is
  [commit `407e109`](https://github.com/gastownhall/gastown/commit/407e109d1743dbeeb75e32910438f4c40783ad28),
  “`Merge #4221 harden codex agent startup`,” dated 2026-06-12 18:36:03 UTC.

Because all official releases list `v1.2.1` (2026-06-06) as the latest, and
PR #4221 merged six days later, no published GitHub release contained that
June fix as of the retrieval date.

## Source boundary and interpretation

The release-note string test above is deliberately literal and
case-insensitive. Commit messages containing labels such as “Agent: codex” or
“Codex-Reviewed-By” were not counted as Codex product/runtime fixes. Only
changes whose first-party description says they alter Codex startup, hooks,
prompt delivery, trust-dialog handling, nudge delivery, or related Codex
runtime behavior are characterized as Codex-specific.
