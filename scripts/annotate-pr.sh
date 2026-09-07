#!/usr/bin/env bash
set -Eeuo pipefail

TOKEN="${ISSUE_WORKER_GITHUB_TOKEN:-}"
unset ISSUE_WORKER_GITHUB_TOKEN

PR_URL="${ISSUE_WORKER_PR_URL:-}"
VERIFY_COMMAND="${ISSUE_WORKER_VERIFY_COMMAND:-}"
VERIFY_RESULT="${ISSUE_WORKER_VERIFICATION_RESULT:-skipped}"
REPOSITORY="${GITHUB_REPOSITORY:-}"
RUN_ID="${GITHUB_RUN_ID:-local}"
SERVER_URL="${GITHUB_SERVER_URL:-https://github.com}"
RUN_URL="${SERVER_URL}/${REPOSITORY}/actions/runs/${RUN_ID}"

[[ -n "$TOKEN" ]] || { printf '[issue-worker] github-token is required to annotate PR.\n' >&2; exit 1; }
[[ -n "$PR_URL" ]] || exit 0

BODY_FILE="$(mktemp "${RUNNER_TEMP:-/tmp}/issue-worker-pr-annotation.XXXXXX")"
trap 'rm -f "$BODY_FILE"' EXIT

GH_TOKEN="$TOKEN" gh pr view "$PR_URL" --json body --jq '.body // ""' > "$BODY_FILE"

if grep -q '<!-- issue-worker-verification -->' "$BODY_FILE"; then
  printf '[issue-worker] Verification metadata already present on PR.\n'
  exit 0
fi

{
  printf '\n\n<!-- issue-worker-verification -->\n'
  printf '## Verification\n\n'

  case "$VERIFY_RESULT" in
    passed)
      printf -- '- ✅ Result: **Passed**\n'
      ;;
    skipped)
      printf -- '- ⏭️ Result: **Skipped**\n'
      ;;
    *)
      printf -- '- ℹ️ Result: **%s**\n' "$VERIFY_RESULT"
      ;;
  esac

  if [[ -n "$VERIFY_COMMAND" ]]; then
    printf -- '- Command:\n\n'
    while IFS= read -r line || [[ -n "$line" ]]; do
      printf '    %s\n' "$line"
    done <<< "$VERIFY_COMMAND"
    printf '\n'
  fi

  printf -- '- Actions run: %s\n' "$RUN_URL"
} >> "$BODY_FILE"

GH_TOKEN="$TOKEN" gh pr edit "$PR_URL" --body-file "$BODY_FILE" >/dev/null
printf '[issue-worker] Added verification metadata to pull request.\n'
