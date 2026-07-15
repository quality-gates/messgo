# Homebrew artifacts, versioning, compatibility, and release lifecycle

## Scope

This note resolves the implementation decisions in `messgo-z2b.1`,
`messgo-z2b.3`, `messgo-z2b.4`, and `messgo-z2b.5`. It uses only first-party
Homebrew, Go, GitHub, and messgo sources. It complements the separate
least-privilege tap-publication decision; it does not reopen that credential
choice.

## Decision summary

| Decision | Choice |
| --- | --- |
| Package artifact | A normal formula in `quality-gates/homebrew-tap` that selects one of two uploaded, CGO-free GitHub release tarballs: `darwin_arm64` or `darwin_amd64`. Do not use Homebrew bottles for the first implementation. |
| Version authority | Stable release tags must be exactly `vMAJOR.MINOR.PATCH`. Strip the leading `v` once and inject that value into one Go package variable at link time. CLI output, reports, archive names, release metadata, and the formula all consume that derived value. |
| Supported surface | Certify macOS 15 and macOS 26 on both arm64 and Intel x86_64. Use the pinned GitHub labels `macos-15`, `macos-15-intel`, `macos-26`, and `macos-26-intel`. |
| Release ordering | Validate tag -> test -> build both archives -> calculate checksums -> smoke-test both artifacts on all supported hosts -> create and publish one immutable GitHub release -> dispatch an idempotent tap update -> validate clean install and upgrade in the tap PR -> merge under tap branch policy. |
| Failure rule | Publication of the immutable GitHub release is the release commit point. A later tap failure must never delete, replace, or mutate that release or its tag; retry only the tap stage. |

## 1. Use upstream prebuilt archives, not bottles

### Chosen artifact contract

For each stable tag `vX.Y.Z`, upload these three assets to the GitHub release:

```text
messgo_X.Y.Z_darwin_arm64.tar.gz
messgo_X.Y.Z_darwin_amd64.tar.gz
checksums.txt
```

Each tarball contains an executable named `messgo` at its root plus `LICENSE`.
Build the executables with:

```text
CGO_ENABLED=0 GOOS=darwin GOARCH=<arm64|amd64> \
  go build -trimpath \
  -ldflags="-s -w -X github.com/quality-gates/messgo/internal/version.Version=X.Y.Z" \
  -o messgo ./cmd/messgo
```

Go documents `darwin/amd64` and `darwin/arm64` as valid targets, and `GOOS` and
`GOARCH` select the target rather than the build host
([Installing Go from source](https://go.dev/doc/install/source#environment)).
`-trimpath` removes filesystem paths from the executable, and `-ldflags` passes
arguments to the linker
([`go build` flags](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies)).
Making `CGO_ENABLED=0` explicit keeps the artifacts independent of a C compiler,
Homebrew Go formula, and build-host SDK.

The tap formula should be generated in this shape:

```ruby
class Messgo < Formula
  desc "Go-native PHP Mess Detector port"
  homepage "https://github.com/quality-gates/messgo"
  version "X.Y.Z"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/quality-gates/messgo/releases/download/vX.Y.Z/messgo_X.Y.Z_darwin_arm64.tar.gz"
      sha256 "ARM64_SHA256"
    end

    on_intel do
      url "https://github.com/quality-gates/messgo/releases/download/vX.Y.Z/messgo_X.Y.Z_darwin_amd64.tar.gz"
      sha256 "AMD64_SHA256"
    end
  end

  def install
    bin.install "messgo"
  end

  test do
    (testpath/"smoke.go").write "package smoke\n"
    output = shell_output("#{bin}/messgo #{testpath} json go")
    assert_match %Q(\"version\": \"#{version}\"), output
  end
end
```

Homebrew explicitly permits formula components inside `on_macos`, `on_arm`, and
`on_intel` blocks, while runtime conditionals belong inside `install` and `test`
([Formula Cookbook: handling different system configurations](https://docs.brew.sh/Formula-Cookbook#handling-different-system-configurations)).
Homebrew downloads the formula URL, verifies its hash, unpacks the archive, and
runs `install`; its tap guide specifically calls out immutable URLs and matching
SHA-256 values as the formula integrity boundary
([Formula API](https://docs.brew.sh/rubydoc/Formula.html#install-instance_method),
[Maintaining a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap#troubleshooting)).
The explicit formula `version` avoids relying on URL inference; Homebrew advises
adding it when inferred versions are not reliable
([Formula Cookbook: audit the formula](https://docs.brew.sh/Formula-Cookbook#audit-the-formula)).

### Why this model wins

| Model | Result |
| --- | --- |
| Source-building formula | Technically workable with Go as a build-only dependency, and it would not require a Go runtime after installation. Rejected because every user would compile the same release, making installation slower and introducing toolchain/network variability. |
| Upstream prebuilt archives | Chosen. There are two OS-wide artifacts, installation only copies the selected executable, and the same assets serve direct-download and Homebrew users. The formula pins each exact archive with SHA-256. |
| Homebrew bottles | Rejected for the first implementation. Homebrew bottles are produced by installing a formula with `--build-bottle` and then running `brew bottle`; their names and DSL are OS/architecture-specific. That adds a Homebrew build-and-pour pipeline and duplicates already-portable Go artifacts without improving this dependency-free CLI ([Homebrew Bottles: creation and format](https://docs.brew.sh/Bottles#creation)). |

Bottles remain a future option if messgo acquires native dependencies or if the
project moves into `homebrew/core`. They are unnecessary for a project-owned tap:
taps are first-class external formula sources and can be installed directly with
`brew install quality-gates/tap/messgo`
([How to create and maintain a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap#direct-installation-recommended)).

## 2. Make the tag the only release version authority

### Code seam

Create one package-level string variable:

```go
package version

var Version = "dev"
```

Both `internal/cli` and `internal/report` must read this variable. Remove their
current independent constants (`0.1.9` and `0.1.8`). Local builds and tests that
do not pass linker flags intentionally report `dev`; there is no source edit for
a release.

The Go linker documents `-X importpath.name=value` specifically for setting a
string variable initialized by a constant string expression
([Go linker command](https://pkg.go.dev/cmd/link)). This is a more direct release
contract than inferring the module's build information: normal repository builds
may identify the main module as `(devel)`, while the release workflow already has
the authoritative tag.

### Tag validation and derivation

Trigger on pushed tags broadly enough for GitHub's glob syntax, then fail before
any build or release mutation unless all of these hold:

1. `GITHUB_REF_TYPE` is `tag`.
2. `GITHUB_REF_NAME` matches `^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`.
3. The checked-out commit is `GITHUB_SHA` and the remote tag already exists.
4. `VERSION=${GITHUB_REF_NAME#v}` is the only value passed to build and packaging steps.

GitHub defines `GITHUB_REF_NAME` as the short branch or tag name,
`GITHUB_REF_TYPE` as `branch` or `tag`, and `GITHUB_SHA` as the triggering commit
([Default variables](https://docs.github.com/en/actions/reference/workflows-and-actions/variables#default-environment-variables)).
Use `gh release create "$TAG" --draft --verify-tag ...`; the official CLI says
`--verify-tag` aborts rather than silently creating a tag from the default branch
([`gh release create`](https://cli.github.com/manual/gh_release_create)).

Only plain stable tags enter this workflow. Pre-release identifiers and build
metadata are deliberately excluded until Homebrew prerelease distribution has a
separate policy. This also makes these identities mechanically identical:

```text
tag             vX.Y.Z
program version X.Y.Z
archive version X.Y.Z
formula version X.Y.Z
release title   messgo X.Y.Z
```

Before publication, assert all of the following:

- the unpacked arm64 and amd64 executables print exactly `messgo X.Y.Z`;
- a real JSON analysis report contains `"version": "X.Y.Z"`;
- both archive filenames contain `X.Y.Z`;
- the formula generated later contains `version "X.Y.Z"` and URLs under the
  matching `vX.Y.Z` release.

## 3. Compatibility and verification matrix

### Supported claim

The initial Homebrew support claim is:

> messgo supports native Homebrew installation on macOS 15 and macOS 26 on
> Apple Silicon and Intel x86_64, in Homebrew's default architecture prefix.

The four required release jobs are:

| Runner label | Host architecture | Artifact exercised | Required result |
| --- | --- | --- | --- |
| `macos-15` | arm64 | `darwin_arm64` | pass |
| `macos-15-intel` | x86_64 | `darwin_amd64` | pass |
| `macos-26` | arm64 | `darwin_arm64` | pass |
| `macos-26-intel` | x86_64 | `darwin_amd64` | pass |

These architecture/label mappings come from GitHub's maintained runner-image
inventory
([`actions/runner-images`: available images](https://github.com/actions/runner-images#available-images)).
Pin these labels rather than `macos-latest`; GitHub documents that `-latest`
migrates over time and recommends a specific OS label when the image must remain
stable
([runner image label scheme](https://github.com/actions/runner-images#label-scheme)).

Homebrew currently classifies macOS 14, 15, and 26 on both architectures as Tier
1, but forecasts Intel macOS moving to Tier 3 in or after September 2026
([Homebrew support tiers](https://docs.brew.sh/Support-Tiers#future-macos-support)).
Therefore:

- do not claim macOS 14 support merely because the binary may run there; GitHub's
  macOS 14 runner images are already deprecated;
- retain Intel as a messgo-tested compatibility commitment while all pinned Intel
  jobs exist and pass, but do not imply that Homebrew itself will keep Intel at
  Tier 1;
- review the matrix before September 2026 and whenever GitHub announces a runner
  image deprecation. A disappearing label blocks a new claim; it does not justify
  silently changing a pinned runner to `latest`.

### Artifact-level checks before publishing the release

Build both executables once in a platform-neutral build job, package them once,
and pass those exact bytes to every matrix job. Do not rebuild on the smoke-test
runners. Each matrix cell must:

1. verify the archive against the generated SHA-256 manifest;
2. unpack it and assert the host architecture matches the expected matrix value;
3. run `messgo --version` and compare exact output to the tag-derived version;
4. write a minimal valid Go package and run `messgo <path> json go`;
5. assert exit code `0` and the JSON report's version equals the tag-derived version.

This is intentionally a real analysis rather than a version-only formula test.
Homebrew's Formula Cookbook says `test do` is run by `brew test` and BrewTestBot,
and recommends testing basic functionality rather than only `--version`
([Formula Cookbook: add a test](https://docs.brew.sh/Formula-Cookbook#add-a-test-to-the-formula)).

### Installed-command checks in the tap PR

The tap workflow repeats the four-cell matrix against the candidate formula. In
each cell it must run:

```text
brew audit --strict --online quality-gates/tap/messgo
brew style quality-gates/tap/messgo
brew install quality-gates/tap/messgo
brew test quality-gates/tap/messgo
messgo --version
messgo <temporary-valid-package> json go
```

Assert the installed CLI and report version exactly equal `X.Y.Z`. Homebrew's tap
guide says installed taps update through `brew update` and outdated formulae
upgrade through `brew upgrade`, like core formulae
([Maintaining a tap: updating](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap#updating)).

Also run an upgrade scenario on macOS 15 arm64 and Intel:

1. In a temporary tap clone, check out the default-branch base SHA and install
   its `messgo` formula.
2. Switch that same tap clone to the candidate PR SHA.
3. Run `brew upgrade quality-gates/tap/messgo`.
4. Assert `messgo --version` and a JSON analysis report now identify `X.Y.Z`.

For the first formula release, record the upgrade test as not applicable and run
the clean-install matrix only. For later releases, a missing predecessor formula
is a workflow error, not a reason to skip upgrade coverage.

## 4. Stable release lifecycle

### Source repository workflow

1. **Trigger and serialize.** Run only for a pushed stable tag and use a
   concurrency key containing the tag. Validate the tag and existing remote tag
   before requesting `contents: write` release authority. GitHub Actions supports
   tag filters on `push`
   ([workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushbranchestagsbranches-ignoretags-ignore)).
2. **Quality gate.** Check out `GITHUB_SHA`; run the full Go tests, build, messgo
   self-analysis, and an ordinary CLI smoke test.
3. **Build once.** Cross-compile the two CGO-free executables from that SHA with
   the one derived version, package the two archives, and generate
   `checksums.txt` over the final archive bytes.
4. **Certify the artifacts.** Run the four artifact-level matrix jobs above
   against those exact archives. Any failure stops before a GitHub release is
   published.
5. **Create an immutable release.** Enable immutable releases in repository
   settings. Create a draft for the already-existing tag, attach both archives
   and `checksums.txt`, confirm the asset names and locally calculated digests,
   then publish the draft. GitHub's recommended immutable-release order is
   draft -> attach every asset -> publish; publication then locks the tag and
   assets and creates a release attestation
   ([Immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)).
6. **Verify the commit point.** Run `gh release verify "$TAG"` and
   `gh release verify-asset "$TAG" <archive>` for both archives. GitHub documents
   that asset verification checks the local digest against the release
   attestation
   ([Verifying release integrity](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/verify-release-integrity)).
7. **Dispatch the tap update.** Only after the release is public and verified,
   invoke the agreed tap-owned workflow with the tag, version, source commit SHA,
   release ID, both asset names, and both SHA-256 values. Wait for and report the
   target run result, but treat it as distribution status rather than permission
   to roll back the release.

Do not use GitHub's automatically generated source `.zip`/`.tar.gz` links as
formula artifacts. GitHub creates those tag snapshots automatically, whereas the
uploaded release assets are the binaries that were actually certified
([About releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)).

### Tap repository workflow

For the exact immutable release identity supplied by the source workflow:

1. Retrieve the release by exact tag, reject drafts/prereleases, verify its commit
   SHA and expected asset set, and independently recalculate both archive hashes.
2. Generate `Formula/messgo.rb` deterministically from the version, asset URLs,
   and hashes. Never accept formula Ruby or a URL supplied by the dispatch caller.
3. If the default branch already contains byte-for-byte equivalent formula
   content, return success without a commit.
4. Otherwise update one deterministic branch, `automation/messgo-vX.Y.Z`, and
   create or update one PR. The generated file is the complete change; no human
   formula edit is part of the lifecycle.
5. Run formula audit/style, the four-cell clean-install matrix, `brew test`, CLI
   and report version assertions, and the two-cell upgrade scenario.
6. Merge only through the tap's configured protected-branch policy. Successful
   checks may drive auto-merge if that policy is enabled; required review remains
   valid if the tap chooses it. Homebrew's tap guidance treats review plus passing
   PR checks as the publication boundary
   ([How to create and maintain a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap#creating-a-tap)).

### Retry and recovery boundaries

| State reached | Retry behavior |
| --- | --- |
| No draft exists | Rebuild from the tag and create a draft after all artifact tests pass. |
| Matching draft exists | Verify every existing asset before resuming. Upload only missing assets. Never use `--clobber`: GitHub CLI warns that clobber deletes the original before uploading the replacement, so an upload failure loses it ([`gh release upload`](https://cli.github.com/manual/gh_release_upload)). |
| Draft contents differ | Stop. Delete/recreate the draft only after a maintainer investigates; never silently replace an unexpected artifact. |
| Matching immutable release exists | Verify the release and assets, skip source publication, and retry only the tap dispatch. |
| Immutable release differs | Stop permanently for that tag. Publish corrected bytes under a new version tag. |
| Tap workflow/PR/check fails | Leave the GitHub release and tag untouched. Fix or rerun the deterministic tap branch/PR. Duplicate dispatches must converge on the same formula and PR. |
| Tap PR already merged | Confirm the default-branch formula matches the immutable release and return success. |

The source workflow may end with a failed Homebrew-publication job even though the
GitHub release is valid. That failure is an operational signal, not a transaction
rollback: immutable assets cannot be modified or deleted after publication, and
the formula retry consumes the same release identity. This separation makes a
Homebrew outage, branch-policy rejection, or runner failure recoverable without
corrupting the stable release channel.

## Implementation acceptance contract

The plan is complete when a test release proves all of these statements:

- one stable tag produces exactly two macOS executable archives and one checksum
  manifest;
- source contains no release number and an un-stamped local binary reports `dev`;
- both architectures report the tag-derived version in CLI and JSON output;
- all four pinned macOS jobs execute the packaged bytes successfully;
- a generated formula selects the correct archive and contains both exact hashes;
- clean Homebrew install passes in all four matrix cells;
- an upgrade from the predecessor passes on macOS 15 arm64 and Intel;
- retrying dispatch creates neither duplicate release assets nor duplicate tap
  PRs; and
- forcing the tap stage to fail leaves the published GitHub release available and
  allows a tap-only retry to complete later.
