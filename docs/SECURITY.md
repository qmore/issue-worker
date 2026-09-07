# Security model

`issue-worker` deliberately connects GitHub Issues to a coding agent running on a real self-hosted machine. That is powerful, but it creates a larger trust boundary than ordinary read-only CI.

Read this document before enabling the Action.

## Threat model

The primary risks are:

1. **Untrusted trigger** — someone without authority causes an automated coding job to start.
2. **Prompt injection** — Issue content instructs the model to ignore repository policy or access secrets.
3. **Credential exposure** — GitHub, Codex, SSH, cloud, package-manager, or other credentials become visible to code or tools running in the job.
4. **Host compromise** — generated code or build/test scripts execute outside a strong isolation boundary.
5. **Supply-chain change** — a referenced Action changes unexpectedly.
6. **Persistent-runner contamination** — one job leaves files/configuration that affect a later job.
7. **Excess GitHub permissions** — a compromised job can modify more repository resources than needed.
8. **Stale workflow state** — runner loss/cancellation leaves `codex:working` behind and misrepresents the run.

## Intended trust boundary

Recommended deployment:

```text
trusted maintainer
    |
    | applies codex:run
    v
private caller repository
    |
    v
trusted setup-command (optional, host-side)
    |
    v
self-hosted runner
    |
    | workspace-write sandbox
    v
Codex CLI
    |
    v
trusted verify-command (optional, host-side)
    |
    v
branch / PR
```

State recovery is intentionally separate:

```text
self-hosted implement job
    |
    | needs + always()
    v
GitHub-hosted cleanup job
```

The self-hosted runner should not be shared with arbitrary public pull-request workloads.

## Trigger authorization

Use an explicit label trigger:

```text
codex:run
```

Do not execute merely because an Issue contains a keyword such as `codex`.

For an `issues:labeled` event, `github.actor` is normally the user who applied the label. The worker queries GitHub and fails closed unless that actor has repository push permission.

This check is defense in depth; the caller repository should still restrict who can manage labels and workflows.

## GitHub token handling

The caller normally passes:

```yaml
github-token: ${{ github.token }}
```

with minimum implementation permissions:

```yaml
permissions:
  contents: write
  issues: write
  pull-requests: write
```

The cleanup job only needs:

```yaml
permissions:
  issues: write
```

`issue-worker` intentionally:

- checks out with `persist-credentials: false`
- removes the Action input token from exported script environment before child commands
- invokes `gh` with the token only for individual GitHub API commands
- does not export the GitHub token to Codex
- does not export the issue-worker GitHub token to `setup-command` or `verify-command` children
- does not embed the token in repository remote URLs
- uses temporary Git AskPass helpers for authenticated private fetch/push operations
- deletes temporary credential helper files after use
- disables Git commit hooks for the automated commit

This reduces exposure but does not turn an untrusted host into a safe host. A caller workflow that separately exports secrets globally can still expose those secrets to child processes; keep caller-level secret handling minimal.

## Codex credentials

The package invokes the local `codex` executable and reuses the runner's own Codex authentication.

Treat Codex credentials as password-equivalent material.

When the standalone daemon uses `codex.backend: app-server`, Issue text, Codex
messages, commands, and file-change events are retained in the worker account's
Codex task history. The GitHub PAT remains excluded from the app-server process
environment and requests. The thread retains `workspace-write`, disabled network
access by default, and non-interactive approvals. Timeout or transport failure
triggers an explicit turn interruption before wrapper Git operations.

Never:

- commit them
- upload them as artifacts
- paste them in an Issue
- print them to workflow logs
- expose them to an LLM setup transcript

Prefer a dedicated runner OS user whose home directory contains only credentials required for this workload.

## Codex sandbox

The worker uses:

```text
--sandbox workspace-write
--ask-for-approval never
```

This is intentional.

`workspace-write` lets Codex edit the repository while restricting broader filesystem access. `never` means an unattended Actions job will not stop waiting for interactive approval.

The worker does not use sandbox-bypass options as a convenience shortcut.

If a project cannot function under `workspace-write`, improve the project/runner environment first. Broader host access should require a separate explicit design review.

## Network access

Default:

```text
allow-network = false
```

Prefer preparing dependencies with trusted `setup-command` plus normal runner/package-manager caches instead of enabling Codex network access merely for dependency installation.

If `allow-network: 'true'` is enabled, only do so when:

- the repository is trusted
- the Issue trigger is trusted
- outbound access is genuinely needed
- the runner does not contain unrelated credentials/data

Consider host-level outbound filtering for higher assurance.

## Prompt injection

Issue content is explicitly marked as untrusted inside the generated prompt. Codex is instructed not to access credentials, escape the workspace, modify runner configuration, or perform GitHub operations.

This is a guardrail, not a security boundary.

The real boundaries are:

- authorized label application
- private repository access
- minimal GitHub token permissions
- OS account isolation
- Codex sandbox
- limited network
- host-side verification policy
- human PR review

Never rely on prompt text alone to protect secrets.

## Trusted `setup-command`

`setup-command` runs **outside the Codex sandbox**, after checkout and before Codex starts.

Its purpose is to prepare dependencies/tooling without giving Codex network access.

Security implications:

- it executes with runner-user privileges
- it may access the network according to host policy
- it can execute repository-controlled package-manager lifecycle scripts
- it must be treated as trusted workflow configuration
- literal credentials should not be embedded in command text
- caller workflows should use their normal approved secret/package-manager mechanisms when authentication is required

If setup fails, Codex does not start and the Issue is marked failed.

## Trusted `verify-command`

`verify-command` runs **outside the Codex sandbox**, after Codex modifies the workspace and before commit/push/PR.

This is a stronger trust boundary than setup because Codex may have changed project scripts before verification executes.

Therefore:

- enable it only for trusted private repositories
- use a dedicated unprivileged runner user
- avoid unrelated secrets in that user's environment
- prefer deterministic verification entrypoints
- consider a container or separate isolation boundary for higher assurance

A failed verification is a hard gate: no commit, push, or PR should be created.

The PR records only command text, pass/skip state, and the Actions run URL. Verification stdout/stderr remains in Actions logs and is not intentionally copied into the PR.

## Abnormal termination cleanup

A shell trap cannot run after power loss, process kill, OS restart, or other runner disappearance. `issue-worker` therefore provides a separate cleanup Composite Action intended to run on GitHub-hosted infrastructure.

Cleanup is conservative:

- `codex:review` is terminal and never overwritten
- `codex:no-change` is terminal and never overwritten
- only `failure`/`cancelled` plus stale `codex:working` transitions to `codex:failed`
- recovery adds an Actions run URL comment

This prevents a later metadata/annotation failure from converting an already-created PR into a false failed state.

A platform-level force-cancel that prevents all remaining jobs from starting cannot be recovered by a later job in the same workflow.

## Git hooks and Git metadata

The worker commits with:

```text
core.hooksPath=/dev/null
```

for the automated commit. This prevents repository changes from introducing a commit hook that executes outside the Codex sandbox during the wrapper's commit step.

The wrapper also snapshots/restores trusted local Git configuration around Codex execution and verifies the expected generated branch is still active before wrapper-side Git operations.

## Persistent self-hosted runner hygiene

Persistent runners retain state between jobs. Recommended practices:

- dedicate the runner to trusted development workloads
- do not keep sensitive unrelated repositories in the runner work directory
- periodically update the Actions runner and Codex CLI
- periodically inspect disk usage and stale work directories
- use one Codex auth cache per runner or serialized stream
- avoid simultaneous jobs sharing the same credential cache
- keep the OS and toolchain patched
- use package-manager caches rather than broad agent network access when possible

A single GitHub runner process executes one job at a time. If multiple runner processes share one machine, account for concurrency explicitly.

## Public repositories

The `issue-worker` source repository may be public, but the target repository/self-hosted execution environment should be private and trusted by default.

Do not attach a sensitive self-hosted runner to public untrusted fork/PR workloads.

The public Action repository does not own the user's self-hosted runner. The caller repository or Organization owns runner selection.

## Supply-chain pinning

During initial development examples may use:

```yaml
uses: qmore/issue-worker@main
uses: qmore/issue-worker/cleanup@main
```

For stable deployments, pin both to the same release tag or full commit SHA.

Also consider pinning other third-party Actions used by the caller workflow.

## Human review

The worker opens a pull request; it does not merge it.

Recommended policy:

- protect the base branch
- require human review
- require normal CI checks where available
- do not allow the worker identity to bypass branch protection

The PR boundary is intentional: automatic implementation and automatic deployment/merge should be separate decisions.

## Incident response

If you suspect credential exposure or malicious execution:

1. Stop/disable the self-hosted runner.
2. Revoke/rotate affected GitHub PATs, SSH keys, API keys, package-manager credentials, and Codex auth as applicable.
3. Inspect Actions logs without publishing secret material.
4. Inspect generated branches/commits and runner work directories.
5. Re-register the runner if its trust is uncertain.
6. Reseed Codex authentication from a trusted machine if required.
7. Restore known-good runner configuration before reenabling jobs.

## Reporting security issues

Do not post credential-bearing security reports as public GitHub Issues. Repository maintainers should enable GitHub Private Vulnerability Reporting before promoting this project broadly.
