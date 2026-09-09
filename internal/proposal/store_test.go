package proposal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStoreFailureBeforeRefUpdateLeavesNoVisibleProposal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	memoryRoot := newStoreMemoryRoot(t)
	store, err := newStore(memoryRoot, root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })
	store.fault = func(stage string) error {
		if stage == "before_ref_update" {
			return errors.New(stage)
		}
		return nil
	}
	view := newStoreTestView(t, memoryRoot)
	if err := store.writeProposal(view); err == nil {
		t.Fatal("writeProposal() error = nil")
	}
	if _, err := store.readProposal(view.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("readProposal() error = %v, want ErrNotFound", err)
	}
}

func TestStoreCreatesProposalWithoutUserGitIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	memoryRoot := newStoreMemoryRoot(t)
	for _, key := range []string{"user.name", "user.email"} {
		command := exec.Command("git", "-C", memoryRoot, "config", "--unset", key)
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
	}
	store, err := newStore(memoryRoot, root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })
	view := newStoreTestView(t, memoryRoot)

	if err := store.writeProposal(view); err != nil {
		t.Fatal(err)
	}
	if _, err := store.readProposal(view.ID); err != nil {
		t.Fatal(err)
	}
}

func TestStoreDoesNotDereferenceSymbolicProposalBranch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	memoryRoot := newStoreMemoryRoot(t)
	store, err := newStore(memoryRoot, root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })
	view := newStoreTestView(t, memoryRoot)
	headRef, err := store.gitText(context.Background(), nil, nil, "symbolic-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	proposalRef := proposalRefPrefix + view.ID
	if _, err := store.gitInput(context.Background(), nil, nil, "symbolic-ref", proposalRef, headRef); err != nil {
		t.Fatal(err)
	}

	if err := store.writeProposal(view); err == nil {
		t.Fatal("writeProposal() replaced a symbolic proposal ref")
	}
	if got, err := store.gitText(context.Background(), nil, nil, "symbolic-ref", proposalRef); err != nil || got != headRef {
		t.Fatalf("symbolic proposal ref = %q, %v; want %q", got, err, headRef)
	}
}

func TestStoreStartupRemovesOnlyControlledTemporaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	memoryRoot := newStoreMemoryRoot(t)
	proposals := filepath.Join(root, "proposals")
	if err := os.MkdirAll(proposals, 0o700); err != nil {
		t.Fatal(err)
	}
	controlled := filepath.Join(proposals, ".git-push-memories-0123456789abcdef.tmp")
	controlledLock := controlled + ".lock"
	unrelated := filepath.Join(proposals, "keep.tmp")
	if err := os.WriteFile(controlled, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controlledLock, []byte("lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newStore(memoryRoot, root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(controlled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("controlled temporary was not removed")
	}
	if _, err := os.Stat(controlledLock); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("controlled temporary lock was not removed")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("unrelated temporary was removed")
	}
}

func TestRepeatedRejectFinishesCleanupAfterDurableReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	memoryRoot := newStoreMemoryRoot(t)
	store, err := newStore(memoryRoot, root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })
	view := newStoreTestView(t, memoryRoot)
	id := view.ID
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
	if _, err := store.readProposal(id); err != nil {
		t.Fatal("Reject() removed the durable proposal branch")
	}
}

func TestStoreReadsPersistedV1StateWithoutV2Fields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := newStore(newStoreMemoryRoot(t), root, true)
	if err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + strings.Repeat("0", 64)
	view := View{
		SchemaVersion: 1, Nonce: "0123456789abcdef0123456789abcdef", Revision: 1, Status: "pending", CreatedAt: time.Unix(1, 0).UTC(),
		Operation: Operation{Type: "replace_section_content", Target: Target{Path: "memories/a.md", SectionID: "a"}, Replacement: "old"},
		Baseline:  Baseline{FileHash: hash, Head: "head", Blob: "blob"}, PostState: PostState{FileHash: hash}, Diff: "diff", SourcePaths: []string{},
	}
	if err := sealProposal(&view); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"exists"`, `"title"`, `"heading"`, `"body"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("v1 fixture contains v2 field %s: %s", forbidden, encoded)
		}
	}
	apply := ApplyReceipt{
		SchemaVersion: 1, ProposalID: view.ID, Revision: 1, Status: "applied_unpublished", AppliedAt: time.Unix(2, 0).UTC(), MemoryRoot: "/memory",
		Target: view.Operation.Target, Baseline: view.Baseline, PostState: view.PostState,
	}
	if err := store.writeApply(apply); err != nil {
		t.Fatal(err)
	}
	gotApply, err := store.readApply(view.ID)
	if err != nil || !reflect.DeepEqual(gotApply, apply) {
		t.Fatalf("readApply() = %+v, %v; want %+v", gotApply, err, apply)
	}
}

func TestStartupCleanupDoesNotRemoveAnActiveTemporary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	memoryRoot := newStoreMemoryRoot(t)
	store, err := newStore(memoryRoot, root, true)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { _ = store.close() })
	store.fault = func(stage string) error {
		if stage == "before_ref_update" {
			close(entered)
			<-release
		}
		return nil
	}
	view := newStoreTestView(t, memoryRoot)
	writeDone := make(chan error, 1)
	go func() { writeDone <- store.writeProposal(view) }()
	<-entered
	cleanupStarted := make(chan struct{})
	cleanupDone := make(chan error, 1)
	go func() {
		close(cleanupStarted)
		_, err := newStore(memoryRoot, root, true)
		cleanupDone <- err
	}()
	<-cleanupStarted
	select {
	case err := <-cleanupDone:
		t.Fatalf("startup cleanup did not wait for active write: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-cleanupDone; err != nil {
		t.Fatal(err)
	}
	if _, err := store.readProposal(view.ID); err != nil {
		t.Fatal("startup cleanup removed published proposal branch")
	}
}

func newStoreTestView(t *testing.T, memoryRoot string) View {
	t.Helper()
	headCommand := exec.Command("git", "-C", memoryRoot, "rev-parse", "HEAD")
	head, err := headCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	absent := false
	view := View{
		SchemaVersion: 2, Nonce: "0123456789abcdef0123456789abcdef", Revision: 1, Status: "pending", CreatedAt: time.Unix(1, 0).UTC(),
		Operation: Operation{Type: OperationCreatePage, Target: Target{Path: "memories/page.md", SectionID: "page-overview"}, Title: "Page", Body: "Body."},
		Baseline:  Baseline{Exists: &absent, Head: strings.TrimSpace(string(head))}, PostState: PostState{FileHash: "sha256:" + strings.Repeat("0", 64)},
		Diff: "diff", SourcePaths: []string{},
	}
	if err := sealProposal(&view); err != nil {
		t.Fatal(err)
	}
	return view
}

func newStoreMemoryRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, arguments := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"commit", "--quiet", "--allow-empty", "-m", "initial"}} {
		command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
	}
	return root
}
