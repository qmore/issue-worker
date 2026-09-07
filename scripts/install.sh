#!/bin/sh
set -eu

REPO="${ISSUE_WORKER_REPO:-qmore/issue-worker}"
INSTALL_DIR="${ISSUE_WORKER_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${ISSUE_WORKER_VERSION:-latest}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'error: required command not found: %s\n' "$1" >&2
    exit 1
  }
}

need curl
need tar
need uname

os="$(uname -s)"
arch="$(uname -m)"

case "$os/$arch" in
  Darwin/arm64)  platform="Darwin_arm64" ;;
  Darwin/x86_64) platform="Darwin_x86_64" ;;
  Linux/aarch64|Linux/arm64) platform="Linux_arm64" ;;
  Linux/x86_64|Linux/amd64) platform="Linux_x86_64" ;;
  *)
    printf 'error: unsupported platform: %s/%s\n' "$os" "$arch" >&2
    exit 1
    ;;
esac

archive="issue-worker_${platform}.tar.gz"
if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/issue-worker-install.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

printf 'Downloading issue-worker (%s)...\n' "$platform"
curl -fL --retry 3 --retry-delay 1 \
  -o "$tmp/$archive" \
  "$base_url/$archive"
curl -fL --retry 3 --retry-delay 1 \
  -o "$tmp/SHA256SUMS" \
  "$base_url/SHA256SUMS"

expected="$(awk -v name="$archive" '$2 == name { print $1; exit }' "$tmp/SHA256SUMS")"
if [ -z "$expected" ]; then
  printf 'error: checksum entry missing for %s\n' "$archive" >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$archive" | awk '{print $1}')"
else
  printf 'error: neither shasum nor sha256sum is available\n' >&2
  exit 1
fi

if [ "$actual" != "$expected" ]; then
  printf 'error: checksum verification failed for %s\n' "$archive" >&2
  exit 1
fi

mkdir -p "$tmp/unpack"
tar -C "$tmp/unpack" -xzf "$tmp/$archive"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/unpack/issue-worker" "$INSTALL_DIR/issue-worker"

printf 'Installed: %s/issue-worker\n' "$INSTALL_DIR"
printf 'Version: '
"$INSTALL_DIR/issue-worker" version

case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf '\n%s is not currently in PATH.\n' "$INSTALL_DIR"
    printf 'For zsh, add this to ~/.zshrc:\n\n'
    printf '  export PATH="%s:$PATH"\n\n' "$INSTALL_DIR"
    ;;
esac

printf '\nNext:\n'
printf '  issue-worker init\n'
printf '  # edit the generated repository allowlist\n'
printf '  issue-worker login\n'
printf '  codex login\n'
printf '  issue-worker doctor\n'
printf '  issue-worker run\n'
