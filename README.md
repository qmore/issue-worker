# issue-worker

Turn a trusted GitHub Issue into a local Codex implementation job on your own self-hosted runner.

`issue-worker` is a reusable GitHub Composite Action for trusted private repositories. A maintainer adds `codex:run` to an Issue, GitHub Actions routes the job to a self-hosted runner, and the local Codex CLI implements the Issue. The wrapper verifies the result, creates a branch, pushes it, and opens a pull request for human review.

> **Status:** early v0.x / experimental. Review every generated pull request before merging.

## Flow

```text
GitHub Issue
   |
   | add codex:run
   v
GitHub Actions
   |
   v
self-hosted runner
   |
   +--> setup-command       (trusted, outside Codex sandbox)
   |
   +--> Codex CLI           (workspace-write sandbox)
   |
   +--> verify-command      (trusted, outside Codex sandbox)
   |
   v
branch -> commit -> push -> Pull Request
   |
   v
codex:review
```

If the self-hosted runner disappears after `codex:working` is applied, an optional GitHub-hosted cleanup job can recover the Issue to `codex:failed`.

## Why this exists

Codex Cloud is useful when a project can run in a cloud environment. Some projects cannot: embedded toolchains, licensed SDKs, local hardware, private networks, large caches, or development environments already installed on a workstation/build machine.

`issue-worker` keeps the implementation environment local while using GitHub Issues and Pull Requests as the job queue and review surface.

## Design boundaries

```text
GitHub Issue       = WHAT to implement
AGENTS.md          = HOW code should be written
caller workflow    = WHEN / WHERE execution is allowed
issue-worker       = orchestration
self-hosted runner = actual development environment
```

Because the package is a Composite Action, `runs-on` stays in the caller repository. The same Action works with either:

- a repository-level self-hosted runner
- an Organization-level self-hosted runner shared by multiple repositories

## Requirements

On the self-hosted runner:

- Git
- GitHub CLI (`gh`)
- Codex CLI
- working Codex authentication
- the project build/test toolchain

The implementation job needs:

```yaml
permissions:
  contents: write
  issues: write
  pull-requests: write
```

The optional cleanup job only needs:

```yaml
permissions:
  issues: write
```

## Quick start

### 1. Prepare the runner

Install Git/`gh`/Codex and authenticate Codex under the OS account that runs the GitHub Actions runner.

Register the machine as a repository-level or Organization-level self-hosted runner and give it a label such as:

```text
codex
```

See [docs/SETUP.md](docs/SETUP.md) and [docs/LLM_SETUP.md](docs/LLM_SETUP.md).

### 2. Add the caller workflow

Create `.github/workflows/issue-worker.yml` in the private target repository:

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

          # Optional trusted preparation before Codex:
          # setup-command: |
          #   composer install --prefer-dist --no-interaction --no-progress
          #   npm ci

          # Optional trusted verification after Codex:
          # verify-command: |
          #   php artisan test
          #   npm run build

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

For production use, pin **both** `uses:` entries to the same release tag or full commit SHA instead of `@main`.

### 3. Trigger a job

Create an Issue with a concrete implementation request and add:

```text
codex:run
```

Typical state transition:

```text
codex:run
   -> codex:working
   -> codex:review
```

Failure/no-change paths:

```text
codex:failed
codex:no-change
```

## `setup-command`

`setup-command` runs after checkout and before Codex, outside the Codex sandbox.

Use it to prepare trusted dependencies without enabling Codex network access:

```yaml
setup-command: |
  composer install --prefer-dist --no-interaction --no-progress
  npm ci
```

Properties:

- optional; empty preserves the original behavior
- failure prevents Codex from starting
- failure marks the Issue `codex:failed`
- command text is not mixed into the Codex prompt
- the issue-worker GitHub token is not exported to the setup child process

Package-manager-specific logic and caches intentionally stay in the caller workflow/runner.

## Verification

`verify-command` runs after Codex and before commit/push/PR:

```yaml
verify-command: |
  php artisan test
  npm run build
```

If verification fails, the worker does **not** commit, push, or create a PR.

Successful PRs receive a concise section like:

```markdown
## Verification

- ✅ Result: **Passed**
- Command:

    php artisan test
    npm run build

- Actions run: ...
```

Only command text, result, and Actions run URL are added to the PR; verification stdout/stderr remains in the Actions log.

The Action exposes:

```text
verification-result = passed | skipped
```

## Abnormal termination recovery

Shell traps cannot run after a hard runner/process failure. The separate `qmore/issue-worker/cleanup` Action is intended to run from a GitHub-hosted cleanup job using `if: always()`.

It follows these rules:

```text
codex:review / codex:no-change
  -> terminal; never overwrite

failure/cancelled + codex:working
  -> remove codex:working
  -> add codex:failed
  -> comment with Actions run URL
```

This covers normal job failure/cancellation/offline-runner recovery once GitHub schedules the cleanup job. A platform-level force-cancel that prevents all remaining jobs from starting cannot be repaired by a later job in the same workflow.

## Security model

This project targets **trusted private repositories**.

Important invariants:

- Issue content is treated as untrusted prompt input
- the trigger actor must have write-level repository access
- checkout credentials are not persisted
- the wrapper GitHub token is not passed to Codex
- Codex starts with a minimized environment via `env -i`
- Codex uses `workspace-write`
- unattended approval mode is non-interactive
- Codex does not commit, push, or create PRs
- wrapper Git operations restore/protect local Git configuration
- automated commits disable Git hooks
- each run gets a new branch
- human PR review remains the default boundary

Read [docs/SECURITY.md](docs/SECURITY.md) before enabling the worker.

## Inputs

| Input | Required | Default | Description |
| --- | --- | --- | --- |
| `github-token` | yes | - | Caller repository `GITHUB_TOKEN`. |
| `issue-number` | yes | - | Issue number to implement. |
| `base-branch` | no | repository default | PR base branch. |
| `trigger-label` | no | `codex:run` | Explicit execution trigger. |
| `working-label` | no | `codex:working` | Active state. |
| `review-label` | no | `codex:review` | PR-created state. |
| `failed-label` | no | `codex:failed` | Failure state. |
| `no-change-label` | no | `codex:no-change` | Successful no-change state. |
| `branch-prefix` | no | `codex/issue-` | Generated branch prefix. |
| `setup-command` | no | empty | Trusted dependency/setup command before Codex. |
| `verify-command` | no | empty | Trusted verification before commit/push/PR. |
| `codex-model` | no | empty | Optional model override. |
| `codex-effort` | no | empty | Optional reasoning-effort override. |
| `allow-network` | no | `false` | Codex sandbox network access. |

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the full reference.

## Documentation

- [Setup guide](docs/SETUP.md)
- [LLM setup guide](docs/LLM_SETUP.md)
- [Configuration reference](docs/CONFIGURATION.md)
- [Security model](docs/SECURITY.md)
- [Example caller workflow](examples/issue-worker.yml)

## License

MIT License. See [LICENSE](LICENSE).
