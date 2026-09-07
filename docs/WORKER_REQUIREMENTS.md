# Worker machine requirements

The standalone daemon is designed so the worker machine can remain a clean execution host.

Required for normal operation:

```text
macOS or supported Linux
Git
Codex CLI
issue-worker release binary
outbound HTTPS
```

Not required for normal operation:

```text
Go
GitHub CLI
GitHub Actions runner
Docker
Python
Node.js (unless the target repository itself needs it)
inbound ports
webhook receiver
```

Project-specific toolchains may still be required by `.issue-worker.yml` setup/verify commands or by the repository being edited. Those are project dependencies, not issue-worker runtime dependencies.
