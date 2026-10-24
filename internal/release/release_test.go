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

// consolidated returns a fixture with a release whose proposals were all promoted.
func consolidated(t *testing.T) (*wt.Managed, *wt.GH, *release.Manager, release.Status) {
	t.Helper()
	ctx := context.Background()
	m := wt.Setup(t, nil)
	gh := wt.FakeGH(t)
	first := m.Remember(t, "First")
	second := m.Remember(t, "Second")
	manager := release.New(m.Config)
	status, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	promoter, err := promotion.New(m.Config, status.ReleaseRoot, status.Marker.ReleaseBranch)
	if err != nil {
		t.Fatal(err)
	}
	defer promoter.Close()
	preview, err := promoter.PreviewBatch(ctx, []promotion.PreviewRequest{
		{Operation: promotion.OperationCreatePage, ProposalID: first.ID, TargetPath: "memories/domains/first.md"},
		{Operation: promotion.OperationDiscard, ProposalID: second.ID, Reason: promotion.DiscardNotRelevant},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := promoter.ApplyBatch(ctx, preview, nil); err != nil {
		t.Fatal(err)
	}
	return m, gh, manager, status
}

// merge replays a PR merge on the remote main the way each GitHub method would.
func merge(t *testing.T, m *wt.Managed, status release.Status, method string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "merger")
	wt.Git(t, filepath.Dir(clone), "clone", "-q", m.Remote, clone)
	branch := status.Marker.ReleaseBranch
	wt.Git(t, clone, "checkout", "-q", "main")
	switch method {
	case "squash":
		wt.Git(t, clone, "merge", "-q", "--squash", "origin/"+branch)
		wt.Git(t, clone, "commit", "-q", "-m", "squash release")
	case "merge":
		wt.Git(t, clone, "merge", "-q", "--no-ff", "-m", "merge release", "origin/"+branch)
	case "rebase":
		wt.Git(t, clone, "merge", "-q", "--ff-only", "origin/"+branch)
	}
	wt.Git(t, clone, "push", "-q", "origin", "main")
	return wt.Git(t, clone, "rev-parse", "HEAD")
}

func TestSubmitThenFinalizeConvergesForEveryMergeMethod(t *testing.T) {
	for _, method := range []string{"squash", "merge", "rebase"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			m, gh, manager, status := consolidated(t)
			url, err := manager.Submit(ctx)
			if err != nil || url == "" {
				t.Fatalf("Submit() = %q, %v", url, err)
			}
			if again, err := manager.Submit(ctx); err != nil || again != url {
				t.Fatalf("second Submit() = %q, %v; want the same PR", again, err)
			}
			// Open PR: nothing to finalize yet.
			if outcome, err := manager.Finalize(ctx, false); err != nil || outcome != release.NotMerged {
				t.Fatalf("Finalize() with open PR = %v, %v", outcome, err)
			}
			oid := merge(t, m, status, method)
			gh.SetPR(t, "MERGED", oid)
			// A late /remember during the release must survive.
			late := m.Remember(t, "Late")
			if outcome, err := manager.Finalize(ctx, false); err != nil || outcome != release.Finalized {
				t.Fatalf("Finalize() = %v, %v", outcome, err)
			}
			listing := wt.Git(t, m.Remote, "ls-tree", "-r", "--name-only", "proposals")
			if strings.Contains(listing, release.MarkerFile) {
				t.Fatalf("marker survived: %s", listing)
			}
			lines := strings.Split(listing, "\n")
			var proposals []string
			for _, line := range lines {
				if strings.HasPrefix(line, "proposals/") {
					proposals = append(proposals, line)
				}
			}
			if len(proposals) != 1 || !strings.Contains(proposals[0], late.ID) {
				t.Fatalf("remote proposals after finalize = %v, want only the late one", proposals)
			}
			if !strings.Contains(listing, "memories/domains/first.md") {
				t.Fatalf("proposals lacks the new memory: %s", listing)
			}
			if out := wt.Git(t, m.Config.Paths.Repo, "branch", "--list", "release/*"); out != "" {
				t.Fatalf("release branch survived: %q", out)
			}
			if _, err := os.Stat(filepath.Join(m.Config.Paths.Read, "memories", "domains", "first.md")); err != nil {
				t.Fatalf("read worktree not refreshed: %v", err)
			}
			// The net diff of the release against main never contains proposals/ or the marker.
			diff := wt.Git(t, m.Remote, "diff", "--name-only", "main~1", "main")
			if strings.Contains(diff, "release.lock.json") {
				t.Fatalf("marker reached main: %s", diff)
			}
			if _, err := manager.Open(ctx); err != nil {
				t.Fatalf("Open after finalize = %v", err)
			}
		})
	}
}

func TestOpenFinalizesMergedReleaseFirst(t *testing.T) {
	ctx := context.Background()
	m, gh, manager, status := consolidated(t)
	if _, err := manager.Submit(ctx); err != nil {
		t.Fatal(err)
	}
	m.Remember(t, "Next")
	if _, err := manager.Open(ctx); !errors.Is(err, release.ErrActive) {
		t.Fatalf("Open with open PR = %v, want ErrActive", err)
	}
	gh.SetPR(t, "MERGED", merge(t, m, status, "squash"))
	next, err := manager.Open(ctx)
	if err != nil || next.Marker.OpenedAt == status.Marker.OpenedAt && next.Marker.Base == status.Marker.Base || len(next.Pending) != 1 {
		t.Fatalf("Open() = %+v, %v", next, err)
	}
}

func TestAbortRefusesMergedPullRequestAndFinishNeedsNoGH(t *testing.T) {
	ctx := context.Background()
	m, gh, manager, status := consolidated(t)
	if _, err := manager.Submit(ctx); err != nil {
		t.Fatal(err)
	}
	gh.SetPR(t, "MERGED", merge(t, m, status, "squash"))
	if err := manager.Abort(ctx); !errors.Is(err, release.ErrMerged) {
		t.Fatalf("Abort() = %v, want ErrMerged", err)
	}
	t.Setenv("PATH", "/usr/bin:/bin") // gh is unavailable; --finish vouches for the merge
	if outcome, err := manager.Finalize(ctx, true); err != nil || outcome != release.Finalized {
		t.Fatalf("Finalize(force) = %v, %v", outcome, err)
	}
}

func TestSubmitRequiresEveryProposalConsumed(t *testing.T) {
	ctx := context.Background()
	m := wt.Setup(t, nil)
	wt.FakeGH(t)
	m.Remember(t, "First")
	manager := release.New(m.Config)
	if _, err := manager.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(ctx); !errors.Is(err, release.ErrPending) {
		t.Fatalf("Submit() = %v, want ErrPending", err)
	}
}

func TestClosedPullRequestGetsANewOneOnSubmit(t *testing.T) {
	ctx := context.Background()
	_, gh, manager, _ := consolidated(t)
	first, err := manager.Submit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gh.SetPR(t, "CLOSED", "")
	if outcome, err := manager.Finalize(ctx, false); !errors.Is(err, release.ErrClosedUnmerged) || outcome != release.NotMerged {
		t.Fatalf("Finalize(closed) = %v, %v", outcome, err)
	}
	second, err := manager.Submit(ctx)
	if err != nil || second == first {
		t.Fatalf("Submit() after close = %q, %v (first %q)", second, err, first)
	}
}

func TestAbortRefusesMergedPullRequestAfterForgeDeletedBranch(t *testing.T) {
	ctx := context.Background()
	m, gh, manager, status := consolidated(t)
	if _, err := manager.Submit(ctx); err != nil {
		t.Fatal(err)
	}
	gh.SetPR(t, "MERGED", merge(t, m, status, "squash"))
	wt.Git(t, m.Remote, "branch", "-D", status.Marker.ReleaseBranch) // "delete head branch on merge"
	if err := manager.Abort(ctx); !errors.Is(err, release.ErrMerged) {
		t.Fatalf("Abort() = %v, want ErrMerged", err)
	}
}
