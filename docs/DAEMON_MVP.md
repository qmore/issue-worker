# Standalone daemon MVP

This document describes the experimental standalone `issue-worker` daemon. It is the first implementation of the architecture where **the worker polls GitHub**, instead of GitHub Actions invoking a self-hosted runner.

The existing Composite Action remains available while this path is evaluated.

## Architecture

```text
GitHub private repository
        |
        | HTTPS polling (ETag / If-None-Match)
        v
issue-worker daemon on macOS
        |
        +-- repository mirror + per-job worktree
        +-- trusted setup commands
        +-- Codex CLI (workspace-write)
        +-- trusted verification commands
        +-- commit / push / Pull Request
        v
GitHub
```

There is no inbound connection to the Mac and no GitHub self-hosted runner registration.

## MVP scope

Implemented now:

- macOS-first CLI
- explicit repository allowlist
- 30 second polling by default
- conditional GET with ETag per repository
- one worker / one job at a time
- Fine-grained PAT authentication
- PAT lookup from macOS Keychain
- persistent Git mirrors and per-job worktrees
- unique branch per execution
- `codex:ready` -> `codex:running` -> `codex:review` / `codex:failed` / `codex:no-change`
- repository-local `.issue-worker.yml`
- setup before Codex
- verification after Codex
- GitHub credential excluded from the Codex environment
- wrapper-owned Git commit/push/PR operations
- optional monitoring of PR feedback and failed GitHub Actions runs
- persistent local PR monitoring cursors and retry state

Not implemented in this MVP:

- multiple workers competing for the same job
- robust distributed claim/locking
- parallel jobs
- GitHub App login
- launchd service installation
- cancellation labels
- automatic restart recovery
- Windows/Linux product support

Those are intentionally deferred until the simple path has been exercised end-to-end.

## Build the prototype

Until binaries/Homebrew releases exist, build from source:

```bash
git clone https://github.com/qmore/issue-worker.git
cd issue-worker
git switch feat/daemon-mvp
go build -o issue-worker ./cmd/issue-worker
```

Required runtime tools:

```text
git
codex
```

`gh` and the GitHub Actions runner are not required by the daemon.

## GitHub credential

Create a Fine-grained PAT limited to the target repositories.

Recommended repository permissions for the MVP:

```text
Contents       Read and write
Issues         Read and write
Pull requests  Read and write
Actions        Read-only (when PR monitoring is enabled)
Metadata       Read-only (automatic)
```

No Organization Administration or self-hosted runner administration permission is required.

Store the PAT in macOS Keychain:

```bash
./issue-worker login
```

The token is read without terminal echo and stored as:

```text
service: issue-worker
account: github
```

For automation/testing, an environment variable takes precedence:

```bash
ISSUE_WORKER_GITHUB_TOKEN=... ./issue-worker doctor
```

Do not put the token in `config.yml`, `.issue-worker.yml`, Issues, or repository files.

## Initialize

```bash
./issue-worker init
```

On macOS this creates:

```text
~/Library/Application Support/issue-worker/config.yml
```

Edit the explicit allowlist:

```yaml
version: 1

worker:
  id: mac-mini
  poll_interval: 30s
  concurrency: 1

repositories:
  - owner/private-repo

pull_requests:
  monitor: true
  command: /issue-worker
  max_fix_attempts: 3
```

The default workspace is:

```text
~/Library/Application Support/issue-worker/data/
```

## Codex authentication

Codex authentication is separate from issue-worker:

```bash
codex login
```

The worker does not manage or copy the Codex credential.

## Codex task visibility

The default `codex.backend: exec` preserves terminal-only ephemeral execution.
Set `codex.backend: app-server` to create one durable, named Codex task per Issue.
The task streams the full Codex turn and its title tracks the wrapper stage. The
worker starts the stdio app-server unless `app_server_socket` selects an existing
shared daemon. `codex.timeout` bounds the turn, and unfinished work is explicitly
interrupted before wrapper-owned Git operations continue.

## Diagnose before running

```bash
./issue-worker doctor
```

It checks:

- `git` is installed
- `codex` is installed
- a GitHub credential is available
- configured repositories are accessible
- workspace is writable
- configuration is valid

## Repository-local configuration

Optional `.issue-worker.yml`:

```yaml
base_branch: ""

setup:
  - npm ci

verify:
  - npm test
  - npm run build
```

Important security property: `setup` and `verify` are read and copied into worker memory **before Codex starts**. If Codex edits `.issue-worker.yml`, the current job does not re-read the modified commands.

`setup` runs before Codex. `verify` runs after Codex and before commit/push/PR.

Both are trusted host-side commands and run outside the Codex sandbox. Use them only for repositories whose project scripts you are willing to execute as the dedicated worker OS user.

## Start in foreground

```bash
./issue-worker run
```

For a single diagnostic poll:

```bash
./issue-worker poll
```

The initial product prototype deliberately runs in the foreground. launchd/service management is deferred until the worker flow itself is validated.

## Submit a job

Create an Issue in an allowlisted repository and add:

```text
codex:ready
```

The daemon polls:

```text
GET /repos/{owner}/{repo}/issues
    ?state=open
    &labels=codex:ready
```

It caches the response ETag and sends `If-None-Match` on later polls. Unchanged repositories normally return `304 Not Modified`.

When a job is found:

```text
codex:ready
    -> codex:running
    -> setup
    -> Codex
    -> verify
    -> commit / push
    -> Pull Request
    -> codex:review
```

Failure becomes `codex:failed`. A successful run with no repository changes becomes `codex:no-change`.

## Pull Request follow-up

When `pull_requests.monitor` is enabled, PR discovery runs in the same polling
loop as Issue discovery. PRs created on `issue-worker/*` branches are watched
automatically. A trusted repository `OWNER`, `MEMBER`, or `COLLABORATOR` can opt
another open same-repository PR in with a conversation or inline review comment:

```text
/issue-worker watch
/issue-worker fix make the requested change
/issue-worker stop
```

The configured command defaults to `/issue-worker`; `@issue-worker` is also
accepted. A bare command means `watch`. `fix` both enables monitoring and submits
an immediate request. `stop` remains in persistent state so automatic discovery
does not re-enable that PR.

For a watched PR, the daemon reads new trusted conversation comments, inline
review comments, `CHANGES_REQUESTED`/commented review bodies, and the latest run
for each GitHub Actions workflow on the current head SHA. A new actionable event
creates a Codex follow-up task in the existing PR worktree (or a new detached
worktree after restart), runs the frozen repository-local `setup` and `verify`
commands sourced from the PR base branch, and pushes a wrapper-owned commit to
the existing head branch. The PR head's `.issue-worker.yml` is untrusted and is
never used as the source of host-side commands.

Only open PRs whose head repository exactly matches the configured repository
are eligible. Fork PRs are rejected. Bot content and commands from other author
associations are ignored. The daemon never approves, merges, or deploys a PR.
`max_fix_attempts` bounds retries for one unchanged event; the default is three.

CI polling uses `GET /repos/{owner}/{repo}/actions/runs` and therefore needs
Fine-grained PAT `Actions: Read-only`. It does not use the Checks API. If that
endpoint is unavailable, the daemon logs the condition and continues monitoring
comments and reviews.

## Workspace layout

Conceptually:

```text
data/
├── repos/
│   └── owner_repo.git       # mirror
├── pr-monitor-state.json    # watched PRs, event cursors, retry state
└── jobs/
    └── owner_repo/
        └── 123-<run-id>/    # worktree
```

Each execution receives a new branch:

```text
issue-worker/<issue>-<run-id>
```

This intentionally avoids reusing stale implementation branches.

Failed worktrees are retained for local diagnosis in the MVP.

## Security notes

The daemon is designed for trusted private repositories.

- A job is started only from the explicit `codex:ready` label.
- PR follow-ups require an issue-worker-owned branch or a trusted explicit command.
- PR follow-up rejects forks and writes only to same-repository head branches.
- The repository must be explicitly present in the local allowlist.
- GitHub authentication is used by the wrapper, not Codex.
- The Codex process receives a minimized environment and no GitHub PAT.
- Codex runs with `workspace-write` and non-interactive approvals.
- Codex is told not to commit, push, or operate GitHub.
- Git commit hooks are disabled for the wrapper-owned automated commit.
- The generated PR remains a human review boundary.

The MVP treats the ability to apply the ready label as the approval boundary. Do not expose this flow to repositories where untrusted users can apply that label.

## Polling load

Default:

```text
30s
```

The daemon stores one Issue-list ETag per repository in memory. The normal idle Issue sequence is therefore:

```text
poll -> 304 -> sleep -> poll -> 304
```

ETags are intentionally part of the MVP rather than a later optimization.

## Current claim semantics

The MVP supports one daemon and `concurrency: 1` only.

Claim currently means:

1. add `codex:running`
2. remove `codex:ready`
3. re-read the Issue
4. verify `running` is present and `ready` is absent

This is not a distributed atomic lock. Do not point multiple worker daemons at the same repository yet. A stronger GitHub-side claim primitive is a later multi-worker feature.
