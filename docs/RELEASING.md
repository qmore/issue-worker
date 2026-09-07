# Releasing issue-worker

Worker machines should install prebuilt binaries. Go is a development/CI dependency only.

## Publish a release

From an up-to-date `main` branch:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Tags matching `v*` trigger `.github/workflows/release.yml`.

The release workflow:

1. builds static binaries for Apple Silicon Mac, Intel Mac, Linux arm64, and Linux x86_64
2. embeds the tag into `issue-worker version`
3. packages each binary with the license
4. generates `SHA256SUMS`
5. verifies the Linux x86_64 binary and all checksums
6. publishes all archives and checksums to GitHub Releases

Published asset names are:

```text
issue-worker_Darwin_arm64.tar.gz
issue-worker_Darwin_x86_64.tar.gz
issue-worker_Linux_arm64.tar.gz
issue-worker_Linux_x86_64.tar.gz
SHA256SUMS
```

## Worker installation

Normal users should not clone the repository or install Go. They should use:

```bash
curl -fsSL https://raw.githubusercontent.com/qmore/issue-worker/main/scripts/install.sh | sh
```

The installer verifies the downloaded archive against `SHA256SUMS` before installing the binary to `~/.local/bin`.

## Source builds

`scripts/install-daemon.sh` remains available for development machines where Go is already intentionally installed. It is not the recommended worker installation path.
