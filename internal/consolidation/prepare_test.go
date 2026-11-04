package consolidation_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

const checkoutPath = "memories/domains/checkout.md"
const checkoutPage = "# Checkout\n<!-- @id: checkout -->\n\n## Ownership\n<!-- @id: checkout-ownership -->\nOwned by team blue.\n"

func proposalPath(n int) string { return fmt.Sprintf("proposals/%032x-fact.md", n) }

func seedProposals(t *testing.T, m *wt.Managed, count int) map[string]string {
	t.Helper()
	files := make(map[string]string, count)
	for n := 1; n <= count; n++ {
		files[proposalPath(n)] = fmt.Sprintf("# Fact %d\n\nCreate or update related memory.\nFact number %d.\n", n, n)
	}
	wt.Seed(t, m.Remote, "proposals", files)
	return files
}

func TestPrepareStartsFromFetchedMain(t *testing.T) {
	m := wt.Setup(t, map[string]string{checkoutPath: checkoutPage})
	seedProposals(t, m, 1)
	m.PublishMain(t, map[string]string{"memories/decision/retry.md": "# Retry\n<!-- @id: retry -->\nThree attempts.\n"})
	main := wt.Git(t, m.Remote, "rev-parse", "main")
	proposals := wt.Git(t, m.Remote, "rev-parse", "proposals")
	prepared, err := consolidation.New(m.Config).Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.MainSHA != main || prepared.ProposalsSHA != proposals || wt.Git(t, prepared.Worktree, "rev-parse", "HEAD") != main {
		t.Fatalf("wrong revisions: %+v; want main %s and proposals %s", prepared, main, proposals)
	}
	wantPaths := []string{"memories/decision/retry.md", checkoutPath}
	if !reflect.DeepEqual(prepared.MemoryPaths, wantPaths) || !reflect.DeepEqual(prepared.Categories, []string{"domains", "decision"}) {
		t.Fatalf("memory inventory = %v, categories = %v", prepared.MemoryPaths, prepared.Categories)
	}
	if wt.Git(t, prepared.Worktree, "log", "-1", "--format=%H") != main || wt.Git(t, m.Remote, "rev-parse", "proposals") != proposals {
		t.Fatal("preparation changed remote or carried proposal history")
	}
}

func TestPrepareCopiesEveryProposalFromPinnedRevision(t *testing.T) {
	m := wt.Setup(t, map[string]string{checkoutPath: checkoutPage})
	files := map[string]string{proposalPath(1): "# Fact 1\n\nA complete durable fact.\n"}
	files[proposalPath(2)] = "# Large free proposal\nAction: update\nTarget: suggested.md\n" + strings.Repeat("A complete durable fact.\n", 5000)
	wt.Seed(t, m.Remote, "proposals", files)
	prepared, err := consolidation.New(m.Config).Prepare(context.Background())
	if err != nil || len(prepared.Proposals) != 2 {
		t.Fatalf("prepare = %+v, %v", prepared, err)
	}
	for _, proposal := range prepared.Proposals {
		got, err := os.ReadFile(proposal.InputPath)
		if err != nil || string(got) != files[proposal.Path] {
			t.Fatalf("proposal %s was truncated or changed: %v", proposal.Path, err)
		}
		if proposal.Blob != wt.Git(t, m.Remote, "rev-parse", prepared.ProposalsSHA+":"+proposal.Path) {
			t.Fatalf("proposal blob = %s", proposal.Blob)
		}
		if _, err := os.Stat(filepath.Join(prepared.Worktree, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("proposal is in publishable worktree: %v", err)
		}
	}
}

func TestPrepareIncludesSeventeenProposals(t *testing.T) {
	m := wt.Setup(t, nil)
	seedProposals(t, m, 17)
	prepared, err := consolidation.New(m.Config).Prepare(context.Background())
	if err != nil || len(prepared.Proposals) != 17 {
		t.Fatalf("prepare returned %d proposals, %v; want 17", len(prepared.Proposals), err)
	}
	for n, p := range prepared.Proposals {
		if p.Path != proposalPath(n+1) {
			t.Fatalf("proposal %d = %s", n+1, p.Path)
		}
	}
}

func TestPreparedInputsExcludeLaterProposals(t *testing.T) {
	m := wt.Setup(t, nil)
	files := seedProposals(t, m, 1)
	prepared, err := consolidation.New(m.Config).Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.Remember(t, "Later proposal")
	if len(prepared.Proposals) != 1 || prepared.ProposalsSHA == wt.Git(t, m.Remote, "rev-parse", "proposals") {
		t.Fatalf("prepared set changed: %+v", prepared)
	}
	got, err := os.ReadFile(prepared.Proposals[0].InputPath)
	if err != nil || string(got) != files[proposalPath(1)] {
		t.Fatalf("prepared input changed: %q, %v", got, err)
	}
}

func TestPrepareWithoutProposalsDoesNotPublish(t *testing.T) {
	m := wt.Setup(t, nil)
	before := wt.Git(t, m.Remote, "show-ref")
	_, err := consolidation.New(m.Config).Prepare(context.Background())
	if !errors.Is(err, consolidation.ErrNoProposals) {
		t.Fatalf("error = %v, want no proposals", err)
	}
	if wt.Git(t, m.Remote, "show-ref") != before {
		t.Fatal("empty prepare published changes")
	}
	if _, err := os.Stat(m.Config.Paths.Promotions); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty prepare created promotion directory: %v", err)
	}
}

func TestConsolidationFailureStopsWithoutDeletingWork(t *testing.T) {
	m := wt.Setup(t, nil)
	seedProposals(t, m, 1)
	if err := os.Rename(m.Remote, m.Remote+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	_, err := consolidation.New(m.Config).Prepare(context.Background())
	if err == nil {
		t.Fatal("failed fetch reported successful preparation")
	}
	if _, err := os.Stat(m.Config.Paths.Read); err != nil {
		t.Fatalf("failure removed read checkout: %v", err)
	}
	if _, err := os.Stat(m.Config.Paths.Promotions); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed fetch created worktree: %v", err)
	}
}

func TestPrepareSelectsManualProposalPathsWithoutInterpretingContent(t *testing.T) {
	m := wt.Setup(t, nil)
	const selected = "proposals/billing/manual note.md"
	const other = "proposals/unrelated.md"
	const body = "No heading or action schema.\nA billing fact.\n"
	wt.Seed(t, m.Remote, "proposals", map[string]string{selected: body, other: "Other fact."})
	p, err := consolidation.New(m.Config).Prepare(context.Background(), selected)
	if err != nil || len(p.Proposals) != 1 || p.Proposals[0].Path != selected {
		t.Fatalf("selection = %+v, %v", p, err)
	}
	got, err := os.ReadFile(p.Proposals[0].InputPath)
	if err != nil || string(got) != body || p.Proposals[0].Blob != wt.Git(t, m.Remote, "rev-parse", p.ProposalsSHA+":"+selected) {
		t.Fatalf("input = %q, %v", got, err)
	}
}

func TestPrepareRejectsUnknownSelectedProposalWithoutCreatingWorktree(t *testing.T) {
	m := wt.Setup(t, nil)
	seedProposals(t, m, 1)
	if _, err := consolidation.New(m.Config).Prepare(context.Background(), "proposals/missing.md"); err == nil {
		t.Fatal("unknown proposal accepted")
	}
	if _, err := os.Stat(m.Config.Paths.Promotions); !os.IsNotExist(err) {
		t.Fatalf("failed selection created worktree: %v", err)
	}
}
