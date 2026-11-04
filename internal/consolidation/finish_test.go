package consolidation_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func publishedFixture(t *testing.T) (*consolidation.Manager, *wt.Managed, consolidation.Prepared) {
	t.Helper()
	manager, m, p := preparedManager(t)
	reviewedChange(t, manager, p)
	if _, err := manager.Submit(context.Background(), proposalSummary(p)); err != nil {
		t.Fatal(err)
	}
	return manager, m, p
}

func squashPromotion(t *testing.T, m *wt.Managed, p consolidation.Prepared) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "review")
	wt.Git(t, filepath.Dir(clone), "clone", "-q", "-b", "main", m.Remote, clone)
	wt.Git(t, clone, "merge", "--squash", "origin/"+p.Branch)
	wt.Git(t, clone, "commit", "-q", "-m", "Approved memory consolidation")
	wt.Git(t, clone, "push", "-q", "origin", "main")
	merged := wt.Git(t, clone, "rev-parse", "HEAD")
	wt.FakeGH(t).SetPR(t, "MERGED", merged)
	return merged
}

func TestFinishKeepsProposalsUntilConfirmedMerge(t *testing.T) {
	_, m, _ := publishedFixture(t)
	before := wt.Git(t, m.Remote, "show-ref")
	got, err := consolidation.New(m.Config).Finish(context.Background())
	if err != nil || !got.Found || got.Merged || len(got.Removed) != 0 {
		t.Fatalf("finish open PR = %+v, %v", got, err)
	}
	if wt.Git(t, m.Remote, "show-ref") != before {
		t.Fatal("finish removed proposals or changed refs before merge")
	}
	if _, err := os.Stat(m.Config.Paths.Published); err != nil {
		t.Fatalf("open PR lost cleanup record: %v", err)
	}
}

func TestFinishAfterSquashPreservesNewAndChangedProposals(t *testing.T) {
	_, m, p := publishedFixture(t)
	later := m.Remember(t, "New knowledge after publication")
	clone := filepath.Join(t.TempDir(), "updated-inbox")
	wt.Git(t, filepath.Dir(clone), "clone", "-q", "-b", "proposals", m.Remote, clone)
	changed := p.Proposals[1].Path
	newContent := "# Corrected proposal\nThe latest content needs a new discussion.\n"
	if err := os.WriteFile(filepath.Join(clone, changed), []byte(newContent), 0o644); err != nil {
		t.Fatal(err)
	}
	wt.Git(t, clone, "add", changed)
	wt.Git(t, clone, "commit", "-q", "-m", "Update pending proposal")
	wt.Git(t, clone, "push", "-q", "origin", "proposals")
	merged := squashPromotion(t, m, p)
	if merged == wt.Git(t, m.Remote, "rev-parse", p.Branch) {
		t.Fatal("fixture must exercise squash rather than branch ancestry")
	}
	got, err := consolidation.New(m.Config).Finish(context.Background())
	if err != nil || !got.Merged {
		t.Fatalf("finish = %+v, %v", got, err)
	}
	wantRemoved := []string{p.Proposals[0].Path, p.Proposals[2].Path}
	if !reflect.DeepEqual(got.Removed, wantRemoved) || !reflect.DeepEqual(got.Preserved, []string{changed}) {
		t.Fatalf("cleanup report = %+v", got)
	}
	wantPending := []string{changed, later.Path}
	sort.Strings(wantPending)
	actual := strings.Fields(wt.Git(t, m.Remote, "ls-tree", "-r", "--name-only", "proposals", "--", "proposals"))
	if !reflect.DeepEqual(actual, wantPending) {
		t.Fatalf("pending proposals = %v, want %v", actual, wantPending)
	}
	if text := wt.Git(t, m.Remote, "show", "proposals:"+changed); text != strings.TrimSuffix(newContent, "\n") {
		t.Fatalf("changed proposal lost its latest bytes: %q", text)
	}
	if _, err := os.Stat(m.Config.Paths.Published); !os.IsNotExist(err) {
		t.Fatalf("completed cleanup retained publication record: %v", err)
	}
}

func TestFinishRefreshesPublishedReads(t *testing.T) {
	_, m, p := publishedFixture(t)
	merged := squashPromotion(t, m, p)
	if wt.Git(t, m.Config.Paths.Read, "rev-parse", "HEAD") == merged {
		t.Fatal("fixture must start with a stale read checkout")
	}
	if got, err := consolidation.New(m.Config).Finish(context.Background()); err != nil || !got.Merged {
		t.Fatalf("finish = %+v, %v", got, err)
	}
	if head := wt.Git(t, m.Config.Paths.Read, "rev-parse", "HEAD"); head != merged {
		t.Fatalf("read HEAD = %s, want merged %s", head, merged)
	}
	data, err := os.ReadFile(filepath.Join(m.Config.Paths.Read, checkoutPath))
	want := checkoutPage + "\nTimeout is nine seconds.\n"
	if err != nil || string(data) != want {
		t.Fatalf("published read = %q, %v; want %q", data, err, want)
	}
}

func TestFinishFailurePreservesProposalsAndWork(t *testing.T) {
	_, m, p := publishedFixture(t)
	before := wt.Git(t, m.Remote, "show-ref")
	if err := os.WriteFile(filepath.Join(os.Getenv("GPM_FAKE_GH_DIR"), "gh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := consolidation.New(m.Config).Finish(context.Background()); err == nil {
		t.Fatal("failed merge lookup reported successful completion")
	}
	if wt.Git(t, m.Remote, "show-ref") != before {
		t.Fatal("failed confirmation changed the inbox")
	}
	for _, file := range []string{m.Config.Paths.Published, filepath.Join(p.Worktree, checkoutPath)} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("failure deleted %s: %v", file, err)
		}
	}
}

func TestFinishPreservesUnselectedManualProposals(t *testing.T) {
	m := wt.Setup(t, nil)
	const selected = "proposals/billing/manual.md"
	const unselected = "proposals/other.md"
	wt.Seed(t, m.Remote, "proposals", map[string]string{selected: "A billing fact.", unselected: "A different fact."})
	manager := consolidation.New(m.Config)
	p, err := manager.Prepare(context.Background(), selected)
	if err != nil {
		t.Fatal(err)
	}
	writeMemory(t, p, "memories/domains/billing.md", "The billing fact.\n")
	if _, err := manager.Submit(context.Background(), selected+" creates billing memory."); err != nil {
		t.Fatal(err)
	}
	squashPromotion(t, m, p)
	got, err := consolidation.New(m.Config).Finish(context.Background())
	if err != nil || !got.Merged || !reflect.DeepEqual(got.Removed, []string{selected}) {
		t.Fatalf("finish = %+v, %v", got, err)
	}
	if pending := wt.Git(t, m.Remote, "ls-tree", "-r", "--name-only", "proposals", "--", "proposals"); pending != unselected {
		t.Fatalf("pending = %q", pending)
	}
}
