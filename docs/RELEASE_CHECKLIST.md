# Release checklist

Before publishing a version tag:

- CI is green on `main`
- `go test ./...` passes
- `go vet ./...` passes
- release workflow YAML parses
- installer shell syntax passes
- release version is a `v*` tag

After publishing:

- GitHub Release contains all four platform archives plus `SHA256SUMS`
- `issue-worker version` matches the tag
- the clean installer succeeds on an Apple Silicon Mac without Go installed
- `issue-worker doctor` reaches the expected prerequisite checks
