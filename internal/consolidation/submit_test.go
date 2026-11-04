package consolidation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func proposalSummary(p consolidation.Prepared) string {
	return fmt.Sprintf("## Consolidation\n\n%s and %s update %s.\n%s is discarded because the current policy already contains this knowledge.\n",
		p.Proposals[0].Path, p.Proposals[1].Path, checkoutPath, p.Proposals[2].Path)
}

func reviewedChange(t *testing.T, manager *consolidation.Manager, p consolidation.Prepared) string {
	t.Helper()
	text := checkoutPage + "\nTimeout is nine seconds.\n"
	writeMemory(t, p, checkoutPath, text)
	if _, err := manager.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	return text
}

func createdPRArgument(t *testing.T, flag string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("GPM_FAKE_GH_DIR"), "create-args"))
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(string(data), "\x00")
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	t.Fatalf("PR argument %s missing in %q", flag, args)
	return ""
}

func TestSubmitCommitsOnlyCanonicalChanges(t *testing.T) {
	manager, m, p := preparedManager(t)
	reviewedChange(t, manager, p)
	writeMemory(t, p, "notes.md", "Local review notes, never canonical.\n")
	writeMemory(t, p, "proposals/extra.md", "A helper input, never canonical.\n")
	wt.Git(t, p.Worktree, "add", "-A")
	before := wt.Git(t, m.Remote, "rev-parse", "proposals")
	got, err := manager.Submit(context.Background(), proposalSummary(p))
	if err != nil {
		t.Fatal(err)
	}
	if changed := wt.Git(t, p.Worktree, "diff-tree", "--no-commit-id", "--name-only", "-r", got.Commit); changed != checkoutPath {
		t.Fatalf("commit contains %q, want only %s", changed, checkoutPath)
	}
	if commits := wt.Git(t, p.Worktree, "rev-list", "--count", p.MainSHA+"..HEAD"); commits != "1" {
		t.Fatalf("consolidation made %s commits, want 1", commits)
	}
	if wt.Git(t, m.Remote, "rev-parse", "proposals") != before {
		t.Fatal("submit removed proposals before merge")
	}
	if _, err := os.Stat(filepath.Join(p.Worktree, "notes.md")); err != nil {
		t.Fatalf("submit removed local review notes: %v", err)
	}
}

func TestSubmitPreservesHumanEdits(t *testing.T) {
	manager, m, p := preparedManager(t)
	first := reviewedChange(t, manager, p)
	human := first + "\nHuman clarification: timeout applies only to authorization.\n"
	writeMemory(t, p, checkoutPath, human)
	if checked, err := manager.Check(context.Background()); err != nil || !strings.Contains(checked.Diff, "+Human clarification: timeout applies only to authorization.") {
		t.Fatalf("latest review = %+v, %v", checked, err)
	}
	got, err := manager.Submit(context.Background(), proposalSummary(p))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := m.Config.Workspace().Repo().Bytes(context.Background(), "show", got.Commit+":"+checkoutPath)
	if err != nil || string(actual) != human {
		t.Fatalf("committed human edits = %q, %v; want %q", actual, err, human)
	}
}

func TestSubmitPushesBranchAndCreatesMainPR(t *testing.T) {
	manager, m, p := preparedManager(t)
	reviewedChange(t, manager, p)
	got, err := manager.Submit(context.Background(), proposalSummary(p))
	if err != nil {
		t.Fatal(err)
	}
	if got.PRURL != "https://github.com/o/r/pull/1" || got.PRNumber != 1 || got.Commit == "" {
		t.Fatalf("submit = %+v", got)
	}
	if wt.Git(t, m.Remote, "rev-parse", p.Branch) != got.Commit || createdPRArgument(t, "--base") != "main" || createdPRArgument(t, "--head") != p.Branch {
		t.Fatal("branch push or PR target mismatch")
	}
	if wt.Git(t, m.Remote, "rev-parse", "main") != p.MainSHA {
		t.Fatal("submit merged its own PR")
	}
}

func TestSubmitRecordsFreeFormProposalSummary(t *testing.T) {
	manager, m, p := preparedManager(t)
	reviewedChange(t, manager, p)
	summary := proposalSummary(p)
	if _, err := manager.Submit(context.Background(), summary); err != nil {
		t.Fatal(err)
	}
	if got := createdPRArgument(t, "--body"); got != summary {
		t.Fatalf("PR body = %q, want approved summary %q", got, summary)
	}
	data, err := os.ReadFile(m.Config.Paths.Published)
	if err != nil {
		t.Fatal(err)
	}
	var record consolidation.Published
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	want := make([]consolidation.ProposalBlob, len(p.Proposals))
	for i, proposal := range p.Proposals {
		want[i] = consolidation.ProposalBlob{Path: proposal.Path, Blob: proposal.Blob}
	}
	if record.PRNumber != 1 || record.PRURL != "https://github.com/o/r/pull/1" || record.Branch != p.Branch || !reflect.DeepEqual(record.Proposals, want) {
		t.Fatalf("publication record = %+v", record)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 4 {
		t.Fatalf("record should contain only PR linkage and input blobs: %s, %v", data, err)
	}
}

func TestSubmitFailureKeepsPreparedWork(t *testing.T) {
	manager, m, p := preparedManager(t)
	want := reviewedChange(t, manager, p)
	before := wt.Git(t, m.Remote, "show-ref")
	wt.Git(t, m.Config.Paths.Repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	if _, err := manager.Submit(context.Background(), proposalSummary(p)); err == nil {
		t.Fatal("failed push reported successful publication")
	}
	data, err := os.ReadFile(filepath.Join(p.Worktree, checkoutPath))
	if err != nil || string(data) != want {
		t.Fatalf("prepared work was lost: %q, %v", data, err)
	}
	if wt.Git(t, m.Remote, "show-ref") != before {
		t.Fatal("failed publication changed the remote")
	}
	if _, err := os.Stat(m.Config.Paths.Published); !os.IsNotExist(err) {
		t.Fatalf("failed push wrote publication record: %v", err)
	}
}

func TestPrepareAndSubmitDoNotUseSharedReleaseMarker(t *testing.T) {
	m := wt.Setup(t, map[string]string{checkoutPath: checkoutPage})
	wt.Seed(t, m.Remote, "proposals", map[string]string{
		proposalPath(1):     "# Fact\nUpdate checkout timeout to nine seconds.\n",
		"release.lock.json": "This obsolete marker is deliberately not valid JSON.\n",
	})
	before := wt.Git(t, m.Remote, "rev-parse", "proposals")
	manager := consolidation.New(m.Config)
	p, err := manager.Prepare(context.Background())
	if err != nil {
		t.Fatalf("prepare interpreted obsolete release marker: %v", err)
	}
	reviewedChange(t, manager, p)
	if _, err := manager.Submit(context.Background(), "The selected proposal updates checkout timeout."); err != nil {
		t.Fatal(err)
	}
	if wt.Git(t, m.Remote, "rev-parse", "proposals") != before {
		t.Fatal("prepare/submit wrote a shared release marker")
	}
	if _, err := os.Stat(filepath.Join(p.Worktree, "release.lock.json")); !os.IsNotExist(err) {
		t.Fatalf("release marker leaked into promotion: %v", err)
	}
}

func TestSubmitPublishesSelectedOrdinaryMarkdownBytes(t *testing.T) {
	manager, m, p := preparedManager(t)
	const selected = "memories/domains/*.md"
	const unselected = "memories/domains/later.md"
	const body = "---\nowner: team-green\n---\nA plain fact without headings or IDs.\n"
	writeMemory(t, p, selected, body)
	writeMemory(t, p, unselected, "A separate pending edit.\n")
	result, err := manager.Submit(context.Background(), "Selected proposals contribute to the plain fact.", selected)
	if err != nil {
		t.Fatal(err)
	}
	if paths := wt.Git(t, m.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", result.Commit); paths != selected {
		t.Fatalf("published paths = %q", paths)
	}
	data, err := m.Config.Workspace().Repo().Bytes(context.Background(), "show", result.Commit+":"+selected)
	if err != nil || string(data) != body {
		t.Fatalf("published bytes = %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(p.Worktree, unselected)); err != nil || string(data) != "A separate pending edit.\n" {
		t.Fatalf("unselected edit = %q, %v", data, err)
	}
}

func TestSubmitPreservesRawMarkdownUnderGitConversionAndCleanFilters(t *testing.T) {
	for _, setup := range []string{"autocrlf", "attributes"} {
		t.Run(setup, func(t *testing.T) {
			manager, m, p := preparedManager(t)
			if setup == "autocrlf" {
				wt.Git(t, p.Worktree, "config", "core.autocrlf", "input")
			} else {
				writeMemory(t, p, ".gitattributes", "*.md text filter=erase\n")
				wt.Git(t, p.Worktree, "config", "filter.erase.clean", "tr a-z A-Z")
			}
			const selected = "memories/domains/raw.md"
			const body = "Plain knowledge.\r\nAnother fact.\r\n"
			writeMemory(t, p, selected, body)
			// Preserve unrelated human staging as well as the working file itself.
			writeMemory(t, p, "human.txt", "An unrelated staged edit.\n")
			wt.Git(t, p.Worktree, "add", "--", "human.txt")
			beforeIndex := wt.Git(t, p.Worktree, "write-tree")
			checked, err := manager.Check(context.Background())
			if err != nil || !strings.Contains(checked.Diff, "+Plain knowledge.\r\n") {
				t.Fatalf("raw check = %+v, %v", checked, err)
			}
			if wt.Git(t, p.Worktree, "write-tree") != beforeIndex {
				t.Fatal("check changed human staging")
			}
			result, err := manager.Submit(context.Background(), "Publish reviewed raw Markdown.", selected)
			if err != nil {
				t.Fatal(err)
			}
			data, err := m.Config.Workspace().Repo().Bytes(context.Background(), "show", result.Commit+":"+selected)
			if err != nil || string(data) != body {
				t.Fatalf("published bytes = %q, %v", data, err)
			}
			if names := wt.Git(t, m.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", result.Commit); names != selected {
				t.Fatalf("commit included unrelated staging: %q", names)
			}
			if staged := wt.Git(t, p.Worktree, "diff", "--cached", "--name-only"); staged != "human.txt" {
				t.Fatalf("human staging changed: %q", staged)
			}
			current, err := os.ReadFile(filepath.Join(p.Worktree, selected))
			if err != nil || string(current) != body {
				t.Fatalf("working bytes = %q, %v", current, err)
			}
		})
	}
}

func TestSubmitPublishesMemoryDeletion(t *testing.T) {
	manager, m, p := preparedManager(t)
	if err := os.Remove(filepath.Join(p.Worktree, checkoutPath)); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Submit(context.Background(), "The selected proposals retire the outdated checkout page.", checkoutPath)
	if err != nil {
		t.Fatal(err)
	}
	if paths := wt.Git(t, m.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", result.Commit); paths != checkoutPath {
		t.Fatalf("deleted paths = %q", paths)
	}
	if m.Config.Workspace().Repo().Ok(context.Background(), "cat-file", "-e", result.Commit+":"+checkoutPath) {
		t.Fatal("deleted page remains in published commit")
	}
	if status := wt.Git(t, p.Worktree, "status", "--porcelain"); status != "" {
		t.Fatalf("deletion left stale staging: %q", status)
	}
}
