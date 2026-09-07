#!/usr/bin/env bash
set -Eeuo pipefail

PREFIX="${ISSUE_WORKER_PREFIX:-$HOME/.local}"
BINDIR="$PREFIX/bin"
TARGET="$BINDIR/issue-worker"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

command -v go >/dev/null 2>&1 || {
  printf 'error: Go is required to build the prototype.\n' >&2
  exit 1
}

mkdir -p "$BINDIR"

printf 'Building issue-worker...\n'
(
  cd "$ROOT"
  go build -trimpath -o "$TARGET" ./cmd/issue-worker
)

printf 'Installed: %s\n' "$TARGET"

case ":${PATH:-}:" in
  *":$BINDIR:"*) ;;
  *)
    printf '\n%s is not currently in PATH.\n' "$BINDIR"
    printf 'For zsh, add this to ~/.zshrc:\n\n'
    printf '  export PATH="%s:$PATH"\n\n' "$BINDIR"
    ;;
esac

printf 'Next:\n'
printf '  issue-worker init\n'
printf '  # edit the generated repository allowlist\n'
printf '  issue-worker login\n'
printf '  codex login\n'
printf '  issue-worker doctor\n'
printf '  issue-worker run\n'
