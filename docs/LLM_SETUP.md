# LLM setup guide

This document is for an LLM / coding agent asked to configure `qmore/issue-worker` on a trusted self-hosted machine.

## Mission

Configure a **private** GitHub repository so that a maintainer can apply `codex:run` to an Issue and have local Codex implement it on a caller-owned self-hosted runner, optionally prepare dependencies before Codex, verify the result before push/PR, and recover stale Issue state if the self-hosted runner disappears.

## Safety invariants

The setup agent must follow all of these:

1. Never ask the user to paste a GitHub PAT, Codex auth file, refresh token, API key, SSH private key, password, or runner registration token into chat.
2. Never commit credentials or place them in Issues, workflow YAML, logs, artifacts, or model-visible notes.
3. Use a private target repository by default.
4. Do not use `--yolo`, `--dangerously-bypass-approvals-and-sandbox`, or `danger-full-access` as a setup shortcut.
5. Trigger only from an explicit maintainer-applied label, not arbitrary Issue text.
6. Keep the implementation job permissions to `contents: write`, `issues: write`, and `pull-requests: write` unless a separate requirement is justified.
7. Keep the cleanup job at `issues: write` only.
8. Configure a finite implementation timeout.
9. Prefer a dedicated, unprivileged runner OS account without unrelated credentials.
10. Do not silently enable Codex network access just because dependencies are missing; prefer trusted `setup-command` preparation first.
11. Do not put literal secrets inside `setup-command` or `verify-command`.

## Values to establish

Resolve these before editing the caller repository:

```text
TARGET_REPO        = owner/repository
RUNNER_SCOPE       = repository | organization
RUNNER_LABEL       = codex
BASE_BRANCH        = empty/default or explicit branch
RUNNER_OS          = macOS | Linux
ISSUE_WORKER_REF   = qmore/issue-worker@main during setup; pin for production
SETUP_COMMAND      = empty or trusted dependency-preparation command
VERIFY_COMMAND     = empty or trusted verification command
```

Choose repository scope for one private repository. Choose Organization scope when multiple repositories in one Organization should share a runner.

Do not confuse runner scope with the location of `qmore/issue-worker`: `issue-worker` is a Composite Action and the caller repository owns `runs-on`.

## Phase A — inspect the machine

Run read-only checks first:

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

Do not install anything until missing prerequisites are known.

## Phase B — install prerequisites

### macOS

If Xcode Command Line Tools are missing:

```bash
xcode-select --install
```

If Homebrew is already approved and `gh` is missing:

```bash
brew install gh
```

Install Codex using the current supported OpenAI installation path for the machine, then verify:

```bash
git --version
gh --version
codex --version
```

### Linux

Install `git` and `gh` through the user's approved package-management process, install Codex using the current supported OpenAI path, and verify all three commands.

## Phase C — verify Codex authentication

First attempt a non-mutating non-interactive run:

```bash
codex exec \
  --sandbox read-only \
  --ask-for-approval never \
  --ephemeral \
  "Reply with the single word OK."
```

If authentication is missing, have the human complete the supported Codex sign-in flow under the same OS account that will run the GitHub Actions service.

Never print or read Codex auth secrets into model context. If only file presence/permissions need inspection, inspect metadata only.

## Phase D — register the self-hosted runner

### Repository scope

Human navigation:

```text
TARGET_REPO -> Settings -> Actions -> Runners -> New self-hosted runner
```

### Organization scope

Human navigation:

```text
Organization -> Settings -> Actions -> Runners -> New runner
```

Use GitHub's generated platform commands and short-lived registration token exactly as provided. Never copy that token into repository files or chat summaries.

Recommended custom label:

```text
codex
```

Acceptance check: GitHub shows the runner online/idle or the local runner reports that it is listening for jobs.

## Phase E — service startup

Configure the GitHub runner to start unattended according to GitHub's current platform instructions.

On macOS, from the runner directory, this is commonly:

```bash
./svc.sh install
./svc.sh start
./svc.sh status
```

Verify the service uses the intended HOME/PATH and can see the same Codex installation/authentication as the interactive account.

## Phase F — allow PR creation

In the private target repository, check:

```text
Settings -> Actions -> General -> Workflow permissions
```

Enable GitHub Actions PR creation if repository/Organization policy permits it.

If Organization policy forbids this, do not bypass policy. Use the organization's approved GitHub App/PAT approach if one exists.

## Phase G — choose setup and verification

### `setup-command`

Use when dependencies must exist before Codex starts, especially when Codex network access should remain disabled.

Examples:

```yaml
setup-command: |
  composer install --prefer-dist --no-interaction --no-progress
  npm ci
```

Rules:

- trusted workflow configuration only
- runs outside Codex sandbox
- failure prevents Codex from starting
- do not embed literal credentials
- prefer package-manager caches already available on the self-hosted runner

Do not make `issue-worker` framework-specific. Laravel/npm/Composer/etc. remain caller concerns.

### `verify-command`

Use when the wrapper must prove the generated workspace passes deterministic checks before commit/push/PR.

Example:

```yaml
verify-command: |
  php artisan test
  npm run build
```

Rules:

- runs after Codex and before commit/push/PR
- failure must stop the run
- command output stays in Actions logs
- the PR only gets command text, pass/skip result, and Actions run URL

Because Codex may modify project scripts, only run verification commands in trusted private repositories where executing modified project code under the runner account is acceptable.

## Phase H — add the caller workflow

Create:

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

          # Add only when needed:
          # setup-command: './scripts/bootstrap-deps.sh'
          # verify-command: './scripts/verify.sh'

  cleanup-state:
    needs: implement
    if: always() && github.event.label.name == 'codex:run'
    runs-on: ubuntu-latest

    permissions:
      issues: write

    steps:
      - name: Recover stale issue-worker state
        uses: qmore/issue-worker/cleanup@main
        with:
          github-token: ${{ github.token }}
          issue-number: ${{ github.event.issue.number }}
          job-result: ${{ needs.implement.result }}
```

Use the same ref for both Actions. Pin a release tag or full commit SHA for production.

Why cleanup is separate:

- a shell trap cannot run if the self-hosted process/machine disappears
- the cleanup job runs on GitHub-hosted infrastructure
- `codex:review` and `codex:no-change` are terminal and must never be overwritten
- `failure`/`cancelled` plus stale `codex:working` becomes `codex:failed`

A platform-level force-cancel that prevents all remaining jobs from starting cannot be repaired by a later job in the same workflow.

## Phase I — project instructions

Inspect `AGENTS.md` before creating or changing it.

Use it for durable rules only, for example:

```markdown
# AGENTS.md

- Build: `./scripts/build.sh`
- Test: `./scripts/test.sh`
- Do not edit generated files under `generated/`.
- Keep changes scoped to the Issue.
```

Do not place credentials or machine-local secrets in `AGENTS.md`.

## Phase J — labels

Ensure this trigger exists:

```text
codex:run
```

The Action manages these state labels when permitted:

```text
codex:working
codex:review
codex:failed
codex:no-change
```

## Phase K — smoke test

Create a harmless Issue:

```text
Title: issue-worker smoke test

Create ISSUE_WORKER_SMOKE_TEST.md with one sentence stating that issue-worker is configured correctly. Do not modify any other files.
```

Have a human with write-level access apply `codex:run`.

Expected successful path:

```text
Issue codex:run
  -> self-hosted runner receives job
  -> optional setup-command passes
  -> codex:working
  -> Codex edits workspace
  -> optional verify-command passes
  -> generated branch is pushed
  -> pull request opens
  -> PR shows Verification section
  -> codex:review
```

Do not auto-merge the smoke test.

## Recovery test

After the normal smoke test works, optionally test cleanup with a disposable Issue/workflow run:

1. Start a harmless worker job.
2. After `codex:working` appears, cancel the implementation job normally from GitHub.
3. Confirm the GitHub-hosted `cleanup-state` job runs.
4. Confirm `codex:working` is removed and `codex:failed` is applied.

Do not simulate recovery by killing unrelated production processes or powering off a shared machine.

## Failure decision tree

### Workflow queued

Check runner online state, scope/group visibility, runner labels, and whether the only runner is busy.

### `codex` missing only in Actions

The service PATH differs from the interactive shell. Check `whoami`, `HOME`, `PATH`, and `command -v codex` in the service context.

### Codex auth works interactively but not as a service

Check service user, HOME, keychain/file-backed credential availability, and `CODEX_HOME`. Do not dump credential contents.

### setup fails

Read the setup step Actions log. Do not enable Codex network access as the first fix. Prefer deterministic runner/package-manager preparation.

### verification fails

The worker should stop before commit/push/PR and mark the Issue failed. Fix the generated code or verification environment; do not bypass the check merely to get a PR.

### `codex:working` remains after cancellation

Check that the caller workflow contains the GitHub-hosted `cleanup-state` job, uses `if: always()`, and gives it `issues: write`.

### PR creation denied

Check repository/Organization Actions workflow permissions.

## Completion criteria

Setup is complete only when all are true:

- [ ] target repository is private
- [ ] self-hosted runner is online and visible to the caller
- [ ] runner has the selected label(s)
- [ ] Codex works non-interactively under the runner service account
- [ ] caller workflow has finite timeout and minimal permissions
- [ ] `codex:run` exists
- [ ] optional setup/verification commands are explicitly chosen and trusted
- [ ] cleanup job exists on GitHub-hosted infrastructure
- [ ] smoke test reaches `codex:review`
- [ ] generated PR shows verification state
- [ ] no credentials were copied into repo/Issue/log/model-visible documentation
