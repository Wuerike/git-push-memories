package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var allowedEvidenceFiles = map[string]bool{
	"protocol.json":          true,
	"preregistration.json":   true,
	"verification.tsv":       true,
	"curation-results.json":  true,
	"blind-evaluations.json": true,
	"report.json":            true,
	"git-manifest.json":      true,
	"privacy-summary.json":   true,
	"checksums.sha256":       true,
	"ledger.json":            true,
}

var forbiddenEvidenceFields = map[string]bool{
	"prompt":             true,
	"response":           true,
	"transcript":         true,
	"session":            true,
	"reasoning":          true,
	"credential":         true,
	"source_excerpt":     true,
	"source_root":        true,
	"preview_token":      true,
	"transaction_marker": true,
	"raw_diff":           true,
	"mcp_payload":        true,
	"stderr":             true,
	"tool_input":         true,
	"tool_result":        true,
}

type ledgerIdentity struct {
	ExperimentID    string `json:"experiment_id"`
	RunID           string `json:"run_id"`
	ProductRevision string `json:"product_revision"`
	ProtocolHash    string `json:"protocol_hash"`
	Approved        bool   `json:"approved"`
}

type ledgerTransition struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

type attemptRecord struct {
	UnitID        string `json:"unit_id"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	Valid         bool   `json:"valid"`
	InvalidReason string `json:"invalid_reason,omitempty"`
}

type ledgerFile struct {
	SchemaVersion int `json:"schema_version"`
	ledgerIdentity
	Status      string             `json:"status"`
	Transitions []ledgerTransition `json:"transitions"`
	Attempts    []attemptRecord    `json:"attempts"`
}

type evidenceStore struct {
	root   string
	ledger ledgerFile
}

func createEvidenceStore(root string, identity ledgerIdentity, now time.Time) (*evidenceStore, error) {
	if root == "" || !identity.Approved {
		return nil, errors.New("evidence root and explicit approval are required")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o700); err != nil {
		return nil, fmt.Errorf("create evidence parent: %w", err)
	}
	if info, err := os.Lstat(filepath.Dir(root)); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("evidence parent is not a real directory")
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return nil, fmt.Errorf("create exclusive evidence root: %w", err)
	}
	store := &evidenceStore{root: root}
	store.ledger = ledgerFile{
		SchemaVersion:  1,
		ledgerIdentity: identity,
		Status:         "started",
		Transitions:    []ledgerTransition{{Status: "started", At: now.UTC().Format(time.RFC3339Nano)}},
		Attempts:       []attemptRecord{},
	}
	if err := store.replaceJSON("ledger.json", store.ledger); err != nil {
		_ = os.RemoveAll(root)
		return nil, err
	}
	return store, nil
}

func (store *evidenceStore) recordAttempt(record attemptRecord) error {
	if store.ledger.Status != "started" || record.UnitID == "" || record.Kind == "" || (record.State != "completed" && record.State != "aborted") {
		return errors.New("invalid ledger attempt")
	}
	for _, existing := range store.ledger.Attempts {
		if existing.UnitID == record.UnitID {
			return errors.New("ledger attempt already exists")
		}
	}
	store.ledger.Attempts = append(store.ledger.Attempts, record)
	return store.replaceJSON("ledger.json", store.ledger)
}

func (store *evidenceStore) finish(status string, now time.Time) error {
	if store.ledger.Status != "started" || (status != "completed" && status != "aborted") {
		return errors.New("invalid ledger transition")
	}
	store.ledger.Status = status
	store.ledger.Transitions = append(store.ledger.Transitions, ledgerTransition{Status: status, At: now.UTC().Format(time.RFC3339Nano)})
	return store.replaceJSON("ledger.json", store.ledger)
}

func (store *evidenceStore) writeJSON(name string, value any) error {
	if !allowedEvidenceFiles[name] || name == "checksums.sha256" || filepath.Base(name) != name {
		return errors.New("evidence filename is not allowed")
	}
	target := filepath.Join(store.root, name)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return errors.New("evidence file already exists")
	}
	return store.replaceJSON(name, value)
}

func (store *evidenceStore) writeFile(name string, contents []byte) error {
	if !allowedEvidenceFiles[name] || filepath.Base(name) != name || name == "ledger.json" || name == "checksums.sha256" {
		return errors.New("evidence filename is not allowed")
	}
	return writeExclusiveFile(filepath.Join(store.root, name), contents)
}

func (store *evidenceStore) replaceJSON(name string, value any) error {
	if !allowedEvidenceFiles[name] || filepath.Base(name) != name {
		return errors.New("evidence filename is not allowed")
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return err
	}
	if err := validateEvidenceValue(generic); err != nil {
		return err
	}
	target := filepath.Join(store.root, name)
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("evidence target is a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(store.root, ".evidence-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return err
	}
	removeTemporary = false
	directory, err := os.Open(store.root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func validateEvidenceValue(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if forbiddenEvidenceFields[strings.ToLower(key)] {
				return fmt.Errorf("forbidden evidence field: %s", key)
			}
			if err := validateEvidenceValue(nested); err != nil {
				return err
			}
		}
	case []any:
		for _, nested := range typed {
			if err := validateEvidenceValue(nested); err != nil {
				return err
			}
		}
	}
	return nil
}

func (store *evidenceStore) writeChecksums() error {
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !allowedEvidenceFiles[name] {
			return fmt.Errorf("invalid evidence entry: %s", name)
		}
		if name == "checksums.sha256" {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(store.root, name))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(contents)
		lines = append(lines, hex.EncodeToString(hash[:])+"  "+name)
	}
	sort.Strings(lines)
	return writeExclusiveFile(filepath.Join(store.root, "checksums.sha256"), []byte(strings.Join(lines, "\n")+"\n"))
}

func writeExclusiveFile(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
