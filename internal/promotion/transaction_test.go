package promotion

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func TestApplyBatchCommitsAllTargetsAndProposalsOnce(t *testing.T) {
	fixture := newBatchFixture(t)
	defer fixture.promoter.Close()

	result, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Commit == "" || gitOutput(t, fixture.root, "rev-parse", "HEAD") != result.Commit {
		t.Fatalf("ApplyBatch() = %+v", result)
	}
	if parent := gitOutput(t, fixture.root, "rev-parse", "HEAD^"); parent != fixture.parent {
		t.Fatalf("batch parent = %s, want %s", parent, fixture.parent)
	}
	changed := strings.Split(gitOutput(t, fixture.root, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"), "\n")
	want := []string{fixture.proposalPaths[0], fixture.proposalPaths[1], "memories/domains/first.md", "memories/domains/second.md"}
	slices.Sort(changed)
	slices.Sort(want)
	if !slices.Equal(changed, want) {
		t.Fatalf("batch paths = %v, want %v", changed, want)
	}
	for _, path := range append(append([]string{}, fixture.proposalPaths...), "promotion-transaction.json") {
		root := fixture.root
		if path == "promotion-transaction.json" {
			root = fixture.stateRoot
		}
		if _, err := os.Stat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("completed transaction retained %s", path)
		}
	}
	repeated, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil || repeated.Commit != result.Commit {
		t.Fatalf("repeated ApplyBatch() = %+v, %v; want commit %s", repeated, err, result.Commit)
	}
}

func TestApplyBatchRejectsRepeatedTargetsAndOversizedBatch(t *testing.T) {
	fixture := newBatchFixture(t)
	defer fixture.promoter.Close()
	repeated := []PreviewRequest{
		fixture.preview.Items[0].request,
		{Operation: OperationCreatePage, ProposalID: fixture.preview.Items[1].ProposalID, TargetPath: fixture.preview.Items[0].TargetPath},
	}
	if _, err := fixture.promoter.PreviewBatch(context.Background(), repeated); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("PreviewBatch(repeated target) error = %v", err)
	}
	requests := make([]PreviewRequest, MaxBatchItems+1)
	if _, err := fixture.promoter.PreviewBatch(context.Background(), requests); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("PreviewBatch(oversized) error = %v", err)
	}
}

func TestApplyBatchRevalidatesEvidenceImmediatelyBeforeWorktreeWrites(t *testing.T) {
	fixture := newBatchFixture(t)
	defer fixture.promoter.Close()
	calls := 0
	_, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, func() (string, error) {
		calls++
		if calls == 1 {
			return contentHash([]byte("reviewed evidence")), nil
		}
		return contentHash([]byte("changed evidence")), nil
	})
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("ApplyBatch() error = %v, want ErrDrift", err)
	}
	if calls != 2 {
		t.Fatalf("evidence validations = %d, want 2", calls)
	}
	if head := gitOutput(t, fixture.root, "rev-parse", "HEAD"); head != fixture.parent {
		t.Fatalf("evidence drift advanced HEAD to %s", head)
	}
	for _, target := range []string{"memories/domains/first.md", "memories/domains/second.md"} {
		if _, err := os.Stat(filepath.Join(fixture.root, target)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("evidence drift wrote %s", target)
		}
	}
	assertTransactionClean(t, fixture)
}

func TestBatchRecoveryReturnsToBaselineBeforeRefUpdate(t *testing.T) {
	fixture := newBatchFixture(t)
	interrupted := errors.New("interrupted after first worktree path")
	fixture.promoter.fault = func(stage string) error {
		if stage == "after_worktree_1" {
			return interrupted
		}
		return nil
	}
	if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
		t.Fatalf("ApplyBatch() error = %v, want interruption", err)
	}
	if head := gitOutput(t, fixture.root, "rev-parse", "HEAD"); head != fixture.parent {
		t.Fatalf("interrupted transaction advanced HEAD to %s", head)
	}
	_ = fixture.promoter.Close()

	recovery, err := New(fixture.cfg, fixture.root, fixture.branch)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if result, err := recovery.Recover(context.Background()); err != nil || result.Commit != "" {
		t.Fatalf("Recover() = %+v, %v", result, err)
	}
	for _, path := range []string{"memories/domains/first.md", "memories/domains/second.md"} {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("baseline recovery retained %s", path)
		}
	}
	for _, path := range fixture.proposalPaths {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); err != nil {
			t.Fatalf("baseline recovery removed %s: %v", path, err)
		}
	}
	assertTransactionClean(t, fixture)
}

func TestBatchRetryRecoversSameCommitAfterRefUpdate(t *testing.T) {
	fixture := newBatchFixture(t)
	interrupted := errors.New("interrupted after ref update")
	fixture.promoter.fault = func(stage string) error {
		if stage == "after_ref_update" {
			return interrupted
		}
		return nil
	}
	if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
		t.Fatalf("ApplyBatch() error = %v, want interruption", err)
	}
	commit := gitOutput(t, fixture.root, "rev-parse", "HEAD")
	if commit == fixture.parent {
		t.Fatal("interruption occurred before ref update")
	}
	_ = fixture.promoter.Close()

	recovery, err := New(fixture.cfg, fixture.root, fixture.branch)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	result, err := recovery.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Commit != commit || gitOutput(t, fixture.root, "rev-parse", "HEAD") != commit {
		t.Fatalf("recovery duplicated commit: result=%s HEAD=%s want=%s", result.Commit, gitOutput(t, fixture.root, "rev-parse", "HEAD"), commit)
	}
	for _, path := range fixture.proposalPaths {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("published recovery retained %s", path)
		}
	}
	assertTransactionClean(t, fixture)
}

func TestBatchRetryRecognizesCommitAfterMarkerRemoval(t *testing.T) {
	fixture := newBatchFixture(t)
	interrupted := errors.New("interrupted after marker removal")
	fixture.promoter.fault = func(stage string) error {
		if stage == "after_marker_remove" {
			return interrupted
		}
		return nil
	}
	if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
		t.Fatalf("ApplyBatch() error = %v, want interruption", err)
	}
	commit := gitOutput(t, fixture.root, "rev-parse", "HEAD")
	if _, err := os.Stat(filepath.Join(fixture.stateRoot, "promotion-transaction.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interruption occurred before marker removal")
	}
	_ = fixture.promoter.Close()
	retry, err := New(fixture.cfg, fixture.root, fixture.branch)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Close()
	result, err := retry.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil || result.Commit != commit || gitOutput(t, fixture.root, "rev-parse", "HEAD") != commit {
		t.Fatalf("retry = %+v, %v; want commit %s", result, err, commit)
	}
	assertTransactionClean(t, fixture)
}

func TestRecoveryNeverRemovesForeignIndexLock(t *testing.T) {
	fixture := newBatchFixture(t)
	interrupted := errors.New("interrupted after marker")
	fixture.promoter.fault = func(stage string) error {
		if stage == "after_marker" {
			return interrupted
		}
		return nil
	}
	if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
		t.Fatalf("ApplyBatch() error = %v, want interruption", err)
	}
	marker, err := os.ReadFile(filepath.Join(fixture.stateRoot, "promotion-transaction.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(marker), "First current fact") || strings.Contains(string(marker), "Second current fact") {
		t.Fatalf("transaction marker persisted proposal content: %s", marker)
	}
	_ = fixture.promoter.Close()
	indexPath := gitOutput(t, fixture.root, "rev-parse", "--path-format=absolute", "--git-path", "index")
	foreign := []byte("foreign lock\n")
	if err := os.WriteFile(indexPath+".lock", foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	recovery, err := New(fixture.cfg, fixture.root, fixture.branch)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if _, err := recovery.Recover(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("Recover() error = %v, want ErrLocked", err)
	}
	if got, err := os.ReadFile(indexPath + ".lock"); err != nil || string(got) != string(foreign) {
		t.Fatalf("foreign lock = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.stateRoot, "promotion-transaction.json")); err != nil {
		t.Fatal("recovery removed marker while foreign lock remained")
	}
}

func TestBatchFaultWindowsConverge(t *testing.T) {
	for _, test := range []struct {
		stage     string
		published bool
	}{
		{stage: "after_marker"},
		{stage: "after_index_lock"},
		{stage: "after_worktree_1"},
		{stage: "after_worktree_2"},
		{stage: "before_ref_update"},
		{stage: "after_ref_update", published: true},
		{stage: "after_index_install", published: true},
		{stage: "after_proposal_removal_1", published: true},
		{stage: "after_proposal_removal_2", published: true},
		{stage: "before_marker_remove", published: true},
	} {
		t.Run(test.stage, func(t *testing.T) {
			fixture := newBatchFixture(t)
			interrupted := errors.New("injected interruption")
			fixture.promoter.fault = func(stage string) error {
				if stage == test.stage {
					return interrupted
				}
				return nil
			}
			if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
				t.Fatalf("ApplyBatch() error = %v, want interruption", err)
			}
			interruptedHead := gitOutput(t, fixture.root, "rev-parse", "HEAD")
			_ = fixture.promoter.Close()
			recovery, err := New(fixture.cfg, fixture.root, fixture.branch)
			if err != nil {
				t.Fatal(err)
			}
			result, err := recovery.Recover(context.Background())
			_ = recovery.Close()
			if err != nil {
				t.Fatal(err)
			}
			if test.published {
				if interruptedHead == fixture.parent || result.Commit != interruptedHead || gitOutput(t, fixture.root, "rev-parse", "HEAD") != interruptedHead {
					t.Fatalf("published recovery = %+v, interrupted HEAD=%s", result, interruptedHead)
				}
				for _, path := range fixture.proposalPaths {
					if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("published recovery retained %s", path)
					}
				}
			} else {
				if interruptedHead != fixture.parent || result.Commit != "" || gitOutput(t, fixture.root, "rev-parse", "HEAD") != fixture.parent {
					t.Fatalf("baseline recovery = %+v, interrupted HEAD=%s", result, interruptedHead)
				}
				for _, path := range fixture.proposalPaths {
					if _, err := os.Stat(filepath.Join(fixture.root, path)); err != nil {
						t.Fatalf("baseline recovery removed %s", path)
					}
				}
			}
			assertTransactionClean(t, fixture)
		})
	}
}

func TestRecoveryWithoutMarkerRemovesOnlyControlledArtifacts(t *testing.T) {
	fixture := newBatchFixture(t)
	defer fixture.promoter.Close()
	for _, name := range []string{".promotion-marker-123.tmp", "keep.tmp"} {
		if err := os.WriteFile(filepath.Join(fixture.stateRoot, name), []byte("incomplete"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitDirectory := gitOutput(t, fixture.root, "rev-parse", "--absolute-git-dir")
	indexArtifact := filepath.Join(gitDirectory, ".git-push-memories-promotion-index-0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(indexArtifact, []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.promoter.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fixture.stateRoot, ".promotion-marker-123.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("controlled marker temporary remains")
	}
	if _, err := os.Stat(indexArtifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("controlled index artifact remains")
	}
	if _, err := os.Stat(filepath.Join(fixture.stateRoot, "keep.tmp")); err != nil {
		t.Fatal("recovery removed unrelated state")
	}
}

type batchFixture struct {
	root          string
	branch        string
	stateRoot     string
	cfg           config.Config
	promoter      *Promoter
	preview       BatchPreview
	parent        string
	proposalPaths []string
}

// addProposals files the requests through the proposals worktree and publishes them.
func addProposals(t *testing.T, m *workspacetest.Managed, requests []inbox.Request) []inbox.Proposal {
	t.Helper()
	box, err := inbox.NewWriter(m.Config.Workspace(), m.Config.Paths.Proposals, "proposals")
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	proposals := make([]inbox.Proposal, 0, len(requests))
	for _, request := range requests {
		proposal, err := box.Add(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		proposals = append(proposals, proposal)
	}
	if pushed, err := box.Publish(context.Background()); err != nil || !pushed {
		t.Fatalf("Publish() = %v, %v", pushed, err)
	}
	return proposals
}

// newBatchPromoter seeds main with seed, files the proposals, opens a release and
// returns a Promoter on the release worktree.
func newBatchPromoter(t *testing.T, seed map[string]string, requests []inbox.Request) (batchFixture, []inbox.Proposal) {
	t.Helper()
	m := workspacetest.Setup(t, seed)
	proposals := addProposals(t, m, requests)
	status := m.OpenRelease(t)
	promoter, err := New(m.Config, status.ReleaseRoot, status.Marker.ReleaseBranch)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(proposals))
	for index, proposal := range proposals {
		paths[index] = proposal.Path
	}
	return batchFixture{
		root: status.ReleaseRoot, branch: status.Marker.ReleaseBranch, stateRoot: m.Config.StateRoot, cfg: m.Config, promoter: promoter,
		parent: gitOutput(t, status.ReleaseRoot, "rev-parse", "HEAD"), proposalPaths: paths,
	}, proposals
}

func newBatchFixture(t *testing.T) batchFixture {
	t.Helper()
	fixture, proposals := newBatchPromoter(t, nil, []inbox.Request{
		{Title: "First memory", SuggestedAction: inbox.ActionCreate, Knowledge: "First current fact.", SourcePaths: []string{"source/first.go"}},
		{Title: "Second memory", SuggestedAction: inbox.ActionCreate, Knowledge: "Second current fact.", SourcePaths: []string{"source/second.go"}},
	})
	preview, err := fixture.promoter.PreviewBatch(context.Background(), []PreviewRequest{
		{Operation: OperationCreatePage, ProposalID: proposals[0].ID, TargetPath: "memories/domains/first.md"},
		{Operation: OperationCreatePage, ProposalID: proposals[1].ID, TargetPath: "memories/domains/second.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.preview = preview
	return fixture
}

func assertTransactionClean(t *testing.T, fixture batchFixture) {
	t.Helper()
	indexPath := gitOutput(t, fixture.root, "rev-parse", "--path-format=absolute", "--git-path", "index")
	for _, path := range []string{filepath.Join(fixture.stateRoot, "promotion-transaction.json"), indexPath + ".lock"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("transaction artifact remains: %s", path)
		}
	}
	gitDirectory := gitOutput(t, fixture.root, "rev-parse", "--absolute-git-dir")
	matches, err := filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-promotion-index-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("transaction index artifacts = %v, %v", matches, err)
	}
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	_ = gitOutput(t, root, arguments...)
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}
