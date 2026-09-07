# Configuration reference

`issue-worker` is a Composite Action. The caller repository owns the event trigger, runner selection, timeout, and GitHub permissions. The Action owns the Issue -> setup -> Codex -> verification -> branch -> PR orchestration.

## Caller workflow controls

### Event trigger

Recommended:

```yaml
on:
  issues:
    types: [labeled]
```

with:

```yaml
if: github.event.label.name == 'codex:run'
```

Do not trigger on arbitrary Issue-body keywords. Applying a label is an explicit authorization step and GitHub normally restricts label changes to trusted collaborators.

### Runner

Repository-level:

```yaml
runs-on: [self-hosted, codex]
```

Organization-level runners exposed to the caller repository can use the same labels, or a runner group as appropriate for the Organization.

### Timeout

The Composite Action cannot set a job timeout. Configure it in the caller:

```yaml
timeout-minutes: 60
```

### GitHub token permissions

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

Do not grant Actions, administration, secrets, packages, deployments, or other write permissions unless your surrounding workflow separately needs them.

## Action inputs

### `github-token`

Required.

Pass the caller's short-lived workflow token:

```yaml
github-token: ${{ github.token }}
```

Do not use a long-lived PAT unless a GitHub limitation in your repository design explicitly requires one.

Checkout credentials are not persisted in `.git`. The orchestration wrapper keeps its GitHub token out of the Codex child process.

### `issue-number`

Required.

Typical value:

```yaml
issue-number: ${{ github.event.issue.number }}
```

### `base-branch`

Default: caller repository's default branch.

Example:

```yaml
base-branch: develop
```

The worker fetches this branch, creates a new generated branch from it, and opens the PR back to it.

### `trigger-label`

Default:

```text
codex:run
```

Removed when the worker accepts the run. If setup fails before Codex starts, the setup helper removes this label and marks the Issue failed.

If you change this input, change the caller workflow's `if:` expression to the same value.

### State labels

Defaults:

```text
working-label   = codex:working
review-label    = codex:review
failed-label    = codex:failed
no-change-label = codex:no-change
```

The worker creates/updates these labels when permitted.

### `branch-prefix`

Default:

```text
codex/issue-
```

Generated branch shape:

```text
codex/issue-<issue-number>-<github-run-id>
```

A unique branch per Actions run avoids stale branch collisions and makes retries independent.

### `setup-command`

Default: empty.

`setup-command` is trusted caller-controlled automation executed **after checkout and before Codex**, outside the Codex sandbox.

Example:

```yaml
setup-command: |
  composer install --prefer-dist --no-interaction --no-progress
  npm ci
```

Use this for deterministic dependency preparation when Codex itself should not receive network access.

Behavior:

- empty means setup is skipped
- failure stops the run before Codex starts
- setup failure marks the Issue `codex:failed`
- the setup command is not included in the Codex prompt
- the issue-worker GitHub token is not exported to the setup child process

Do not put literal credentials in `setup-command`. If the project needs package-manager authentication, configure it using the caller's normal secure runner/workflow mechanisms.

`issue-worker` intentionally does not contain package-manager-specific setup logic. Composer, npm, Cargo, pip, and other caches remain a caller/runner concern.

### `verify-command`

Default: empty.

Example:

```yaml
verify-command: |
  php artisan test
  npm run build
```

This command runs **after Codex and before commit/push/PR**, outside the Codex sandbox. It is caller-owned trusted automation and does not receive the issue-worker GitHub token.

Important: Codex may have modified scripts in the repository before this command runs. Therefore this feature should only be used in trusted private repositories where executing modified project code under the runner account is acceptable.

Behavior:

- empty means verification is `skipped`
- success records `passed`
- failure stops before commit, push, and PR creation
- PRs created after successful verification receive a concise `Verification` section
- the PR records command text, result, and Actions run URL, but not command stdout/stderr

For higher isolation, keep `verify-command` empty and have Codex run build/tests inside its sandbox according to `AGENTS.md`.

### `codex-model`

Default: empty, meaning the local Codex CLI default.

Example:

```yaml
codex-model: gpt-5.6-sol
```

Prefer leaving this empty unless the project deliberately pins a model. Codex model availability changes over time.

### `codex-effort`

Default: empty, meaning the runner's Codex default.

Example:

```yaml
codex-effort: high
```

The value is passed as a Codex configuration override for `model_reasoning_effort`.

### `allow-network`

Default:

```text
false
```

When `true`, the worker enables network access inside the Codex `workspace-write` sandbox via a per-run configuration override.

Prefer `setup-command` plus local package caches when dependency preparation is the only reason network access is needed.

Example:

```yaml
allow-network: 'true'
```

## Codex invocation

The current worker uses the equivalent of:

```bash
codex exec \
  --sandbox workspace-write \
  --ask-for-approval never \
  --ephemeral \
  --output-last-message <temp-file> \
  -
```

The prompt is passed through stdin.

Why:

- `workspace-write` allows source edits without unrestricted host access
- `never` prevents an unattended job from hanging waiting for a person
- `--ephemeral` avoids persisting normal session rollout data for the automated job
- Git operations are handled by the wrapper, not Codex

The worker does **not** use `--yolo` or sandbox bypass flags.

## Outputs

### `result`

Possible success values:

```text
review
no-change
```

Failures exit non-zero instead of producing a success result.

### `branch`

Generated branch name.

### `pr-url`

Created pull request URL, empty for `no-change`.

### `final-message`

Last Codex agent message captured from `codex exec`.

### `verification-result`

Successful action values:

```text
passed
skipped
```

A failed verification fails the Action before commit/push/PR, so callers should use the job result for the failure path rather than expecting a successful `failed` output.

## Abnormal termination cleanup

A Composite Action cannot execute its shell trap if the self-hosted runner disappears, loses power, is killed, or otherwise stops executing the process. To recover stale `codex:working` state, add the separate cleanup Action as a second job.

Recommended pattern:

```yaml
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

  cleanup-state:
    needs: implement
    if: always() && github.event.label.name == 'codex:run'
    runs-on: ubuntu-latest
    permissions:
      issues: write
    steps:
      - uses: qmore/issue-worker/cleanup@main
        with:
          github-token: ${{ github.token }}
          issue-number: ${{ github.event.issue.number }}
          job-result: ${{ needs.implement.result }}
```

Cleanup behavior:

- `codex:review` or `codex:no-change` is treated as terminal and never overwritten
- `failure`/`cancelled` plus a stale `codex:working` becomes `codex:failed`
- a recovery comment links the Actions run
- cleanup itself runs on GitHub-hosted infrastructure, so it does not depend on the Codex machine still being alive

A platform-level force-cancel that prevents all remaining jobs from starting cannot be recovered by a later job in the same workflow. The cleanup job covers the normal failure/cancellation/offline-runner cases once GitHub schedules it.

## Example with project-specific settings

```yaml
name: Issue Worker

on:
  issues:
    types: [labeled]

jobs:
  implement:
    if: github.event.label.name == 'codex:run'
    runs-on: [self-hosted, codex, embedded]
    timeout-minutes: 45

    permissions:
      contents: write
      issues: write
      pull-requests: write

    steps:
      - uses: qmore/issue-worker@main
        with:
          github-token: ${{ github.token }}
          issue-number: ${{ github.event.issue.number }}
          base-branch: develop
          setup-command: './scripts/bootstrap-deps.sh'
          verify-command: './scripts/verify.sh'
          codex-effort: high
          allow-network: 'false'
```

Put durable project rules in `AGENTS.md` rather than duplicating them in workflow YAML.
