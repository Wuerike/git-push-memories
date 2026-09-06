package proposal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/intake"
)

func TestApplyRecoversWhenReceiptPublicationFailsAfterMemoryRename(t *testing.T) {
	workflow, view, memoryRoot, stateRoot := newFaultWorkflow(t)
	workflow.store.fault = func(string) error { return errors.New("receipt fault") }
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err == nil {
		t.Fatal("Apply() error = nil")
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "\nnew\n") {
		t.Fatal("memory rename did not precede receipt failure")
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "applies", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed receipt became visible")
	}
	workflow.store.fault = nil
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err != nil {
		t.Fatalf("Apply() retry error = %v", err)
	}
}

func TestApplyRetryOnlyCleansUpAfterDurableReceipt(t *testing.T) {
	workflow, view, _, stateRoot := newFaultWorkflow(t)
	workflow.store.cleanupFault = func(string) error { return errors.New("cleanup fault") }
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err == nil {
		t.Fatal("Apply() error = nil")
	}
	stored, err := workflow.store.readApply(view.ID)
	if err != nil {
		t.Fatal("apply receipt was not durable before cleanup failure")
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals", view.ID+".json")); err != nil {
		t.Fatal("cleanup failure removed proposal")
	}
	workflow.store.cleanupFault = nil
	repeated, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil || !reflect.DeepEqual(repeated, stored) {
		t.Fatalf("Apply() retry = %+v, %v; want %+v", repeated, err, stored)
	}
}

func TestApplyDoesNotCleanUpWhenReceiptDirectorySyncFails(t *testing.T) {
	workflow, view, _, stateRoot := newFaultWorkflow(t)
	workflow.store.fault = func(stage string) error {
		if stage == "after_rename" {
			return errors.New("directory sync fault")
		}
		return nil
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err == nil {
		t.Fatal("Apply() error = nil")
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "applies", view.ID+".json")); err != nil {
		t.Fatal("renamed apply receipt is not visible")
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals", view.ID+".json")); err != nil {
		t.Fatal("directory sync failure removed recovery payload")
	}
	workflow.store.fault = nil
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err != nil {
		t.Fatalf("Apply() retry error = %v", err)
	}
}

func TestRejectCannotWinAfterApplyRenamesMemory(t *testing.T) {
	workflow, view, _, stateRoot := newFaultWorkflow(t)
	workflow.store.fault = func(string) error { return errors.New("receipt fault") }
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err == nil {
		t.Fatal("Apply() error = nil")
	}
	workflow.store.fault = nil
	if _, err := workflow.Reject(view.ID, "incorrect"); !errors.Is(err, ErrTerminal) {
		t.Fatalf("Reject() error = %v, want ErrTerminal", err)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "applies", view.ID+".json")); err != nil {
		t.Fatal("reject did not reconcile the winning apply")
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "decisions", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reject published a competing terminal receipt")
	}
}

func TestApplyRescansSensitiveContentAfterProposalWasSealedAgain(t *testing.T) {
	workflow, view, memoryRoot, stateRoot := newFaultWorkflow(t)
	view.Operation.Replacement = "Authorization: Bearer secret"
	if err := sealProposal(&view); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateRoot, "proposals", view.ID+".json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Apply() error = %v, want ErrIntegrity", err)
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "Bearer") {
		t.Fatal("sensitive replacement reached memory")
	}
}

func newFaultWorkflow(t *testing.T) (*Workflow, View, string, string) {
	t.Helper()
	root := t.TempDir()
	memoryRoot := filepath.Join(root, "memory")
	codeRoot := filepath.Join(root, "code")
	stateRoot := filepath.Join(root, "state")
	if err := os.MkdirAll(memoryRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(codeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	page := "# Memory\n<!-- @id: memory -->\n\n## State\n<!-- @id: state -->\n\nold\n"
	if err := os.WriteFile(filepath.Join(memoryRoot, "memory.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"add", "."}, {"commit", "--quiet", "-m", "initial"}} {
		command := exec.Command("git", append([]string{"-C", memoryRoot}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"target_path": "memory.md", "section_id": "state", "replacement": "\nnew\n", "source_paths": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := New(memoryRoot, codeRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowed)
	if err != nil {
		t.Fatal(err)
	}
	return workflow, view, memoryRoot, stateRoot
}
