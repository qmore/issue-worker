# issue-worker

Turn a trusted GitHub Issue into a local Codex implementation job on your own self-hosted runner.

`issue-worker` is a reusable GitHub Composite Action for private repositories. A maintainer adds a trigger label such as `codex:run` to an Issue, GitHub Actions routes the job to a self-hosted runner, and the action asks the Codex CLI on that machine to implement the Issue. The action creates a dedicated branch, commits the result, pushes it, and opens a pull request for human review.

> **Status:** early v0.x / experimental. Use only on trusted private repositories and review every generated pull request before merging.

## Why this exists

Codex Cloud is useful when the project can run in a cloud environment. Some projects cannot: embedded toolchains, licensed SDKs, local hardware, private networks, large caches, or development environments that already exist on a workstation or build machine.

`issue-worker` keeps execution on your machine:

```text
GitHub Issue
   |
   | add codex:run
   v
GitHub Actions
   |
   | runs-on: self-hosted
   v
Your Mac / Linux runner
   |
   v
Codex CLI
   |
   +-- edits the checked-out repository
   +-- may run local build/test tools allowed by the Codex sandbox
   v
branch -> commit -> push -> Pull Request
```

The public `issue-worker` repository contains orchestration logic only. Your private source code and Codex authentication remain on the caller repository / self-hosted runner.

## Design

There are three separate responsibilities:

- **Caller workflow** — decides *when* to run and *which runner* executes the job.
- **issue-worker** — implements the common Issue -> Codex -> branch -> PR orchestration.
- **Self-hosted runner** — provides the real local development environment and Codex CLI authentication.

Because `issue-worker` is a Composite Action, `runs-on` stays in the caller repository. The same package therefore works with either:

- a repository-level self-hosted runner, or
- an Organization-level self-hosted runner shared by several repositories.

## Requirements

On the self-hosted runner:

- Git
- GitHub CLI (`gh`)
- Codex CLI
- working Codex authentication
- the build/test toolchain required by the target repository

The caller workflow needs these `GITHUB_TOKEN` permissions:

```yaml
permissions:
  contents: write
  issues: write
  pull-requests: write
```

## Quick start

### 1. Prepare the self-hosted runner

Install and authenticate Codex on the runner. On macOS/Linux, OpenAI currently documents:

```bash
curl -fsSL https://chatgpt.com/codex/install.sh | sh
codex
```

Complete sign-in once, then verify non-interactive execution:

```bash
codex exec --sandbox read-only --ask-for-approval never \
  "Reply with the single word OK."
```

Install GitHub CLI if needed (macOS/Homebrew):

```bash
brew install gh
```

Register the machine as a GitHub self-hosted runner for the target repository or Organization. Giving it a custom label such as `codex` is recommended.

Detailed setup: [docs/SETUP.md](docs/SETUP.md)

LLM-oriented deterministic setup guide: [docs/LLM_SETUP.md](docs/LLM_SETUP.md)

### 2. Add a workflow to the private project

Create `.github/workflows/issue-worker.yml` in the **private target repository**:

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
      - name: Run issue-worker
        uses: qmore/issue-worker@main
        with:
          github-token: ${{ github.token }}
          issue-number: ${{ github.event.issue.number }}
```

For production use, pin to a release tag or commit SHA instead of `@main`.

### 3. Add the trigger label

Create the label:

```text
codex:run
```

Create an Issue with a concrete implementation request, then add `codex:run`.

The action will change state labels automatically:

```text
codex:run
   -> codex:working
   -> codex:review   (PR created)

or
   -> codex:failed
   -> codex:no-change
```

## What the worker does

1. Verifies the workflow actor has write/maintain/admin access to the caller repository.
2. Fetches the Issue title and body.
3. Removes the trigger label and applies `codex:working`.
4. Creates a branch based on the configured base branch.
5. Builds a guarded prompt from the Issue.
6. Runs `codex exec` non-interactively with a workspace-write sandbox.
7. Optionally executes a caller-supplied verification command.
8. Commits changed files.
9. Pushes the branch.
10. Opens a pull request referencing the Issue.
11. Applies `codex:review`, or `codex:failed` on failure.

Codex is explicitly instructed **not** to commit or push. Git operations are performed by the wrapper after Codex finishes.

## Inputs

| Input | Required | Default | Description |
| --- | --- | --- | --- |
| `github-token` | yes | - | Caller repository `GITHUB_TOKEN`. |
| `issue-number` | yes | - | Issue number to implement. |
| `base-branch` | no | repository default | Branch the generated branch starts from and PR targets. |
| `trigger-label` | no | `codex:run` | Trigger label removed when work starts. |
| `working-label` | no | `codex:working` | Applied while Codex is running. |
| `review-label` | no | `codex:review` | Applied after a PR is opened. |
| `failed-label` | no | `codex:failed` | Applied when the worker fails. |
| `no-change-label` | no | `codex:no-change` | Applied when Codex makes no tracked changes. |
| `branch-prefix` | no | `codex/issue-` | Prefix for generated branches. |
| `verify-command` | no | empty | Trusted shell command run after Codex, e.g. `npm test`. |
| `codex-model` | no | empty | Optional model override. Empty uses the local Codex default. |
| `codex-effort` | no | empty | Optional reasoning effort override. |
| `allow-network` | no | `false` | Enables network access inside the Codex workspace-write sandbox for this run. |

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for details.

## Project instructions

Put repository-specific rules in `AGENTS.md` in the **target repository**. Examples:

- language / compiler restrictions
- forbidden directories
- build commands
- test commands
- formatting rules
- architectural conventions
- hardware-specific cautions

A useful separation is:

```text
GitHub Issue       = WHAT to implement
AGENTS.md          = HOW code should be written
caller workflow    = WHEN / WHERE it is allowed to run
issue-worker       = orchestration
self-hosted runner = actual environment
```

## Security model

This project intentionally targets **trusted private repositories**.

An Issue body becomes input to a coding agent running on a real machine. Treat triggering a job as equivalent to granting an automated developer access to the checked-out workspace and the commands allowed by the Codex sandbox.

Important defaults / recommendations:

- Trigger by a label, not by arbitrary words in an Issue body.
- The worker checks that the actor who triggered the workflow has write-level repository access.
- Use a dedicated runner account with the minimum OS permissions required.
- Keep Codex in `workspace-write` and non-interactive `never` approval mode.
- Do not give the Codex process GitHub tokens, PATs, SSH private keys, or unrelated secrets.
- Keep credentials outside the checked-out repository.
- Do not use the same self-hosted runner for untrusted public-repository pull requests.
- Review the generated PR before merging.
- Prefer release tags or commit-SHA pinning for third-party Actions.

Read [docs/SECURITY.md](docs/SECURITY.md) before enabling the worker.

## Authentication

`issue-worker` is designed for a persistent trusted self-hosted runner and invokes the locally installed `codex` CLI. It therefore reuses the runner's existing Codex authentication.

OpenAI recommends API-key authentication as the default for CI/CD. ChatGPT-managed Codex authentication can also be maintained on trusted persistent runners; if you use it, treat `~/.codex/auth.json` as a password and never commit or log it.

This package does not read, upload, or manage your Codex credential itself.

## Runner scope

Repository-level runner:

```text
repo A -> repo A runner -> issue-worker -> Codex
```

Organization-level runner:

```text
repo A --\
repo B ----> Organization runner -> issue-worker -> Codex
repo C --/
```

The Action is the same in both cases. Only `runs-on` and the GitHub runner registration scope change.

## Documentation

- [Setup guide](docs/SETUP.md)
- [LLM setup guide](docs/LLM_SETUP.md)
- [Configuration reference](docs/CONFIGURATION.md)
- [Security model](docs/SECURITY.md)
- [Example caller workflow](examples/issue-worker.yml)

## Upstream references

- Codex CLI: https://developers.openai.com/codex/cli
- Codex non-interactive mode: https://developers.openai.com/codex/non-interactive-mode
- Codex security / approvals: https://developers.openai.com/codex/agent-approvals-security
- ChatGPT-managed Codex auth in CI/CD: https://developers.openai.com/codex/auth/ci-cd-auth
- GitHub self-hosted runners: https://docs.github.com/actions/hosting-your-own-runners
- GitHub composite actions: https://docs.github.com/actions/sharing-automations/creating-actions/creating-a-composite-action

## License

MIT License. See [LICENSE](LICENSE).
