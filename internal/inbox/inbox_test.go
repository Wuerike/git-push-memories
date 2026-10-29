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
	"github.com/Wuerike/git-push-memories/internal/reference"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func TestAddCommitsOneMarkdownProposalOnProposalsBranch(t *testing.T) {
	box, root, _ := newBox(t)
	parent := gitOutput(t, root, "rev-parse", "HEAD")

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
	if status := gitOutput(t, root, "status", "--short", "--", proposal.Path); status != "" {
		t.Fatalf("proposal path is not clean after commit: %q", status)
	}
}

func TestAddCommitsAndReadsStructuredReferenceProposalV2(t *testing.T) {
	box, root, _ := newBox(t)
	source := reference.Reference{
		Repository: "git-lineage-sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("b", 40),
		Path: "services/checkout/config.yaml", ContentHash: "sha256:" + strings.Repeat("c", 64),
	}
	request := inbox.Request{
		Title: "Checkout source", SuggestedAction: inbox.ActionCreate, Knowledge: "Checkout uses committed configuration.",
		SourceReferences: []reference.Reference{source},
	}
	proposal, err := box.Add(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SchemaVersion != 2 || len(proposal.SourcePaths) != 0 || len(proposal.SourceReferences) != 1 || proposal.SourceReferences[0] != source {
		t.Fatalf("Add() = %+v", proposal)
	}
	want := `---
schema_version: 2
suggested_action: "create"
source_references:
  - repository: "git-lineage-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    path: "services/checkout/config.yaml"
    content_hash: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
---

# Checkout source

Checkout uses committed configuration.
`
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(proposal.Path)))
	if err != nil || string(contents) != want {
		t.Fatalf("proposal Markdown = %q, want %q, err = %v", contents, want, err)
	}
	listed, err := box.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Proposal.SchemaVersion != 2 || len(listed[0].Proposal.SourceReferences) != 1 || listed[0].Proposal.SourceReferences[0] != source {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
}

func TestListRejectsMalformedPersistedProposalV2(t *testing.T) {
	box, root, _ := newBox(t)
	if err := os.Mkdir(filepath.Join(root, "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	proposalPath := "proposals/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-malformed.md"
	contents := "---\nschema_version: 2\nsuggested_action: \"create\"\nsource_references:\n---\n\n# Malformed\n\nKnowledge.\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(proposalPath)), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", proposalPath)
	runGit(t, root, "commit", "--quiet", "-m", "malformed proposal")
	if _, err := box.List(context.Background()); !errors.Is(err, inbox.ErrInvalidProposal) {
		t.Fatalf("List() error = %v, want ErrInvalidProposal", err)
	}
}

func TestConcurrentSessionsCommitDistinctProposals(t *testing.T) {
	first, root, m := newBox(t)
	second, err := inbox.NewWriter(m.Config.Workspace(), root, "proposals")
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

func TestAddRequiresExpectedBranch(t *testing.T) {
	box, root, _ := newBox(t)
	runGit(t, root, "switch", "--quiet", "-c", "feature")
	_, err := box.Add(context.Background(), inbox.Request{Title: "Fact", SuggestedAction: inbox.ActionCreate, Knowledge: "Knowledge.", SourcePaths: []string{}})
	if !errors.Is(err, inbox.ErrWrongBranch) {
		t.Fatalf("Add() error = %v, want ErrWrongBranch", err)
	}
	if _, err := os.Stat(filepath.Join(root, "proposals")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong-branch Add() created proposals: %v", err)
	}
}

func TestAddIgnoresPersonalGitConfiguration(t *testing.T) {
	box, _, _ := newBox(t)
	hookMarker := filepath.Join(t.TempDir(), "hook-ran")
	hooks := t.TempDir()
	for _, name := range []string{"pre-commit", "commit-msg", "post-commit"} {
		if err := os.WriteFile(filepath.Join(hooks, name), []byte("#!/bin/sh\ntouch \""+hookMarker+"\"\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	home := os.Getenv("HOME")
	config := "[core]\n\thooksPath = " + hooks + "\n[commit]\n\tgpgsign = true\n"
	f, err := os.OpenFile(filepath.Join(home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(config)
	f.Close()
	if _, err := box.Add(context.Background(), inbox.Request{Title: "Fact", SuggestedAction: inbox.ActionCreate, Knowledge: "Knowledge.", SourcePaths: []string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Add() ran post-commit hook: %v", err)
	}
}

func newBox(t *testing.T) (*inbox.Inbox, string, *wt.Managed) {
	t.Helper()
	m := wt.Setup(t, nil)
	box, err := inbox.NewWriter(m.Config.Workspace(), m.Config.Paths.Proposals, "proposals")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = box.Close() })
	return box, m.Config.Paths.Proposals, m
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

func TestAddRefusesDirtyWorktree(t *testing.T) {
	box, root, _ := newBox(t)
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := box.Add(context.Background(), inbox.Request{Title: "Fact", SuggestedAction: inbox.ActionCreate, Knowledge: "Knowledge.", SourcePaths: []string{}})
	if !errors.Is(err, workspace.ErrDirty) {
		t.Fatalf("Add() error = %v, want ErrDirty", err)
	}
}

func TestPublishSendsProposalsAndDrainsPendingAfterOutage(t *testing.T) {
	box, root, m := newBox(t)
	ctx := context.Background()
	request := func(title string) inbox.Request {
		return inbox.Request{Title: title, SuggestedAction: inbox.ActionCreate, Knowledge: "A durable fact.", SourcePaths: []string{}}
	}
	first, err := box.Add(ctx, request("First"))
	if err != nil {
		t.Fatal(err)
	}
	if pushed, err := box.Publish(ctx); err != nil || !pushed {
		t.Fatalf("Publish() = %v, %v", pushed, err)
	}
	if got := gitOutput(t, m.Remote, "ls-tree", "--name-only", "proposals:proposals"); got != strings.TrimPrefix(first.Path, "proposals/") {
		t.Fatalf("remote proposals = %q", got)
	}

	if err := os.Rename(m.Remote, m.Remote+".down"); err != nil {
		t.Fatal(err)
	}
	second, err := box.Add(ctx, request("Second"))
	if err != nil {
		t.Fatalf("offline Add() = %v", err)
	}
	if pushed, err := box.Publish(ctx); err != nil || pushed {
		t.Fatalf("offline Publish() = %v, %v; want kept local", pushed, err)
	}
	if err := os.Rename(m.Remote+".down", m.Remote); err != nil {
		t.Fatal(err)
	}
	third, err := box.Add(ctx, request("Third"))
	if err != nil {
		t.Fatal(err)
	}
	if pushed, err := box.Publish(ctx); err != nil || !pushed {
		t.Fatalf("online Publish() = %v, %v", pushed, err)
	}
	remote := gitOutput(t, m.Remote, "ls-tree", "--name-only", "proposals:proposals")
	for _, proposal := range []inbox.Proposal{first, second, third} {
		if !strings.Contains(remote, strings.TrimPrefix(proposal.Path, "proposals/")) {
			t.Fatalf("remote lacks %s: %q", proposal.Path, remote)
		}
	}
	if head := gitOutput(t, root, "rev-parse", "HEAD"); head != gitOutput(t, m.Remote, "rev-parse", "proposals") {
		t.Fatal("local proposals and remote diverge")
	}
}
