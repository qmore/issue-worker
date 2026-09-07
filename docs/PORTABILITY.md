# Binary portability

Release builds set `CGO_ENABLED=0` and cross-compile the standalone Go binary in GitHub Actions. This keeps the issue-worker executable independent of a local Go installation and avoids a C runtime/toolchain dependency introduced by cgo.

Target project setup/verify commands may still require their own project-specific runtimes or SDKs; those are outside the issue-worker binary contract.
