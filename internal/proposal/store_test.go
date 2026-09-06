package proposal

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreFailureBeforeRenameLeavesNoVisibleProposal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := newStore(root, true)
	if err != nil {
		t.Fatal(err)
	}
	store.fault = func(stage string) error { return errors.New(stage) }
	view := View{SchemaVersion: 1, ID: "0123456789abcdef0123456789abcdef", Revision: 1, CreatedAt: time.Unix(1, 0)}
	if err := store.writeProposal(view); err == nil {
		t.Fatal("writeProposal() error = nil")
	}
	entries, err := os.ReadDir(filepath.Join(root, "proposals"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("proposal directory contains %v", entries)
	}
}

func TestStoreStartupRemovesOnlyControlledTemporaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	proposals := filepath.Join(root, "proposals")
	if err := os.MkdirAll(proposals, 0o700); err != nil {
		t.Fatal(err)
	}
	controlled := filepath.Join(proposals, ".git-push-memories-0123456789abcdef.tmp")
	unrelated := filepath.Join(proposals, "keep.tmp")
	if err := os.WriteFile(controlled, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newStore(root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(controlled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("controlled temporary was not removed")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("unrelated temporary was removed")
	}
}

func TestRepeatedRejectFinishesCleanupAfterDurableReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := newStore(root, true)
	if err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	hash := "sha256:" + strings.Repeat("0", 64)
	view := View{
		SchemaVersion: 1, ID: id, Revision: 1, Status: "pending", CreatedAt: time.Unix(1, 0).UTC(),
		Operation: Operation{Type: "replace_section_content", Target: Target{Path: "a.md", SectionID: "a"}},
		Baseline:  Baseline{FileHash: hash, Head: "head", Blob: "blob"}, PostState: PostState{FileHash: hash},
		Diff: "diff", SourcePaths: []string{},
	}
	if err := store.writeProposal(view); err != nil {
		t.Fatal(err)
	}
	receipt := DecisionReceipt{SchemaVersion: 1, ProposalID: id, Revision: 1, Status: "rejected", Reason: "incorrect", DecidedAt: time.Unix(2, 0).UTC()}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.durableWrite("decisions", id+".json", encoded); err != nil {
		t.Fatal(err)
	}
	workflow := &Workflow{store: store, now: time.Now}
	got, err := workflow.Reject(id, "incorrect")
	if err != nil || got != receipt {
		t.Fatalf("Reject() = %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "proposals", id+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Reject() did not finish payload cleanup")
	}
}
