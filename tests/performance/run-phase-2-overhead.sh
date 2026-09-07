#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RUN_ID="${1:-$(date -u +%Y%m%dT%H%M%SZ)-overhead-profile}"
EVIDENCE_ROOT="$ROOT/.artifacts/phase-2/$RUN_ID"
PROTOCOL="$ROOT/docs/phase-2/overhead-profile-protocol.md"
umask 077
if [[ -e "$EVIDENCE_ROOT" ]]; then
  printf 'evidence root already exists: %s\n' "$EVIDENCE_ROOT" >&2
  exit 1
fi
SUCCESS=false
TEMPORARY=""
cleanup() {
  if [[ -n "$TEMPORARY" ]]; then
    rm -rf "$TEMPORARY"
  fi
  if [[ "$SUCCESS" != true ]]; then
    rm -rf "$EVIDENCE_ROOT"
  fi
}
trap cleanup EXIT INT TERM
mkdir -p "$EVIDENCE_ROOT"
TEMPORARY="$(mktemp -d /tmp/git-push-memories-overhead.XXXXXX)"

PROTOCOL_HASH="$(sha256sum "$PROTOCOL")"
PROTOCOL_HASH="${PROTOCOL_HASH%% *}"
REVISION="$(git -C "$ROOT" rev-parse HEAD)"
SOURCE_MANIFEST="$TEMPORARY/evaluated-sources.sha256"
(cd "$ROOT" && LC_ALL=C git ls-files -co --exclude-standard -z -- '*.go' '*.sh' go.mod go.sum Makefile | LC_ALL=C sort -z | xargs -0 sha256sum) >"$SOURCE_MANIFEST"
SOURCE_HASH="$(sha256sum "$SOURCE_MANIFEST")"
SOURCE_HASH="${SOURCE_HASH%% *}"
printf '{"schema_version":1,"registered_at":"%s","product_revision":"%s","protocol_sha256":"%s","evaluated_tree_sha256":"%s"}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$REVISION" "$PROTOCOL_HASH" "$SOURCE_HASH" >"$EVIDENCE_ROOT/preregistration.json"
cp "$SOURCE_MANIFEST" "$EVIDENCE_ROOT/evaluated-sources.sha256"
(cd "$EVIDENCE_ROOT" && sha256sum preregistration.json >preregistration.sha256)
chmod 0400 "$EVIDENCE_ROOT/preregistration.json" "$EVIDENCE_ROOT/preregistration.sha256" "$EVIDENCE_ROOT/evaluated-sources.sha256"

run_bootstrap() {
  local name="$1"
  local output="$TEMPORARY/$name"
  mkdir -p "$output"
  timeout 3m env GOTOOLCHAIN=go1.25.12 GOPROXY=off GPM_BOOTSTRAP_E2E_EVIDENCE_ROOT="$output" \
    go -C "$ROOT" test ./tests/e2e -run '^TestBootstrapWorkflow$' -count=1 >"$TEMPORARY/$name.log" 2>&1
}

run_bootstrap warmup
for index in $(seq -w 1 10); do
  run_bootstrap "run-$index"
done

timeout 1m env GOTOOLCHAIN=go1.25.12 GOPROXY=off go -C "$ROOT" run ./tests/performance/aggregate \
  --root "$TEMPORARY" \
  --output "$EVIDENCE_ROOT/overhead-summary.json" \
  --revision "$REVISION" \
  --protocol-hash "$PROTOCOL_HASH" \
  --source-hash "$SOURCE_HASH"

(cd "$EVIDENCE_ROOT" && sha256sum overhead-summary.json preregistration.json preregistration.sha256 evaluated-sources.sha256 >checksums.sha256)
SUCCESS=true
printf '%s\n' "$EVIDENCE_ROOT"
