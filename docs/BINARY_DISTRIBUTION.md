# Binary distribution contract

`issue-worker` worker machines consume release binaries; they do not build the project locally.

## Asset naming

The installer depends on these stable asset names:

```text
issue-worker_Darwin_arm64.tar.gz
issue-worker_Darwin_x86_64.tar.gz
issue-worker_Linux_arm64.tar.gz
issue-worker_Linux_x86_64.tar.gz
SHA256SUMS
```

Each archive contains:

```text
issue-worker
LICENSE
```

`SHA256SUMS` covers every release archive.

Changing these names is a compatibility change for `scripts/install.sh`.

## Version contract

Release builds inject the Git tag into the binary. For a `v0.1.0` release:

```bash
issue-worker version
```

must print:

```text
v0.1.0
```

Development/source builds print `dev` unless a version is explicitly injected with linker flags.
