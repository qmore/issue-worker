# Why prebuilt binaries

The standalone worker is intended to run on a dedicated machine with as little unrelated tooling as possible. Building issue-worker locally would force every worker to carry a Go toolchain that is not needed at runtime.

Prebuilt release binaries keep the boundary simple:

```text
CI/development: Go -> build -> checksum -> GitHub Release
worker:          download -> verify -> run
```

This reduces worker setup, removes a compiler/runtime maintenance obligation from the execution host, and makes issue-worker itself easier to replace or upgrade atomically.
