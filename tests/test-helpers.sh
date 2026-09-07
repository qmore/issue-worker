#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/issue-worker-tests.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local text="$2"
  grep -F -- "$text" "$file" >/dev/null 2>&1 || fail "Expected '$text' in $file"
}

# verification: skipped, passed and failed metadata.
OUT="$TMP/verify.out"
: > "$OUT"
GITHUB_OUTPUT="$OUT" ISSUE_WORKER_USER_VERIFY_COMMAND='' bash "$ROOT/scripts/run-verification.sh"
assert_contains "$OUT" 'verification-result=skipped'

: > "$OUT"
GITHUB_OUTPUT="$OUT" ISSUE_WORKER_USER_VERIFY_COMMAND='true' bash "$ROOT/scripts/run-verification.sh"
assert_contains "$OUT" 'verification-result=passed'

: > "$OUT"
if GITHUB_OUTPUT="$OUT" ISSUE_WORKER_USER_VERIFY_COMMAND='false' bash "$ROOT/scripts/run-verification.sh"; then
  fail "verification helper unexpectedly succeeded for false"
fi
assert_contains "$OUT" 'verification-result=failed'

# Mock gh for authorization, cleanup and PR annotation tests.
MOCKBIN="$TMP/bin"
mkdir -p "$MOCKBIN"
GH_LOG="$TMP/gh.log"
GH_BODY_CAPTURE="$TMP/pr-body.txt"
export GH_LOG GH_BODY_CAPTURE

cat > "$MOCKBIN/gh" <<'MOCK_GH'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$GH_LOG"

if [[ "$1" == 'api' ]]; then
  printf '%s\n' "${MOCK_CAN_PUSH:-true}"
  exit 0
fi

if [[ "$1 $2" == 'issue view' ]]; then
  printf '%s\n' "${MOCK_ISSUE_LABELS:-}"
  exit 0
fi

if [[ "$1 $2" == 'pr view' ]]; then
  printf '%s\n' 'Existing PR body.'
  exit 0
fi

if [[ "$1 $2" == 'pr edit' ]]; then
  previous=''
  for arg in "$@"; do
    if [[ "$previous" == '--body-file' ]]; then
      cp "$arg" "$GH_BODY_CAPTURE"
      break
    fi
    previous="$arg"
  done
  exit 0
fi

exit 0
MOCK_GH
chmod +x "$MOCKBIN/gh"

COMMON_ENV=(
  "PATH=$MOCKBIN:$PATH"
  "ISSUE_WORKER_GITHUB_TOKEN=dummy"
  "ISSUE_WORKER_ISSUE_NUMBER=123"
  "GITHUB_REPOSITORY=owner/repo"
  "GITHUB_ACTOR=tester"
  "GITHUB_RUN_ID=456"
)

# setup-command: authorized success and failure preserve child result.
: > "$GH_LOG"
env "${COMMON_ENV[@]}" \
  MOCK_CAN_PUSH='true' \
  ISSUE_WORKER_SETUP_COMMAND='true' \
  bash "$ROOT/scripts/run-setup.sh"
assert_contains "$GH_LOG" 'api repos/owner/repo/collaborators/tester/permission'

: > "$GH_LOG"
if env "${COMMON_ENV[@]}" \
  MOCK_CAN_PUSH='true' \
  ISSUE_WORKER_SETUP_COMMAND='false' \
  bash "$ROOT/scripts/run-setup.sh"; then
  fail "setup helper unexpectedly succeeded for false"
fi
assert_contains "$GH_LOG" '--add-label codex:failed'

# Unauthorized actors must be rejected before host-side setup executes.
SETUP_SENTINEL="$TMP/setup-ran"
rm -f "$SETUP_SENTINEL"
if env "${COMMON_ENV[@]}" \
  MOCK_CAN_PUSH='false' \
  SETUP_SENTINEL="$SETUP_SENTINEL" \
  ISSUE_WORKER_SETUP_COMMAND='touch "$SETUP_SENTINEL"' \
  bash "$ROOT/scripts/run-setup.sh"; then
  fail "unauthorized setup unexpectedly succeeded"
fi
[[ ! -e "$SETUP_SENTINEL" ]] || fail "unauthorized setup command executed"

# cleanup: failure + working => failed, with working removed.
: > "$GH_LOG"
env "${COMMON_ENV[@]}" \
  MOCK_ISSUE_LABELS='codex:working' \
  ISSUE_WORKER_JOB_RESULT='failure' \
  bash "$ROOT/scripts/cleanup-state.sh"
assert_contains "$GH_LOG" '--remove-label codex:working'
assert_contains "$GH_LOG" '--add-label codex:failed'

# cleanup: terminal review state wins even when the job result is failure.
: > "$GH_LOG"
env "${COMMON_ENV[@]}" \
  MOCK_ISSUE_LABELS='codex:review' \
  ISSUE_WORKER_JOB_RESULT='failure' \
  bash "$ROOT/scripts/cleanup-state.sh"
if grep -F -- '--remove-label codex:working' "$GH_LOG" >/dev/null 2>&1; then
  fail "cleanup modified a terminal review state"
fi

# PR annotation: append concise verification metadata, not command output.
: > "$GH_LOG"
rm -f "$GH_BODY_CAPTURE"
env \
  "PATH=$MOCKBIN:$PATH" \
  ISSUE_WORKER_GITHUB_TOKEN='dummy' \
  ISSUE_WORKER_PR_URL='https://github.com/owner/repo/pull/9' \
  ISSUE_WORKER_VERIFY_COMMAND=$'php artisan test\nnpm run build' \
  ISSUE_WORKER_VERIFICATION_RESULT='passed' \
  GITHUB_REPOSITORY='owner/repo' \
  GITHUB_RUN_ID='456' \
  bash "$ROOT/scripts/annotate-pr.sh"

[[ -f "$GH_BODY_CAPTURE" ]] || fail "annotation body was not captured"
assert_contains "$GH_BODY_CAPTURE" '## Verification'
assert_contains "$GH_BODY_CAPTURE" 'Result: **Passed**'
assert_contains "$GH_BODY_CAPTURE" 'php artisan test'
assert_contains "$GH_BODY_CAPTURE" 'npm run build'
assert_contains "$GH_BODY_CAPTURE" '/actions/runs/456'

printf 'All helper tests passed.\n'
