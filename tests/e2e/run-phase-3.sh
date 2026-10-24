#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
if [[ -n "$(git -C "$ROOT" status --porcelain=v1)" ]]; then
  printf 'technical E2E requires a clean product worktree\n' >&2
  exit 1
fi

RUN_ID="${1:-$(date -u +%Y%m%dT%H%M%SZ)-technical-e2e}"
if [[ ! "$RUN_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ || "$RUN_ID" == *..* ]]; then
  printf 'invalid run id: %s\n' "$RUN_ID" >&2
  exit 1
fi
EVIDENCE_ROOT="$ROOT/.artifacts/phase-3/$RUN_ID"
umask 077
if [[ -e "$EVIDENCE_ROOT" ]]; then
  printf 'evidence root already exists: %s\n' "$EVIDENCE_ROOT" >&2
  exit 1
fi
mkdir -p "$EVIDENCE_ROOT"
VERIFICATION="$EVIDENCE_ROOT/verification.tsv"
printf 'name\texit_code\tduration_ms\n' >"$VERIFICATION"
REVISION="$(git -C "$ROOT" rev-parse HEAD)"

assert_fixed_point() {
  if [[ "$(git -C "$ROOT" rev-parse HEAD)" != "$REVISION" || -n "$(git -C "$ROOT" status --porcelain=v1)" ]]; then
    printf 'product worktree changed during the E2E run\n' >&2
    exit 1
  fi
}

run_recorded() {
  local name="$1"
  local log="$2"
  shift 2
  local started ended status
  started="$(date +%s%N)"
  set +e
  "$@" >"$EVIDENCE_ROOT/$log" 2>&1
  status=$?
  set -e
  ended="$(date +%s%N)"
  printf '%s\t%d\t%d\n' "$name" "$status" "$(((ended - started) / 1000000))" >>"$VERIFICATION"
  if [[ "$status" -ne 0 ]]; then
    printf '%s failed; inspect %s/%s\n' "$name" "$EVIDENCE_ROOT" "$log" >&2
    exit "$status"
  fi
}

run_recorded \
  technical-e2e focused-e2e.log \
  timeout 4m env GOTOOLCHAIN=go1.25.12 GOPROXY=off GPM_PHASE3_E2E_EVIDENCE_ROOT="$EVIDENCE_ROOT" \
  go -C "$ROOT" test ./tests/e2e -run '^TestManagedWorkflowEndToEnd$' -count=1 -v

assert_fixed_point
run_recorded verify verify.log timeout 4m make -C "$ROOT" verify
assert_fixed_point
run_recorded verify-offline verify-offline.log timeout 4m make -C "$ROOT" verify-offline
assert_fixed_point

CHECKSUMS="$EVIDENCE_ROOT/checksums.sha256"
: >"$CHECKSUMS"
for file in "$EVIDENCE_ROOT"/*; do
  if [[ "$file" == "$CHECKSUMS" ]]; then
    continue
  fi
  digest="$(sha256sum "$file")"
  printf '%s  %s\n' "${digest%% *}" "$(basename "$file")" >>"$CHECKSUMS"
done

printf '%s\n' "$EVIDENCE_ROOT"
