#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
if [[ -n "$(git -C "$ROOT" status --porcelain=v1)" ]]; then
  printf 'T9 classificatory run requires a clean product worktree\n' >&2
  exit 1
fi

RUN_ID="${1:-$(date -u +%Y%m%dT%H%M%SZ)-synthetic-e2e}"
EVIDENCE_ROOT="$ROOT/.artifacts/phase-2/$RUN_ID"
umask 077
if [[ -e "$EVIDENCE_ROOT" ]]; then
  printf 'evidence root already exists: %s\n' "$EVIDENCE_ROOT" >&2
  exit 1
fi
mkdir -p "$EVIDENCE_ROOT"
VERIFICATION="$EVIDENCE_ROOT/verification.tsv"
printf 'name\texit_code\tduration_ms\n' >"$VERIFICATION"

scan_artifacts() {
  local marker
  local files
  shopt -s nullglob
  files=("$EVIDENCE_ROOT"/*)
  shopt -u nullglob
  while IFS='=' read -r _ marker; do
    if [[ -z "$marker" ]]; then
      continue
    fi
    if [[ "${#files[@]}" -gt 0 ]] && grep -a -l -F -- "$marker" "${files[@]}" >/dev/null; then
      printf 'synthetic privacy marker found in preserved evidence\n' >&2
      return 1
    fi
  done <"$ROOT/docs/phase-1/corpus/code-repo/private-markers.txt"
}

scan_on_failure() {
  local status=$?
  local scan_status=0
  trap - EXIT
  set +e
  scan_artifacts
  scan_status=$?
  printf 'artifact-marker-scan\t%d\t0\n' "$scan_status" >>"$VERIFICATION"
  if [[ "$status" -eq 0 && "$scan_status" -ne 0 ]]; then
    status="$scan_status"
  fi
  exit "$status"
}

trap scan_on_failure EXIT

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
  synthetic-e2e focused-e2e.log \
  timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off GPM_E2E_EVIDENCE_ROOT="$EVIDENCE_ROOT" \
  go -C "$ROOT" test ./tests/e2e -run TestSyntheticWorkflow -count=1 -v

run_recorded \
  fault-injection fault-injection.log \
  timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off \
  go -C "$ROOT" test ./internal/proposal ./internal/publisher \
  -run 'Recovers|Retry|SyncFails|Interrupted|Recovery|Interruption|Reuses|ObservesEdits|ConcurrentBranch|IndexLock|Temporar' -count=1 -v

run_recorded verify verify.log timeout 3m make -C "$ROOT" verify
run_recorded verify-offline verify-offline.log timeout 3m make -C "$ROOT" verify-offline

scan_artifacts
printf 'artifact-marker-scan\t0\t0\n' >>"$VERIFICATION"
trap - EXIT

CHECKSUMS="$EVIDENCE_ROOT/checksums.sha256"
: >"$CHECKSUMS"
for file in "$EVIDENCE_ROOT"/*; do
  if [[ "$file" == "$CHECKSUMS" ]]; then
    continue
  fi
  hash="$(sha256sum "$file")"
  printf '%s  %s\n' "${hash%% *}" "$(basename "$file")" >>"$CHECKSUMS"
done

printf '%s\n' "$EVIDENCE_ROOT"
