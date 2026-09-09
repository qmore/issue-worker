# LLM setup guide — standalone daemon

This guide is for an LLM/coding agent asked to configure the experimental standalone `qmore/issue-worker` daemon on a trusted macOS machine.

The goal is to achieve:

```text
GitHub Issue + codex:ready
        |
        | outbound HTTPS polling
        v
local issue-worker daemon
        |
        v
Codex CLI -> verify -> branch -> PR
```

Do **not** register a GitHub self-hosted runner for this mode.
Do **not** install Go on the worker machine just to run issue-worker.

## Safety invariants

The setup agent must follow all of these rules:

1. Never ask the user to paste a GitHub PAT, Codex credential, API key, SSH private key, password, or macOS Keychain content into chat.
2. Never commit credentials or put them in Issues, repository files, logs, prompts, or documentation.
3. Use a trusted **private** target repository for the daemon MVP.
4. Keep the repository list explicit; do not silently broaden access to an entire Organization.
5. Do not enable Codex network access unless the user/project explicitly requires it.
6. Do not use Codex sandbox-bypass flags.
7. Do not configure multiple daemons against the same repository in the MVP; distributed locking is not implemented yet.
8. Do not install a launchd service unless service support is explicitly present in the current issue-worker version. The MVP runs in the foreground.
9. Prefer the prebuilt release binary. Do not install a Go toolchain solely to build issue-worker on the worker machine.

## 1. Check the machine

Expected target:

```text
macOS
trusted local worker account
outbound HTTPS access to GitHub/OpenAI
no inbound port required
```

Check:

```bash
uname -a
git --version
codex --version
curl --version
```

`issue-worker` itself has no runtime dependency on Go, Python, Node.js, Docker, or GitHub CLI.

## 2. Install issue-worker

Use the prebuilt GitHub Release binary:

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh | sh
```

The installer detects the local platform, downloads the matching release archive and `SHA256SUMS`, verifies the archive, and installs only the `issue-worker` binary to `~/.local/bin` by default.

Do not replace this with a source build unless the user explicitly wants a development environment.

If `~/.local/bin` is not on PATH, follow the installer's printed PATH instruction.

Verify:

```bash
issue-worker version
```

A specific release can be selected without installing Go:

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh \
  | ISSUE_WORKER_VERSION=v0.1.0 sh
```

## 3. Initialize local configuration

Run:

```bash
issue-worker init
```

On macOS the default file is:

```text
~/Library/Application Support/issue-worker/config.yml
```

Open it locally and replace the placeholder repository with the exact private repository/repositories authorized for this worker.

Example:

```yaml
version: 1

worker:
  id: mac-mini
  poll_interval: 30s
  concurrency: 1

repositories:
  - owner/private-repo
```

Do not add an Organization wildcard. The MVP uses an explicit allowlist.

## 4. Configure GitHub authentication

The MVP supports a Fine-grained Personal Access Token.

Guide the user in GitHub UI to create a token restricted to the selected private repositories, with repository permissions approximately:

```text
Contents       Read and write
Issues         Read and write
Pull requests  Read and write
Actions        Read-only (when PR monitoring is enabled)
Metadata       Read-only (automatic)
```

Do not request Organization Administration or self-hosted runner Administration permissions.

Do not ask the user to send the token to the LLM.

The daemon uses the Actions Runs API for CI failure monitoring. Do not ask for a
Checks permission when it is unavailable in the Fine-grained PAT UI. If PR
monitoring is disabled, `Actions: Read-only` is not needed.

Have the user enter it directly in the terminal:

```bash
issue-worker login
```

The input is hidden and stored in macOS Keychain as the issue-worker GitHub credential.

## 5. Configure Codex separately

Codex authentication is intentionally separate from GitHub authentication.

Run under the same macOS account that will run issue-worker:

```bash
codex login
```

Never inspect, copy, print, or relocate Codex credential files.

To expose issue-worker jobs in Codex clients, enable the durable app-server
backend:

```yaml
codex:
  backend: app-server
  app_server_socket: ""
  timeout: 30m
  allow_network: false
```

Each Issue gets a named task containing the Codex turn, command activity, and
file changes. The title shows the current wrapper stage. An empty socket uses the
stdio app-server transport and normal Codex history. Configure a socket only
when the machine already has a shared app-server daemon.

With repository `verify` commands configured, Codex is told that host
verification remains pending after its sandboxed turn. issue-worker appends the
authoritative pass/fail result to the same task after running the frozen commands;
it does not grant the Codex turn access to host-only sockets such as Docker.

## 6. Run diagnostics

Run:

```bash
issue-worker doctor
```

Expected checks include:

```text
[OK] git installed
[OK] codex installed
[OK] GitHub credential available
[OK] repo access: owner/private-repo
[OK] workspace writable
[OK] daemon configuration
```

If a repository is public, the daemon MVP should reject it.

If a check fails, fix that specific prerequisite rather than weakening sandbox/security settings.

## 7. Optional repository configuration

If the target project needs dependency preparation or worker-side validation, add `.issue-worker.yml` to the target repository.

Example:

```yaml
base_branch: ""

setup:
  - npm ci

verify:
  - npm test
  - npm run build
```

Security behavior:

- the daemon reads and freezes `setup`/`verify` before Codex runs
- Codex cannot change the commands used by the current job by editing `.issue-worker.yml`
- these commands execute outside the Codex sandbox as the worker OS user

Only configure commands appropriate for a trusted private repository.

To follow up on review feedback and failed GitHub Actions runs, enable:

```yaml
pull_requests:
  monitor: true
  command: /issue-worker
  max_fix_attempts: 3
```

The daemon then watches its own `issue-worker/*` PRs. Trusted repository owners,
members, and collaborators can opt another open same-repository PR in with
`/issue-worker watch`, request an immediate change with `/issue-worker fix ...`,
or pause it with `/issue-worker stop`. `@issue-worker` is an alias. Never enable
this for untrusted public PR authors, and do not broaden it to fork PRs. During
follow-up, host-side `setup` and `verify` commands are sourced from the PR base
branch; never substitute the PR head's `.issue-worker.yml`.

The host verification result is also posted to the PR conversation. The daemon
marks and ignores its own status comments so they do not recursively trigger
another follow-up.

## 8. Test polling without a job

Run one poll:

```bash
issue-worker poll
```

A successful no-job poll should exit cleanly.

The daemon uses conditional GitHub requests with ETag/`If-None-Match`; repeated idle polls should normally become `304 Not Modified` responses internally.

## 9. Start the worker in foreground

Run:

```bash
issue-worker run
```

Leave it running while performing the smoke test.

Do not create a GitHub Actions workflow for daemon mode.
Do not register a self-hosted runner.
Do not open an inbound port.

## 10. End-to-end smoke test

In one allowlisted private repository, create a harmless Issue such as:

```text
Title: issue-worker daemon smoke test

Create ISSUE_WORKER_SMOKE_TEST.md containing one sentence that says the daemon is working. Do not modify unrelated files.
```

Apply:

```text
codex:ready
```

Expected transition:

```text
codex:ready
  -> codex:running
  -> Codex implementation
  -> verification if configured
  -> Pull Request
  -> codex:review
```

Expected Git branch shape:

```text
issue-worker/<issue-number>-<run-id>
```

If Codex makes no changes, expect `codex:no-change`.
If the worker fails after claiming the Issue, expect `codex:failed` and inspect the local worker log/terminal output.

When PR monitoring is enabled, add a harmless trusted comment to the generated
PR and confirm that a follow-up Codex task runs, verification passes, and a new
commit is pushed to the same PR. `/issue-worker stop` should prevent later
comments from triggering work until `/issue-worker watch` is posted.

## 11. Important MVP limits

Do not claim features that are not implemented yet:

- no GitHub App login yet
- no Homebrew package yet
- no launchd service installer yet
- no multi-worker distributed lock yet
- no parallel jobs yet
- no cancel label yet
- no robust restart recovery yet

PR monitor cursors survive daemon restart, but Issue job execution itself still
does not have robust restart recovery.

The MVP's job claim is designed only for one daemon with `concurrency: 1`.

## 12. Release/development boundary

Worker machines should use prebuilt binaries.

The Go toolchain belongs only on development/CI machines. Version tags (`v*`) trigger `.github/workflows/release.yml`, which cross-compiles release archives, creates checksums, verifies them, and publishes a GitHub Release.

Do not install Go on a clean worker Mac merely because the repository itself is written in Go.

## 13. Success criteria

Setup is complete when all of the following are true:

```text
issue-worker runs locally from a prebuilt binary
Go is not required on the worker
configured private repo is accessible
no self-hosted runner is registered
no inbound port is open
GitHub PAT remains outside repo/config/prompt
Codex auth remains separate
codex:ready is detected
Codex edits a per-job worktree
verification passes (if configured)
branch is pushed
PR is created
Issue reaches codex:review
trusted PR feedback can update the same PR when monitoring is enabled
```
