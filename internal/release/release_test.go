package release_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/promotion"
	"github.com/Wuerike/git-push-memories/internal/release"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func TestOpenResumeAbort(t *testing.T) {
	ctx := context.Background()
	m := wt.Setup(t, nil)
	manager := release.New(m.Config)

	if _, err := manager.Open(ctx); !errors.Is(err, release.ErrNoProposals) {
		t.Fatalf("Open without proposals = %v", err)
	}
	first := m.Remember(t, "First")
	second := m.Remember(t, "Second")

	status, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Open || !status.Local || !status.PendingKnown || len(status.Pending) != 2 || len(status.Marker.Proposals) != 2 {
		t.Fatalf("Open() = %+v", status)
	}
	if !strings.HasPrefix(status.Marker.ReleaseBranch, "release/") || status.Marker.Base == "" {
		t.Fatalf("marker = %+v", status.Marker)
	}
	for _, id := range []string{first.ID, second.ID} {
		if !strings.Contains(strings.Join(status.Pending, ","), id) {
			t.Fatalf("pending lacks %s: %v", id, status.Pending)
		}
	}
	// The marker is the lock: on the remote proposals, but never inside the release worktree.
	if wt.Git(t, m.Remote, "show", "proposals:"+release.MarkerFile) == "" {
		t.Fatal("marker not pushed")
	}
	if _, err := os.Stat(filepath.Join(status.ReleaseRoot, release.MarkerFile)); err == nil {
		t.Fatal("marker leaked into the release worktree")
	}
	if branch := wt.Git(t, status.ReleaseRoot, "rev-parse", "--abbrev-ref", "HEAD"); branch != status.Marker.ReleaseBranch {
		t.Fatalf("release branch = %q", branch)
	}
	if _, err := manager.Open(ctx); !errors.Is(err, release.ErrActive) {
		t.Fatalf("second Open = %v, want ErrActive", err)
	}

	// A new proposal during the release stays out of it.
	third := m.Remember(t, "Third")
	again, err := manager.Resume(ctx)
	if err != nil || len(again.Pending) != 2 || strings.Contains(strings.Join(again.Pending, ","), third.ID) {
		t.Fatalf("Resume() = %+v, %v", again, err)
	}

	if err := manager.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(status.ReleaseRoot); err == nil {
		t.Fatal("release worktree survived abort")
	}
	if out := wt.Git(t, m.Config.Paths.Repo, "branch", "--list", "release/*"); out != "" {
		t.Fatalf("local release branches after abort: %q", out)
	}
	after, err := manager.Status(ctx)
	if err != nil || after.Open {
		t.Fatalf("Status after abort = %+v, %v", after, err)
	}
	if listing := wt.Git(t, m.Remote, "ls-tree", "--name-only", "proposals:proposals"); strings.Count(listing, "\n") != 2 {
		t.Fatalf("proposals after abort = %q", listing)
	}
	if _, err := manager.Open(ctx); err != nil {
		t.Fatalf("Open after abort = %v", err)
	}
}

func TestOpenCleansOrphanLocalRelease(t *testing.T) {
	ctx := context.Background()
	m := wt.Setup(t, nil)
	manager := release.New(m.Config)
	m.Remember(t, "First")
	first, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Another machine aborts: the marker disappears from the remote while this one keeps its release.
	wt.Git(t, m.Config.Paths.Proposals, "rm", "-q", release.MarkerFile)
	wt.Git(t, m.Config.Paths.Proposals, "commit", "-q", "-m", "close")
	wt.Git(t, m.Config.Paths.Proposals, "push", "-q", "origin", "proposals")
	second, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Marker.ReleaseBranch == first.Marker.ReleaseBranch {
		// opened_at has one-second resolution; the orphan must not survive either way
		t.Log("same second")
	}
	if out := wt.Git(t, m.Config.Paths.Repo, "branch", "--list", "release/*"); strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("local release branches = %q", out)
	}
}

func TestConsolidateBatchesInReleaseWorktree(t *testing.T) {
	ctx := context.Background()
	m := wt.Setup(t, nil)
	first := m.Remember(t, "First")
	second := m.Remember(t, "Second")
	manager := release.New(m.Config)
	status, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, branch, ok := manager.Active(ctx)
	if !ok || root != status.ReleaseRoot || branch != status.Marker.ReleaseBranch {
		t.Fatalf("Active() = %q, %q, %v", root, branch, ok)
	}
	promoter, err := promotion.New(m.Config, root, branch)
	if err != nil {
		t.Fatal(err)
	}
	defer promoter.Close()
	listed, err := promoter.List(ctx)
	if err != nil || len(listed.Proposals) != 2 {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
	// Two batches: the second must see the release HEAD left by the first without drift.
	for index, proposal := range []string{first.ID, second.ID} {
		preview, err := promoter.PreviewBatch(ctx, []promotion.PreviewRequest{{
			Operation: promotion.OperationCreatePage, ProposalID: proposal, TargetPath: []string{"memories/domains/first.md", "memories/domains/second.md"}[index],
		}})
		if err != nil {
			t.Fatalf("PreviewBatch %d: %v", index, err)
		}
		if _, err := promoter.ApplyBatch(ctx, preview, nil); err != nil {
			t.Fatalf("ApplyBatch %d: %v", index, err)
		}
	}
	after, err := manager.Status(ctx)
	if err != nil || len(after.Pending) != 0 {
		t.Fatalf("Status after consolidation = %+v, %v", after, err)
	}
	for _, page := range []string{"first", "second"} {
		if _, err := os.Stat(filepath.Join(root, "memories", "domains", page+".md")); err != nil {
			t.Fatal(err)
		}
	}
	if count := wt.Git(t, root, "rev-list", "--count", status.Marker.Base+"..HEAD"); count != "2" {
		t.Fatalf("release commits = %s, want 2", count)
	}
	if _, err := os.Stat(filepath.Join(m.Config.StateRoot, "promotion-transaction.json")); err == nil {
		t.Fatal("transaction marker survived")
	}
}
