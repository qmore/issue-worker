#!/usr/bin/env bash
set -Eeuo pipefail

VERIFY_COMMAND="${ISSUE_WORKER_USER_VERIFY_COMMAND:-}"

if [[ -z "${GITHUB_OUTPUT:-}" ]]; then
  printf '[issue-worker] GITHUB_OUTPUT is not available for verification metadata.\n' >&2
  exit 1
fi

if [[ -z "$VERIFY_COMMAND" ]]; then
  printf 'verification-result=skipped\n' >> "$GITHUB_OUTPUT"
  exit 0
fi

printf '[issue-worker] Running trusted verification command.\n'

status=0
bash -lc "$VERIFY_COMMAND" || status=$?

if [[ "$status" -eq 0 ]]; then
  printf 'verification-result=passed\n' >> "$GITHUB_OUTPUT"
  printf '[issue-worker] Verification passed.\n'
  exit 0
fi

printf 'verification-result=failed\n' >> "$GITHUB_OUTPUT"
printf '[issue-worker] Verification failed with exit code %s.\n' "$status" >&2
exit "$status"
