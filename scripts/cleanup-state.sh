#!/usr/bin/env bash
set -Eeuo pipefail

TOKEN="${ISSUE_WORKER_GITHUB_TOKEN:-}"
unset ISSUE_WORKER_GITHUB_TOKEN

ISSUE_NUMBER="${ISSUE_WORKER_ISSUE_NUMBER:-}"
JOB_RESULT="${ISSUE_WORKER_JOB_RESULT:-}"
WORKING_LABEL="${ISSUE_WORKER_WORKING_LABEL:-codex:working}"
REVIEW_LABEL="${ISSUE_WORKER_REVIEW_LABEL:-codex:review}"
FAILED_LABEL="${ISSUE_WORKER_FAILED_LABEL:-codex:failed}"
NO_CHANGE_LABEL="${ISSUE_WORKER_NO_CHANGE_LABEL:-codex:no-change}"
REPOSITORY="${GITHUB_REPOSITORY:-}"
RUN_ID="${GITHUB_RUN_ID:-local}"
SERVER_URL="${GITHUB_SERVER_URL:-https://github.com}"
RUN_URL="${SERVER_URL}/${REPOSITORY}/actions/runs/${RUN_ID}"

[[ -n "$TOKEN" ]] || { printf '[issue-worker cleanup] github-token is required.\n' >&2; exit 1; }
[[ -n "$ISSUE_NUMBER" ]] || { printf '[issue-worker cleanup] issue-number is required.\n' >&2; exit 1; }
[[ -n "$REPOSITORY" ]] || { printf '[issue-worker cleanup] GITHUB_REPOSITORY is not set.\n' >&2; exit 1; }

LABELS="$(GH_TOKEN="$TOKEN" gh issue view "$ISSUE_NUMBER" --repo "$REPOSITORY" --json labels --jq '.labels[].name')"

has_label() {
  local wanted="$1"
  printf '%s\n' "$LABELS" | grep -Fx -- "$wanted" >/dev/null 2>&1
}

# A terminal state always wins. This prevents cleanup from damaging a completed run
# if a later metadata/annotation step fails after the PR was already created.
if has_label "$REVIEW_LABEL" || has_label "$NO_CHANGE_LABEL"; then
  printf '[issue-worker cleanup] Terminal state already present; nothing to recover.\n'
  exit 0
fi

case "$JOB_RESULT" in
  failure|cancelled)
    ;;
  *)
    printf '[issue-worker cleanup] Job result is %s; no abnormal-state recovery required.\n' "${JOB_RESULT:-unknown}"
    exit 0
    ;;
esac

if ! has_label "$WORKING_LABEL"; then
  printf '[issue-worker cleanup] No stale working label found.\n'
  exit 0
fi

GH_TOKEN="$TOKEN" gh label create "$FAILED_LABEL" \
  --repo "$REPOSITORY" --color d73a4a --description "issue-worker failed" --force \
  >/dev/null 2>&1 || true

GH_TOKEN="$TOKEN" gh issue edit "$ISSUE_NUMBER" --repo "$REPOSITORY" \
  --remove-label "$WORKING_LABEL" >/dev/null
GH_TOKEN="$TOKEN" gh issue edit "$ISSUE_NUMBER" --repo "$REPOSITORY" \
  --add-label "$FAILED_LABEL" >/dev/null
GH_TOKEN="$TOKEN" gh issue comment "$ISSUE_NUMBER" --repo "$REPOSITORY" \
  --body "issue-worker recovered a stale \`${WORKING_LABEL}\` state after the implementation job ended as \`${JOB_RESULT}\`. Review the Actions run: ${RUN_URL}" \
  >/dev/null

printf '[issue-worker cleanup] Recovered stale working state as failed.\n'
