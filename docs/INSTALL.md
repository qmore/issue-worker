# Installing issue-worker

The worker machine should stay minimal. A normal installation does not require Go, GitHub CLI, Docker, Python, or Node.js for issue-worker itself.

## Requirements

Runtime:

```text
git
codex
```

Installation uses standard system tools:

```text
curl
tar
shasum (macOS) or sha256sum (Linux)
```

## Install the latest release

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh | sh
```

By default the binary is installed to:

```text
~/.local/bin/issue-worker
```

No `sudo` is used.

## Install a specific version

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh \
  | ISSUE_WORKER_VERSION=v0.1.0 sh
```

## Custom install directory

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh \
  | ISSUE_WORKER_INSTALL_DIR="$HOME/bin" sh
```

## Verify

```bash
issue-worker version
```

Then continue with:

```bash
issue-worker init
issue-worker login
codex login
issue-worker doctor
issue-worker run
```

The GitHub credential is stored separately in macOS Keychain and is not embedded in the binary.
