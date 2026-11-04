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
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func TestAddCommitsFreeTextProposalOnRemoteProposalsBranch(t *testing.T) {
	box, root, m := newBox(t)
	parent := gitOutput(t, m.Remote, "rev-parse", "proposals")
	content := "Update memories/domains/charge.md: card transactions can originate from the authorizer adapter.\n"

	proposal, err := box.Add(context.Background(), inbox.Request{Title: "  Card transactions from authorizer ", Content: content})
	if err != nil {
		t.Fatal(err)
	}

	if len(proposal.ID) != 32 || proposal.Path != "proposals/"+proposal.ID+"-card-transactions-from-authorizer.md" {
		t.Fatalf("Add() proposal = %+v", proposal)
	}
	if proposal.Commit != gitOutput(t, m.Remote, "rev-parse", "proposals") || gitOutput(t, m.Remote, "rev-parse", "proposals^") != parent {
		t.Fatalf("Add() commit = %q, parent = %q", proposal.Commit, parent)
	}
	if changed := gitOutput(t, m.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", proposal.Commit); changed != proposal.Path {
		t.Fatalf("proposal commit changed %q, want %q", changed, proposal.Path)
	}
	if subject := gitOutput(t, m.Remote, "log", "-1", "--format=%s", proposal.Commit); subject != "Add memory proposal: Card transactions from authorizer" {
		t.Fatalf("commit subject = %q", subject)
	}
	if contents := gitOutput(t, m.Remote, "show", proposal.Commit+":"+proposal.Path); contents != strings.TrimSuffix(content, "\n") {
		t.Fatalf("remote proposal = %q, want the content as written", contents)
	}
	if _, err := os.Stat(filepath.Join(root, "proposals")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Add() touched the local worktree: %v", err)
	}
	if err := box.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	listed, err := box.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Proposal.Path != proposal.Path || listed[0].Proposal.Blob == "" || listed[0].Proposal.Content != content {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
}

func TestAddRejectsEmptyTitleOrContent(t *testing.T) {
	box, _, m := newBox(t)
	before := gitOutput(t, m.Remote, "rev-parse", "proposals")
	for _, request := range []inbox.Request{{Title: " ", Content: "x"}, {Title: "x", Content: " \n"}} {
		if _, err := box.Add(context.Background(), request); !errors.Is(err, inbox.ErrInvalidProposal) {
			t.Fatalf("Add(%+v) error = %v, want ErrInvalidProposal", request, err)
		}
	}
	if gitOutput(t, m.Remote, "rev-parse", "proposals") != before {
		t.Fatal("rejected Add() committed to the remote")
	}
}

func TestTitleCannotEscapeProposalsDirectory(t *testing.T) {
	box, _, _ := newBox(t)
	proposal, err := box.Add(context.Background(), inbox.Request{Title: "../../etc/passwd", Content: "x"})
	if err != nil || filepath.Dir(proposal.Path) != "proposals" {
		t.Fatalf("Add() = %+v, %v", proposal, err)
	}
}

func TestListIncludesManualMarkdownWithPathBlobAndOriginalContent(t *testing.T) {
	box, root, _ := newBox(t)
	if err := os.Mkdir(filepath.Join(root, "proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"proposals/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-with-heading.md": "intro\n# Real title\nbody\n",
		"proposals/not-a-proposal.md":                                "x\n",
		"proposals/README.txt":                                       "x\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", "proposals")
	runGit(t, root, "commit", "--quiet", "-m", "arbitrary files")
	listed, err := box.List(context.Background())
	if err != nil || len(listed) != 2 || listed[0].Proposal.Path != "proposals/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-with-heading.md" || listed[0].Proposal.Content != files[listed[0].Proposal.Path] || listed[0].Proposal.Blob == "" || listed[1].Proposal.Path != "proposals/not-a-proposal.md" {
		t.Fatalf("List() = %+v, %v", listed, err)
	}
}

func TestConcurrentSessionsAddDistinctProposals(t *testing.T) {
	first, root, m := newBox(t)
	second, err := inbox.NewWriter(m.Config.Workspace(), root, "proposals")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan struct {
		proposal inbox.Proposal
		err      error
	}, 2)
	for index, box := range []*inbox.Inbox{first, second} {
		go func() {
			<-start
			proposal, err := box.Add(context.Background(), inbox.Request{Title: []string{"First fact", "Second fact"}[index], Content: "A durable fact."})
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
	listed := gitOutput(t, m.Remote, "ls-tree", "--name-only", "proposals:proposals")
	for _, proposal := range []inbox.Proposal{left.proposal, right.proposal} {
		if !strings.Contains(listed, strings.TrimPrefix(proposal.Path, "proposals/")) {
			t.Fatalf("remote lacks %s: %q", proposal.Path, listed)
		}
	}
}

func newBox(t *testing.T) (*inbox.Inbox, string, *wt.Managed) {
	t.Helper()
	m := wt.Setup(t, nil)
	box, err := inbox.NewWriter(m.Config.Workspace(), m.Config.Paths.Proposals, "proposals")
	if err != nil {
		t.Fatal(err)
	}
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

func TestAddFailureLeavesNothingBehind(t *testing.T) {
	box, root, m := newBox(t)
	before := gitOutput(t, m.Remote, "rev-parse", "proposals")
	t.Setenv("GPM_FAKE_API_FAIL", "Post: connection refused")
	if _, err := box.Add(context.Background(), inbox.Request{Title: "Fact", Content: "Knowledge."}); err == nil {
		t.Fatal("Add() succeeded while the API failed")
	}
	t.Setenv("GPM_FAKE_API_FAIL", "Forbidden (HTTP 403)")
	if _, err := box.Add(context.Background(), inbox.Request{Title: "Fact", Content: "Knowledge."}); !errors.Is(err, workspace.ErrPermission) {
		t.Fatalf("Add() error = %v, want ErrPermission", err)
	}
	if gitOutput(t, m.Remote, "rev-parse", "proposals") != before || gitOutput(t, root, "status", "--porcelain") != "" {
		t.Fatal("a failed Add() left a commit or a stray file")
	}
	t.Setenv("GPM_FAKE_API_FAIL", "")
	if _, err := box.Add(context.Background(), inbox.Request{Title: "Fact", Content: "Knowledge."}); err != nil {
		t.Fatalf("Add() after recovery = %v", err)
	}
}

func TestRefreshBringsInProposalsFromOtherMachines(t *testing.T) {
	box, root, m := newBox(t)
	other, err := inbox.NewWriter(m.Config.Workspace(), root, "proposals")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := other.Add(context.Background(), inbox.Request{Title: "Theirs", Content: "theirs\n"})
	if err != nil {
		t.Fatal(err)
	}
	if listed, err := box.List(context.Background()); err != nil || len(listed) != 0 {
		t.Fatalf("List() before Refresh = %+v, %v", listed, err)
	}
	if err := box.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if listed, err := box.List(context.Background()); err != nil || len(listed) != 1 || listed[0].Proposal.Path != theirs.Path {
		t.Fatalf("List() after Refresh = %+v, %v", listed, err)
	}
}
