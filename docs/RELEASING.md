# Releasing issue-worker

Worker machines should install prebuilt binaries. Go is a development/CI dependency only.

## Publish a release

`VERSION` is the release source of truth.

To publish a new release, change `VERSION` in a normal pull request, for example:

```text
v0.1.1
```

When that change is merged to `main`, `.github/workflows/release.yml` runs automatically. No maintainer needs to build locally or manually create a tag.

The release workflow:

1. validates the version string
2. refuses to overwrite an existing release
3. builds static binaries for Apple Silicon Mac, Intel Mac, Linux arm64, and Linux x86_64
4. embeds `VERSION` into `issue-worker version`
5. packages each binary with the license
6. generates `SHA256SUMS`
7. verifies the Linux x86_64 binary and all checksums
8. creates the Git tag and GitHub Release at the merged commit
9. uploads all archives and checksums

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
