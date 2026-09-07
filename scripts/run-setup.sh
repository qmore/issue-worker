#!/usr/bin/env bash
set -Eeuo pipefail

TOKEN="${ISSUE_WORKER_GITHUB_TOKEN:-}"
unset ISSUE_WORKER_GITHUB_TOKEN

SETUP_COMMAND="${ISSUE_WORKER_SETUP_COMMAND:-}"
ISSUE_NUMBER="${ISSUE_WORKER_ISSUE_NUMBER:-}"
TRIGGER_LABEL="${ISSUE_WORKER_TRIGGER_LABEL:-codex:run}"
WORKING_LABEL="${ISSUE_WORKER_WORKING_LABEL:-codex:working}"
REVIEW_LABEL="${ISSUE_WORKER_REVIEW_LABEL:-codex:review}"
FAILED_LABEL="${ISSUE_WORKER_FAILED_LABEL:-codex:failed}"
REPOSITORY="${GITHUB_REPOSITORY:-}"
ACTOR="${GITHUB_ACTOR:-}"
RUN_ID="${GITHUB_RUN_ID:-local}"
SERVER_URL="${GITHUB_SERVER_URL:-https://github.com}"
RUN_URL="${SERVER_URL}/${REPOSITORY}/actions/runs/${RUN_ID}"

if [[ -z "$SETUP_COMMAND" ]]; then
  exit 0
fi

mark_failed() {
  local message="$1"

  if [[ -n "$TOKEN" && -n "$REPOSITORY" && -n "$ISSUE_NUMBER" ]] && command -v gh >/dev/null 2>&1; then
    GH_TOKEN="$TOKEN" gh label create "$FAILED_LABEL" \
      --repo "$REPOSITORY" --color d73a4a --description "issue-worker failed" --force \
      >/dev/null 2>&1 || true

    GH_TOKEN="$TOKEN" gh issue edit "$ISSUE_NUMBER" --repo "$REPOSITORY" \
      --remove-label "$TRIGGER_LABEL" >/dev/null 2>&1 || true
    GH_TOKEN="$TOKEN" gh issue edit "$ISSUE_NUMBER" --repo "$REPOSITORY" \
      --remove-label "$WORKING_LABEL" >/dev/null 2>&1 || true
    GH_TOKEN="$TOKEN" gh issue edit "$ISSUE_NUMBER" --repo "$REPOSITORY" \
      --remove-label "$REVIEW_LABEL" >/dev/null 2>&1 || true
    GH_TOKEN="$TOKEN" gh issue edit "$ISSUE_NUMBER" --repo "$REPOSITORY" \
      --add-label "$FAILED_LABEL" >/dev/null 2>&1 || true
    GH_TOKEN="$TOKEN" gh issue comment "$ISSUE_NUMBER" --repo "$REPOSITORY" \
      --body "${message} Review the Actions log: ${RUN_URL}" \
      >/dev/null 2>&1 || true
  fi
}

[[ -n "$TOKEN" ]] || { printf '[issue-worker] github-token is required before setup.\n' >&2; exit 1; }
[[ -n "$REPOSITORY" ]] || { printf '[issue-worker] GITHUB_REPOSITORY is required before setup.\n' >&2; exit 1; }
[[ -n "$ISSUE_NUMBER" ]] || { printf '[issue-worker] issue-number is required before setup.\n' >&2; exit 1; }
[[ -n "$ACTOR" ]] || { printf '[issue-worker] GITHUB_ACTOR is required before setup.\n' >&2; exit 1; }
command -v gh >/dev/null 2>&1 || { printf '[issue-worker] gh is required before setup.\n' >&2; exit 1; }

# setup-command runs outside the Codex sandbox, so the existing trigger-actor
# authorization invariant must be enforced before any setup command executes.
CAN_PUSH=""
if ! CAN_PUSH="$(GH_TOKEN="$TOKEN" gh api "repos/${REPOSITORY}/collaborators/${ACTOR}/permission" \
  --jq '.user.permissions.push // false' 2>/dev/null)"; then
  printf '[issue-worker] Could not verify repository permission for actor %s before setup.\n' "$ACTOR" >&2
  mark_failed "issue-worker refused to run setup because trigger authorization could not be verified."
  exit 1
fi

if [[ "$CAN_PUSH" != "true" ]]; then
  printf '[issue-worker] Actor %s does not have write-level repository access; setup will not run.\n' "$ACTOR" >&2
  mark_failed "issue-worker refused to run setup because the triggering actor is not authorized."
  exit 1
fi

printf '[issue-worker] Running trusted setup command before Codex.\n'

status=0
bash -lc "$SETUP_COMMAND" || status=$?

if [[ "$status" -eq 0 ]]; then
  printf '[issue-worker] Setup command completed successfully.\n'
  exit 0
fi

printf '[issue-worker] Setup command failed with exit code %s.\n' "$status" >&2
mark_failed "issue-worker setup failed before Codex started."
exit "$status"
