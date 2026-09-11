#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
if [[ "${1:-}" != "--approved" ]]; then
  printf 'usage: %s --approved [run-id]\n' "$0" >&2
  printf 'authenticated execution can consume at most USD 25 and requires explicit approval\n' >&2
  exit 2
fi
if [[ -n "$(git -C "$ROOT" status --porcelain=v1)" ]]; then
  printf 'phase 3 value evaluation requires a clean worktree\n' >&2
  exit 1
fi

RUN_ID="${2:-$(date -u +%Y%m%dT%H%M%SZ)-phase3-value}"
EVIDENCE_ROOT="$ROOT/.artifacts/phase-3/$RUN_ID"
if [[ -e "$EVIDENCE_ROOT" ]]; then
  printf 'evidence root already exists: %s\n' "$EVIDENCE_ROOT" >&2
  exit 1
fi

umask 077
VERIFICATION="$(mktemp /tmp/gpm-phase3-verification.XXXXXX)"
trap 'rm -f "$VERIFICATION"' EXIT
printf 'name\tcommand\texit_code\tduration_ms\n' >"$VERIFICATION"

run_check() {
  local name="$1"
  local rendered="$2"
  shift 2
  local started ended status
  started="$(date +%s%N)"
  set +e
  # Fixture permissions in the repository checks assume the normal build umask.
  # Keep the enclosing process private for verification.tsv and runtime artifacts.
  (umask 022; "$@")
  status=$?
  set -e
  ended="$(date +%s%N)"
  printf '%s\t%s\t%d\t%d\n' "$name" "$rendered" "$status" "$(((ended - started) / 1000000))" >>"$VERIFICATION"
  if [[ "$status" -ne 0 ]]; then
    printf 'verification failed before inference: %s\n' "$name" >&2
    exit "$status"
  fi
}

run_check tests 'go test ./...' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" test ./...
run_check race 'go test -race ./...' timeout 3m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" test -race ./...
run_check vet 'go vet ./...' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" vet ./...
run_check build 'go build ./...' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" build ./...

printf 'Experiment: phase3-curated-memory-value-v2\n'
printf 'Evidence: %s\n' "$EVIDENCE_ROOT"
printf 'Maximum authenticated cost: USD 25.00\n'

timeout --foreground 8h env GOTOOLCHAIN=go1.25.12 GOPROXY=off \
  go -C "$ROOT" run ./tests/experiment/runner \
  --approved \
  --run-id "$RUN_ID" \
  --evidence-root "$EVIDENCE_ROOT" \
  --verification-file "$VERIFICATION"
