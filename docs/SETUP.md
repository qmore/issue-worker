# Setup guide

This guide prepares a trusted private repository and a self-hosted Mac/Linux machine to run `qmore/issue-worker`.

## 1. Decide runner scope

Choose one:

### Repository-level runner

Use this when one machine/job registration belongs to one repository.

```text
private repo A -> repo A runner -> issue-worker -> Codex
```

Register it at:

```text
Repository -> Settings -> Actions -> Runners -> New self-hosted runner
```

### Organization-level runner

Use this when several repositories in one Organization should share the same runner pool.

```text
repo A --\
repo B ----> Organization runner -> issue-worker -> Codex
repo C --/
```

Register it at:

```text
Organization -> Settings -> Actions -> Runners -> New runner
```

Control which repositories may use the runner with Organization runner groups / repository access policy.

`issue-worker` itself does not care which scope you choose. The caller workflow decides `runs-on`.

## 2. Prepare the runner account

Prefer a dedicated macOS/Linux user for automated development. The account should have:

- write access to the GitHub Actions runner working directory
- access to the project toolchain
- Codex CLI authentication
- no unnecessary administrator privileges
- no unrelated SSH keys, cloud credentials, browser profiles, or production secrets

Do not use a public-repository self-hosted runner for this workload.

## 3. Install prerequisites

### macOS

Git is available after Xcode Command Line Tools:

```bash
xcode-select --install
```

Install GitHub CLI, for example with Homebrew:

```bash
brew install gh
```

Install Codex CLI using the current OpenAI standalone installer:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
```

Check:

```bash
git --version
gh --version
codex --version
```

### Linux

Install Git and GitHub CLI using your distribution/vendor instructions, then install Codex:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
```

## 4. Authenticate Codex

OpenAI recommends API-key authentication for general CI/CD. `issue-worker` is intentionally optimized for trusted persistent self-hosted runners and can reuse local Codex CLI authentication.

Run once interactively on the runner account:

```bash
codex
```

Choose the desired sign-in method.

Verify non-interactive authentication without granting write access:

```bash
codex exec \
  --sandbox read-only \
  --ask-for-approval never \
  --ephemeral \
  "Reply with the single word OK."
```

### ChatGPT-managed authentication on a persistent runner

If you intentionally use ChatGPT-managed Codex authentication in CI, OpenAI documents this as an advanced trusted-runner pattern.

Important rules:

- treat `~/.codex/auth.json` like a password
- never commit or upload it as an artifact
- use one auth cache per runner / serialized stream
- do not overwrite a refreshed auth file with an old seed every run
- reseed with `codex login` if refresh fails

On headless or service-oriented runners, file-backed credential storage can be easier to operate than a GUI keychain. Follow the current OpenAI guidance rather than copying auth tokens into workflow YAML.

Reference: https://developers.openai.com/codex/auth/ci-cd-auth

## 5. Register the GitHub self-hosted runner

Open GitHub's **New self-hosted runner** page at the repository or Organization scope selected in step 1.

GitHub generates platform-specific download/configuration commands and a time-limited registration token. Run exactly the commands GitHub provides.

Recommended custom label:

```text
codex
```

You can supply labels during runner configuration or add them later in GitHub settings.

The runner should eventually show:

```text
Connected to GitHub
Listening for Jobs
```

GitHub registration tokens are short-lived; do not hard-code them in documentation or scripts.

## 6. Run the runner as a service

After the runner has been registered, GitHub creates `svc.sh` in the runner directory.

On macOS, GitHub supports a launchd-backed service:

```bash
./svc.sh install
./svc.sh start
./svc.sh status
```

Run these from the installed Actions runner directory. Follow GitHub's generated/current platform instructions if they differ.

Reference: https://docs.github.com/actions/how-tos/manage-runners/self-hosted-runners/configure-the-application

## 7. Allow GitHub Actions to create pull requests

`issue-worker` opens a PR after Codex finishes. GitHub can disable PR creation by `GITHUB_TOKEN` at the repository/Organization policy level. New personal-account repositories may have this disabled by default.

In the target repository, check:

```text
Settings -> Actions -> General -> Workflow permissions
```

Enable:

```text
Allow GitHub Actions to create and approve pull requests
```

The worker only creates a PR; it does not approve or merge it.

If an Organization policy disables this option, change the Organization policy or use an appropriately scoped GitHub App installation token / PAT according to your security policy.

Reference: https://docs.github.com/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository

## 8. Add the caller workflow

In the **private target repository**, copy `examples/issue-worker.yml` to:

```text
.github/workflows/issue-worker.yml
```

Minimum form:

```yaml
name: Issue Worker

on:
  issues:
    types: [labeled]

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
      - uses: qmore/issue-worker@main
        with:
          github-token: ${{ github.token }}
          issue-number: ${{ github.event.issue.number }}
```

Pin a release or full commit SHA once stable.

### Note about CI on generated pull requests

GitHub deliberately limits recursive workflow triggering by the repository `GITHUB_TOKEN`.

With the default `${{ github.token }}`:

- the worker's `git push` does **not** trigger normal `push` workflows
- a generated PR can create `pull_request` workflow runs for `opened` / `synchronize` / `reopened`, but those runs are placed in an **approval-required** state
- a user with write access can approve those workflows from the PR

If your generated PR must start downstream CI automatically without manual approval, pass a GitHub App installation token (preferred for automation) or a suitably scoped PAT as `github-token` instead of `${{ github.token }}`. Protect that credential as a secret and keep its permissions minimal.

Reference: https://docs.github.com/actions/concepts/security/github_token

## 9. Add project instructions

Add an `AGENTS.md` to the target repository if the project has rules Codex must follow.

Example:

```markdown
# AGENTS.md

- Build with `./scripts/build.sh`.
- Do not change generated files under `vendor/`.
- C++11 and later are not allowed.
- Keep public API compatibility unless an Issue explicitly changes it.
- Run `./scripts/test.sh` before finishing when available.
```

## 10. Create the trigger label

Create:

```text
codex:run
```

The other state labels are created/updated automatically by the worker when permissions allow it:

```text
codex:working
codex:review
codex:failed
codex:no-change
```

## 11. Smoke test

Create a harmless Issue such as:

```text
Title: Add issue-worker smoke-test note

Add a file named ISSUE_WORKER_SMOKE_TEST.md containing one sentence that says the worker is configured correctly. Do not change any other files.
```

Apply `codex:run` manually as a repository maintainer.

Expected result:

1. workflow starts on the self-hosted runner
2. label changes to `codex:working`
3. Codex edits the checkout
4. worker commits/pushes a `codex/issue-...` branch
5. worker opens a pull request
6. Issue receives `codex:review`

Review the PR, then delete the smoke-test file/branch if not wanted.

## Troubleshooting

### Job stays queued

The caller repository cannot see an online runner matching `runs-on`. Check:

- runner scope (repository vs Organization)
- runner group access
- custom `codex` label
- service status

### `codex` not found

The Actions runner service may have a different PATH than your interactive shell. Install Codex in a PATH visible to the service account or configure the service environment accordingly.

### Codex authentication fails only as a service

The service account/session may not have access to the same keychain or `CODEX_HOME` as your terminal. Verify the service user and credential storage. For persistent CI use, follow OpenAI's ChatGPT-managed auth guide if applicable.

### Worker rejects the actor

The workflow actor must have write-level repository access. For `issues:labeled`, the actor is normally the person who applied the trigger label.

### Private repository fetch fails

`issue-worker` intentionally checks out with `persist-credentials: false`. Its wrapper performs required private fetch/push operations using a temporary AskPass helper and the supplied `github-token`. If this fails, verify the token has `contents: write` on the caller repository.

### Pull request creation is denied

Check **Settings -> Actions -> General -> Workflow permissions -> Allow GitHub Actions to create and approve pull requests**. Organization policy can override the repository setting.

### Generated PR CI says approval is required

This is expected when the PR was created with the repository `GITHUB_TOKEN`. Approve the workflow manually, or use a GitHub App installation token / PAT when fully automatic downstream CI is required.

### Codex cannot download dependencies

`workspace-write` has restricted network access by default. Prefer preinstalled/cached dependencies. If the project genuinely requires network access, set `allow-network: 'true'` only on a trusted runner/repository and review the security implications.

### A command needs interactive approval

`issue-worker` runs Codex with `--ask-for-approval never`. The job will not pause for a person. Adjust the runner/project so required commands fit the configured sandbox rather than bypassing the sandbox.
