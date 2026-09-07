package publisher_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/proposal"
	"github.com/Wuerike/git-push-memories/internal/publisher"
)

func TestPublishCommitsOnlyAppliedTargetAndPreservesOtherWork(t *testing.T) {
	root, receipt := newAppliedRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	stagedBefore := gitOutput(t, root, "ls-files", "--stage", "staged.txt")
	gitRun(t, root, "update-index", "--assume-unchanged", "unstaged.txt")
	flagsBefore := gitOutput(t, root, "ls-files", "-v", "staged.txt", "unstaged.txt")
	debugBefore := gitOutput(t, root, "ls-files", "--debug", "staged.txt", "unstaged.txt")
	unstagedPath := filepath.Join(root, "unstaged.txt")
	unstagedBefore, err := os.ReadFile(unstagedPath)
	if err != nil {
		t.Fatal(err)
	}

	pub, err := publisher.New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	publication, err := pub.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Status != "published" || publication.ProposalID != receipt.ProposalID || publication.Parent != receipt.Baseline.Head || publication.Commit == "" {
		t.Fatalf("Publish() receipt = %+v", publication)
	}
	if got := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD")); got != publication.Commit {
		t.Fatalf("HEAD = %s, want %s", got, publication.Commit)
	}
	changed := strings.TrimSpace(gitOutput(t, root, "diff-tree", "--no-commit-id", "--name-only", "-r", publication.Commit))
	if changed != receipt.Target.Path {
		t.Fatalf("commit paths = %q, want %q", changed, receipt.Target.Path)
	}
	message := gitOutput(t, root, "show", "-s", "--format=%B", publication.Commit)
	trailer := fmt.Sprintf("Git-Push-Memories-Proposal: %s/%d", receipt.ProposalID, receipt.Revision)
	if !strings.Contains(message, trailer) {
		t.Fatalf("commit message = %q, want trailer %q", message, trailer)
	}
	if got := gitOutput(t, root, "diff", "--cached", "--", receipt.Target.Path); got != "" {
		t.Fatalf("target remains staged: %q", got)
	}
	if got := gitOutput(t, root, "ls-files", "--stage", "staged.txt"); got != stagedBefore {
		t.Fatalf("unrelated staged entry changed: before %q, after %q", stagedBefore, got)
	}
	if got := gitOutput(t, root, "ls-files", "-v", "staged.txt", "unstaged.txt"); got != flagsBefore {
		t.Fatalf("unrelated index flags changed: before %q, after %q", flagsBefore, got)
	}
	if got := gitOutput(t, root, "ls-files", "--debug", "staged.txt", "unstaged.txt"); got != debugBefore {
		t.Fatalf("unrelated index entries changed: before %q, after %q", debugBefore, got)
	}
	unstagedAfter, err := os.ReadFile(unstagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unstagedAfter) != string(unstagedBefore) {
		t.Fatal("unrelated unstaged file changed")
	}
	repeated, err := pub.Publish(context.Background(), receipt)
	if err != nil || !reflect.DeepEqual(repeated, publication) {
		t.Fatalf("repeated Publish() = %+v, %v; want %+v", repeated, err, publication)
	}
}

func TestPublishCommitsNewAppliedTargetFromAbsentBaseline(t *testing.T) {
	root, receipt, postState := newCreatedRepository(t)
	stagedBefore := gitOutput(t, root, "ls-files", "--stage", "staged.txt")
	unstagedBefore, err := os.ReadFile(filepath.Join(root, "unstaged.txt"))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := publisher.New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	publication, err := pub.Publish(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Status != "published" || publication.Parent != receipt.Baseline.Head || publication.Commit == "" {
		t.Fatalf("Publish() receipt = %+v", publication)
	}
	if got := strings.TrimSpace(gitOutput(t, root, "diff-tree", "--no-commit-id", "--name-status", "-r", publication.Commit)); got != "A\tparcel-dispatch.md" {
		t.Fatalf("commit change = %q", got)
	}
	if got := gitOutput(t, root, "show", publication.Commit+":"+receipt.Target.Path); got != string(postState) {
		t.Fatalf("published contents = %q, want %q", got, postState)
	}
	if got := gitOutput(t, root, "diff", "--cached", "--", receipt.Target.Path); got != "" {
		t.Fatalf("new target remains staged: %q", got)
	}
	if got := gitOutput(t, root, "ls-files", "--stage", "staged.txt"); got != stagedBefore {
		t.Fatalf("unrelated staged entry changed: before %q, after %q", stagedBefore, got)
	}
	unstagedAfter, err := os.ReadFile(filepath.Join(root, "unstaged.txt"))
	if err != nil || !bytes.Equal(unstagedAfter, unstagedBefore) {
		t.Fatal("unrelated unstaged file changed")
	}
	repeated, err := pub.Publish(context.Background(), receipt)
	if err != nil || !reflect.DeepEqual(repeated, publication) {
		t.Fatalf("repeated Publish() = %+v, %v; want %+v", repeated, err, publication)
	}
}

func TestPublishNewTargetRejectsSnapshotDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root string, receipt proposal.ApplyReceipt)
	}{
		{name: "target staged", mutate: func(t *testing.T, root string, receipt proposal.ApplyReceipt) {
			gitRun(t, root, "add", receipt.Target.Path)
		}},
		{name: "post-state changed", mutate: func(t *testing.T, root string, receipt proposal.ApplyReceipt) {
			if err := os.WriteFile(filepath.Join(root, receipt.Target.Path), []byte("changed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "head changed", mutate: func(t *testing.T, root string, _ proposal.ApplyReceipt) {
			gitRun(t, root, "commit", "--quiet", "-m", "advance unrelated staging")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, receipt, _ := newCreatedRepository(t)
			test.mutate(t, root, receipt)
			headBefore := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
			pub, err := publisher.New(root, filepath.Join(t.TempDir(), "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer pub.Close()
			if _, err := pub.Publish(context.Background(), receipt); !errors.Is(err, publisher.ErrDrift) {
				t.Fatalf("Publish() error = %v, want ErrDrift", err)
			}
			if got := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD")); got != headBefore {
				t.Fatalf("failed publish changed HEAD from %s to %s", headBefore, got)
			}
		})
	}
}

func TestPublishNewTargetRejectsNonCanonicalCreateReceipt(t *testing.T) {
	for _, mutate := range []func(*proposal.ApplyReceipt){
		func(receipt *proposal.ApplyReceipt) { receipt.Target.Path = "nested/page.md" },
		func(receipt *proposal.ApplyReceipt) { receipt.Target.Path = "Page.md" },
		func(receipt *proposal.ApplyReceipt) { receipt.Target.SectionID = "wrong-overview" },
	} {
		root, receipt, _ := newCreatedRepository(t)
		mutate(&receipt)
		pub, err := publisher.New(root, filepath.Join(t.TempDir(), "state"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pub.Publish(context.Background(), receipt); !errors.Is(err, proposal.ErrIntegrity) {
			t.Fatalf("Publish() error = %v, want ErrIntegrity", err)
		}
		_ = pub.Close()
	}
}

func TestPublishRejectsSnapshotDriftBeforeAdvancingBranch(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root string, receipt proposal.ApplyReceipt)
	}{
		{name: "target staged", mutate: func(t *testing.T, root string, receipt proposal.ApplyReceipt) {
			gitRun(t, root, "add", receipt.Target.Path)
		}},
		{name: "post-state changed", mutate: func(t *testing.T, root string, receipt proposal.ApplyReceipt) {
			if err := os.WriteFile(filepath.Join(root, receipt.Target.Path), []byte("changed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "head changed", mutate: func(t *testing.T, root string, _ proposal.ApplyReceipt) {
			gitRun(t, root, "commit", "--quiet", "-m", "advance unrelated staging")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, receipt := newAppliedRepository(t)
			test.mutate(t, root, receipt)
			headBefore := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
			indexPath := strings.TrimSpace(gitOutput(t, root, "rev-parse", "--path-format=absolute", "--git-path", "index"))
			indexBefore, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			pub, err := publisher.New(root, filepath.Join(t.TempDir(), "state"))
			if err != nil {
				t.Fatal(err)
			}
			defer pub.Close()
			if _, err := pub.Publish(context.Background(), receipt); !errors.Is(err, publisher.ErrDrift) {
				t.Fatalf("Publish() error = %v, want ErrDrift", err)
			}
			if got := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD")); got != headBefore {
				t.Fatalf("HEAD changed from %s to %s", headBefore, got)
			}
			indexAfter, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(indexBefore, indexAfter) {
				t.Fatal("failed publication changed real index")
			}
			if _, err := os.Stat(indexPath + ".lock"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed publication retained its index lock")
			}
		})
	}
}

func TestPublishDoesNotRemoveAnotherProcessIndexLock(t *testing.T) {
	root, receipt := newAppliedRepository(t)
	indexPath := strings.TrimSpace(gitOutput(t, root, "rev-parse", "--path-format=absolute", "--git-path", "index"))
	foreign := []byte("foreign lock")
	if err := os.WriteFile(indexPath+".lock", foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	pub, err := publisher.New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	if _, err := pub.Publish(context.Background(), receipt); !errors.Is(err, publisher.ErrLocked) {
		t.Fatalf("Publish() error = %v, want ErrLocked", err)
	}
	got, err := os.ReadFile(indexPath + ".lock")
	if err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("foreign lock changed: %q, %v", got, err)
	}
}

func newAppliedRepository(t *testing.T) (string, proposal.ApplyReceipt) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	target := "memory.md"
	baseline := []byte("# Memory\n<!-- @id: memory -->\n\nold\n")
	postState := []byte("# Memory\n<!-- @id: memory -->\n\nnew\n")
	for path, contents := range map[string][]byte{target: baseline, "staged.txt": []byte("staged old\n"), "unstaged.txt": []byte("unstaged old\n")} {
		if err := os.WriteFile(filepath.Join(root, path), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, root, "init", "--quiet")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "user.email", "test@example.com")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "--quiet", "-m", "initial")
	head := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	blob := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD:"+target))
	if err := os.WriteFile(filepath.Join(root, target), postState, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(root, "unstaged.txt"), []byte("unstaged new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, proposal.ApplyReceipt{
		SchemaVersion: 1, ProposalID: "0123456789abcdef0123456789abcdef", Revision: 1, Status: "applied_unpublished", AppliedAt: time.Now().UTC(),
		MemoryRoot: root, Target: proposal.Target{Path: target, SectionID: "memory"},
		Baseline: proposal.Baseline{FileHash: hash(baseline), Head: head, Blob: blob}, PostState: proposal.PostState{FileHash: hash(postState)},
	}
}

func newCreatedRepository(t *testing.T) (string, proposal.ApplyReceipt, []byte) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "--quiet")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unstaged.txt"), []byte("unstaged old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "--quiet", "-m", "initial")
	head := strings.TrimSpace(gitOutput(t, root, "rev-parse", "HEAD"))
	postState := []byte("# Parcel Dispatch\n<!-- @id: parcel-dispatch -->\n\n## Overview\n<!-- @id: parcel-dispatch-overview -->\nCurrent state.\n")
	if err := os.WriteFile(filepath.Join(root, "parcel-dispatch.md"), postState, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "staged.txt"), []byte("staged new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(root, "unstaged.txt"), []byte("unstaged new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	absent := false
	return root, proposal.ApplyReceipt{
		SchemaVersion: 2, ProposalID: "0123456789abcdef0123456789abcdef", Revision: 1, Status: "applied_unpublished", AppliedAt: time.Now().UTC(),
		MemoryRoot: root, Target: proposal.Target{Path: "parcel-dispatch.md", SectionID: "parcel-dispatch-overview"},
		Baseline: proposal.Baseline{Exists: &absent, Head: head}, PostState: proposal.PostState{FileHash: hash(postState)},
	}, postState
}

func hash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return fmt.Sprintf("sha256:%x", sum)
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = gitOutput(t, root, args...)
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}
