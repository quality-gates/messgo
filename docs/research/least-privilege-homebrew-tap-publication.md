# Least-privilege Homebrew tap publication from GitHub Actions

## Question

How should a release workflow in `quality-gates/messgo` cause the public
`quality-gates/homebrew-tap` repository to update while minimizing cross-repository
authority and remaining compatible with protected branches, auditable operation,
and safe retries?

This note uses only first-party GitHub and Homebrew sources. It describes a design,
not an implementation.

## Recommendation

Use an **organization-owned GitHub App to call `workflow_dispatch` in the tap**, and
perform the content change in a workflow owned by `quality-gates/homebrew-tap`:

1. Register a private GitHub App owned by `quality-gates`, grant it only **Actions:
   write**, and install it only on `quality-gates/homebrew-tap`. GitHub Apps can act
   independently of a user, can be installed on selected repositories, and have
   narrow permissions; GitHub also advises choosing the minimum app permissions
   necessary ([About creating GitHub Apps](https://docs.github.com/en/apps/creating-github-apps/about-creating-github-apps/about-creating-github-apps),
   [Registering a GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app)).
2. In the trusted `messgo` release job, mint an installation access token narrowed
   to the tap repository and Actions permission, then call the tap's fixed
   publication workflow with `workflow_dispatch`. The endpoint accepts GitHub App
   installation tokens, requires **Actions: write**, takes a target `ref` and
   declared inputs, and returns the workflow run ID and URLs
   ([Create a workflow dispatch event](https://docs.github.com/en/rest/actions/workflows#create-a-workflow-dispatch-event)).
3. In the tap workflow, use its repository-scoped `GITHUB_TOKEN` with only
   `contents: write` and `pull-requests: write` to update a deterministic automation
   branch and create or update a PR against the protected default branch. A
   `GITHUB_TOKEN` is limited to the repository containing its workflow, so the
   content-writing authority stays in the tap rather than crossing into `messgo`
   ([The `GITHUB_TOKEN`](https://docs.github.com/en/actions/concepts/security/github_token),
   [Use `GITHUB_TOKEN` for authentication](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication)).
4. Do **not** put the App on a ruleset bypass list and do not push directly to the
   protected default branch. Required PR reviews and status checks therefore remain
   the merge gate. Rulesets can explicitly grant GitHub Apps bypass permission,
   including a “for pull requests only” mode, but that should be an intentional
   later policy decision rather than a publication prerequisite
   ([Creating rulesets for a repository](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository#granting-bypass-permissions-for-your-branch-or-tag-ruleset)).

This split is the least-privilege practical event-driven design because the only
credential exposed to `messgo` can start a named tap workflow but cannot write tap
contents or pull requests. The tap's default-branch workflow is the policy boundary:
it validates the release input and decides exactly what may be written.

## Why `workflow_dispatch` is the right indirection

`workflow_dispatch` is narrower and more observable than `repository_dispatch` for
this use case:

| Pattern | Minimum fine-grained permission on the tap | Event contract and response | Consequence |
| --- | --- | --- | --- |
| `workflow_dispatch` | **Actions: write** | Names a workflow, supplies a `ref` and declared inputs, and returns a run ID plus API/UI URLs ([REST endpoint](https://docs.github.com/en/rest/actions/workflows#create-a-workflow-dispatch-event)). The workflow file must exist on the default branch ([event documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_dispatch)). | Preferred: the source credential cannot write repository contents, inputs are an explicit interface, and the release job can poll the exact target run. |
| `repository_dispatch` | **Contents: write** | Supplies an `event_type` and arbitrary `client_payload`; the event uses the default branch and only runs when its workflow exists there ([REST endpoint](https://docs.github.com/en/rest/repos/repos#create-a-repository-dispatch-event), [event documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#repository_dispatch)). The endpoint returns only `204 No Content` ([REST endpoint](https://docs.github.com/en/rest/repos/repos#create-a-repository-dispatch-event)). | Rejected: merely signaling publication gives the source credential direct content-write capability and weaker run correlation. |

Both dispatch events are explicit exceptions to GitHub's normal recursion prevention:
they create workflow runs even when initiated with a repository's `GITHUB_TOKEN`
([Triggering a workflow from a workflow](https://docs.github.com/en/actions/how-tos/writing-workflows/choosing-when-your-workflow-runs/triggering-a-workflow#triggering-a-workflow-from-a-workflow)).
That exception does not make `messgo`'s own `GITHUB_TOKEN` sufficient, because that
token can access only `messgo`, not the tap
([The `GITHUB_TOKEN`](https://docs.github.com/en/actions/concepts/security/github_token)).

## Authentication comparison

### GitHub App installation token — recommended

- **Scope and lifetime.** The app installation can be limited to the tap; each token
  can be further limited to named repositories and permissions and cannot exceed the
  installation's grants. An installation token expires after one hour
  ([Generating an installation access token](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app#generating-an-installation-access-token)).
- **Ownership and continuity.** Register the app under the organization, not a human.
  GitHub Apps act independently of users, while installation tokens are described by
  GitHub as first-class actors decoupled from any GitHub user
  ([About creating GitHub Apps](https://docs.github.com/en/apps/creating-github-apps/about-creating-github-apps/about-creating-github-apps),
  [Managing deploy keys: GitHub App installation access tokens](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/managing-deploy-keys#github-app-installation-access-tokens)).
- **Secret exposure.** The App private key is the long-lived credential held by the
  trusted release job. GitHub private keys do not expire, must be manually revoked,
  and can provide persistent App authentication if an attacker reads one
  ([Managing private keys for GitHub Apps](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/managing-private-keys-for-github-apps#about-private-keys-for-github-apps),
  [storing private keys](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/managing-private-keys-for-github-apps#storing-private-keys)).
  Limit the blast radius at the App registration and installation—not merely when
  minting a token—store the key as a protected release-environment secret available
  only to the publication job, rotate it, and never expose it to pull-request code.
  GitHub environments can restrict which branches/tags deploy and gate access to
  environment secrets behind protection rules
  ([Managing environments](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments)).
  Actions secrets are not passed to workflows triggered from forks, but called
  actions can access `github.token` and should receive minimum permissions
  ([Using secrets in GitHub Actions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions#using-secrets-in-a-workflow),
  [Use `GITHUB_TOKEN` for authentication](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow)).
- **Audit and protection.** The dispatch is attributable to a durable App identity;
  the target run URL, PR, checks, and commits provide a second local trail. An
  organization audit log records who performed an action, what happened, and when
  ([Reviewing the audit log](https://docs.github.com/en/organizations/keeping-your-organization-secure/managing-security-settings-for-your-organization/reviewing-the-audit-log-for-your-organization)).
  Apps are eligible ruleset-bypass actors, so verify that this App is absent from
  bypass lists ([Creating rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository#granting-bypass-permissions-for-your-branch-or-tag-ruleset)).
- **Setup cost.** An organization owner or App manager must register the App,
  generate/manage its private key, and install it on the tap. GitHub documents these
  as required setup for App authentication in Actions
  ([Making authenticated API requests with a GitHub App in Actions](https://docs.github.com/en/apps/creating-github-apps/writing-code-for-a-github-app/making-authenticated-api-requests-with-a-github-app-in-a-github-actions-workflow)).

### Fine-grained personal access token — acceptable fallback, not preferred

- A fine-grained PAT can be restricted to one resource owner, selected repositories,
  and specific permissions, so a PAT limited to the tap with Actions write can call
  the same `workflow_dispatch` endpoint
  ([Managing personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#about-personal-access-tokens),
  [workflow-dispatch endpoint](https://docs.github.com/en/rest/actions/workflows#create-a-workflow-dispatch-event)).
- The token is nevertheless a human credential: both fine-grained and classic PATs
  are tied to their creator and become inactive if that user loses resource access.
  GitHub explicitly recommends a GitHub App for organization access or long-lived
  integrations
  ([Managing personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#about-personal-access-tokens)).
- Organization policy can require owner approval and can enforce token lifetime. The
  documented default maximum lifetime for organization fine-grained PATs is 366 days
  ([PAT organization policy](https://docs.github.com/en/organizations/managing-programmatic-access-to-your-organization/setting-a-personal-access-token-policy-for-your-organization#enforcing-a-maximum-lifetime-policy-for-personal-access-tokens),
  [approval policy](https://docs.github.com/en/organizations/managing-programmatic-access-to-your-organization/setting-a-personal-access-token-policy-for-your-organization#enforcing-an-approval-policy-for-fine-grained-personal-access-tokens)).
- Actions and audit records are attributed to the human token owner rather than a
  purpose-named service identity. That is less clear operationally and creates
  offboarding/rotation coupling. Use this only if App administration is unavailable,
  with a dedicated maintainer-owned token, explicit expiry, and the same target-side
  workflow/PR design.

### Deploy key — reject

- A deploy key is an SSH credential attached to one repository, read-only by default
  but optionally writable. A write deploy key can perform the same actions as an
  organization member with admin access; it has no expiry date and remains active
  after the creating user leaves because it is attached to the repository
  ([Managing deploy keys](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/managing-deploy-keys#deploy-keys)).
- It is suitable for Git transport, not the Actions API or pull-request API. Directly
  exposing a non-expiring, broadly writable SSH key to the source release job would
  defeat the indirection and give poorer authentication attribution. Opening a PR
  would still require another API credential.
- Protected-branch behavior is also a poor fit: write deploy keys carry admin-like
  power, while GitHub's ruleset bypass UI explicitly models roles, teams, GitHub Apps,
  and Dependabot—not deploy keys
  ([Managing deploy keys](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/managing-deploy-keys#deploy-keys),
  [ruleset bypass actors](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository#granting-bypass-permissions-for-your-branch-or-tag-ruleset)).

### Direct cross-repository write — reject

Using either an App token or PAT directly from `messgo` to clone, push, and open a PR
can be made repository-scoped, but it requires at least tap Contents write and Pull
requests write throughout the source job. It also makes the source workflow responsible
for tap policy. The target-dispatch design reduces source-side authority to Actions
write and makes the protected tap workflow the reviewable publication contract.

### Target-side scheduled polling — safest credential boundary, weaker release semantics

A tap-owned scheduled workflow could periodically inspect public `messgo` releases and
publish with only its own `GITHUB_TOKEN`; no cross-repository secret would exist in
`messgo`. GitHub warns, however, that scheduled workflows can be delayed or even
dropped during high load, run only from the default branch, and are automatically
disabled in a public repository after 60 days without repository activity
([`schedule` event](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#schedule)).
This is a reasonable fallback if eventual publication is acceptable, but it is weaker
than an exact release-to-run handshake.

## Branch protection and merge mechanics

The target workflow should never write the protected default branch directly:

1. Validate all dispatch inputs. Accept a strict version/tag syntax, retrieve the
   corresponding public `messgo` release, and verify the release/tag and expected
   assets before generating formula content. Treat all dispatch inputs as untrusted
   even though the caller is an App.
2. Push only to a deterministic unprotected branch such as
   `automation/messgo/<version>`. Do not grant the dispatch App or the Actions bot a
   ruleset bypass.
3. Configure the tap's Actions settings to permit `GITHUB_TOKEN` to create pull
   requests, then create or update one PR to the protected default branch. GitHub
   exposes this as the **Allow GitHub Actions to create and approve pull requests**
   workflow-permissions setting
   ([Managing GitHub Actions settings](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository#preventing-github-actions-from-creating-or-approving-pull-requests)).
   Required status
   checks must pass before a protected branch can be merged, and required PR reviews
   remain enforceable
   ([About protected branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches#require-status-checks-before-merging)).
4. If desired, enable repository auto-merge only after the team chooses the review
   policy; auto-merge still waits for all required reviews and checks
   ([Automatically merging a pull request](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/automatically-merging-a-pull-request)).

One operational caveat is deliberate: when a workflow uses `GITHUB_TOKEN` to create or
update a PR, GitHub currently puts the resulting `pull_request` workflow runs into an
approval-required state. A writer must approve those runs. Using an App installation
token or PAT for the content/PR operation avoids that prompt
([When `GITHUB_TOKEN` triggers workflow runs](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs)).
For the strictest least-privilege design, accept that human approval as part of the PR
gate. If unattended CI becomes a hard requirement, use a **separate target-local App**
with Contents/Pull requests write rather than broadening the dispatch App whose private
key is stored in `messgo`; keep that second App off all bypass lists.

## Idempotent retry mechanics

The release workflow should regard target publication as an asynchronous operation and
record the returned workflow run ID/URL. Because the current workflow-dispatch endpoint
returns those identifiers, it can poll that exact run and fail the release publication
stage on target failure or timeout
([Create a workflow dispatch event](https://docs.github.com/en/rest/actions/workflows#create-a-workflow-dispatch-event)).

Retries still need target-side idempotence: a network failure can occur after GitHub
accepted a dispatch but before the caller persisted its response, and a manual rerun
can legitimately send the same release twice. Use all of these guards:

- Pass an immutable release identity (`tag`, release ID, and source commit SHA where
  available), not “latest”.
- Serialize tap publication with one workflow-level concurrency group such as
  `homebrew-tap-messgo`. GitHub concurrency permits at most one running workflow per
  group; queued behavior is explicit and ordering is not guaranteed unless the queue
  mode is chosen appropriately
  ([Using concurrency](https://docs.github.com/en/actions/using-jobs/using-concurrency)).
- Before writing, compare the requested version and checksums with the formula on the
  default branch. If already correct, finish successfully without a commit or PR.
- Reuse the deterministic version branch. Generate the complete expected formula,
  commit only when its tree differs, and update with lease semantics rather than
  accumulating duplicate commits.
- Query for an existing open PR by exact head and base; update it if found, create one
  only if absent, and return success if an equivalent PR has already merged.
- Put the release identity and target run URL in the PR body and source workflow
  summary. This correlates the source release, App dispatch, target run, commit, and PR.

These controls make duplicate dispatches harmless while preserving every branch rule.

## Homebrew-specific boundary

The tap PR should contain the generated formula update and let the tap's own checks
validate it. Homebrew's tap documentation treats PR review and successful checks as the
publication gate for tap changes and recommends binding publication to the reviewed
commit SHA when publishing bottles
([How to create and maintain a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)).
That reinforces keeping formula-generation and review policy in the tap repository.

## Decision summary

Choose **GitHub App installation token + `workflow_dispatch` + target-owned PR
workflow**. It has more initial setup than a PAT, but gives the durable service identity,
one-repository installation, one-hour runtime token, narrow Actions-only source
permission, exact target-run correlation, and clean separation from protected content
writes. Keep the App off bypass lists, keep content writes behind a tap PR, and make the
target workflow idempotent around an immutable release identity.
