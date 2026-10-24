package promotion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
)

const (
	rewritePagePath    = "memories/domains/time-camada-de-leitura.md"
	rewriteBaseline    = "# Wuerike é do time da camada de leitura\n<!-- @id: time-camada-de-leitura -->\n\n## Overview\n<!-- @id: time-camada-de-leitura-overview -->\nO usuário se chama Wuerike e faz parte do time da camada de leitura.\n"
	rewriteReplacement = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n- Guilherme\n"
)

type rewriteFixture struct {
	batchFixture
	proposals   []inbox.Proposal
	contentHash string
	requests    []PreviewRequest
}

// newRewriteFixture commits the one-section team page, files proposal A (update,
// targeting the page) and proposal B (create), and previews the batch
// [rewrite_page(A), fold_proposal(B -> A)].
func newRewriteFixture(t *testing.T) rewriteFixture {
	t.Helper()
	batch, proposals := newBatchPromoter(t, map[string]string{rewritePagePath: rewriteBaseline}, []inbox.Request{
		{Title: "Wuerike é do time da camada de leitura", SuggestedAction: inbox.ActionUpdate, Target: rewritePagePath, Knowledge: "Wuerike faz parte do time da camada de leitura.", SourcePaths: []string{}},
		{Title: "Guilherme é membro do time da camada de leitura", SuggestedAction: inbox.ActionCreate, Knowledge: "Guilherme é membro do time da camada de leitura.", SourcePaths: []string{}},
		{Title: "Time usa Go", SuggestedAction: inbox.ActionCreate, Knowledge: "O time da camada de leitura usa Go.", SourcePaths: []string{}},
	})
	promoter := batch.promoter
	page, err := promoter.repository.ReadPage(context.Background(), rewritePagePath)
	if err != nil {
		t.Fatal(err)
	}
	requests := []PreviewRequest{
		{Operation: OperationRewritePage, ProposalID: proposals[0].ID, TargetPath: rewritePagePath, ContentHash: page.ContentHash, Replacement: rewriteReplacement},
		{Operation: OperationFoldProposal, ProposalID: proposals[1].ID, FoldInto: proposals[0].ID},
	}
	preview, err := promoter.PreviewBatch(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	batch.preview = preview
	batch.proposalPaths = []string{proposals[0].Path, proposals[1].Path}
	return rewriteFixture{batchFixture: batch, proposals: proposals, contentHash: page.ContentHash, requests: requests}
}

func readRewriteGolden(t *testing.T) string {
	t.Helper()
	golden, err := os.ReadFile("../memory/testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(golden)
}

func readRewritePage(t *testing.T, fixture rewriteFixture) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(fixture.root, rewritePagePath))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestRewritePageConsumesTwoProposalsInOneCommit(t *testing.T) {
	fixture := newRewriteFixture(t)
	defer fixture.promoter.Close()
	if items := fixture.preview.Items; len(items) != 2 || items[0].SectionID != "time-camada-de-leitura" || items[0].ContentHash != fixture.contentHash ||
		items[1].TargetPath != "" || items[1].FoldInto != fixture.proposals[0].ID {
		t.Fatalf("PreviewBatch() items = %+v", items)
	}

	result, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Commit == "" || gitOutput(t, fixture.root, "rev-parse", "HEAD") != result.Commit {
		t.Fatalf("ApplyBatch() = %+v", result)
	}
	if count := gitOutput(t, fixture.root, "rev-list", "--count", fixture.parent+"..HEAD"); count != "1" {
		t.Fatalf("commits after baseline = %s, want 1", count)
	}
	for index, item := range result.Items {
		if item.Decision != "consumed" || item.ProposalID != fixture.proposals[index].ID {
			t.Fatalf("result item %d = %+v", index, item)
		}
	}
	if result.Items[0].TargetPath != rewritePagePath || result.Items[0].SectionID != "time-camada-de-leitura" || result.Items[1].TargetPath != "" {
		t.Fatalf("result targets = %+v", result.Items)
	}
	if got, want := readRewritePage(t, fixture), readRewriteGolden(t); got != want {
		t.Fatalf("rewritten page = %q, want golden %q", got, want)
	}
	if committed := gitOutput(t, fixture.root, "show", "HEAD:"+rewritePagePath); committed+"\n" != readRewriteGolden(t) {
		t.Fatalf("committed page = %q", committed)
	}
	changed := strings.Split(gitOutput(t, fixture.root, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"), "\n")
	want := []string{fixture.proposalPaths[0], fixture.proposalPaths[1], rewritePagePath}
	slices.Sort(changed)
	slices.Sort(want)
	if !slices.Equal(changed, want) {
		t.Fatalf("batch paths = %v, want %v", changed, want)
	}
	for _, path := range fixture.proposalPaths {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("consumed proposal remains: %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.root, fixture.proposals[2].Path)); err != nil {
		t.Fatalf("unrelated proposal was removed: %v", err)
	}
	message := gitOutput(t, fixture.root, "show", "-s", "--format=%B", "HEAD")
	if count := strings.Count(message, "Git-Push-Memories-Decision: consumed"); count != 2 {
		t.Fatalf("commit message has %d consumed trailers, want 2:\n%s", count, message)
	}
	for _, proposal := range fixture.proposals[:2] {
		if !strings.Contains(message, "Git-Push-Memories-Proposal: "+proposal.ID+"\nGit-Push-Memories-Decision: consumed") {
			t.Fatalf("commit message lacks trailer for %s:\n%s", proposal.ID, message)
		}
	}
	assertTransactionClean(t, fixture.batchFixture)
}

func TestRewritePageDiffShowsRemovedSections(t *testing.T) {
	fixture := newRewriteFixture(t)
	defer fixture.promoter.Close()
	rewrite, fold := fixture.preview.Items[0], fixture.preview.Items[1]
	for _, line := range []string{
		"-## Overview",
		"-O usuário se chama Wuerike e faz parte do time da camada de leitura.",
		"+## Membros",
		"+- Guilherme",
	} {
		if !strings.Contains(rewrite.Diff, line) {
			t.Fatalf("rewrite diff lacks %q:\n%s", line, rewrite.Diff)
		}
	}
	if !strings.Contains(rewrite.Diff, fixture.proposalPaths[0]) {
		t.Fatalf("rewrite diff lacks proposal removal:\n%s", rewrite.Diff)
	}
	if strings.Contains(fold.Diff, rewritePagePath) || !strings.Contains(fold.Diff, fixture.proposalPaths[1]) {
		t.Fatalf("fold diff must only remove its proposal:\n%s", fold.Diff)
	}
	if !strings.Contains(fixture.preview.Diff, rewrite.Diff) || !strings.Contains(fixture.preview.Diff, fold.Diff) {
		t.Fatalf("batch diff does not include every item diff:\n%s", fixture.preview.Diff)
	}
}

func TestRewritePageDriftFailsApplyWithoutWrites(t *testing.T) {
	fixture := newRewriteFixture(t)
	defer fixture.promoter.Close()
	drifted := rewriteBaseline + "\nConcurrent edit.\n"
	if err := os.WriteFile(filepath.Join(fixture.root, rewritePagePath), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, fixture.root, "commit", "--quiet", "-am", "concurrent edit")
	driftedHead := gitOutput(t, fixture.root, "rev-parse", "HEAD")

	if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !isDrift(err) {
		t.Fatalf("ApplyBatch() error = %v, want drift", err)
	}
	if head := gitOutput(t, fixture.root, "rev-parse", "HEAD"); head != driftedHead {
		t.Fatalf("drifted apply moved HEAD to %s", head)
	}
	if got := readRewritePage(t, fixture); got != drifted {
		t.Fatalf("drifted apply wrote the page: %q", got)
	}
	for _, path := range fixture.proposalPaths {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); err != nil {
			t.Fatalf("drifted apply removed %s: %v", path, err)
		}
	}
	if status := gitOutput(t, fixture.root, "status", "--porcelain"); status != "" {
		t.Fatalf("drifted apply left changes: %q", status)
	}
	assertTransactionClean(t, fixture.batchFixture)

	stale := fixture.requests[0]
	if _, err := fixture.promoter.PreviewBatch(context.Background(), []PreviewRequest{stale, fixture.requests[1]}); !isDrift(err) {
		t.Fatalf("PreviewBatch(stale content hash) error = %v, want drift", err)
	}
}

func isDrift(err error) bool {
	return errors.Is(err, ErrDrift) || errors.Is(err, memory.ErrDrift)
}

func TestFoldProposalValidation(t *testing.T) {
	fixture := newRewriteFixture(t)
	defer fixture.promoter.Close()
	rewrite := fixture.requests[0]
	a, b, c := fixture.proposals[0].ID, fixture.proposals[1].ID, fixture.proposals[2].ID
	ctx := context.Background()

	t.Run("fold alone via Preview", func(t *testing.T) {
		if _, err := fixture.promoter.Preview(ctx, PreviewRequest{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a}); !errors.Is(err, ErrInvalidBatch) {
			t.Fatalf("Preview(fold) error = %v, want ErrInvalidBatch", err)
		}
	})
	for _, test := range []struct {
		name     string
		requests []PreviewRequest
	}{
		{name: "unknown FoldInto", requests: []PreviewRequest{rewrite, {Operation: OperationFoldProposal, ProposalID: b, FoldInto: c}}},
		{name: "FoldInto is a discard", requests: []PreviewRequest{
			{Operation: OperationDiscard, ProposalID: a, Reason: DiscardDuplicate},
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a},
		}},
		{name: "fold of itself", requests: []PreviewRequest{rewrite, {Operation: OperationFoldProposal, ProposalID: b, FoldInto: b}}},
		{name: "fold chain", requests: []PreviewRequest{
			rewrite,
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a},
			{Operation: OperationFoldProposal, ProposalID: c, FoldInto: b},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := fixture.promoter.PreviewBatch(ctx, test.requests); !errors.Is(err, ErrInvalidBatch) {
				t.Fatalf("PreviewBatch() error = %v, want ErrInvalidBatch", err)
			}
		})
	}
	t.Run("fold before its rewrite is accepted", func(t *testing.T) {
		if _, err := fixture.promoter.PreviewBatch(ctx, []PreviewRequest{fixture.requests[1], rewrite}); err != nil {
			t.Fatalf("PreviewBatch(fold first) error = %v", err)
		}
	})
	t.Run("fold forbids other fields", func(t *testing.T) {
		for _, fold := range []PreviewRequest{
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a, TargetPath: "memories/domains/other.md"},
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a, SectionID: "time-camada-de-leitura"},
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a, ContentHash: fixture.contentHash},
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a, Heading: "Membros"},
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a, Replacement: "x"},
			{Operation: OperationFoldProposal, ProposalID: b, FoldInto: a, Reason: DiscardDuplicate},
		} {
			if _, err := fixture.promoter.PreviewBatch(ctx, []PreviewRequest{rewrite, fold}); !errors.Is(err, memory.ErrInvalidPath) {
				t.Fatalf("PreviewBatch(%+v) error = %v, want ErrInvalidPath", fold, err)
			}
		}
		missing := PreviewRequest{Operation: OperationFoldProposal, ProposalID: b}
		if _, err := fixture.promoter.PreviewBatch(ctx, []PreviewRequest{rewrite, missing}); !errors.Is(err, ErrInvalidBatch) {
			t.Fatalf("PreviewBatch(fold without FoldInto) error = %v, want ErrInvalidBatch", err)
		}
	})
	t.Run("fold item revalidates outside the public Preview", func(t *testing.T) {
		if err := fixture.promoter.RevalidatePreview(ctx, fixture.preview.Items[1]); err != nil {
			t.Fatalf("RevalidatePreview(fold) error = %v", err)
		}
	})
	t.Run("rewrite forbids other fields", func(t *testing.T) {
		for _, bad := range []PreviewRequest{
			{Operation: OperationRewritePage, ProposalID: a, TargetPath: rewritePagePath, Replacement: rewriteReplacement},
			{Operation: OperationRewritePage, ProposalID: a, TargetPath: rewritePagePath, ContentHash: fixture.contentHash},
			{Operation: OperationRewritePage, ProposalID: a, ContentHash: fixture.contentHash, Replacement: rewriteReplacement},
			{Operation: OperationRewritePage, ProposalID: a, TargetPath: rewritePagePath, ContentHash: fixture.contentHash, Replacement: rewriteReplacement, SectionID: "time-camada-de-leitura"},
			{Operation: OperationRewritePage, ProposalID: a, TargetPath: rewritePagePath, ContentHash: fixture.contentHash, Replacement: rewriteReplacement, Heading: "Membros"},
			{Operation: OperationRewritePage, ProposalID: a, TargetPath: rewritePagePath, ContentHash: fixture.contentHash, Replacement: rewriteReplacement, Reason: DiscardDuplicate},
			{Operation: OperationRewritePage, ProposalID: a, TargetPath: rewritePagePath, ContentHash: fixture.contentHash, Replacement: rewriteReplacement, FoldInto: b},
		} {
			if _, err := fixture.promoter.Preview(ctx, bad); !errors.Is(err, memory.ErrInvalidPath) {
				t.Fatalf("Preview(%+v) error = %v, want ErrInvalidPath", bad, err)
			}
		}
	})
	t.Run("two rewrites of one page", func(t *testing.T) {
		second := rewrite
		second.ProposalID = b
		if _, err := fixture.promoter.PreviewBatch(ctx, []PreviewRequest{rewrite, second}); !errors.Is(err, ErrInvalidBatch) {
			t.Fatalf("PreviewBatch(two rewrites) error = %v, want ErrInvalidBatch", err)
		}
	})
	t.Run("rewrite accepts any suggested action", func(t *testing.T) {
		for _, action := range []inbox.Action{inbox.ActionCreate, inbox.ActionUpdate, inbox.ActionRemove} {
			for _, operation := range []Operation{OperationRewritePage, OperationFoldProposal} {
				if !supports(operation, action) {
					t.Fatalf("supports(%s, %s) = false", operation, action)
				}
			}
		}
		single, err := fixture.promoter.Preview(ctx, PreviewRequest{Operation: OperationRewritePage, ProposalID: b, TargetPath: rewritePagePath, ContentHash: fixture.contentHash, Replacement: rewriteReplacement})
		if err != nil || single.SectionID != "time-camada-de-leitura" {
			t.Fatalf("Preview(rewrite of create proposal) = %+v, %v", single, err)
		}
	})
	t.Run("fold is part of the batch hash", func(t *testing.T) {
		changed := slices.Clone(fixture.preview.Items)
		changed[1].FoldInto = c
		if batchPreviewHash(changed) == batchPreviewHash(fixture.preview.Items) || samePreview(changed[1], fixture.preview.Items[1]) {
			t.Fatal("FoldInto does not affect the batch hash or preview identity")
		}
	})
}

func TestRewritePageRejectsLeakedSecretInReplacement(t *testing.T) {
	fixture := newRewriteFixture(t)
	defer fixture.promoter.Close()
	leaked := fixture.requests[0]
	leaked.Replacement = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n\npassword=private-value\n"
	_, err := fixture.promoter.PreviewBatch(context.Background(), []PreviewRequest{leaked, fixture.requests[1]})
	var rejection intake.Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("PreviewBatch(leaked secret) error = %v, want guard rejection", err)
	}
	if _, err := fixture.promoter.Preview(context.Background(), leaked); !errors.As(err, &rejection) {
		t.Fatalf("Preview(leaked secret) error = %v, want guard rejection", err)
	}
	if got := readRewritePage(t, fixture); got != rewriteBaseline {
		t.Fatalf("rejected rewrite wrote the page: %q", got)
	}
}

func TestRewriteBatchFaultWindowsConverge(t *testing.T) {
	for _, test := range []struct {
		stage     string
		published bool
	}{
		{stage: "after_marker"},
		{stage: "after_index_lock"},
		{stage: "after_worktree_1"},
		{stage: "after_worktree_2"},
		{stage: "before_ref_update"},
		{stage: "after_ref_update", published: true},
		{stage: "after_index_install", published: true},
		{stage: "after_proposal_removal_1", published: true},
		{stage: "after_proposal_removal_2", published: true},
		{stage: "before_marker_remove", published: true},
	} {
		t.Run(test.stage, func(t *testing.T) {
			fixture := newRewriteFixture(t)
			interrupted := errors.New("injected interruption")
			fixture.promoter.fault = func(stage string) error {
				if stage == test.stage {
					return interrupted
				}
				return nil
			}
			if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
				t.Fatalf("ApplyBatch() error = %v, want interruption", err)
			}
			interruptedHead := gitOutput(t, fixture.root, "rev-parse", "HEAD")
			_ = fixture.promoter.Close()
			recovery, err := New(fixture.cfg, fixture.root, fixture.branch)
			if err != nil {
				t.Fatal(err)
			}
			result, err := recovery.Recover(context.Background())
			_ = recovery.Close()
			if err != nil {
				t.Fatal(err)
			}
			if test.published {
				if interruptedHead == fixture.parent || result.Commit != interruptedHead || gitOutput(t, fixture.root, "rev-parse", "HEAD") != interruptedHead {
					t.Fatalf("published recovery = %+v, interrupted HEAD=%s", result, interruptedHead)
				}
				if len(result.Items) != 2 || result.Items[0].Decision != "consumed" || result.Items[1].Decision != "consumed" {
					t.Fatalf("published recovery items = %+v", result.Items)
				}
				if got, want := readRewritePage(t, fixture), readRewriteGolden(t); got != want {
					t.Fatalf("published recovery page = %q, want golden", got)
				}
				for _, path := range fixture.proposalPaths {
					if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("published recovery retained %s", path)
					}
				}
			} else {
				if interruptedHead != fixture.parent || result.Commit != "" || gitOutput(t, fixture.root, "rev-parse", "HEAD") != fixture.parent {
					t.Fatalf("baseline recovery = %+v, interrupted HEAD=%s", result, interruptedHead)
				}
				if got := readRewritePage(t, fixture); got != rewriteBaseline {
					t.Fatalf("baseline recovery page = %q, want baseline", got)
				}
				for _, path := range fixture.proposalPaths {
					if _, err := os.Stat(filepath.Join(fixture.root, path)); err != nil {
						t.Fatalf("baseline recovery removed %s", path)
					}
				}
			}
			if status := gitOutput(t, fixture.root, "status", "--porcelain"); status != "" {
				t.Fatalf("recovery left changes: %q", status)
			}
			assertTransactionClean(t, fixture.batchFixture)
		})
	}
}

func TestRewritePageAfterMarkerRemovalIsRecognized(t *testing.T) {
	fixture := newRewriteFixture(t)
	interrupted := errors.New("interrupted after marker removal")
	fixture.promoter.fault = func(stage string) error {
		if stage == "after_marker_remove" {
			return interrupted
		}
		return nil
	}
	if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
		t.Fatalf("ApplyBatch() error = %v, want interruption", err)
	}
	commit := gitOutput(t, fixture.root, "rev-parse", "HEAD")
	if _, err := os.Stat(filepath.Join(fixture.stateRoot, "promotion-transaction.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interruption occurred before marker removal")
	}
	_ = fixture.promoter.Close()
	retry, err := New(fixture.cfg, fixture.root, fixture.branch)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Close()
	result, err := retry.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil || result.Commit != commit || gitOutput(t, fixture.root, "rev-parse", "HEAD") != commit {
		t.Fatalf("retry = %+v, %v; want commit %s", result, err, commit)
	}
	if len(result.Items) != 2 || result.Items[1].Decision != "consumed" || result.Items[1].TargetPath != "" {
		t.Fatalf("retry items = %+v", result.Items)
	}
	if got, want := readRewritePage(t, fixture), readRewriteGolden(t); got != want {
		t.Fatalf("recognized page = %q, want golden", got)
	}
	assertTransactionClean(t, fixture.batchFixture)
}

// newCreateReplacementFixture files two create proposals with no page on disk and
// previews the batch [create_page(A, replacement), fold_proposal(B -> A)].
func newCreateReplacementFixture(t *testing.T) rewriteFixture {
	t.Helper()
	batch, proposals := newBatchPromoter(t, nil, []inbox.Request{
		{Title: "Wuerike é do time da camada de leitura", SuggestedAction: inbox.ActionCreate, Knowledge: "Wuerike faz parte do time da camada de leitura.", SourcePaths: []string{}},
		{Title: "Guilherme é membro do time da camada de leitura", SuggestedAction: inbox.ActionCreate, Knowledge: "Guilherme é membro do time da camada de leitura.", SourcePaths: []string{}},
		{Title: "Time usa Go", SuggestedAction: inbox.ActionCreate, Knowledge: "O time da camada de leitura usa Go.", SourcePaths: []string{}},
	})
	promoter := batch.promoter
	requests := []PreviewRequest{
		{Operation: OperationCreatePage, ProposalID: proposals[0].ID, TargetPath: rewritePagePath, Replacement: rewriteReplacement},
		{Operation: OperationFoldProposal, ProposalID: proposals[1].ID, FoldInto: proposals[0].ID},
	}
	preview, err := promoter.PreviewBatch(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	batch.preview = preview
	batch.proposalPaths = []string{proposals[0].Path, proposals[1].Path}
	return rewriteFixture{batchFixture: batch, proposals: proposals, requests: requests}
}

func assertPageAbsent(t *testing.T, fixture rewriteFixture) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(fixture.root, rewritePagePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("page exists: %v", err)
	}
}

func TestCreatePageWithReplacementConsumesTwoProposalsInOneCommit(t *testing.T) {
	fixture := newCreateReplacementFixture(t)
	defer fixture.promoter.Close()
	if items := fixture.preview.Items; len(items) != 2 || items[0].SectionID != "time-camada-de-leitura" || items[0].Replacement != rewriteReplacement ||
		items[1].TargetPath != "" || items[1].FoldInto != fixture.proposals[0].ID {
		t.Fatalf("PreviewBatch() items = %+v", items)
	}
	if !strings.Contains(fixture.preview.Items[0].Diff, "+- Guilherme") {
		t.Fatalf("create diff lacks folded knowledge:\n%s", fixture.preview.Items[0].Diff)
	}
	assertPageAbsent(t, fixture)
	altered := slices.Clone(fixture.preview.Items)
	altered[0].Replacement = "# Outra\n"
	if batchPreviewHash(altered) == batchPreviewHash(fixture.preview.Items) || samePreview(altered[0], fixture.preview.Items[0]) {
		t.Fatal("create_page replacement does not affect the batch hash or preview identity")
	}

	result, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Commit == "" || gitOutput(t, fixture.root, "rev-parse", "HEAD") != result.Commit {
		t.Fatalf("ApplyBatch() = %+v", result)
	}
	if count := gitOutput(t, fixture.root, "rev-list", "--count", fixture.parent+"..HEAD"); count != "1" {
		t.Fatalf("commits after baseline = %s, want 1", count)
	}
	for index, item := range result.Items {
		if item.Decision != "consumed" || item.ProposalID != fixture.proposals[index].ID {
			t.Fatalf("result item %d = %+v", index, item)
		}
	}
	if result.Items[0].TargetPath != rewritePagePath || result.Items[0].SectionID != "time-camada-de-leitura" || result.Items[1].TargetPath != "" {
		t.Fatalf("result targets = %+v", result.Items)
	}
	if got, want := readRewritePage(t, fixture), readRewriteGolden(t); got != want {
		t.Fatalf("created page = %q, want golden %q", got, want)
	}
	if committed := gitOutput(t, fixture.root, "show", "HEAD:"+rewritePagePath); committed+"\n" != readRewriteGolden(t) {
		t.Fatalf("committed page = %q", committed)
	}
	changed := strings.Split(gitOutput(t, fixture.root, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"), "\n")
	want := []string{fixture.proposalPaths[0], fixture.proposalPaths[1], rewritePagePath}
	slices.Sort(changed)
	slices.Sort(want)
	if !slices.Equal(changed, want) {
		t.Fatalf("batch paths = %v, want %v", changed, want)
	}
	for _, path := range fixture.proposalPaths {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("consumed proposal remains: %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.root, fixture.proposals[2].Path)); err != nil {
		t.Fatalf("unrelated proposal was removed: %v", err)
	}
	message := gitOutput(t, fixture.root, "show", "-s", "--format=%B", "HEAD")
	if count := strings.Count(message, "Git-Push-Memories-Decision: consumed"); count != 2 {
		t.Fatalf("commit message has %d consumed trailers, want 2:\n%s", count, message)
	}
	for _, proposal := range fixture.proposals[:2] {
		if !strings.Contains(message, "Git-Push-Memories-Proposal: "+proposal.ID+"\nGit-Push-Memories-Decision: consumed") {
			t.Fatalf("commit message lacks trailer for %s:\n%s", proposal.ID, message)
		}
	}
	assertTransactionClean(t, fixture.batchFixture)
}

func TestFoldIntoCreatePageWithoutReplacementIsInvalid(t *testing.T) {
	fixture := newCreateReplacementFixture(t)
	defer fixture.promoter.Close()
	plain := fixture.requests[0]
	plain.Replacement = ""
	if _, err := fixture.promoter.PreviewBatch(context.Background(), []PreviewRequest{plain, fixture.requests[1]}); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("PreviewBatch(fold into create_page without replacement) error = %v, want ErrInvalidBatch", err)
	}
	section := PreviewRequest{Operation: OperationCreateSection, ProposalID: fixture.proposals[0].ID, TargetPath: rewritePagePath, Heading: "Membros"}
	if _, err := fixture.promoter.PreviewBatch(context.Background(), []PreviewRequest{section, fixture.requests[1]}); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("PreviewBatch(fold into create_section) error = %v, want ErrInvalidBatch", err)
	}
	if single, err := fixture.promoter.Preview(context.Background(), plain); err != nil || single.SectionID != "time-camada-de-leitura-overview" {
		t.Fatalf("Preview(create_page without replacement) = %+v, %v", single, err)
	}
	assertPageAbsent(t, fixture)
	if status := gitOutput(t, fixture.root, "status", "--porcelain"); status != "" {
		t.Fatalf("rejected batch left changes: %q", status)
	}
}

func TestCreatePageWithReplacementRejectsLeakedSecret(t *testing.T) {
	fixture := newCreateReplacementFixture(t)
	defer fixture.promoter.Close()
	leaked := fixture.requests[0]
	leaked.Replacement = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n\npassword=private-value\n"
	_, err := fixture.promoter.PreviewBatch(context.Background(), []PreviewRequest{leaked, fixture.requests[1]})
	var rejection intake.Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("PreviewBatch(leaked secret) error = %v, want guard rejection", err)
	}
	if _, err := fixture.promoter.Preview(context.Background(), leaked); !errors.As(err, &rejection) {
		t.Fatalf("Preview(leaked secret) error = %v, want guard rejection", err)
	}
	assertPageAbsent(t, fixture)
}

func TestCreatePageWithReplacementFaultWindowsConverge(t *testing.T) {
	for _, test := range []struct {
		stage     string
		published bool
	}{
		{stage: "after_marker"},
		{stage: "after_index_lock"},
		{stage: "after_worktree_1"},
		{stage: "after_worktree_2"},
		{stage: "before_ref_update"},
		{stage: "after_ref_update", published: true},
		{stage: "after_index_install", published: true},
		{stage: "after_proposal_removal_1", published: true},
		{stage: "after_proposal_removal_2", published: true},
		{stage: "before_marker_remove", published: true},
	} {
		t.Run(test.stage, func(t *testing.T) {
			fixture := newCreateReplacementFixture(t)
			interrupted := errors.New("injected interruption")
			fixture.promoter.fault = func(stage string) error {
				if stage == test.stage {
					return interrupted
				}
				return nil
			}
			if _, err := fixture.promoter.ApplyBatch(context.Background(), fixture.preview, nil); !errors.Is(err, interrupted) {
				t.Fatalf("ApplyBatch() error = %v, want interruption", err)
			}
			interruptedHead := gitOutput(t, fixture.root, "rev-parse", "HEAD")
			_ = fixture.promoter.Close()
			recovery, err := New(fixture.cfg, fixture.root, fixture.branch)
			if err != nil {
				t.Fatal(err)
			}
			result, err := recovery.Recover(context.Background())
			_ = recovery.Close()
			if err != nil {
				t.Fatal(err)
			}
			if test.published {
				if interruptedHead == fixture.parent || result.Commit != interruptedHead || gitOutput(t, fixture.root, "rev-parse", "HEAD") != interruptedHead {
					t.Fatalf("published recovery = %+v, interrupted HEAD=%s", result, interruptedHead)
				}
				if len(result.Items) != 2 || result.Items[0].Decision != "consumed" || result.Items[1].Decision != "consumed" {
					t.Fatalf("published recovery items = %+v", result.Items)
				}
				if got, want := readRewritePage(t, fixture), readRewriteGolden(t); got != want {
					t.Fatalf("published recovery page = %q, want golden", got)
				}
				for _, path := range fixture.proposalPaths {
					if _, err := os.Stat(filepath.Join(fixture.root, path)); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("published recovery retained %s", path)
					}
				}
			} else {
				if interruptedHead != fixture.parent || result.Commit != "" || gitOutput(t, fixture.root, "rev-parse", "HEAD") != fixture.parent {
					t.Fatalf("baseline recovery = %+v, interrupted HEAD=%s", result, interruptedHead)
				}
				assertPageAbsent(t, fixture)
				for _, path := range fixture.proposalPaths {
					if _, err := os.Stat(filepath.Join(fixture.root, path)); err != nil {
						t.Fatalf("baseline recovery removed %s", path)
					}
				}
			}
			if status := gitOutput(t, fixture.root, "status", "--porcelain"); status != "" {
				t.Fatalf("recovery left changes: %q", status)
			}
			assertTransactionClean(t, fixture.batchFixture)
		})
	}
}
