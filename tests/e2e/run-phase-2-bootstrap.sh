#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RUN_ID="${1:-$(date -u +%Y%m%dT%H%M%SZ)-bootstrap-e2e}"
EVIDENCE_ROOT="$ROOT/.artifacts/phase-2/$RUN_ID"
umask 077
if [[ -e "$EVIDENCE_ROOT" ]]; then
  printf 'evidence root already exists: %s\n' "$EVIDENCE_ROOT" >&2
  exit 1
fi
mkdir -p "$EVIDENCE_ROOT"
VERIFICATION="$EVIDENCE_ROOT/verification.tsv"
printf 'name\texit_code\tduration_ms\n' >"$VERIFICATION"

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

scan_artifacts() {
  local marker
  local files
  shopt -s nullglob
  files=("$EVIDENCE_ROOT"/*)
  shopt -u nullglob
  while IFS='=' read -r _ marker; do
    [[ -z "$marker" ]] && continue
    if [[ "${#files[@]}" -gt 0 ]] && grep -a -l -F -- "$marker" "${files[@]}" >/dev/null; then
      printf 'synthetic privacy marker found in bootstrap evidence\n' >&2
      return 1
    fi
  done <"$ROOT/docs/phase-1/corpus/code-repo/private-markers.txt"
}

scan_on_exit() {
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

trap scan_on_exit EXIT

run_recorded bootstrap-e2e bootstrap-e2e.log \
  timeout 3m env GOTOOLCHAIN=go1.25.12 GOPROXY=off GPM_BOOTSTRAP_E2E_EVIDENCE_ROOT="$EVIDENCE_ROOT" \
  go -C "$ROOT" test ./tests/e2e -run '^TestBootstrapWorkflow$' -count=1 -v
run_recorded bootstrap-faults bootstrap-faults.log \
  timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off \
  go -C "$ROOT" test ./internal/memory ./internal/proposal ./internal/publisher \
  -run 'CreatePage|CreateSection|ImmediatelyBeforeRename|NewTarget' -count=1 -v
run_recorded verify verify.log timeout 4m make -C "$ROOT" verify
run_recorded verify-offline verify-offline.log timeout 4m make -C "$ROOT" verify-offline
scan_artifacts
printf 'artifact-marker-scan\t0\t0\n' >>"$VERIFICATION"
trap - EXIT

CHECKSUMS="$EVIDENCE_ROOT/checksums.sha256"
: >"$CHECKSUMS"
for file in "$EVIDENCE_ROOT"/*; do
  [[ "$file" == "$CHECKSUMS" ]] && continue
  digest="$(sha256sum "$file")"
  printf '%s  %s\n' "${digest%% *}" "$(basename "$file")" >>"$CHECKSUMS"
done
printf '%s\n' "$EVIDENCE_ROOT"
