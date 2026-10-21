package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

func TestReadPageReturnsFullMarkdownAndFileHash(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, root)
	page, err := repository.ReadPage(context.Background(), "memories/checkout-api.md")
	if err != nil {
		t.Fatal(err)
	}
	if page.Path != "memories/checkout-api.md" || page.Markdown != validPage || page.ContentHash != snapshot.FileHash {
		t.Fatalf("ReadPage() = %+v, want full page with hash %s", page, snapshot.FileHash)
	}
	if after := fingerprint(t, root); after != before {
		t.Fatal("ReadPage() changed the repository")
	}
}

func TestReadPageRejectsPathOutsideCategories(t *testing.T) {
	repository := openRepository(t, newMemoryRoot(t, validPage))
	for _, requested := range []string{"../outside.md", "/etc/passwd", "notes/checkout-api.md", "memories/checkout-api.txt", "memories/missing.md"} {
		if _, err := repository.ReadPage(context.Background(), requested); !errors.Is(err, memory.ErrInvalidPath) {
			t.Fatalf("ReadPage(%q) error = %v, want ErrInvalidPath", requested, err)
		}
	}
}

const rewriteTestPath = "memories/domains/time-camada-de-leitura.md"

const rewriteTestPage = "# Wuerike é do time da camada de leitura\n<!-- @id: time-camada-de-leitura -->\n\n## Overview\n<!-- @id: time-camada-de-leitura-overview -->\nO usuário se chama Wuerike.\n"

const rewriteTestReplacement = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n- Guilherme\n"

func newRewriteRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "memories", "domains"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rewriteTestPath), []byte(rewriteTestPage), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", rewriteTestPath)
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "initial")
	return root
}

func TestPreviewRewritePageDetectsDrift(t *testing.T) {
	root := newRewriteRoot(t)
	repository := openRepository(t, root)
	page, err := repository.ReadPage(context.Background(), rewriteTestPath)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := repository.PreviewRewritePage(context.Background(), memory.RewritePageRequest{TargetPath: rewriteTestPath, ContentHash: page.ContentHash, Replacement: rewriteTestReplacement})
	if err != nil {
		t.Fatal(err)
	}
	if preview.SectionHash != page.ContentHash || preview.Target.SectionID != "time-camada-de-leitura" || preview.Diff == "" {
		t.Fatalf("PreviewRewritePage() = %+v", preview)
	}
	if _, err := repository.PreviewRewritePage(context.Background(), memory.RewritePageRequest{TargetPath: rewriteTestPath, ContentHash: "sha256:wrong", Replacement: rewriteTestReplacement}); !errors.Is(err, memory.ErrDrift) {
		t.Fatalf("wrong hash error = %v, want ErrDrift", err)
	}
	if err := os.WriteFile(filepath.Join(root, rewriteTestPath), []byte(rewriteTestPage+"\nExtra.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.PreviewRewritePage(context.Background(), memory.RewritePageRequest{TargetPath: rewriteTestPath, ContentHash: page.ContentHash, Replacement: rewriteTestReplacement}); err == nil {
		t.Fatal("PreviewRewritePage() on modified file succeeded, want error")
	}
}

func TestReconcileRewritePageIsIdempotent(t *testing.T) {
	root := newRewriteRoot(t)
	repository := openRepository(t, root)
	page, err := repository.ReadPage(context.Background(), rewriteTestPath)
	if err != nil {
		t.Fatal(err)
	}
	request := memory.RewritePageRequest{TargetPath: rewriteTestPath, ContentHash: page.ContentHash, Replacement: rewriteTestReplacement}
	preview, err := repository.PreviewRewritePage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	reconcile := memory.ReconcileRewritePageRequest{RewritePageRequest: request, BaselineHash: preview.BaselineHash, PostStateHash: preview.PostStateHash, Head: preview.Head, Blob: preview.Blob, Mode: memory.ApplyBaseline}
	first, err := repository.ReconcileRewritePage(context.Background(), reconcile)
	if err != nil || first.State != memory.ReconcilePostState {
		t.Fatalf("first reconcile = %+v, %v", first, err)
	}
	written, err := os.ReadFile(filepath.Join(root, rewriteTestPath))
	if err != nil || string(written) != string(preview.PostState) {
		t.Fatalf("written = %q, %v", written, err)
	}
	second, err := repository.ReconcileRewritePage(context.Background(), reconcile)
	if err != nil || second.State != memory.ReconcilePostState {
		t.Fatalf("second reconcile = %+v, %v", second, err)
	}
}

func TestReconcileRewritePagePreservesFrontmatter(t *testing.T) {
	root := newRewriteRoot(t)
	page := "---\ntags: [a]\n---\n" + rewriteTestPage
	if err := os.WriteFile(filepath.Join(root, rewriteTestPath), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", rewriteTestPath)
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "frontmatter")
	repository := openRepository(t, root)
	read, err := repository.ReadPage(context.Background(), rewriteTestPath)
	if err != nil {
		t.Fatal(err)
	}
	request := memory.RewritePageRequest{TargetPath: rewriteTestPath, ContentHash: read.ContentHash, Replacement: rewriteTestReplacement}
	preview, err := repository.PreviewRewritePage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.ReconcileRewritePage(context.Background(), memory.ReconcileRewritePageRequest{RewritePageRequest: request, BaselineHash: preview.BaselineHash, PostStateHash: preview.PostStateHash, Head: preview.Head, Blob: preview.Blob, Mode: memory.ApplyBaseline})
	if err != nil || result.State != memory.ReconcilePostState {
		t.Fatalf("reconcile = %+v, %v", result, err)
	}
	written, _ := os.ReadFile(filepath.Join(root, rewriteTestPath))
	if string(written) != string(preview.PostState) || !strings.HasPrefix(string(written), "---\ntags: [a]\n---\n# Time") {
		t.Fatalf("written = %q", written)
	}
}

func readTeamGolden(t *testing.T) string {
	t.Helper()
	golden, err := os.ReadFile("testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(golden)
}

func TestCreatePageWithReplacementGeneratesIDsAndMatchesGolden(t *testing.T) {
	root := newEmptyMemoryRoot(t)
	repository := openRepository(t, root)
	before := fingerprint(t, root)
	preview, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{
		TargetPath: rewriteTestPath, Title: "ignored title", Body: "ignored body", Replacement: rewriteTestReplacement,
	})
	if err != nil {
		t.Fatal(err)
	}
	golden := readTeamGolden(t)
	if string(preview.PostState) != golden {
		t.Fatalf("PostState =\n%q\nwant golden\n%q", preview.PostState, golden)
	}
	if preview.Target != (memory.Target{Path: rewriteTestPath, SectionID: "time-camada-de-leitura"}) || preview.PageID != "time-camada-de-leitura" {
		t.Fatalf("preview identity = %+v", preview)
	}
	if !strings.Contains(preview.Diff, "+++ b/"+rewriteTestPath) || !strings.Contains(preview.Diff, "+<!-- @id: time-camada-de-leitura-membros -->") {
		t.Fatalf("Diff = %s", preview.Diff)
	}
	if got := fingerprint(t, root); got != before {
		t.Fatal("PreviewCreatePage() modified the memory root")
	}
}

func TestCreatePageWithReplacementRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name        string
		replacement string
		want        error
	}{
		{"h2 marker supplied", "# T\n\n## A\n<!-- @id: time-camada-de-leitura-a -->\nx\n", memory.ErrInvalidID},
		{"h1 id mismatch", "# T\n<!-- @id: outra-pagina -->\nx\n", memory.ErrInvalidID},
		{"frontmatter", "---\ntags: [a]\n---\n# T\n\n## A\nx\n", memory.ErrInvalidSchema},
		{"two h1", "# T\n\n# U\nx\n", memory.ErrInvalidSchema},
		{"no heading", "só texto\n", memory.ErrInvalidSchema},
		{"duplicate generated id", "# T\n\n## A b\nx\n\n## A-b\ny\n", memory.ErrSectionExists},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newEmptyMemoryRoot(t)
			repository := openRepository(t, root)
			before := fingerprint(t, root)
			if _, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{TargetPath: rewriteTestPath, Replacement: tc.replacement}); !errors.Is(err, tc.want) {
				t.Fatalf("PreviewCreatePage() error = %v, want %v", err, tc.want)
			}
			if got := fingerprint(t, root); got != before {
				t.Fatal("rejected preview modified the memory root")
			}
		})
	}
}

func TestCreatePageWithoutReplacementIsUnchanged(t *testing.T) {
	root := newEmptyMemoryRoot(t)
	repository := openRepository(t, root)
	request := memory.CreatePageRequest{TargetPath: rewriteTestPath, Title: "Time da camada de leitura", Body: "Wuerike e Guilherme."}
	preview, err := repository.PreviewCreatePage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Time da camada de leitura\n<!-- @id: time-camada-de-leitura -->\n\n## Overview\n<!-- @id: time-camada-de-leitura-overview -->\nWuerike e Guilherme.\n"
	if string(preview.PostState) != want || preview.Target != (memory.Target{Path: rewriteTestPath, SectionID: "time-camada-de-leitura-overview"}) {
		t.Fatalf("preview = %+v, PostState %q", preview, preview.PostState)
	}
	result, err := repository.ReconcileCreatePage(context.Background(), memory.ReconcileCreatePageRequest{
		CreatePageRequest: request, PostStateHash: preview.PostStateHash, Head: preview.Head, Mode: memory.ApplyBaseline,
	})
	if err != nil || result.State != memory.ReconcilePostState || result.Target.SectionID != "time-camada-de-leitura-overview" {
		t.Fatalf("reconcile = %+v, %v", result, err)
	}
	if written, err := os.ReadFile(filepath.Join(root, rewriteTestPath)); err != nil || string(written) != want {
		t.Fatalf("written = %q, %v", written, err)
	}
	if _, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{TargetPath: "memories/other.md", Body: "Body."}); !errors.Is(err, memory.ErrInvalidSchema) {
		t.Fatalf("missing title without replacement error = %v, want ErrInvalidSchema", err)
	}
}

func TestReconcileCreatePageWithReplacementMatchesPreview(t *testing.T) {
	root := newEmptyMemoryRoot(t)
	repository := openRepository(t, root)
	request := memory.CreatePageRequest{TargetPath: rewriteTestPath, Replacement: rewriteTestReplacement}
	preview, err := repository.PreviewCreatePage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	drifted := request
	drifted.Replacement = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n"
	if _, err := repository.ReconcileCreatePage(context.Background(), memory.ReconcileCreatePageRequest{
		CreatePageRequest: drifted, PostStateHash: preview.PostStateHash, Head: preview.Head, Mode: memory.ApplyBaseline,
	}); !errors.Is(err, memory.ErrDrift) {
		t.Fatalf("reconcile with a different replacement error = %v, want ErrDrift", err)
	}
	reconcile := memory.ReconcileCreatePageRequest{CreatePageRequest: request, PostStateHash: preview.PostStateHash, Head: preview.Head, Mode: memory.ApplyBaseline}
	first, err := repository.ReconcileCreatePage(context.Background(), reconcile)
	if err != nil || first.State != memory.ReconcilePostState || string(first.PostState) != string(preview.PostState) || first.Target != preview.Target {
		t.Fatalf("first reconcile = %+v, %v", first, err)
	}
	written, err := os.ReadFile(filepath.Join(root, rewriteTestPath))
	if err != nil || string(written) != string(preview.PostState) || string(written) != readTeamGolden(t) {
		t.Fatalf("written = %q, %v", written, err)
	}
	second, err := repository.ReconcileCreatePage(context.Background(), reconcile)
	if err != nil || second.State != memory.ReconcilePostState {
		t.Fatalf("second reconcile = %+v, %v", second, err)
	}
}

func TestCreatePageOverviewIDLimitAppliesOnlyToLegacyPath(t *testing.T) {
	pageID := strings.Repeat("a", 60)
	target := "memories/domains/" + pageID + ".md"
	t.Run("replacement with short headings", func(t *testing.T) {
		repository := openRepository(t, newEmptyMemoryRoot(t))
		preview, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{TargetPath: target, Replacement: "# T\n\n## A\nx\n"})
		if err != nil || preview.PageID != pageID || !strings.Contains(string(preview.PostState), "<!-- @id: "+pageID+"-a -->") {
			t.Fatalf("PreviewCreatePage() = %+v, %v", preview, err)
		}
	})
	t.Run("legacy overview", func(t *testing.T) {
		repository := openRepository(t, newEmptyMemoryRoot(t))
		if _, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{TargetPath: target, Title: "T", Body: "x"}); !errors.Is(err, memory.ErrGeneratedIDTooLong) {
			t.Fatalf("PreviewCreatePage() error = %v, want ErrGeneratedIDTooLong", err)
		}
	})
	t.Run("replacement with long generated h2 id", func(t *testing.T) {
		repository := openRepository(t, newEmptyMemoryRoot(t))
		if _, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{TargetPath: target, Replacement: "# T\n\n## Membros\nx\n"}); !errors.Is(err, memory.ErrGeneratedIDTooLong) {
			t.Fatalf("PreviewCreatePage() error = %v, want ErrGeneratedIDTooLong", err)
		}
	})
}
