package publisher

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/proposal"
)

func TestPublishObservesEditsAroundBlobCapture(t *testing.T) {
	t.Run("before capture", func(t *testing.T) {
		root, receipt, _ := publisherFixture(t)
		publisher, err := New(root, filepath.Join(t.TempDir(), "state"))
		if err != nil {
			t.Fatal(err)
		}
		defer publisher.Close()
		publisher.fault = func(stage string) error {
			if stage == "before_capture" {
				return os.WriteFile(filepath.Join(root, receipt.Target.Path), []byte("edited before capture\n"), 0o644)
			}
			return nil
		}
		if _, err := publisher.Publish(context.Background(), receipt); !errors.Is(err, ErrDrift) {
			t.Fatalf("Publish() error = %v, want ErrDrift", err)
		}
		if got := gitText(t, root, "rev-parse", "HEAD"); got != receipt.Baseline.Head {
			t.Fatal("edit before capture advanced HEAD")
		}
	})

	t.Run("after capture", func(t *testing.T) {
		root, receipt, postState := publisherFixture(t)
		publisher, err := New(root, filepath.Join(t.TempDir(), "state"))
		if err != nil {
			t.Fatal(err)
		}
		defer publisher.Close()
		publisher.fault = func(stage string) error {
			if stage == "after_capture" {
				return os.WriteFile(filepath.Join(root, receipt.Target.Path), []byte("edited after capture\n"), 0o644)
			}
			return nil
		}
		publication, err := publisher.Publish(context.Background(), receipt)
		if err != nil {
			t.Fatal(err)
		}
		if got := gitOutputBytes(t, root, "show", publication.Commit+":"+receipt.Target.Path); string(got) != string(postState) {
			t.Fatalf("captured commit contents = %q, want %q", got, postState)
		}
		if got := gitOutputBytes(t, root, "diff", "--", receipt.Target.Path); len(got) == 0 {
			t.Fatal("edit after capture was not preserved as unstaged")
		}
	})
}

func TestPublishRecoversSameCommitAfterRefUpdateInterruption(t *testing.T) {
	root, receipt, _ := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	interrupted := errors.New("interrupted after ref update")
	first, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	first.fault = func(stage string) error {
		if stage == "after_ref_update" {
			return interrupted
		}
		return nil
	}
	if _, err := first.Publish(context.Background(), receipt); !errors.Is(err, interrupted) {
		t.Fatalf("Publish() error = %v, want interruption", err)
	}
	commit := gitText(t, root, "rev-parse", "HEAD")
	if commit == receipt.Baseline.Head {
		t.Fatal("interruption did not occur after ref update")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	publication, err := second.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Commit != commit || gitText(t, root, "rev-parse", "HEAD") != commit {
		t.Fatalf("recovery duplicated commit: receipt=%s HEAD=%s want=%s", publication.Commit, gitText(t, root, "rev-parse", "HEAD"), commit)
	}
	indexPath := gitText(t, root, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if _, err := os.Stat(indexPath + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("recovery retained owned index lock")
	}
}

func TestPublishRecoveryAfterIndexInstallReturnsSameReceipt(t *testing.T) {
	root, receipt, _ := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	interrupted := errors.New("interrupted before receipt")
	first, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	first.fault = func(stage string) error {
		if stage == "after_index_install" {
			return interrupted
		}
		return nil
	}
	if _, err := first.Publish(context.Background(), receipt); !errors.Is(err, interrupted) {
		t.Fatalf("Publish() error = %v, want interruption", err)
	}
	commit := gitText(t, root, "rev-parse", "HEAD")
	_ = first.Close()
	second, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	publication, err := second.Publish(context.Background(), receipt)
	if err != nil || publication.Commit != commit {
		t.Fatalf("recovered Publish() = %+v, %v", publication, err)
	}
}

func TestPublishNewTargetRecoversSameCommitAfterRefUpdate(t *testing.T) {
	root, receipt, postState := createdPublisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	interrupted := errors.New("interrupted after ref update")
	first, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	first.fault = func(stage string) error {
		if stage == "after_ref_update" {
			return interrupted
		}
		return nil
	}
	if _, err := first.Publish(context.Background(), receipt); !errors.Is(err, interrupted) {
		t.Fatalf("Publish() error = %v, want interruption", err)
	}
	commit := gitText(t, root, "rev-parse", "HEAD")
	_ = first.Close()
	second, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	publication, err := second.Publish(context.Background(), receipt)
	if err != nil || publication.Commit != commit {
		t.Fatalf("recovered Publish() = %+v, %v", publication, err)
	}
	if got := gitOutputBytes(t, root, "show", commit+":"+receipt.Target.Path); string(got) != string(postState) {
		t.Fatalf("recovered contents = %q", got)
	}
}

func TestPublishNewTargetPreservesEditAfterCapture(t *testing.T) {
	root, receipt, postState := createdPublisherFixture(t)
	publisher, err := New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	publisher.fault = func(stage string) error {
		if stage == "after_capture" {
			return os.WriteFile(filepath.Join(root, receipt.Target.Path), []byte("edited after capture\n"), 0o644)
		}
		return nil
	}
	publication, err := publisher.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if got := gitOutputBytes(t, root, "show", publication.Commit+":"+receipt.Target.Path); string(got) != string(postState) {
		t.Fatalf("captured contents = %q, want %q", got, postState)
	}
	if got := gitOutputBytes(t, root, "diff", "--", receipt.Target.Path); len(got) == 0 {
		t.Fatal("edit after capture was not preserved as unstaged")
	}
}

func TestPublishRetryReusesCommitCreatedBeforePreparedMarker(t *testing.T) {
	root, receipt, _ := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	interrupted := errors.New("interrupted after commit object")
	first, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	first.fault = func(stage string) error {
		if stage == "after_commit_object" {
			return interrupted
		}
		return nil
	}
	if _, err := first.Publish(context.Background(), receipt); !errors.Is(err, interrupted) {
		t.Fatalf("Publish() error = %v, want interruption", err)
	}
	unreachable := gitText(t, root, "fsck", "--unreachable", "--no-reflogs")
	createdCommit := ""
	for _, line := range strings.Split(unreachable, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "unreachable" && fields[1] == "commit" {
			createdCommit = fields[2]
		}
	}
	if createdCommit == "" {
		t.Fatalf("git fsck output = %q", unreachable)
	}
	_ = first.Close()
	second, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	publication, err := second.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Commit != createdCommit {
		t.Fatalf("retry commit = %s, want existing %s", publication.Commit, createdCommit)
	}
}

func TestPreparedMarkerRecoveryPreservesEditAfterCapture(t *testing.T) {
	root, receipt, postState := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	interrupted := errors.New("interrupted after prepared marker")
	first, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	first.fault = func(stage string) error {
		if stage == "after_marker_prepared" {
			if err := os.WriteFile(filepath.Join(root, receipt.Target.Path), []byte("edited after prepared capture\n"), 0o644); err != nil {
				return err
			}
			return interrupted
		}
		return nil
	}
	if _, err := first.Publish(context.Background(), receipt); !errors.Is(err, interrupted) {
		t.Fatalf("Publish() error = %v, want interruption", err)
	}
	_ = first.Close()
	second, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	publication, err := second.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if got := gitOutputBytes(t, root, "show", publication.Commit+":"+receipt.Target.Path); string(got) != string(postState) {
		t.Fatalf("recovered commit contents = %q, want captured %q", got, postState)
	}
	if got := gitOutputBytes(t, root, "diff", "--", receipt.Target.Path); len(got) == 0 {
		t.Fatal("recovery discarded edit made after capture")
	}
}

func TestGitAddIsBlockedWhilePublisherOwnsIndexLock(t *testing.T) {
	root, receipt, _ := publisherFixture(t)
	publisher, err := New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	publisher.fault = func(stage string) error {
		if stage == "before_capture" {
			close(entered)
			<-release
		}
		return nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := publisher.Publish(context.Background(), receipt)
		done <- err
	}()
	<-entered
	command := exec.Command("git", "-C", root, "add", "other.txt")
	if err := command.Run(); err == nil {
		close(release)
		t.Fatal("git add succeeded while publisher held index.lock")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentBranchAdvanceAbortsCASAndRecoversOwnedLock(t *testing.T) {
	root, receipt, _ := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	publisher, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	branch := gitText(t, root, "symbolic-ref", "HEAD")
	publisher.fault = func(stage string) error {
		if stage != "before_ref_update" {
			return nil
		}
		tree := gitText(t, root, "rev-parse", "HEAD^{tree}")
		command := exec.Command("git", "-C", root, "commit-tree", tree, "-p", receipt.Baseline.Head)
		command.Stdin = strings.NewReader("Concurrent commit\n")
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("concurrent commit: %w: %s", err, output)
		}
		commit := strings.TrimSpace(string(output))
		command = exec.Command("git", "-C", root, "update-ref", branch, commit, receipt.Baseline.Head)
		return command.Run()
	}
	if _, err := publisher.Publish(context.Background(), receipt); !errors.Is(err, ErrDrift) {
		t.Fatalf("Publish() error = %v, want ErrDrift", err)
	}
	if err := publisher.Close(); err != nil {
		t.Fatal(err)
	}
	recovery, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	if _, err := recovery.Publish(context.Background(), receipt); !errors.Is(err, ErrDrift) {
		t.Fatalf("recovery Publish() error = %v, want ErrDrift", err)
	}
	indexPath := gitText(t, root, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if _, err := os.Stat(indexPath + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("recovery retained its lock after failed CAS")
	}
}

func TestPublisherStartupRemovesOnlyControlledStateTemporaries(t *testing.T) {
	root, _, _ := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	publishing := filepath.Join(stateRoot, "publishing")
	if err := os.MkdirAll(publishing, 0o700); err != nil {
		t.Fatal(err)
	}
	controlled := []string{
		filepath.Join(stateRoot, ".git-push-memories-commit-index-123.tmp"),
		filepath.Join(stateRoot, ".git-push-memories-real-index-456.tmp"),
		filepath.Join(publishing, ".git-push-memories-marker-789.tmp"),
	}
	for _, path := range controlled {
		if err := os.WriteFile(path, []byte("incomplete"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(stateRoot, "keep.tmp")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	publisher, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	for _, path := range controlled {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("controlled temporary remains: %s", path)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("publisher removed unrelated state")
	}
}

func TestRecoveryDoesNotRemoveLockThatDoesNotMatchDurableMarker(t *testing.T) {
	root, receipt, _ := publisherFixture(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	publisher, err := New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	if err := privateDirectory(stateRoot); err != nil {
		t.Fatal(err)
	}
	indexPath := gitText(t, root, "rev-parse", "--path-format=absolute", "--git-path", "index")
	marker, err := newPublicationMarker(receipt, indexPath, "Test", "test@example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.writeMarker(marker, true); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("not owned by this marker")
	if err := os.WriteFile(indexPath+".lock", foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), receipt); !errors.Is(err, ErrLocked) {
		t.Fatalf("Publish() error = %v, want ErrLocked", err)
	}
	contents, err := os.ReadFile(indexPath + ".lock")
	if err != nil || string(contents) != string(foreign) {
		t.Fatalf("recovery changed foreign lock: %q, %v", contents, err)
	}
	if _, err := os.Stat(publisher.markerPath(receipt.ProposalID)); err != nil {
		t.Fatal("recovery removed evidence while foreign lock remained")
	}
}

func publisherFixture(t *testing.T) (string, proposal.ApplyReceipt, []byte) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := []byte("# Memory\n<!-- @id: memory -->\n\nold\n")
	postState := []byte("# Memory\n<!-- @id: memory -->\n\nnew\n")
	if err := os.WriteFile(filepath.Join(root, "memory.md"), baseline, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"add", "."}, {"commit", "--quiet", "-m", "initial"}} {
		gitCommand(t, root, args...)
	}
	head := gitText(t, root, "rev-parse", "HEAD")
	blob := gitText(t, root, "rev-parse", "HEAD:memory.md")
	if err := os.WriteFile(filepath.Join(root, "memory.md"), postState, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, proposal.ApplyReceipt{
		SchemaVersion: 1, ProposalID: "0123456789abcdef0123456789abcdef", Revision: 1, Status: "applied_unpublished", AppliedAt: time.Now().UTC(),
		MemoryRoot: root, Target: proposal.Target{Path: "memory.md", SectionID: "memory"},
		Baseline: proposal.Baseline{FileHash: publisherHash(baseline), Head: head, Blob: blob}, PostState: proposal.PostState{FileHash: publisherHash(postState)},
	}, postState
}

func createdPublisherFixture(t *testing.T) (string, proposal.ApplyReceipt, []byte) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.com"}, {"commit", "--quiet", "--allow-empty", "-m", "initial"}} {
		gitCommand(t, root, args...)
	}
	head := gitText(t, root, "rev-parse", "HEAD")
	postState := []byte("# Page\n<!-- @id: page -->\n\n## Overview\n<!-- @id: page-overview -->\nBody.\n")
	if err := os.WriteFile(filepath.Join(root, "page.md"), postState, 0o644); err != nil {
		t.Fatal(err)
	}
	absent := false
	return root, proposal.ApplyReceipt{
		SchemaVersion: 2, ProposalID: "0123456789abcdef0123456789abcdef", Revision: 1, Status: "applied_unpublished", AppliedAt: time.Now().UTC(),
		MemoryRoot: root, Target: proposal.Target{Path: "page.md", SectionID: "page-overview"},
		Baseline: proposal.Baseline{Exists: &absent, Head: head}, PostState: proposal.PostState{FileHash: publisherHash(postState)},
	}, postState
}

func publisherHash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return fmt.Sprintf("sha256:%x", sum)
}

func gitCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = gitOutputBytes(t, root, args...)
}

func gitText(t *testing.T, root string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(string(gitOutputBytes(t, root, args...)))
}

func gitOutputBytes(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return output
}
