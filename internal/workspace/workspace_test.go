package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func TestNewPromotionStartsAtExactBase(t *testing.T) {
	w, remote := setup(t)
	base := wt.Git(t, w.Paths.Repo, "rev-parse", "origin/main")
	wt.Seed(t, remote, "main", map[string]string{"memories/later.md": "Later.\n"})
	if err := w.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	prepared, err := w.NewPromotion(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if got := wt.Git(t, prepared.Worktree, "rev-parse", "HEAD"); got != base {
		t.Fatalf("HEAD = %s, want pinned base %s", got, base)
	}
	if branch := wt.Git(t, prepared.Worktree, "branch", "--show-current"); branch != prepared.Branch || !strings.HasPrefix(branch, "promote/") {
		t.Fatalf("branch = %q, preparation = %+v", branch, prepared)
	}
	if _, err := os.Stat(filepath.Join(prepared.Worktree, "memories", "later.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("later memory leaked into pinned worktree: %v", err)
	}
	if refs := wt.Git(t, remote, "for-each-ref", "--format=%(refname)", "refs/heads/promote/"); refs != "" {
		t.Fatalf("preparation pushed a branch: %s", refs)
	}
}

func TestPromotionInputsAndPublicationRecordStayOutsideWorktree(t *testing.T) {
	w, _ := setup(t)
	base := wt.Git(t, w.Paths.Repo, "rev-parse", "origin/main")
	prepared, err := w.NewPromotion(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if w.Paths.Promotions != filepath.Join(w.Paths.State, "promotions") || w.Paths.Published != filepath.Join(w.Paths.State, "promotion-published.json") {
		t.Fatalf("promotion layout = %+v", w.Paths)
	}
	if filepath.Dir(prepared.InputsRoot) != filepath.Dir(prepared.Worktree) || filepath.Base(prepared.InputsRoot) != "inputs" {
		t.Fatalf("inputs must be a sibling of worktree: %+v", prepared)
	}
	for _, outside := range []string{prepared.InputsRoot, w.Paths.Published} {
		relative, err := filepath.Rel(prepared.Worktree, outside)
		if err != nil || !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("auxiliary path %s is not outside %s", outside, prepared.Worktree)
		}
	}
	if dirty := wt.Git(t, prepared.Worktree, "status", "--porcelain"); dirty != "" {
		t.Fatalf("new worktree is dirty: %q", dirty)
	}
}

func setup(t *testing.T) (*workspace.Workspace, string) {
	t.Helper()
	wt.Env(t)
	remote := wt.Remote(t)
	wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{}\n", "memories/a.md": "a\n"})
	w, err := workspace.Open(remote)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := w.Clone(ctx); err != nil {
		t.Fatal(err)
	}
	return w, remote
}

func TestHappyPath(t *testing.T) {
	ctx := context.Background()
	w, remote := setup(t)

	if err := w.EnsureRead(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.EnsureWorktree(ctx, w.Paths.Proposals, workspace.ProposalsBranch, "origin/main"); err != nil {
		t.Fatal(err)
	}
	if refs := wt.Git(t, remote, "for-each-ref", "refs/heads/proposals"); refs != "" {
		t.Fatalf("unexpected remote proposals branch: %s", refs)
	}
	os.MkdirAll(filepath.Join(w.Paths.Proposals, "proposals"), 0o755)
	os.WriteFile(filepath.Join(w.Paths.Proposals, "proposals", "1.md"), []byte("p1\n"), 0o644)
	if err := workspace.Normalize(ctx, w.Paths.Proposals); !errors.Is(err, workspace.ErrDirty) {
		t.Fatalf("Normalize dirty = %v", err)
	}
	if err := w.Commit(ctx, w.Paths.Proposals, "proposal 1", "proposals/1.md"); err != nil {
		t.Fatal(err)
	}
	if err := w.Push(ctx, w.Paths.Proposals, "proposals"); err != nil {
		t.Fatal(err)
	}
	if refs := wt.Git(t, remote, "for-each-ref", "refs/heads/proposals"); refs == "" {
		t.Fatal("proposals not on remote")
	}

	// A second writer moves origin/proposals: PushCAS is rejected, Push merges and succeeds.
	other := filepath.Join(t.TempDir(), "other")
	wt.Git(t, filepath.Dir(other), "clone", "-q", "-b", "proposals", remote, other)
	os.MkdirAll(filepath.Join(other, "proposals"), 0o755)
	os.WriteFile(filepath.Join(other, "proposals", "2.md"), []byte("p2\n"), 0o644)
	wt.Git(t, other, "add", "-A")
	wt.Git(t, other, "commit", "-q", "-m", "p2")
	wt.Git(t, other, "push", "-q", "origin", "proposals")
	os.WriteFile(filepath.Join(w.Paths.Proposals, "proposals", "3.md"), []byte("p3\n"), 0o644)
	if err := w.Commit(ctx, w.Paths.Proposals, "proposal 3", "proposals/3.md"); err != nil {
		t.Fatal(err)
	}
	if err := w.PushCAS(ctx, w.Paths.Proposals, "proposals"); !errors.Is(err, workspace.ErrRejected) {
		t.Fatalf("PushCAS = %v, want ErrRejected", err)
	}
	if err := w.Push(ctx, w.Paths.Proposals, "proposals"); err != nil {
		t.Fatal(err)
	}
	if got := wt.Git(t, remote, "ls-tree", "--name-only", "proposals:proposals"); got != "1.md\n2.md\n3.md" {
		t.Fatalf("remote proposals = %q", got)
	}

	// main advances; the read worktree follows only after fetch + UpdateRead.
	wt.Seed(t, remote, "main", map[string]string{"memories/b.md": "b\n"})
	if _, err := w.FetchIfStale(ctx, workspace.FetchTTL); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.Paths.Read, "memories", "b.md")); err == nil {
		t.Fatal("read changed before UpdateRead")
	}
	if err := w.UpdateRead(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.Paths.Read, "memories", "b.md")); err != nil {
		t.Fatal("read did not follow main")
	}
}

func TestFetchIfStaleRecordsAttemptOnFailure(t *testing.T) {
	ctx := context.Background()
	w, remote := setup(t)
	if err := os.Rename(remote, remote+".gone"); err != nil {
		t.Fatal(err)
	}
	if ran, err := w.FetchIfStale(ctx, time.Hour); !ran || !errors.Is(err, workspace.ErrNetwork) {
		t.Fatalf("first attempt = %v, %v", ran, err)
	}
	if ran, err := w.FetchIfStale(ctx, time.Hour); ran || err != nil {
		t.Fatalf("second attempt = %v, %v; want skipped", ran, err)
	}
}

func TestPushPermissionDenied(t *testing.T) {
	ctx := context.Background()
	w, remote := setup(t)
	wt.Git(t, remote, "config", "receive.denyCurrentBranch", "refuse")
	hook := filepath.Join(remote, "hooks", "pre-receive")
	os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	w.EnsureWorktree(ctx, w.Paths.Proposals, "proposals", "origin/main")
	os.WriteFile(filepath.Join(w.Paths.Proposals, "x.md"), []byte("x\n"), 0o644)
	if err := w.Commit(ctx, w.Paths.Proposals, "x", "x.md"); err != nil {
		t.Fatal(err)
	}
	if err := w.Push(ctx, w.Paths.Proposals, "proposals"); !errors.Is(err, workspace.ErrPermission) {
		t.Fatalf("Push = %v, want ErrPermission", err)
	}
}

func TestGitHub(t *testing.T) {
	ctx := context.Background()
	for url, want := range map[string]string{
		"git@github.com:o/r.git":       "o/r",
		"https://github.com/o/r":       "o/r",
		"ssh://git@github.com/o/r.git": "o/r",
	} {
		if got, err := github.Slug(url); err != nil || got != want {
			t.Fatalf("Slug(%q) = %q, %v", url, got, err)
		}
	}
	if _, err := github.Slug("/tmp/x.git"); !errors.Is(err, github.ErrNotGitHub) {
		t.Fatalf("Slug(local) = %v", err)
	}
	gh := wt.FakeGH(t)
	client, _ := github.New("git@github.com:o/r.git")
	if err := client.AuthStatus(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PRCreate(ctx, "main", "release/x", "t", "b"); err != nil {
		t.Fatal(err)
	}
	gh.SetPR(t, "MERGED", "abc")
	if view, err := client.PRView(ctx, 1); err != nil || view.State != "MERGED" || view.MergeCommit.OID != "abc" {
		t.Fatalf("PRView = %+v, %v", view, err)
	}
}
