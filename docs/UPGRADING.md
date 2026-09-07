# Upgrading issue-worker

Re-run the release installer to replace the local binary with the latest published version:

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh | sh
```

The installer downloads to a temporary directory, verifies the release checksum, and only then replaces `~/.local/bin/issue-worker`.

To pin an upgrade/downgrade to a specific version:

```bash
ISSUE_WORKER_VERSION=v0.1.0 \
  curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh | sh
```

Configuration and macOS Keychain credentials are stored separately from the binary and are not overwritten by installation.
