package inbox_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/inbox"
)

func TestAddCommitsOneMarkdownProposalOnMain(t *testing.T) {
	root := newInboxRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "unrelated.txt")
	parent := gitOutput(t, root, "rev-parse", "HEAD")

	box, err := inbox.New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = box.Close() })
	proposal, err := box.Add(context.Background(), inbox.Request{
		Title:           "Card transactions from authorizer",
		SuggestedAction: inbox.ActionUpdate,
		Target:          "memories/domains/charge-entity-payment-methods.md",
		Knowledge:       "Card transactions can originate from the authorizer adapter.",
		SourcePaths:     []string{"adapters/authorizer/config.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(proposal.ID) != 32 || proposal.Path != "proposals/"+proposal.ID+"-card-transactions-from-authorizer.md" {
		t.Fatalf("Add() proposal = %+v", proposal)
	}
	if proposal.Commit != gitOutput(t, root, "rev-parse", "HEAD") || gitOutput(t, root, "rev-parse", "HEAD^") != parent {
		t.Fatalf("Add() commit = %q, parent = %q", proposal.Commit, parent)
	}
	if changed := gitOutput(t, root, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); changed != proposal.Path {
		t.Fatalf("proposal commit changed %q, want %q", changed, proposal.Path)
	}
	want := `---
schema_version: 1
suggested_action: "update"
target: "memories/domains/charge-entity-payment-methods.md"
source_paths:
  - "adapters/authorizer/config.yaml"
---

# Card transactions from authorizer

Card transactions can originate from the authorizer adapter.
`
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(proposal.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Fatalf("proposal Markdown mismatch\ngot:\n%s\nwant:\n%s", contents, want)
	}
	if staged := gitOutput(t, root, "diff", "--cached", "--name-only"); staged != "unrelated.txt" {
		t.Fatalf("unrelated staging after Add() = %q", staged)
	}
	if status := gitOutput(t, root, "status", "--short", "--", proposal.Path); status != "" {
		t.Fatalf("proposal path is not clean after commit: %q", status)
	}
}

func TestConcurrentSessionsCommitDistinctProposalsOnLatestMain(t *testing.T) {
	root := newInboxRoot(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	first, err := inbox.New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := inbox.New(root, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	start := make(chan struct{})
	results := make(chan struct {
		proposal inbox.Proposal
		err      error
	}, 2)
	for index, box := range []*inbox.Inbox{first, second} {
		go func() {
			<-start
			proposal, err := box.Add(context.Background(), inbox.Request{
				Title:           []string{"First fact", "Second fact"}[index],
				SuggestedAction: inbox.ActionCreate,
				Knowledge:       "A durable fact.",
				SourcePaths:     []string{},
			})
			results <- struct {
				proposal inbox.Proposal
				err      error
			}{proposal, err}
		}()
	}
	close(start)
	left, right := <-results, <-results
	if left.err != nil || right.err != nil {
		t.Fatalf("concurrent Add() errors = %v, %v", left.err, right.err)
	}
	if left.proposal.ID == right.proposal.ID || left.proposal.Path == right.proposal.Path {
		t.Fatalf("concurrent proposals collided: %+v, %+v", left.proposal, right.proposal)
	}
	for _, proposal := range []inbox.Proposal{left.proposal, right.proposal} {
		if changed := gitOutput(t, root, "diff-tree", "--no-commit-id", "--name-only", "-r", proposal.Commit); changed != proposal.Path {
			t.Fatalf("commit %s changed %q, want %q", proposal.Commit, changed, proposal.Path)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(proposal.Path))); err != nil {
			t.Fatal(err)
		}
	}
	if !gitSucceeds(root, "merge-base", "--is-ancestor", left.proposal.Commit, right.proposal.Commit) && !gitSucceeds(root, "merge-base", "--is-ancestor", right.proposal.Commit, left.proposal.Commit) {
		t.Fatalf("proposal commits are not linear: %s, %s", left.proposal.Commit, right.proposal.Commit)
	}
}

func TestAddRequiresExactMainBranch(t *testing.T) {
	root := newInboxRoot(t)
	runGit(t, root, "switch", "--quiet", "-c", "feature")
	box, err := inbox.New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	_, err = box.Add(context.Background(), inbox.Request{Title: "Fact", SuggestedAction: inbox.ActionCreate, Knowledge: "Knowledge.", SourcePaths: []string{}})
	if !errors.Is(err, inbox.ErrNotMain) {
		t.Fatalf("Add() error = %v, want ErrNotMain", err)
	}
	if _, err := os.Stat(filepath.Join(root, "proposals")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-main Add() created proposals: %v", err)
	}
}

func TestAddDoesNotRunGitHooks(t *testing.T) {
	root := newInboxRoot(t)
	hookMarker := filepath.Join(root, "hook-ran")
	hookPath := filepath.Join(root, ".git", "hooks", "post-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\ntouch \""+hookMarker+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	box, err := inbox.New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	if _, err := box.Add(context.Background(), inbox.Request{Title: "Fact", SuggestedAction: inbox.ActionCreate, Knowledge: "Knowledge.", SourcePaths: []string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Add() ran post-commit hook: %v", err)
	}
}

func TestAddReportsExistingGitIndexLockWithoutCommit(t *testing.T) {
	root := newInboxRoot(t)
	parent := gitOutput(t, root, "rev-parse", "HEAD")
	lockPath := filepath.Join(root, ".git", "index.lock")
	if err := os.WriteFile(lockPath, []byte("other process"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(lockPath)
	box, err := inbox.New(root, filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer box.Close()
	_, err = box.Add(context.Background(), inbox.Request{Title: "Fact", SuggestedAction: inbox.ActionCreate, Knowledge: "Knowledge.", SourcePaths: []string{}})
	if !errors.Is(err, inbox.ErrIndexLocked) {
		t.Fatalf("Add() error = %v, want ErrIndexLocked", err)
	}
	if head := gitOutput(t, root, "rev-parse", "HEAD"); head != parent {
		t.Fatalf("failed Add() changed HEAD from %s to %s", parent, head)
	}
	if matches, err := filepath.Glob(filepath.Join(root, "proposals", "*.md")); err != nil || len(matches) != 0 {
		t.Fatalf("failed Add() retained proposals %v: %v", matches, err)
	}
}

func newInboxRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "gpm.json"), []byte("{\n  \"schema_version\": 1,\n  \"memory_categories\": [\"domains\", \"decision\"]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "gpm.json")
	runGit(t, root, "commit", "--quiet", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(arguments, " "), err)
	}
	return strings.TrimSpace(string(output))
}

func gitSucceeds(root string, arguments ...string) bool {
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	return command.Run() == nil
}
