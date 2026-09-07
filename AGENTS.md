# AGENTS.md

Repository instructions for coding agents working on `issue-worker` itself.

## Purpose

`issue-worker` is a reusable GitHub Composite Action that turns an explicitly authorized GitHub Issue into a local Codex implementation job on a caller-owned self-hosted runner.

## Architectural invariants

- The caller repository owns `runs-on`, event triggers, timeout, and GitHub permissions.
- This repository provides orchestration only; it must not assume ownership of the user's runner.
- Repository-level and Organization-level self-hosted runners must both remain supported.
- The target repository is expected to be private/trusted by default.
- The generated implementation must stop at a pull request. Do not add automatic merge as a default behavior.
- Codex must not receive the caller's GitHub token from this wrapper.
- `actions/checkout` must keep `persist-credentials: false` unless the credential-exposure model is deliberately redesigned.
- Do not add `--yolo`, `--dangerously-bypass-approvals-and-sandbox`, or `danger-full-access` as a default/shortcut.
- Keep unattended Codex execution finite and non-interactive.

## Portability

The core script must work with:

- macOS system Bash 3.2
- current common Linux Bash versions

Avoid Bash features introduced after 3.2 in `scripts/issue-worker.sh`, including lowercase/uppercase parameter expansion such as `${var,,}` and associative arrays.

The action should not require executable file mode on repository scripts; invoke scripts through `bash` from `action.yml`.

## Dependencies on the self-hosted runner

Runtime dependencies are intentionally small:

- `git`
- `gh`
- `codex`
- Bash
- standard POSIX utilities

Do not silently install packages from the Action. Environment provisioning belongs in setup documentation / runner administration.

## Security review checklist for changes

Before changing token, Git, prompt, verification, or runner behavior, check:

1. Can Issue content cause a secret to be exposed?
2. Does Codex inherit a GitHub token or other wrapper secret?
3. Can generated repository content execute outside the Codex sandbox?
4. Can Git hooks execute generated code during wrapper Git operations?
5. Does the change broaden `GITHUB_TOKEN` permissions?
6. Does it weaken actor authorization?
7. Does it make public-repository self-hosted execution easier by default?
8. Does it introduce a path to automatic merge/deploy without human review?

Update `docs/SECURITY.md` when the trust model changes.

## Codex invocation

For new automation, follow current OpenAI guidance. The intended baseline is:

```text
codex exec
--sandbox workspace-write
--ask-for-approval never
--ephemeral
```

The wrapper, not Codex, performs branch/commit/push/PR operations.

## Documentation

When behavior changes, keep these synchronized:

- `README.md`
- `docs/SETUP.md`
- `docs/LLM_SETUP.md`
- `docs/CONFIGURATION.md`
- `docs/SECURITY.md`
- `examples/issue-worker.yml`

`docs/LLM_SETUP.md` must remain deterministic and must never tell an LLM to ask users to paste long-lived credentials into chat.

## Validation

At minimum run:

```bash
bash -n scripts/issue-worker.sh
git diff --check
```

For macOS-specific changes, review syntax against Bash 3.2 constraints.

End-to-end behavior requires a private test repository, a self-hosted runner, working Codex authentication, and a harmless smoke-test Issue.
