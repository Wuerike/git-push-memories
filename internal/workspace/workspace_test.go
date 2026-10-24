package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

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
	if exists, err := w.RemoteBranchExists(ctx, "proposals"); err != nil || exists {
		t.Fatalf("RemoteBranchExists before push = %v, %v", exists, err)
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
	if exists, _ := w.RemoteBranchExists(ctx, "proposals"); !exists {
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
	if pr, err := client.PRFind(ctx, "release/x"); err != nil || pr != nil {
		t.Fatalf("PRFind none = %v, %v", pr, err)
	}
	if _, err := client.PRCreate(ctx, "main", "release/x", "t", "b"); err != nil {
		t.Fatal(err)
	}
	gh.SetPR(t, "MERGED", "abc")
	pr, err := client.PRFind(ctx, "release/x")
	if err != nil || pr == nil || pr.State != "MERGED" || pr.MergeCommit.OID != "abc" {
		t.Fatalf("PRFind = %+v, %v", pr, err)
	}
	if view, err := client.PRView(ctx, pr.Number); err != nil || view.State != "MERGED" {
		t.Fatalf("PRView = %+v, %v", view, err)
	}
}
