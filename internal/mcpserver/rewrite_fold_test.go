package mcpserver_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const rewriteBaselinePath = "memories/domains/time-camada-de-leitura.md"

const rewriteBaselinePage = `# Wuerike é do time da camada de leitura
<!-- @id: time-camada-de-leitura -->

## Overview
<!-- @id: time-camada-de-leitura-overview -->
O usuário se chama Wuerike e faz parte do time da camada de leitura.
`

const rewriteReplacement = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n- Guilherme\n"

type rewriteFixture struct {
	*env
	root     string // the release worktree
	hash     string
	proposal [2]string
	paths    [2]string
}

// newRewriteFixture proposes two team proposals, opens the release and, with a baseline page, reads its release hash.
func newRewriteFixture(t *testing.T, baseline bool) *rewriteFixture {
	t.Helper()
	files := map[string]string(nil)
	if baseline {
		files = map[string]string{rewriteBaselinePath: rewriteBaselinePage}
	}
	e := newEnv(t, files)
	fixture := &rewriteFixture{env: e}
	for index, title := range []string{"Wuerike é do time da camada de leitura", "Guilherme é membro do time da camada de leitura"} {
		arguments := map[string]any{"title": title, "content": title + "."}
		if index == 0 && baseline {
			arguments["content"] = "Update " + rewriteBaselinePath + ": " + title + "."
		}
		proposal := e.propose(t, arguments)
		fixture.proposal[index], fixture.paths[index] = proposal.ID, proposal.Path
	}
	fixture.root = e.openRelease(t)
	if baseline {
		result, page := e.pageRead(t, "release", rewriteBaselinePath, 8192)
		if result.IsError || page.Data == nil || page.Data.ContentHash == "" {
			t.Fatalf("memory_page_read = %+v, %+v", result, page)
		}
		fixture.hash = page.Data.ContentHash
	}
	return fixture
}

func (f *rewriteFixture) rewriteItem(proposalID, replacement string) map[string]any {
	return map[string]any{"operation": "rewrite_page", "proposal_id": proposalID, "target_path": rewriteBaselinePath, "content_hash": f.hash, "replacement": replacement}
}

func (f *rewriteFixture) call(t *testing.T, name string, arguments map[string]any) (*mcp.CallToolResult, string, string) {
	t.Helper()
	result := f.env.call(t, name, arguments)
	var decoded struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, result, &decoded)
	code := ""
	if decoded.Error != nil {
		code = decoded.Error.Code
	}
	return result, decoded.Data.PreviewToken, code
}

func (f *rewriteFixture) preview(t *testing.T, item map[string]any) string {
	t.Helper()
	result, token, _ := f.call(t, "memory_promote_preview", item)
	if result.IsError || token == "" {
		t.Fatalf("memory_promote_preview = %+v", result)
	}
	return token
}

func (f *rewriteFixture) batchPreview(t *testing.T, items ...map[string]any) string {
	t.Helper()
	result, token, _ := f.call(t, "memory_promote_batch_preview", map[string]any{"items": items})
	if result.IsError || token == "" {
		t.Fatalf("memory_promote_batch_preview = %+v", result)
	}
	return token
}

func (f *rewriteFixture) wantInvalid(t *testing.T, tool, token string) {
	t.Helper()
	head := gitOutput(t, f.root, "rev-parse", "HEAD")
	result, _, code := f.call(t, tool, map[string]any{"preview_token": token})
	if !result.IsError || code != "invalid_preview" {
		t.Fatalf("%s(%s) = %+v, code %q, want invalid_preview", tool, token, result, code)
	}
	if got := gitOutput(t, f.root, "rev-parse", "HEAD"); got != head {
		t.Fatalf("rejected apply moved HEAD from %s to %s", head, got)
	}
}

func (f *rewriteFixture) wantPage(t *testing.T, want string) {
	t.Helper()
	page, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rewriteBaselinePath)))
	if err != nil || string(page) != want {
		t.Fatalf("page = %q, want %q, err = %v", page, want, err)
	}
}

func TestBatchPreviewAcceptsRewriteAndFold(t *testing.T) {
	f := newRewriteFixture(t, true)
	parent := gitOutput(t, f.root, "rev-parse", "HEAD")
	result, token, _ := f.call(t, "memory_promote_batch_preview", map[string]any{"items": []map[string]any{
		f.rewriteItem(f.proposal[0], rewriteReplacement),
		{"operation": "fold_proposal", "proposal_id": f.proposal[1], "fold_into": f.proposal[0]},
	}})
	if result.IsError || token == "" {
		t.Fatalf("memory_promote_batch_preview = %+v", result)
	}
	var preview struct {
		Data struct {
			Items []struct {
				Operation  string `json:"operation"`
				ProposalID string `json:"proposal_id"`
				FoldInto   string `json:"fold_into"`
			} `json:"items"`
		} `json:"data"`
	}
	decodeStructured(t, result, &preview)
	if len(preview.Data.Items) != 2 || preview.Data.Items[0].Operation != "rewrite_page" || preview.Data.Items[1].Operation != "fold_proposal" || preview.Data.Items[1].FoldInto != f.proposal[0] {
		t.Fatalf("batch preview items = %+v", preview.Data.Items)
	}
	f.wantPage(t, rewriteBaselinePage)
	applyResult, _, code := f.call(t, "memory_promote_batch_apply", map[string]any{"preview_token": token})
	if applyResult.IsError {
		t.Fatalf("memory_promote_batch_apply = %+v, code %q", applyResult, code)
	}
	golden, err := os.ReadFile("../memory/testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	f.wantPage(t, string(golden))
	for _, path := range f.paths {
		if _, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(path))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("proposal %s remains: %v", path, err)
		}
	}
	if got := gitOutput(t, f.root, "rev-parse", "HEAD^"); got != parent {
		t.Fatalf("batch commit parent = %s, want %s (one commit)", got, parent)
	}
	if status := gitOutput(t, f.root, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatalf("dirty status after apply: %q", status)
	}
}

func TestSinglePreviewRejectsFold(t *testing.T) {
	f := newRewriteFixture(t, true)
	result, token, code := f.call(t, "memory_promote_preview", map[string]any{
		"operation": "fold_proposal", "proposal_id": f.proposal[1], "fold_into": f.proposal[0],
	})
	if !result.IsError || token != "" || code != "invalid_promotion_batch" {
		t.Fatalf("single fold preview = %+v, code %q", result, code)
	}
}

func TestNewPreviewInvalidatesOlderTokensForSameProposal(t *testing.T) {
	t.Run("same proposal single previews", func(t *testing.T) {
		f := newRewriteFixture(t, true)
		first := f.preview(t, f.rewriteItem(f.proposal[0], rewriteReplacement))
		second := f.preview(t, f.rewriteItem(f.proposal[0], "# Time\n\n## Membros\n- Outra pessoa\n"))
		f.wantInvalid(t, "memory_promote_apply", first)
		f.wantPage(t, rewriteBaselinePage)
		if result, _, code := f.call(t, "memory_promote_apply", map[string]any{"preview_token": second}); result.IsError {
			t.Fatalf("second apply = %+v, code %q", result, code)
		}
		f.wantPage(t, "# Time\n<!-- @id: time-camada-de-leitura -->\n\n## Membros\n<!-- @id: time-camada-de-leitura-membros -->\n- Outra pessoa\n")
	})
	t.Run("different proposal does not invalidate", func(t *testing.T) {
		f := newRewriteFixture(t, true)
		first := f.preview(t, f.rewriteItem(f.proposal[0], rewriteReplacement))
		f.preview(t, map[string]any{"operation": "create_page", "proposal_id": f.proposal[1], "target_path": "memories/domains/guilherme.md"})
		if result, _, code := f.call(t, "memory_promote_apply", map[string]any{"preview_token": first}); result.IsError {
			t.Fatalf("apply after unrelated preview = %+v, code %q", result, code)
		}
	})
	t.Run("batch preview invalidates single token sharing a proposal", func(t *testing.T) {
		f := newRewriteFixture(t, true)
		single := f.preview(t, f.rewriteItem(f.proposal[0], rewriteReplacement))
		batch := f.batchPreview(t, f.rewriteItem(f.proposal[0], rewriteReplacement),
			map[string]any{"operation": "fold_proposal", "proposal_id": f.proposal[1], "fold_into": f.proposal[0]})
		f.wantInvalid(t, "memory_promote_apply", single)
		if result, _, code := f.call(t, "memory_promote_batch_apply", map[string]any{"preview_token": batch}); result.IsError {
			t.Fatalf("batch apply = %+v, code %q", result, code)
		}
	})
	t.Run("single preview invalidates batch token sharing a proposal", func(t *testing.T) {
		f := newRewriteFixture(t, true)
		batch := f.batchPreview(t, f.rewriteItem(f.proposal[0], rewriteReplacement),
			map[string]any{"operation": "fold_proposal", "proposal_id": f.proposal[1], "fold_into": f.proposal[0]})
		single := f.preview(t, map[string]any{"operation": "create_page", "proposal_id": f.proposal[1], "target_path": "memories/domains/guilherme.md"})
		f.wantInvalid(t, "memory_promote_batch_apply", batch)
		f.wantPage(t, rewriteBaselinePage)
		if result, _, code := f.call(t, "memory_promote_apply", map[string]any{"preview_token": single}); result.IsError {
			t.Fatalf("single apply = %+v, code %q", result, code)
		}
	})
	t.Run("failing preview keeps existing tokens", func(t *testing.T) {
		f := newRewriteFixture(t, true)
		first := f.preview(t, f.rewriteItem(f.proposal[0], rewriteReplacement))
		bad := f.rewriteItem(f.proposal[0], rewriteReplacement)
		bad["content_hash"] = strings.Repeat("0", 64)
		if result, token, _ := f.call(t, "memory_promote_preview", bad); !result.IsError || token != "" {
			t.Fatalf("stale preview = %+v", result)
		}
		if result, _, code := f.call(t, "memory_promote_apply", map[string]any{"preview_token": first}); result.IsError {
			t.Fatalf("apply after failing preview = %+v, code %q", result, code)
		}
	})
}

func TestBatchPreviewAcceptsCreatePageWithReplacementAndFold(t *testing.T) {
	f := newRewriteFixture(t, false)
	root := f.root
	parent := gitOutput(t, root, "rev-parse", "HEAD")
	token := f.batchPreview(t,
		map[string]any{"operation": "create_page", "proposal_id": f.proposal[0], "target_path": rewriteBaselinePath, "replacement": rewriteReplacement},
		map[string]any{"operation": "fold_proposal", "proposal_id": f.proposal[1], "fold_into": f.proposal[0]},
	)
	if exists(root, rewriteBaselinePath) {
		t.Fatal("preview created the page")
	}
	if result, _, code := f.call(t, "memory_promote_batch_apply", map[string]any{"preview_token": token}); result.IsError {
		t.Fatalf("memory_promote_batch_apply = %+v, code %q", result, code)
	}
	golden, err := os.ReadFile("../memory/testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	f.wantPage(t, string(golden))
	for _, path := range f.paths {
		if exists(root, path) {
			t.Fatalf("proposal %s remains", path)
		}
	}
	if got := gitOutput(t, root, "rev-parse", "HEAD^"); got != parent {
		t.Fatalf("batch commit parent = %s, want %s (one commit)", got, parent)
	}
	if message := gitOutput(t, root, "show", "-s", "--format=%B", "HEAD"); strings.Count(message, "Git-Push-Memories-Decision: consumed") != 2 {
		t.Fatalf("commit message lacks two consumed trailers:\n%s", message)
	}
	if status := gitOutput(t, root, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatalf("dirty status after apply: %q", status)
	}
}
