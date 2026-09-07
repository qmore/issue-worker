# LLM setup guide

This document is for an LLM / coding agent asked to configure `qmore/issue-worker` on a trusted self-hosted machine.

## Mission

Configure a **private** GitHub repository so that a maintainer can apply `codex:run` to an Issue and have local Codex implement it on a caller-owned self-hosted runner, producing a branch and pull request.

## Safety invariants

The setup agent must follow all of these:

1. Never ask the user to paste a GitHub PAT, Codex auth file, refresh token, API key, SSH private key, password, or runner registration token into chat.
2. Never commit credentials or place them in Issues, workflow YAML, logs, artifacts, or model-visible notes.
3. Use a private target repository by default.
4. Do not use `--yolo`, `--dangerously-bypass-approvals-and-sandbox`, or `danger-full-access` as a setup shortcut.
5. Trigger only from an explicit maintainer-applied label, not arbitrary Issue text.
6. Keep caller workflow permissions to `contents: write`, `issues: write`, and `pull-requests: write` unless a separate requirement is justified.
7. Configure a finite job timeout.
8. Prefer a dedicated, unprivileged runner OS account without unrelated credentials.
9. Do not silently install a package manager or alter Organization security policy.
10. Stop at a pull request; do not enable automatic merge as part of setup.

## Values to resolve

```text
TARGET_REPO       = owner/repository
RUNNER_SCOPE      = repository | organization
RUNNER_LABEL      = codex
BASE_BRANCH       = repository default unless explicitly specified
RUNNER_OS         = macOS | Linux
RUNNER_USER       = OS account used by GitHub Actions service
ISSUE_WORKER_REF  = qmore/issue-worker@main for initial testing
```

Choose repository scope for one repo. Choose Organization scope when several repositories in the same Organization should share a runner pool.

`issue-worker` is a Composite Action. The caller repository owns `runs-on`; the public Action does not own or select the user's machine.

## Phase A — inspect first

Run read-only checks:

```bash
uname -a
whoami
pwd
git --version || true
gh --version || true
codex --version || true
command -v git || true
command -v gh || true
command -v codex || true
```

On macOS:

```bash
xcode-select -p || true
```

Do not install components that are already present.

## Phase B — install missing prerequisites

Required runtime tools:

```text
git
gh
codex
bash
```

### macOS

If Apple Command Line Tools are missing:

```bash
xcode-select --install
```

This may open a GUI installer; allow the human to complete it.

If Homebrew already exists and `gh` is missing:

```bash
brew install gh
```

Do not install Homebrew without user approval.

Install Codex using the current official OpenAI installer:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
```

### Linux

Install Git and GitHub CLI through the user's approved package-management process, then install Codex using the current official OpenAI instructions.

Verify:

```bash
git --version
gh --version
codex --version
```

## Phase C — verify Codex authentication

Try a read-only non-interactive run under the intended runner account:

```bash
codex exec \
  --sandbox read-only \
  --ask-for-approval never \
  --ephemeral \
  "Reply with the single word OK."
```

If sign-in is required, launch:

```bash
codex
```

and let the human complete the supported authentication flow.

Do not print or inspect credential contents. If checking a file-backed auth cache is necessary, inspect only metadata such as existence/permissions.

For trusted persistent CI authentication, follow current OpenAI guidance:

https://developers.openai.com/codex/auth/ci-cd-auth

## Phase D — register the self-hosted runner

### Repository-level

Human navigation:

```text
TARGET_REPO -> Settings -> Actions -> Runners -> New self-hosted runner
```

### Organization-level

Human navigation:

```text
Organization -> Settings -> Actions -> Runners -> New runner
```

GitHub generates platform-specific commands containing a short-lived registration credential. Use the commands GitHub generated on the runner; never copy that credential into repository files or a chat summary.

Recommended custom runner label:

```text
codex
```

Acceptance condition: GitHub shows the runner Online/Idle or its console shows it is listening for jobs.

## Phase E — enable unattended runner service

After registration, from the Actions runner installation directory on macOS:

```bash
./svc.sh install
./svc.sh start
./svc.sh status
```

Use GitHub's current platform instructions on Linux.

Verify the service runs as `RUNNER_USER` and that its `HOME` and `PATH` can find the same Codex installation/authentication verified in Phase C.

## Phase F — verify GitHub Actions PR policy

`issue-worker` creates a pull request after implementation. Check the target repository:

```text
Settings -> Actions -> General -> Workflow permissions
```

The repository/Organization policy must allow GitHub Actions to create pull requests when the default `${{ github.token }}` is used.

Do not weaken a higher-level Organization policy without explicit administrator approval.

## Phase G — add the caller workflow

Create in the **private target repository**:

```text
.github/workflows/issue-worker.yml
```

Recommended initial workflow:

```yaml
name: Issue Worker

on:
  issues:
    types: [labeled]

concurrency:
  group: issue-worker-${{ github.repository }}
  cancel-in-progress: false

jobs:
  implement:
    if: github.event.label.name == 'codex:run'
    runs-on: [self-hosted, codex]
    timeout-minutes: 60

    permissions:
      contents: write
      issues: write
      pull-requests: write

    steps:
      - name: Implement Issue with local Codex
        uses: qmore/issue-worker@main
        with:
          github-token: ${{ github.token }}
          issue-number: ${{ github.event.issue.number }}
```

If the project explicitly uses a non-default base branch, add for example:

```yaml
          base-branch: develop
```

Do not add `secrets: inherit` or unrelated secrets.

For production, pin `issue-worker` to a release tag or full commit SHA rather than `@main`.

### Generated PR CI behavior

With the repository `${{ github.token }}`, GitHub intentionally limits recursive workflow execution. A PR created by the worker can have `pull_request` CI in an approval-required state, and pushes made with that token do not trigger normal `push` workflows.

This is acceptable for the default human-review workflow. If fully automatic downstream CI is a requirement, follow GitHub's current guidance for a GitHub App installation token or other approved credential. Do not improvise a long-lived credential in workflow YAML.

Reference:

https://docs.github.com/actions/concepts/security/github_token

## Phase H — add project instructions

Inspect `AGENTS.md` in the target repo.

If it exists, preserve it and modify only when necessary. If the repository has durable non-obvious rules and no `AGENTS.md`, create a concise one, for example:

```markdown
# AGENTS.md

- Build: `./scripts/build.sh`
- Test: `./scripts/test.sh`
- Do not edit generated files under `generated/`.
- Keep changes scoped to the Issue.
```

Never put credentials or machine-specific secrets in `AGENTS.md`.

## Phase I — trigger label

Ensure this label exists:

```text
codex:run
```

The Action will attempt to manage these state labels:

```text
codex:working
codex:review
codex:failed
codex:no-change
```

## Phase J — smoke test

Create a harmless Issue:

```text
Title: issue-worker smoke test

Create ISSUE_WORKER_SMOKE_TEST.md with one sentence stating that issue-worker is configured correctly. Do not modify any other files.
```

A human with write access applies `codex:run`.

Expected flow:

```text
Issue + codex:run
  -> Actions run
  -> self-hosted runner
  -> codex:working
  -> Codex edits workspace
  -> branch + commit + push
  -> pull request
  -> codex:review
```

Review the generated PR manually. Do not auto-merge the smoke test.

## Failure decision tree

### Job remains queued

Check:

1. runner is online
2. caller repository can access its repository/Organization runner scope
3. runner has every label in `runs-on`
4. another job is not occupying the only runner

Do not switch to a GitHub-hosted runner merely to clear the queue; local execution is the point of this package.

### `codex` works in Terminal but not Actions

Compare the interactive account and runner service context:

```bash
whoami
printf '%s\n' "$HOME"
printf '%s\n' "$PATH"
command -v codex
```

Likely causes are different user, HOME, PATH, keychain availability, or `CODEX_HOME`.

### Worker rejects the actor

The user applying `codex:run` must have write-level repository access. Do not disable this check as the first fix.

### PR creation is denied

Check the Actions workflow-permissions setting from Phase F and any inherited Organization policy.

### Generated PR CI requires approval

That is expected with the repository `GITHUB_TOKEN`. A user with write access can approve the workflow. Only change authentication if unattended downstream CI is an explicit requirement.

### Codex cannot access dependencies

Network is disabled in the workspace-write sandbox by default. Prefer preinstalled/cached dependencies. If the trusted project genuinely needs outbound network, the caller can explicitly set:

```yaml
          allow-network: 'true'
```

Document why this is required.

### Deterministic post-check is needed

Prefer normal verification instructions in `AGENTS.md`. The optional caller input:

```yaml
          verify-command: './scripts/verify.sh'
```

runs after Codex **outside** the Codex sandbox. Because Codex may have modified repository scripts, use this only in a trusted private repository and under a dedicated unprivileged runner account.

## Completion checklist

Do not declare setup complete until all are true:

- [ ] target repository is private
- [ ] runner is online and visible to the caller repo
- [ ] runner labels match `runs-on`
- [ ] runner service uses the intended OS account
- [ ] Git/gh/Codex are visible in the service PATH
- [ ] non-interactive Codex auth works under that account
- [ ] Actions policy permits the worker to create a PR
- [ ] caller workflow exists with minimal permissions and finite timeout
- [ ] `codex:run` exists
- [ ] smoke-test Issue reaches `codex:review`
- [ ] generated PR is reviewed by a human
- [ ] no credential was copied into repo content, Issues, logs, artifacts, or model-visible notes

## Canonical references

When upstream behavior differs, follow current official documentation:

- OpenAI Codex CLI: https://developers.openai.com/codex/cli
- OpenAI non-interactive mode: https://developers.openai.com/codex/non-interactive-mode
- OpenAI approvals/security: https://developers.openai.com/codex/agent-approvals-security
- OpenAI trusted CI/CD auth: https://developers.openai.com/codex/auth/ci-cd-auth
- GitHub self-hosted runners: https://docs.github.com/actions/how-tos/manage-runners/self-hosted-runners/add-runners
- GitHub runner service: https://docs.github.com/actions/how-tos/manage-runners/self-hosted-runners/configure-the-application
- GitHub `GITHUB_TOKEN`: https://docs.github.com/actions/concepts/security/github_token
- GitHub Actions repository settings: https://docs.github.com/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository
