# issue-worker

Turn a trusted GitHub Issue into a local Codex implementation job and return the result as a Pull Request.

> **Status:** experimental. The new standalone daemon is the preferred prototype path and is currently being validated. The existing GitHub Actions / self-hosted runner mode remains in this repository as an alternative implementation.

## Standalone daemon

The daemon reverses the original GitHub Actions model: **GitHub does not invoke your Mac. `issue-worker` polls GitHub and uses Issues as a lightweight job store.**

```text
GitHub private repository
        |
        | Issue + codex:ready
        | outbound HTTPS polling
        v
issue-worker daemon on your Mac
        |
        +-- trusted setup
        +-- Codex CLI
        +-- trusted verification
        +-- commit / push
        v
Pull Request + Issue status
```

No GitHub self-hosted runner registration is required. No inbound port, webhook endpoint, `gh` CLI, or repository Actions workflow is required for daemon mode.

### MVP scope

The current prototype intentionally stays small:

- macOS-first
- Go single binary
- explicit private-repository allowlist
- Fine-grained PAT stored in macOS Keychain
- 30-second polling by default
- ETag / `If-None-Match` conditional requests
- one daemon / one concurrent job
- persistent repository mirrors + per-job Git worktrees
- unique branch for every execution
- optional trusted setup before Codex
- Codex `workspace-write` sandbox with minimized environment
- optional trusted verification after Codex
- wrapper-owned commit, push, PR, labels, and Issue comments

Multi-worker distributed locking, GitHub App login, launchd installation, Homebrew packaging, cancellation, and restart recovery are intentionally deferred until the basic flow has been exercised end-to-end.

## Prototype installation

Until release binaries/Homebrew packaging exist:

```bash
git clone https://github.com/qmore/issue-worker.git
cd issue-worker
git switch feat/daemon-mvp
./scripts/install-daemon.sh
```

Runtime requirements:

```text
git
codex
```

Go is currently required only to build the prototype installer. It will not be a runtime dependency once release binaries are published.

### 1. Initialize

```bash
issue-worker init
```

On macOS this creates:

```text
~/Library/Application Support/issue-worker/config.yml
```

Edit the explicit repository allowlist:

```yaml
version: 1

worker:
  id: mac-mini
  poll_interval: 30s
  concurrency: 1

repositories:
  - owner/private-repository
```

See [`examples/daemon-config.yml`](examples/daemon-config.yml).

### 2. GitHub login

Create a Fine-grained PAT limited to the selected private repositories. Recommended repository permissions for the MVP:

```text
Contents       Read and write
Issues         Read and write
Pull requests  Read and write
Metadata       Read-only (automatic)
```

Then enter it directly in the local terminal:

```bash
issue-worker login
```

The input is hidden and stored in macOS Keychain. The token is not stored in `config.yml` and is not passed to Codex.

### 3. Codex login

Codex authentication stays separate:

```bash
codex login
```

`issue-worker` does not manage the Codex credential itself.

### 4. Diagnose

```bash
issue-worker doctor
```

This validates the local tools, GitHub credential, configured private repositories, workspace, and configuration.

### 5. Start

Foreground mode:

```bash
issue-worker run
```

Single diagnostic poll:

```bash
issue-worker poll
```

The prototype intentionally does not install a background service yet.

## Submit a job

Create an Issue in an allowlisted private repository and apply:

```text
codex:ready
```

Typical flow:

```text
codex:ready
   -> codex:running
   -> setup
   -> Codex
   -> verification
   -> branch / commit / push
   -> Pull Request
   -> codex:review
```

Failure/no-change states:

```text
codex:failed
codex:no-change
```

Each execution gets a new branch similar to:

```text
issue-worker/123-20260907T150000000000000Z
```

## Repository-local configuration

Projects can optionally commit `.issue-worker.yml`:

```yaml
base_branch: ""

setup:
  - npm ci

verify:
  - npm test
  - npm run build
```

See [`examples/repository.issue-worker.yml`](examples/repository.issue-worker.yml).

`setup` and `verify` are copied into worker memory **before Codex starts**. If Codex edits `.issue-worker.yml`, that does not change the host-side commands for the current job.

Both command groups run outside the Codex sandbox under the worker OS account, so use them only for trusted private repositories.

## Polling

Default interval:

```text
30s
```

For each repository, the daemon remembers the response ETag and sends `If-None-Match` on later requests. The normal idle path is therefore:

```text
poll -> 304 Not Modified -> sleep -> poll
```

ETag support is part of the MVP rather than a later optimization.

## Security model

The standalone MVP targets **trusted private repositories**.

Important invariants:

- only repositories explicitly listed in local configuration are considered
- public target repositories are rejected by the MVP
- the explicit `codex:ready` label is the approval boundary
- the GitHub credential belongs to the wrapper, not the Codex process
- the PAT is not put into repository files or prompts
- Codex receives a minimized environment
- Codex uses `workspace-write` and non-interactive approval mode
- Codex is told not to commit, push, create PRs, or operate GitHub
- automated commits disable Git hooks
- each execution uses a separate worktree and branch
- human Pull Request review remains the final boundary

Do not run multiple daemon instances against the same repository yet. The MVP claim mechanism is not a distributed atomic lock.

Detailed prototype design: [`docs/DAEMON_MVP.md`](docs/DAEMON_MVP.md)

LLM-oriented installation guide: [`docs/LLM_DAEMON_SETUP.md`](docs/LLM_DAEMON_SETUP.md)

## Existing GitHub Actions mode

The original Composite Action implementation is still available while the standalone daemon is validated. It uses:

```text
Issue -> GitHub Actions -> self-hosted runner -> Codex -> PR
```

It supports repository-level and Organization-level self-hosted runners, trusted `setup-command`, worker-side verification, and a separate cleanup Action for stale state recovery.

Existing-mode documentation:

- [`docs/SETUP.md`](docs/SETUP.md)
- [`docs/LLM_SETUP.md`](docs/LLM_SETUP.md)
- [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md)
- [`docs/SECURITY.md`](docs/SECURITY.md)
- [`examples/issue-worker.yml`](examples/issue-worker.yml)

The goal of the daemon experiment is specifically to remove the installation overhead of runner registration and caller workflows, not to delete the working Actions implementation before the new path is proven.

## Development

The daemon is written in Go.

```bash
go test ./...
go vet ./...
go build ./cmd/issue-worker
```

CI runs these checks on both macOS and Linux in addition to the existing Composite Action tests.

## License

MIT License. See [LICENSE](LICENSE).
