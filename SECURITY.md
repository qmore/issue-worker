# Security Policy

`issue-worker` executes an AI coding agent on self-hosted infrastructure. Please read the full security model before deployment:

- [Security model](docs/SECURITY.md)
- [Setup guide](docs/SETUP.md)
- [LLM setup guide](docs/LLM_SETUP.md)

## Supported versions

The project is currently experimental / pre-1.0. Security fixes are applied to the latest development/release line only.

## Reporting a vulnerability

Do **not** include credentials, access tokens, private repository contents, Codex authentication files, SSH keys, or other secrets in a public GitHub Issue.

Before broad public release, repository maintainers should enable GitHub Private Vulnerability Reporting and update this section with the private reporting path.

If you suspect credentials were exposed through a runner job, stop the runner and rotate/revoke affected credentials immediately before investigating further.
