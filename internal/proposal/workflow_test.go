package proposal_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/proposal"
	gitpublisher "github.com/Wuerike/git-push-memories/internal/publisher"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
)

const initialPage = `# checkout-api
<!-- @id: checkout-api -->

## Operacao
<!-- @id: checkout-api-operation -->

- timeout: cinco segundos
- owner: team-a
`

func TestPreparePersistsReviewableProposalOnDedicatedBranch(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	beforeHead := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	beforeStatus := gitOutput(t, memoryRoot, "status", "--porcelain=v1", "--untracked-files=all")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	allowed := allowedCandidate(t, "\n- timeout: tres segundos\n- owner: team-b\n")
	view, err := workflow.Prepare(context.Background(), allowed)
	if err != nil {
		t.Fatal(err)
	}
	if view.SchemaVersion != 2 || view.Baseline.Exists == nil || !*view.Baseline.Exists || len(view.ID) != 32 || view.Revision != 1 || view.Status != "pending" {
		t.Fatalf("Prepare() identity = %+v", view)
	}
	if _, err := workflow.Applied(view.ID); !errors.Is(err, proposal.ErrNotApplied) {
		t.Fatalf("Applied() error = %v, want ErrNotApplied", err)
	}
	if view.Operation.Type != "replace_section_content" || view.Operation.Target.Path != "memories/checkout-api.md" || view.Operation.Target.SectionID != "checkout-api-operation" {
		t.Fatalf("Prepare() operation = %+v", view.Operation)
	}
	if view.Baseline.FileHash == view.PostState.FileHash || view.Baseline.Head == "" || view.Baseline.Blob == "" {
		t.Fatalf("Prepare() states = baseline %+v, post-state %+v", view.Baseline, view.PostState)
	}
	if !strings.Contains(view.Diff, "--- a/memories/checkout-api.md") || !strings.Contains(view.Diff, "\n-- timeout: cinco segundos") || !strings.Contains(view.Diff, "\n+- timeout: tres segundos") {
		t.Fatalf("Prepare() diff = %q", view.Diff)
	}
	assertMode(t, stateRoot, 0o700)
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state proposal directory exists: %v", err)
	}
	branch := "gpm/proposal/" + view.ID
	if parent := gitOutput(t, memoryRoot, "rev-parse", branch+"^"); parent != view.Baseline.Head {
		t.Fatalf("proposal branch parent = %q, want %q", parent, view.Baseline.Head)
	}
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", branch); changed != "proposals/"+view.ID+".json" {
		t.Fatalf("proposal branch changed %q", changed)
	}
	var stored proposal.View
	if err := json.Unmarshal([]byte(gitOutput(t, memoryRoot, "show", branch+":proposals/"+view.ID+".json")), &stored); err != nil || !reflect.DeepEqual(stored, view) {
		t.Fatalf("stored proposal = %+v, %v; want %+v", stored, err, view)
	}
	if got := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("Prepare() changed HEAD from %s to %s", beforeHead, got)
	}
	if got := gitOutput(t, memoryRoot, "status", "--porcelain=v1", "--untracked-files=all"); got != beforeStatus {
		t.Fatalf("Prepare() changed worktree/index: before %q, after %q", beforeStatus, got)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stateRoot); err != nil {
		t.Fatal(err)
	}

	restarted, err := proposal.OpenReadOnly(memoryRoot, filepath.Join(t.TempDir(), "fresh-state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	review, err := restarted.Review(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if review.Proposal == nil || !reflect.DeepEqual(*review.Proposal, view) {
		t.Fatalf("Review() = %+v, want %+v", review, view)
	}
}

func TestCreatePageProposalAppliesOnceFromAbsentBaseline(t *testing.T) {
	memoryRoot := newEmptyMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })

	view, err := workflow.Prepare(context.Background(), allowedCreatePageCandidate(t, "Current state."))
	if err != nil {
		t.Fatal(err)
	}
	if view.SchemaVersion != 2 || view.Operation.Type != "create_page" || view.Operation.Title != "Parcel Dispatch" || view.Operation.Body != "Current state." {
		t.Fatalf("Prepare() create operation = %+v", view)
	}
	if view.Baseline.Exists == nil || *view.Baseline.Exists || view.Baseline.FileHash != "" || view.Baseline.Blob != "" || view.Baseline.Head == "" {
		t.Fatalf("Prepare() baseline = %+v", view.Baseline)
	}
	if !strings.HasPrefix(view.Diff, "--- /dev/null\n+++ b/memories/parcel-dispatch.md\n") {
		t.Fatalf("Prepare() diff = %q", view.Diff)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "memories", "parcel-dispatch.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Prepare() created the target")
	}

	receipt, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 2 || receipt.Baseline.Exists == nil || *receipt.Baseline.Exists || receipt.Target != view.Operation.Target || receipt.PostState != view.PostState {
		t.Fatalf("Apply() receipt = %+v", receipt)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "parcel-dispatch.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "<!-- @id: parcel-dispatch-overview -->\nCurrent state.\n") {
		t.Fatalf("created page = %q", page)
	}
	repeated, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil || !reflect.DeepEqual(repeated, receipt) {
		t.Fatalf("repeated Apply() = %+v, %v; want %+v", repeated, err, receipt)
	}
}

func TestCreatePageApplyRejectsTargetCreatedAfterPrepare(t *testing.T) {
	memoryRoot := newEmptyMemoryRoot(t)
	workflow, err := proposal.New(memoryRoot, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCreatePageCandidate(t, "Current state."))
	if err != nil {
		t.Fatal(err)
	}
	pagePath := filepath.Join(memoryRoot, "memories", "parcel-dispatch.md")
	concurrent := []byte("concurrent content\n")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pagePath, concurrent, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); !errors.Is(err, memory.ErrDrift) {
		t.Fatalf("Apply() error = %v, want ErrDrift", err)
	}
	got, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, concurrent) {
		t.Fatal("Apply() overwrote the concurrent target")
	}
}

func TestCreatePageStartupRecoversPostStateWithoutApplyReceipt(t *testing.T) {
	memoryRoot := newEmptyMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowedCreatePageCandidate(t, "Current state."))
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	postState := "# Parcel Dispatch\n<!-- @id: parcel-dispatch -->\n\n## Overview\n<!-- @id: parcel-dispatch-overview -->\nCurrent state.\n"
	if err := os.MkdirAll(filepath.Join(memoryRoot, "memories"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memoryRoot, "memories", "parcel-dispatch.md"), []byte(postState), 0o644); err != nil {
		t.Fatal(err)
	}
	restarted, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	receipt, err := restarted.Applied(view.ID)
	if err != nil || receipt.SchemaVersion != 2 || receipt.ProposalID != view.ID {
		t.Fatalf("startup Applied() = %+v, %v", receipt, err)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("startup recovery retained create proposal")
	}
}

func TestCreatePageConcurrentApplyReturnsOneDurableResult(t *testing.T) {
	memoryRoot := newEmptyMemoryRoot(t)
	workflow, err := proposal.New(memoryRoot, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCreatePageCandidate(t, "Current state."))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan struct {
		receipt proposal.ApplyReceipt
		err     error
	}, 2)
	for range 2 {
		go func() {
			<-start
			receipt, err := workflow.Apply(context.Background(), view.ID, view.Revision)
			results <- struct {
				receipt proposal.ApplyReceipt
				err     error
			}{receipt, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || !reflect.DeepEqual(first.receipt, second.receipt) {
		t.Fatalf("concurrent Apply() = (%+v, %v), (%+v, %v)", first.receipt, first.err, second.receipt, second.err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "memories", "parcel-dispatch.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePagePublicationIsRecordedAndIdempotent(t *testing.T) {
	memoryRoot := newEmptyMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCreatePageCandidate(t, "Current state."))
	if err != nil {
		t.Fatal(err)
	}
	apply, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := gitpublisher.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	publication, err := publisher.Publish(context.Background(), apply)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := workflow.RecordPublication(context.Background(), publication)
	if err != nil || !reflect.DeepEqual(stored, publication) {
		t.Fatalf("RecordPublication() = %+v, %v", stored, err)
	}
	repeated, err := workflow.RecordPublication(context.Background(), publication)
	if err != nil || !reflect.DeepEqual(repeated, publication) {
		t.Fatalf("repeated RecordPublication() = %+v, %v", repeated, err)
	}
}

func TestCreateSectionProposalAppliesPublishesAndRetrievesGeneratedID(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCreateSectionCandidate(t, "Circuit breaker opens."))
	if err != nil {
		t.Fatal(err)
	}
	if view.Operation.Type != "create_section" || view.Operation.Target.SectionID != "checkout-api-failure-modes" || view.Operation.Heading != "Failure Modes" {
		t.Fatalf("Prepare() operation = %+v", view.Operation)
	}
	receipt, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	repeatedApply, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil || !reflect.DeepEqual(repeatedApply, receipt) {
		t.Fatalf("repeated Apply() = %+v, %v", repeatedApply, err)
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "## Failure Modes\n<!-- @id: checkout-api-failure-modes -->\nCircuit breaker opens.\n") {
		t.Fatalf("created section missing: %s", contents)
	}
	publisher, err := gitpublisher.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	publication, err := publisher.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	repeatedPublication, err := publisher.Publish(context.Background(), receipt)
	if err != nil || !reflect.DeepEqual(repeatedPublication, publication) {
		t.Fatalf("repeated Publish() = %+v, %v", repeatedPublication, err)
	}
	if _, err := workflow.RecordPublication(context.Background(), publication); err != nil {
		t.Fatal(err)
	}
	retriever, err := retrieval.New(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer retriever.Close()
	bundle, err := retriever.Retrieve(context.Background(), "circuit breaker failures", retrieval.Scope{}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range bundle.Items {
		found = found || item.Handle.SectionID == "checkout-api-failure-modes" && strings.Contains(item.Content, "Circuit breaker opens.")
	}
	if !found {
		t.Fatalf("generated section not retrievable: %+v", bundle.Items)
	}
}

func TestCreateSectionApplyRejectsWholeFileDrift(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	workflow, err := proposal.New(memoryRoot, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCreateSectionCandidate(t, "Circuit breaker opens."))
	if err != nil {
		t.Fatal(err)
	}
	pagePath := filepath.Join(memoryRoot, "memories", "checkout-api.md")
	drift := []byte(initialPage + "drift\n")
	if err := os.WriteFile(pagePath, drift, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); !errors.Is(err, memory.ErrDrift) {
		t.Fatalf("Apply() error = %v, want ErrDrift", err)
	}
	got, err := os.ReadFile(pagePath)
	if err != nil || !bytes.Equal(got, drift) {
		t.Fatalf("drifted page = %q, %v", got, err)
	}
}

func TestCreateSectionStartupRecoversPostState(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowedCreateSectionCandidate(t, "Circuit breaker opens."))
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	postState := initialPage + "\n## Failure Modes\n<!-- @id: checkout-api-failure-modes -->\nCircuit breaker opens.\n"
	if err := os.WriteFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"), []byte(postState), 0o644); err != nil {
		t.Fatal(err)
	}
	restarted, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	receipt, err := restarted.Applied(view.ID)
	if err != nil || receipt.Target.SectionID != "checkout-api-failure-modes" {
		t.Fatalf("startup Applied() = %+v, %v", receipt, err)
	}
}

func TestCreateSectionConcurrentApplyReturnsOneDurableResult(t *testing.T) {
	workflow, err := proposal.New(newMemoryRoot(t), filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer workflow.Close()
	view, err := workflow.Prepare(context.Background(), allowedCreateSectionCandidate(t, "Circuit breaker opens."))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan struct {
		receipt proposal.ApplyReceipt
		err     error
	}, 2)
	for range 2 {
		go func() {
			<-start
			receipt, err := workflow.Apply(context.Background(), view.ID, view.Revision)
			results <- struct {
				receipt proposal.ApplyReceipt
				err     error
			}{receipt, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || !reflect.DeepEqual(first.receipt, second.receipt) {
		t.Fatalf("concurrent Apply() = (%+v, %v), (%+v, %v)", first.receipt, first.err, second.receipt, second.err)
	}
}

func TestPrepareRejectsDirtyTarget(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(fmt.Sprintf("staged=%v", staged), func(t *testing.T) {
			memoryRoot := newMemoryRoot(t)
			pagePath := filepath.Join(memoryRoot, "memories", "checkout-api.md")
			if err := os.WriteFile(pagePath, []byte(strings.Replace(initialPage, "cinco", "quatro", 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if staged {
				runGit(t, memoryRoot, "add", "memories/checkout-api.md")
			}
			stateRoot := filepath.Join(t.TempDir(), "state")
			workflow, err := proposal.New(memoryRoot, stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workflow.Close() })
			_, err = workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
			if !errors.Is(err, memory.ErrTargetDirty) {
				t.Fatalf("Prepare() error = %v, want ErrTargetDirty", err)
			}
			if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("dirty prepare created state: %v", err)
			}
		})
	}
}

func TestRejectIsClosedDurableAndIdempotent(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Reject(view.ID, "free text"); !errors.Is(err, proposal.ErrInvalidReason) {
		t.Fatalf("Reject() error = %v, want ErrInvalidReason", err)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "decisions")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid rejection created a receipt directory")
	}
	receipt, err := workflow.Reject(view.ID, "incorrect")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := workflow.Reject(view.ID, "incorrect")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receipt, repeated) || receipt.Reason != "incorrect" || receipt.Status != "rejected" {
		t.Fatalf("Reject() receipts = %+v and %+v", receipt, repeated)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Reject() retained proposal payload")
	}
	review, err := workflow.Review(view.ID)
	if err != nil || review.Decision == nil || !reflect.DeepEqual(*review.Decision, receipt) {
		t.Fatalf("Review() after reject = %+v, %v", review, err)
	}
}

func TestApplyExactRevisionIsDurableAndIdempotent(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\n- timeout: tres segundos\n- owner: team-b\n"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ProposalID != view.ID || receipt.Revision != 1 || receipt.Status != "applied_unpublished" ||
		receipt.Target != view.Operation.Target || !reflect.DeepEqual(receipt.Baseline, view.Baseline) || receipt.PostState != view.PostState {
		t.Fatalf("Apply() receipt = %+v", receipt)
	}
	pagePath := filepath.Join(memoryRoot, "memories", "checkout-api.md")
	applied, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(applied), "timeout: tres segundos") || strings.Contains(string(applied), "timeout: cinco segundos") {
		t.Fatalf("Apply() contents = %q", applied)
	}
	repeated, err := workflow.Apply(context.Background(), view.ID, view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receipt, repeated) {
		t.Fatalf("repeated Apply() = %+v, want %+v", repeated, receipt)
	}
	afterRepeat, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(applied, afterRepeat) {
		t.Fatal("repeated Apply() rewrote memory")
	}
	stored, err := workflow.Applied(view.ID)
	if err != nil || !reflect.DeepEqual(stored, receipt) {
		t.Fatalf("Applied() = %+v, %v", stored, err)
	}
	review, err := workflow.Review(view.ID)
	if err != nil || review.Applied == nil || !reflect.DeepEqual(*review.Applied, receipt) {
		t.Fatalf("Review() after apply = %+v, %v", review, err)
	}
	assertMode(t, filepath.Join(stateRoot, "applies", view.ID+".json"), 0o600)
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Apply() retained proposal payload")
	}
}

func TestApplyRejectsDriftWrongRevisionAndTamperingWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, memoryRoot, stateRoot string, view proposal.View)
		want   error
	}{
		{
			name: "drift outside section",
			mutate: func(t *testing.T, memoryRoot, _ string, _ proposal.View) {
				path := filepath.Join(memoryRoot, "memories", "checkout-api.md")
				if err := os.WriteFile(path, []byte(initialPage+"\noutside\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: memory.ErrDrift,
		},
		{
			name: "tampered diff",
			mutate: func(t *testing.T, memoryRoot, _ string, view proposal.View) {
				editStoredProposal(t, memoryRoot, view.ID, func(stored *proposal.View) { stored.Diff += "tampered" })
			},
			want: proposal.ErrIntegrity,
		},
		{
			name: "sensitive replacement",
			mutate: func(t *testing.T, memoryRoot, _ string, view proposal.View) {
				editStoredProposal(t, memoryRoot, view.ID, func(stored *proposal.View) {
					stored.Operation.Replacement = "Authorization: Bearer secret"
				})
			},
			want: proposal.ErrIntegrity,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			memoryRoot := newMemoryRoot(t)
			stateRoot := filepath.Join(t.TempDir(), "state")
			workflow, err := proposal.New(memoryRoot, stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = workflow.Close() })
			view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, memoryRoot, stateRoot, view)
			before, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); !errors.Is(err, test.want) {
				t.Fatalf("Apply() error = %v, want %v", err, test.want)
			}
			after, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("failed Apply() changed memory")
			}
		})
	}

	memoryRoot := newMemoryRoot(t)
	workflow, err := proposal.New(memoryRoot, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision+1); !errors.Is(err, proposal.ErrWrongRevision) {
		t.Fatalf("Apply() wrong revision error = %v", err)
	}
}

func editStoredProposal(t *testing.T, memoryRoot, id string, edit func(*proposal.View)) {
	t.Helper()
	branch := "gpm/proposal/" + id
	proposalPath := "proposals/" + id + ".json"
	encoded := []byte(gitOutput(t, memoryRoot, "show", branch+":"+proposalPath))
	var view proposal.View
	if err := json.Unmarshal(encoded, &view); err != nil {
		t.Fatal(err)
	}
	edit(&view)
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	parent := gitOutput(t, memoryRoot, "rev-parse", branch+"^")
	oldCommit := gitOutput(t, memoryRoot, "rev-parse", branch)
	indexPath := filepath.Join(t.TempDir(), "index")
	environment := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	gitInput(t, memoryRoot, environment, nil, "read-tree", parent)
	blob := gitInput(t, memoryRoot, os.Environ(), encoded, "hash-object", "-w", "--stdin")
	gitInput(t, memoryRoot, environment, nil, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+proposalPath)
	tree := gitInput(t, memoryRoot, environment, nil, "write-tree")
	commit := gitInput(t, memoryRoot, os.Environ(), []byte("Tamper proposal\n"), "commit-tree", tree, "-p", parent)
	gitInput(t, memoryRoot, os.Environ(), nil, "update-ref", "refs/heads/"+branch, commit, oldCommit)
}

func gitInput(t *testing.T, root string, environment []string, input []byte, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = environment
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return strings.TrimSpace(string(output))
}

func TestApplyAndRejectRaceHasOneTerminalReceipt(t *testing.T) {
	for iteration := range 64 {
		memoryRoot := newMemoryRoot(t)
		stateRoot := filepath.Join(t.TempDir(), "state")
		workflow, err := proposal.New(memoryRoot, stateRoot)
		if err != nil {
			t.Fatal(err)
		}
		view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := workflow.Apply(context.Background(), view.ID, view.Revision)
			results <- err
		}()
		go func() {
			<-start
			_, err := workflow.Reject(view.ID, "incorrect")
			results <- err
		}()
		close(start)
		first, second := <-results, <-results
		if (first == nil) == (second == nil) || (first != nil && !errors.Is(first, proposal.ErrTerminal)) || (second != nil && !errors.Is(second, proposal.ErrTerminal)) {
			t.Fatalf("iteration %d race errors = %v, %v", iteration, first, second)
		}
		_, applyErr := os.Stat(filepath.Join(stateRoot, "applies", view.ID+".json"))
		_, rejectErr := os.Stat(filepath.Join(stateRoot, "decisions", view.ID+".json"))
		if (applyErr == nil) == (rejectErr == nil) {
			t.Fatalf("iteration %d terminal receipts: apply=%v reject=%v", iteration, applyErr, rejectErr)
		}
		contents, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
		if err != nil {
			t.Fatal(err)
		}
		if applyErr == nil && !strings.Contains(string(contents), "\ncurrent\n") {
			t.Fatalf("iteration %d apply won without post-state", iteration)
		}
		if rejectErr == nil && string(contents) != initialPage {
			t.Fatalf("iteration %d reject won with changed memory", iteration)
		}
		if err := workflow.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStartupReconcilesPostStateWithoutApplyReceipt(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := memory.New(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: view.Operation.Target.Path, SectionID: view.Operation.Target.SectionID},
		BaselineHash: view.Baseline.FileHash, Replacement: view.Operation.Replacement,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	receipt, err := restarted.Applied(view.ID)
	if err != nil || receipt.ProposalID != view.ID || receipt.Status != "applied_unpublished" {
		t.Fatalf("startup Applied() = %+v, %v", receipt, err)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "proposals", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("startup recovery retained proposal payload")
	}
}

func TestStartupDoesNotApplyBaselineOrRepairDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, memoryRoot string)
	}{
		{name: "baseline", mutate: func(*testing.T, string) {}},
		{name: "drift", mutate: func(t *testing.T, memoryRoot string) {
			path := filepath.Join(memoryRoot, "memories", "checkout-api.md")
			if err := os.WriteFile(path, []byte(initialPage+"\ndrift\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			memoryRoot := newMemoryRoot(t)
			stateRoot := filepath.Join(t.TempDir(), "state")
			workflow, err := proposal.New(memoryRoot, stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
			if err != nil {
				t.Fatal(err)
			}
			if err := workflow.Close(); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, memoryRoot)
			before, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
			if err != nil {
				t.Fatal(err)
			}
			restarted, err := proposal.New(memoryRoot, stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			if _, err := restarted.Applied(view.ID); !errors.Is(err, proposal.ErrNotApplied) {
				t.Fatalf("startup Applied() error = %v", err)
			}
			after, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("startup changed baseline or drift")
			}
		})
	}
}

func TestBaselineRestoredAfterInterruptedApplyRemainsEligible(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	workflow, err := proposal.New(memoryRoot, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := memory.New(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: view.Operation.Target.Path, SectionID: view.Operation.Target.SectionID},
		BaselineHash: view.Baseline.FileHash, Replacement: view.Operation.Replacement,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"), []byte(initialPage), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "\ncurrent\n") {
		t.Fatal("retry did not apply after baseline was restored")
	}
}

func TestStartupRetainsProposalBranchAfterDurableApplyReceipt(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if branch := gitOutput(t, memoryRoot, "rev-parse", "--verify", "gpm/proposal/"+view.ID); branch == "" {
		t.Fatal("startup removed durable proposal branch")
	}
}

func TestPublishPersistsReceiptAndIsIdempotent(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workflow.Close() })
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Apply(context.Background(), view.ID, view.Revision); err != nil {
		t.Fatal(err)
	}
	driver, err := gitpublisher.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	applied, err := workflow.Applied(view.ID)
	if err != nil {
		t.Fatal(err)
	}
	published, err := driver.Publish(context.Background(), applied)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := workflow.RecordPublication(context.Background(), published)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := workflow.RecordPublication(context.Background(), published)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receipt, repeated) || receipt.Status != "published" {
		t.Fatalf("Publish() receipts = %+v and %+v", receipt, repeated)
	}
	review, err := workflow.Review(view.ID)
	if err != nil || review.Published == nil || !reflect.DeepEqual(*review.Published, receipt) {
		t.Fatalf("Review() after publish = %+v, %v", review, err)
	}
	assertMode(t, filepath.Join(stateRoot, "publications", view.ID+".json"), 0o600)
	if _, err := os.Stat(filepath.Join(stateRoot, "publishing", view.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("durable publication receipt retained operation marker")
	}
}

func TestWorkflowRejectsStateRootResolvedInsideRepository(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	link := filepath.Join(t.TempDir(), "linked-memory")
	if err := os.Symlink(memoryRoot, link); err != nil {
		t.Fatal(err)
	}
	if _, err := proposal.New(memoryRoot, filepath.Join(link, "state")); err == nil {
		t.Fatal("New() accepted state root resolving inside memory root")
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("New() created state through symlinked parent")
	}
}

func TestReadOnlyReviewDoesNotCleanState(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	view, err := workflow.Prepare(context.Background(), allowedCandidate(t, "\ncurrent\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(stateRoot, ".git-push-memories-0123456789abcdef.tmp")
	if err := os.WriteFile(temporary, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := proposal.OpenReadOnly(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Review(view.ID); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(temporary); err != nil {
		t.Fatal("read-only review removed state")
	}
}

func allowedCandidate(t *testing.T, replacement string) intake.AllowedCandidate {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"target_path": "memories/checkout-api.md", "section_id": "checkout-api-operation",
		"replacement": replacement, "source_paths": []string{"services/checkout-api/service.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	return allowed
}

func allowedCreatePageCandidate(t *testing.T, body string) intake.AllowedCandidate {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"operation": "create_page", "target_path": "memories/parcel-dispatch.md", "title": "Parcel Dispatch",
		"body": body, "source_paths": []string{"services/parcel/service.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	return allowed
}

func allowedCreateSectionCandidate(t *testing.T, body string) intake.AllowedCandidate {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"operation": "create_section", "target_path": "memories/checkout-api.md", "heading": "Failure Modes",
		"body": body, "source_paths": []string{"services/checkout-api/service.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	return allowed
}

func newMemoryRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	pagePath := filepath.Join(root, "memories", "checkout-api.md")
	if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pagePath, []byte(initialPage), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "initial")
	return root
}

func newEmptyMemoryRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "commit", "--quiet", "--allow-empty", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return strings.TrimSpace(string(output))
}

func assertMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), mode)
	}
}

func treeFingerprint(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", relative, info.Mode())
		if info.Mode().IsRegular() {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash.Write(contents)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
