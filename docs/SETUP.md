# Setup guide

This guide prepares a trusted private repository and a self-hosted Mac/Linux machine to run `qmore/issue-worker`.

## 1. Decide runner scope

### Repository-level runner

Use when the runner registration belongs to one repository.

```text
private repo A -> repo A runner -> issue-worker -> Codex
```

GitHub navigation:

```text
Repository -> Settings -> Actions -> Runners -> New self-hosted runner
```

### Organization-level runner

Use when multiple Organization repositories should share a runner pool.

```text
repo A --\
repo B ----> Organization runner -> issue-worker -> Codex
repo C --/
```

GitHub navigation:

```text
Organization -> Settings -> Actions -> Runners -> New runner
```

Control repository access with Organization runner groups/policy.

`issue-worker` does not care which scope you choose. The caller repository owns `runs-on`.

## 2. Prepare the runner account

Prefer a dedicated macOS/Linux user with:

- write access to the Actions runner working directory
- the project toolchain
- Codex CLI authentication
- no unnecessary administrator privileges
- no unrelated production/cloud/SSH/browser credentials

Do not use a public-repository self-hosted runner for this workload.

## 3. Install prerequisites

Required:

```text
git
gh
codex
```

### macOS

Install Xcode Command Line Tools if needed:

```bash
xcode-select --install
```

Install GitHub CLI, for example with Homebrew:

```bash
brew install gh
```

Install Codex using the current supported OpenAI installation path, then check:

```bash
git --version
gh --version
codex --version
```

### Linux

Install Git and GitHub CLI through your distribution/vendor process, install Codex using the current supported OpenAI path, and verify all three commands.

## 4. Authenticate Codex

Authenticate under the same OS account that will run the GitHub Actions service.

Verify non-interactive authentication with a read-only run:

```bash
codex exec \
  --sandbox read-only \
  --ask-for-approval never \
  --ephemeral \
  "Reply with the single word OK."
```

Treat Codex authentication material like a password. Never commit it, upload it as an artifact, or paste it into workflow YAML/Issues/chat.

## 5. Register the self-hosted runner

Use GitHub's **New self-hosted runner** page for the selected repository/Organization scope.

GitHub generates platform-specific commands and a short-lived registration token. Run the generated commands exactly as provided.

Recommended custom label:

```text
codex
```

Acceptance check:

```text
Connected to GitHub
Listening for Jobs
```

or GitHub shows the runner online/idle.

## 6. Run the runner as a service

On macOS, after registration, GitHub commonly supports:

```bash
./svc.sh install
./svc.sh start
./svc.sh status
```

Run these from the Actions runner directory and follow GitHub's current generated instructions if they differ.

Verify the service sees the same HOME/PATH/Codex authentication as the intended runner account.

## 7. Allow Actions to create pull requests

`issue-worker` opens a PR after successful verification.

Check:

```text
Settings -> Actions -> General -> Workflow permissions
```

Enable GitHub Actions PR creation if repository/Organization policy permits it.

The worker does not approve or merge its own PR.

## 8. Add the caller workflow

Copy [../examples/issue-worker.yml](../examples/issue-worker.yml) to the private target repository as:

```text
.github/workflows/issue-worker.yml
```

Recommended form:

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

          # Optional:
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

Pin **both** `uses:` entries to the same release tag or full commit SHA for production.

## 9. Configure dependency setup

Use `setup-command` when dependencies must be prepared before Codex starts:

```yaml
setup-command: |
  composer install --prefer-dist --no-interaction --no-progress
  npm ci
```

This step:

- runs after checkout
- runs outside the Codex sandbox
- runs before Codex
- stops the job if it fails
- marks the Issue `codex:failed` on setup failure
- does not pass the issue-worker GitHub token into the setup child process
- does not inject the setup command into the Codex prompt

Prefer this plus runner/package-manager caches instead of granting Codex network access solely for dependency installation.

Do not put literal secrets in the command text.

## 10. Configure verification

Use `verify-command` for deterministic checks after Codex:

```yaml
verify-command: |
  php artisan test
  npm run build
```

Verification runs outside the Codex sandbox **after Codex and before commit/push/PR**.

If verification fails:

```text
no commit
no push
no PR
Issue -> codex:failed
```

If it succeeds, the generated PR contains a concise Verification section with:

- command text
- passed/skipped state
- Actions run URL

stdout/stderr remains in the Actions log and is not copied into the PR.

## 11. Why the cleanup job is separate

The main Action can catch ordinary shell errors, but no shell trap can execute after:

- runner power loss
- runner process kill
- OS restart
- runner disappearance
- some cancellation paths

Therefore the recommended workflow has a second job:

```text
self-hosted implement
        |
        | needs + always()
        v
GitHub-hosted cleanup-state
```

Cleanup treats these as terminal and leaves them alone:

```text
codex:review
codex:no-change
```

If the implementation result is `failure` or `cancelled` and the Issue still has `codex:working`, cleanup:

```text
remove codex:working
add codex:failed
comment Actions run URL
```

A platform-level force-cancel that prevents all remaining jobs from starting cannot be repaired by a later job in the same workflow.

## 12. Add project instructions

Add or maintain `AGENTS.md` in the target repository for durable project constraints.

Example:

```markdown
# AGENTS.md

- Build with `./scripts/build.sh`.
- Run `./scripts/test.sh` before finishing.
- Do not change generated files under `vendor/`.
- C++11 and later are not allowed.
- Keep changes scoped to the Issue.
```

Do not put credentials or machine-local secrets in `AGENTS.md`.

## 13. Create the trigger label

Create:

```text
codex:run
```

The worker manages these state labels when allowed:

```text
codex:working
codex:review
codex:failed
codex:no-change
```

## 14. Smoke test

Create a harmless Issue:

```text
Title: Add issue-worker smoke-test note

Add ISSUE_WORKER_SMOKE_TEST.md containing one sentence that says the worker is configured correctly. Do not change any other files.
```

Apply `codex:run` as a repository maintainer.

Expected path:

1. workflow starts on the self-hosted runner
2. optional setup passes
3. Issue becomes `codex:working`
4. Codex edits the checkout
5. optional verification passes
6. worker pushes a `codex/issue-...` branch
7. worker opens a PR
8. PR shows Verification state
9. Issue becomes `codex:review`
10. cleanup job observes terminal state and does nothing

Review the PR manually.

## 15. Optional cleanup recovery test

With a disposable smoke-test Issue:

1. start the worker
2. wait for `codex:working`
3. cancel the implementation job normally
4. confirm GitHub-hosted `cleanup-state` runs
5. confirm `codex:working` is removed
6. confirm `codex:failed` is added

Do not test this by powering off a shared production machine.

## Troubleshooting

### Job stays queued

Check runner scope, runner group access, `codex` label, service status, and whether the only runner is busy.

### `codex` not found

The service PATH may differ from the interactive shell. Check `whoami`, `HOME`, `PATH`, and `command -v codex` in the runner service context.

### Codex authentication fails only as a service

Verify the service account/session and credential storage. Do not print auth contents into logs.

### setup-command fails

Read the setup step Actions log. Fix dependency/toolchain preparation rather than enabling broad Codex network access as the first response.

### verification fails

This is a hard gate by design. The worker should not create a PR. Fix the generated code or the verification environment; do not bypass the gate merely to produce a PR.

### `codex:working` remains after cancellation

Confirm the caller has the `cleanup-state` job, `if: always()`, `needs: implement`, and `issues: write`.

### Private repository fetch fails

The worker intentionally uses `persist-credentials: false` and temporary AskPass authentication. Verify the supplied token has caller-repository `contents: write`.

### Pull request creation denied

Check Actions workflow permissions at repository and Organization scope.

### Generated PR CI requires approval

This can occur when the PR is created using the repository `GITHUB_TOKEN`. Approve according to repository policy or use an approved GitHub App/PAT design when fully automatic downstream CI is required.
