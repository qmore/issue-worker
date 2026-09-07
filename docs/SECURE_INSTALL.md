# Secure installation notes

The release installer treats the GitHub Release archive as untrusted until its SHA-256 digest matches the corresponding entry in `SHA256SUMS` downloaded from the same release.

The installer:

- uses HTTPS GitHub endpoints
- fails on HTTP errors
- retries transient download failures
- compares the full SHA-256 digest before extraction
- installs without `sudo` by default
- writes the final binary only after successful verification

For stronger supply-chain guarantees in a future release, signed checksums or artifact attestations can be added without changing the worker runtime model.
