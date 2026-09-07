# Security model

`issue-worker` deliberately connects GitHub Issues to a coding agent running on a real self-hosted machine. That is powerful, but it creates a larger trust boundary than ordinary read-only CI.

Read this document before enabling the Action.

## Threat model

The primary risks are:

1. **Untrusted trigger** — someone without authority causes an automated coding job to start.
2. **Prompt injection** — Issue content instructs the model to ignore repository policy or access secrets.
3. **Credential exposure** — GitHub, Codex, SSH, cloud, or other credentials are visible to code or tools running in the job.
4. **Host compromise** — generated code or build/test scripts execute outside a strong isolation boundary.
5. **Supply-chain change** — a referenced Action changes unexpectedly.
6. **Persistent-runner contamination** — one job leaves files/configuration that affect a later job.
7. **Excess GitHub permissions** — a compromised job can modify more repository resources than needed.

## Intended trust boundary

Recommended deployment:

```text
trusted maintainer
    |
    | applies codex:run
    v
private caller repository
    |
    | short-lived GITHUB_TOKEN
    v
self-hosted runner dedicated to trusted development
    |
    | workspace-write sandbox
    v
Codex CLI
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

The caller passes:

```yaml
github-token: ${{ github.token }}
```

with minimum permissions:

```yaml
permissions:
  contents: write
  issues: write
  pull-requests: write
```

`issue-worker` intentionally:

- checks out with `persist-credentials: false`
- removes the action input token from the exported environment at script start
- invokes `gh` with the token only for individual GitHub API commands
- does not export the GitHub token to Codex
- does not embed the token in the repository remote URL
- uses a temporary Git askpass helper only at push time
- disables Git commit hooks for the automated commit

This reduces exposure but does not turn an untrusted host into a safe host.

## Codex credentials

The package invokes the local `codex` executable and reuses the runner's own Codex authentication.

Treat Codex credentials as secrets. In particular, OpenAI documents ChatGPT-managed `~/.codex/auth.json` as password-equivalent material.

Never:

- commit it
- upload it as an artifact
- paste it in an Issue
- print it to workflow logs
- expose it to an LLM setup transcript

Prefer a dedicated runner OS user whose home directory contains only credentials required for this workload.

## Codex sandbox

The worker currently uses:

```text
--sandbox workspace-write
--ask-for-approval never
```

This is intentional.

`workspace-write` lets Codex edit the repository while restricting broader filesystem access. `never` means an unattended Actions job will not stop waiting for interactive approval.

The worker does not use:

```text
--dangerously-bypass-approvals-and-sandbox
--yolo
danger-full-access
```

as a convenience shortcut.

If a project cannot function under `workspace-write`, improve the project/runner environment first. Broader host access should require a separate explicit design review.

## Network access

Default:

```text
allow-network = false
```

This is preferable for deterministic private build environments and reduces exfiltration opportunities.

If `allow-network: 'true'` is enabled, Codex can make network requests from its workspace sandbox according to Codex's current sandbox semantics. Only enable it when:

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
- human PR review

Never rely on prompt text alone to protect secrets.

## Build and test execution

Codex may run project commands inside its sandbox.

The optional `verify-command` is different: it runs after Codex, outside the Codex sandbox, under the runner OS account. Because Codex may have modified repository scripts, this can execute generated code with broader runner-user access.

Therefore:

- leave `verify-command` empty by default
- enable it only for trusted private repositories
- prefer a dedicated unprivileged runner user
- avoid making unrelated secrets available to that user
- consider running deterministic verification in a container or other isolation boundary

## Git hooks

The worker commits with:

```text
core.hooksPath=/dev/null
```

for the automated commit. This prevents a repository change from introducing a commit hook that executes outside the Codex sandbox during the wrapper's commit step.

## Persistent self-hosted runner hygiene

Persistent runners retain state between jobs. Recommended practices:

- dedicate the runner to trusted development workloads
- use `actions/checkout` cleanup and unique branches
- do not keep sensitive unrelated repos in the runner work directory
- periodically update the Actions runner and Codex CLI
- periodically inspect disk usage and stale work directories
- use one Codex auth cache per runner or serialized stream
- avoid simultaneous jobs sharing the same credential cache
- keep the OS and toolchain patched

A single GitHub runner process executes one job at a time. If you install multiple runner processes on the same machine, account for concurrency explicitly.

## Public repositories

GitHub recommends self-hosted runners primarily for private repositories because public forks/PRs can create routes to execute attacker-controlled code on the runner.

`issue-worker` itself may be public as a reusable Action, but the target repository / self-hosted execution environment should be private and trusted by default.

The public Action repository should not own the user's self-hosted runner. The caller repository or Organization owns runner selection.

## Supply-chain pinning

During initial development examples may use:

```yaml
uses: qmore/issue-worker@main
```

For stable deployments, prefer a release tag or full commit SHA.

Also consider pinning other third-party Actions used by the caller workflow.

## Human review

The worker opens a pull request; it does not merge it.

Recommended policy:

- protect the base branch
- require a human review
- require normal CI checks where available
- do not allow the worker identity to bypass branch protection

The PR boundary is intentional: automatic implementation and automatic deployment/merge should be separate decisions.

## Incident response

If you suspect credential exposure or malicious execution:

1. Stop/disable the self-hosted runner.
2. Revoke/rotate affected GitHub PATs, SSH keys, API keys, and Codex auth as applicable.
3. Inspect Actions logs without publishing secret material.
4. Inspect generated branches/commits and runner work directories.
5. Re-register the runner if its trust is uncertain.
6. Reseed Codex authentication from a trusted machine if required.
7. Restore known-good runner configuration before reenabling jobs.

## Reporting security issues

Until a dedicated private vulnerability-reporting channel is configured, do not post credential-bearing security reports as public GitHub Issues. Repository maintainers should enable GitHub Private Vulnerability Reporting before promoting this project broadly.
