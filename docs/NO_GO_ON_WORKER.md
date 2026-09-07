# No Go toolchain on worker hosts

Normal issue-worker worker hosts install a versioned release binary. The Go toolchain is intentionally excluded from worker prerequisites and remains confined to development and GitHub Actions release jobs.
