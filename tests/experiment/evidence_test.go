package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvidenceStoreRecordsDurableLedgerTransitions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	store, err := createEvidenceStore(root, ledgerIdentity{
		ExperimentID:    "phase3-curated-memory-value-v1",
		RunID:           "run-test",
		ProductRevision: "e92a96b",
		ProtocolHash:    "protocol-hash",
		Approved:        true,
	}, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createEvidenceStore(root, ledgerIdentity{}, time.Unix(2, 0)); err == nil {
		t.Fatal("existing evidence root was reopened")
	}
	if err := store.recordAttempt(attemptRecord{UnitID: "preflight-model", Kind: "model_preflight", State: "completed", Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.finish("completed", time.Unix(3, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	var ledger ledgerFile
	decodeFileForTest(t, filepath.Join(root, "ledger.json"), &ledger)
	if ledger.Status != "completed" || len(ledger.Transitions) != 2 || len(ledger.Attempts) != 1 || ledger.Attempts[0].UnitID != "preflight-model" {
		t.Fatalf("ledger = %+v", ledger)
	}
	if info, err := os.Stat(filepath.Join(root, "ledger.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ledger mode = %v, err = %v", info.Mode().Perm(), err)
	}
}

func TestEvidenceStoreRejectsForbiddenOrUnlistedContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	store, err := createEvidenceStore(root, ledgerIdentity{ExperimentID: "experiment", RunID: "run", Approved: true}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writeJSON("report.json", map[string]any{"safe": map[string]any{"Tool_Result": "raw"}}); err == nil {
		t.Fatal("recursive forbidden key was accepted")
	}
	if err := store.writeJSON("extra.json", map[string]any{"safe": true}); err == nil {
		t.Fatal("unlisted evidence file was accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "report.json")); err != nil {
		t.Fatal(err)
	}
	if err := store.writeJSON("report.json", map[string]any{"safe": true}); err == nil {
		t.Fatal("evidence symlink was followed")
	}
}

func TestEvidenceStoreWritesCompleteChecksums(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	store, err := createEvidenceStore(root, ledgerIdentity{ExperimentID: "experiment", RunID: "run", Approved: true}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writeJSON("report.json", map[string]any{"schema_version": 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.writeChecksums(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(root, "checksums.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "  ledger.json") || !strings.HasSuffix(lines[1], "  report.json") {
		t.Fatalf("checksums = %q", contents)
	}
}

func decodeFileForTest(t *testing.T, path string, target any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, target); err != nil {
		t.Fatal(err)
	}
}
