# Configuration reference

`issue-worker` is a Composite Action. The caller repository owns the event trigger, runner selection, timeout, and GitHub permissions. The Action owns the Issue -> Codex -> branch -> PR orchestration.

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

Organization-level runner exposed to the caller repo can use the same labels, or a runner group as appropriate for the Organization.

### Timeout

The Composite Action cannot set a job timeout. Configure it in the caller:

```yaml
timeout-minutes: 60
```

### GitHub token permissions

Minimum write permissions required by the current worker:

```yaml
permissions:
  contents: write
  issues: write
  pull-requests: write
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

The worker removes checkout credentials from `.git`, keeps the token only in wrapper memory, and does not export it to the Codex process.

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

Removed when the job starts.

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

Use it only when necessary. Prefer preinstalled tools, local package caches, vendored dependencies, and deterministic build environments.

Example:

```yaml
allow-network: 'true'
```

### `verify-command`

Default: empty.

Example:

```yaml
verify-command: './scripts/verify.sh'
```

This command runs **after Codex and outside the Codex sandbox**. It is caller-owned trusted automation and does not receive the wrapper's GitHub token.

Important: Codex may have modified scripts in the repository before this command runs. Therefore this feature should only be used in trusted private repositories where executing the modified project code under the runner account is acceptable.

For higher isolation, keep `verify-command` empty and have Codex run build/tests inside its sandbox according to `AGENTS.md`.

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

- `workspace-write` allows source edits without unrestricted host access.
- `never` prevents an unattended job from hanging waiting for a person.
- `--ephemeral` avoids persisting normal session rollout data for the automated job.
- Git operations are handled by the wrapper, not Codex.

The worker does **not** use deprecated `--full-auto` and does not use `--yolo` / sandbox bypass flags.

## Outputs

### `result`

Possible current values:

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
          codex-effort: high
          allow-network: 'false'
```

Put durable project rules in `AGENTS.md` rather than duplicating them in workflow YAML.
