package proposal_test

import (
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
)

const initialPage = `# checkout-api
<!-- @id: checkout-api -->

## Operacao
<!-- @id: checkout-api-operation -->

- timeout: cinco segundos
- owner: team-a
`

func TestPreparePersistsReviewableProposalWithoutChangingMemoryOrGit(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	codeRoot := newCodeRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	before := treeFingerprint(t, memoryRoot)
	workflow, err := proposal.New(memoryRoot, codeRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	allowed := allowedCandidate(t, "\n- timeout: tres segundos\n- owner: team-b\n")
	view, err := workflow.Prepare(context.Background(), allowed)
	if err != nil {
		t.Fatal(err)
	}
	if view.SchemaVersion != 1 || len(view.ID) != 32 || view.Revision != 1 || view.Status != "pending" {
		t.Fatalf("Prepare() identity = %+v", view)
	}
	if _, err := workflow.Applied(view.ID); !errors.Is(err, proposal.ErrNotApplied) {
		t.Fatalf("Applied() error = %v, want ErrNotApplied", err)
	}
	if view.Operation.Type != "replace_section_content" || view.Operation.Target.Path != "services/checkout-api.md" || view.Operation.Target.SectionID != "checkout-api-operation" {
		t.Fatalf("Prepare() operation = %+v", view.Operation)
	}
	if view.Baseline.FileHash == view.PostState.FileHash || view.Baseline.Head == "" || view.Baseline.Blob == "" {
		t.Fatalf("Prepare() states = baseline %+v, post-state %+v", view.Baseline, view.PostState)
	}
	if !strings.Contains(view.Diff, "--- a/services/checkout-api.md") || !strings.Contains(view.Diff, "\n-- timeout: cinco segundos") || !strings.Contains(view.Diff, "\n+- timeout: tres segundos") {
		t.Fatalf("Prepare() diff = %q", view.Diff)
	}
	assertMode(t, stateRoot, 0o700)
	assertMode(t, filepath.Join(stateRoot, "proposals"), 0o700)
	assertMode(t, filepath.Join(stateRoot, "proposals", view.ID+".json"), 0o600)
	if got := treeFingerprint(t, memoryRoot); got != before {
		t.Fatalf("Prepare() changed memory/Git: before %s, after %s", before, got)
	}
	if err := workflow.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := proposal.New(memoryRoot, codeRoot, stateRoot)
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

func TestPrepareRejectsDirtyTarget(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(fmt.Sprintf("staged=%v", staged), func(t *testing.T) {
			memoryRoot := newMemoryRoot(t)
			pagePath := filepath.Join(memoryRoot, "services", "checkout-api.md")
			if err := os.WriteFile(pagePath, []byte(strings.Replace(initialPage, "cinco", "quatro", 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if staged {
				runGit(t, memoryRoot, "add", "services/checkout-api.md")
			}
			stateRoot := filepath.Join(t.TempDir(), "state")
			codeRoot := newCodeRoot(t)
			workflow, err := proposal.New(memoryRoot, codeRoot, stateRoot)
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
	codeRoot := newCodeRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, codeRoot, stateRoot)
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

func TestWorkflowRejectsStateRootResolvedInsideRepository(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	codeRoot := newCodeRoot(t)
	link := filepath.Join(t.TempDir(), "linked-memory")
	if err := os.Symlink(memoryRoot, link); err != nil {
		t.Fatal(err)
	}
	if _, err := proposal.New(memoryRoot, codeRoot, filepath.Join(link, "state")); err == nil {
		t.Fatal("New() accepted state root resolving inside memory root")
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("New() created state through symlinked parent")
	}
}

func TestReadOnlyReviewDoesNotCleanState(t *testing.T) {
	memoryRoot := newMemoryRoot(t)
	codeRoot := newCodeRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	workflow, err := proposal.New(memoryRoot, codeRoot, stateRoot)
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
	temporary := filepath.Join(stateRoot, "proposals", ".git-push-memories-0123456789abcdef.tmp")
	if err := os.WriteFile(temporary, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := proposal.OpenReadOnly(memoryRoot, codeRoot, stateRoot)
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
		"target_path": "services/checkout-api.md", "section_id": "checkout-api-operation",
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

func newMemoryRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	pagePath := filepath.Join(root, "services", "checkout-api.md")
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

func newCodeRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "code")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
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
