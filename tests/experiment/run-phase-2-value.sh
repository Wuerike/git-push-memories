#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
if [[ -n "$(git -C "$ROOT" status --porcelain=v1)" ]]; then
  printf 'value experiment requires a clean product worktree\n' >&2
  exit 1
fi

LEDGER="$ROOT/.artifacts/phase-2/parcel-dispatch-value-v2.completed"
if [[ -e "$LEDGER" ]]; then
  printf 'this pre-registered experiment already has a completed run\n' >&2
  exit 1
fi

RUN_ID="${1:-$(date -u +%Y%m%dT%H%M%SZ)-value-experiment}"
EVIDENCE_ROOT="$ROOT/.artifacts/phase-2/$RUN_ID"
umask 077

timeout 2m env GOTOOLCHAIN=go1.25.12 GOPROXY=off \
  go -C "$ROOT" test ./tests/experiment/... -count=1

AUTH_ROOT="$(mktemp -d /dev/shm/gpm-value-auth.XXXXXX)"
trap 'rm -rf "$AUTH_ROOT"' EXIT
printf 'experiment_id=parcel-dispatch-value-v2\nstatus=started\nrevision=%s\nevidence=%s\n' \
  "$(git -C "$ROOT" rev-parse HEAD)" "$EVIDENCE_ROOT" >"$LEDGER"

timeout 35m env GOTOOLCHAIN=go1.25.12 GOPROXY=off \
  go -C "$ROOT" run ./tests/experiment/runner --evidence-root "$EVIDENCE_ROOT" --auth-root "$AUTH_ROOT"

printf 'experiment_id=parcel-dispatch-value-v2\nstatus=completed\nrevision=%s\nevidence=%s\n' \
  "$(git -C "$ROOT" rev-parse HEAD)" "$EVIDENCE_ROOT" >"$LEDGER"

printf '%s\n' "$EVIDENCE_ROOT"
