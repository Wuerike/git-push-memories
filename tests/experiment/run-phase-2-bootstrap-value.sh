#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
if [[ "${1:-}" != "--approved" ]]; then
  printf 'explicit human approval required: pass --approved after reviewing docs/phase-2/bootstrap-value-experiment-protocol.md\n' >&2
  exit 1
fi

LEDGER="$ROOT/.artifacts/phase-2/parcel-dispatch-bootstrap-value-v1.completed"

RUN_ID="${2:-$(date -u +%Y%m%dT%H%M%SZ)-bootstrap-value-experiment}"
EVIDENCE_ROOT="$ROOT/.artifacts/phase-2/$RUN_ID"
PROTOCOL_ROOT="$ROOT/testdata/phase-2/bootstrap-value-experiment"
umask 077

PREFLIGHT="$(mktemp /tmp/gpm-bootstrap-value-preflight.XXXXXX)"
AUTH_ROOT=""
cleanup() {
  rm -f "$PREFLIGHT"
  if [[ -n "$AUTH_ROOT" ]]; then
    rm -rf "$AUTH_ROOT"
  fi
}
trap cleanup EXIT
printf 'name\tcommand\texit_code\tduration_ms\n' >"$PREFLIGHT"

run_preflight() {
  local name="$1"
  local rendered="$2"
  shift 2
  local started ended status
  started="$(date +%s%N)"
  set +e
  "$@" >/dev/null 2>&1
  status=$?
  set -e
  ended="$(date +%s%N)"
  printf '%s\t%s\t%d\t%d\n' "$name" "$rendered" "$status" "$(((ended - started) / 1000000))" >>"$PREFLIGHT"
  if [[ "$status" -ne 0 ]]; then
    printf 'preflight failed before inference: %s\n' "$name" >&2
    exit "$status"
  fi
}

run_preflight tests 'go test ./...' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" test ./...
run_preflight race 'go test -race ./...' timeout 3m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" test -race ./...
run_preflight vet 'go vet ./...' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" vet ./...
run_preflight build 'go build ./...' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" build ./...
run_preflight fault-injection 'go test proposal publisher -run fault/recovery' timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" test ./internal/proposal ./internal/publisher -run 'Recovers|Retry|SyncFails|Interrupted|Recovery|Interruption|Reuses|ObservesEdits|ConcurrentBranch|IndexLock|Temporar' -count=1

SOURCE_HASH="$(GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" run ./tests/experiment/runner --print-source-hash)"
PROTOCOL_HASH="$(sha256sum "$PROTOCOL_ROOT/experiment.json" | cut -d' ' -f1)"
AUTH_ROOT="$(mktemp -d /dev/shm/gpm-bootstrap-value-auth.XXXXXX)"
mkdir -p "$(dirname "$LEDGER")"
if ! (set -o noclobber; printf 'experiment_id=parcel-dispatch-bootstrap-value-v1\nstatus=started\napproved=true\nrevision_base=%s\nprotocol_hash=%s\nsource_hash=%s\nevidence=%s\n' \
  "$(git -C "$ROOT" rev-parse HEAD)" "$PROTOCOL_HASH" "$SOURCE_HASH" "$EVIDENCE_ROOT" >"$LEDGER") 2>/dev/null; then
  printf 'this pre-registered experiment already has a completed or started run\n' >&2
  exit 1
fi
chmod 400 "$LEDGER"

timeout 35m env GOTOOLCHAIN=go1.25.12 GOPROXY=off \
  go -C "$ROOT" run ./tests/experiment/runner --evidence-root "$EVIDENCE_ROOT" --auth-root "$AUTH_ROOT" --verification-file "$PREFLIGHT" --protocol-root "$PROTOCOL_ROOT" --source-hash "$SOURCE_HASH" --preregistration-file "$LEDGER"

chmod 600 "$LEDGER"
printf 'status=completed\n' >>"$LEDGER"
chmod 400 "$LEDGER"
printf '%s\n' "$EVIDENCE_ROOT"
