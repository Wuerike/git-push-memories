package mcpserver_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// harness is a set-up repository plus the real gpm binary, driven over stdio like Claude does.
type harness struct {
	m      *workspacetest.Managed
	binary string
}

func newHarness(t *testing.T, files map[string]string) *harness {
	t.Helper()
	binary := buildBinary(t) // before setup redirects HOME, so the build keeps its caches
	m := workspacetest.Setup(t, files)
	quietRefresh(t, m)
	return &harness{m: m, binary: binary}
}

func (h *harness) connect(t *testing.T, name string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(h.binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func e2eCall(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("%s = %+v, %v", name, result, err)
	}
	return result
}

func e2eToken(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, result, &preview)
	if preview.Data.PreviewToken == "" {
		t.Fatal("preview returned no preview_token")
	}
	return preview.Data.PreviewToken
}

func e2eOpenRelease(t *testing.T, session *mcp.ClientSession) string {
	t.Helper()
	var status struct {
		Data struct {
			Open   bool `json:"open"`
			Local  bool `json:"local"`
			Marker struct {
				ReleaseBranch string `json:"release_branch"`
			} `json:"marker"`
		} `json:"data"`
	}
	decodeStructured(t, e2eCall(t, session, "memory_promote_open", map[string]any{}), &status)
	if !status.Data.Open || !status.Data.Local || status.Data.Marker.ReleaseBranch == "" {
		t.Fatalf("memory_promote_open = %+v", status)
	}
	return status.Data.Marker.ReleaseBranch
}

func TestClaudeAdapterCommitsProposalToMemoryRepository(t *testing.T) {
	h := newHarness(t, map[string]string{checkoutPath: memoryPage})
	session := h.connect(t, "claude-harness")
	result := e2eCall(t, session, "memory_propose", map[string]any{
		"title":   "Update checkout timeout",
		"content": "The checkout timeout is five seconds.",
	})
	response := decodeInboxResponse(t, result)
	if response.Data == nil {
		t.Fatalf("proposal result = %+v, response = %+v", result, response)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	proposals := h.m.Config.Paths.Proposals
	if remote := gitOutput(t, h.m.Remote, "show", response.Data.Commit+":"+response.Data.Path); remote != "The checkout timeout is five seconds." {
		t.Fatalf("remote proposal Markdown = %q", remote)
	}
	if gitOutput(t, h.m.Remote, "rev-parse", "refs/heads/proposals") != response.Data.Commit {
		t.Fatal("remote proposals does not point at the proposal commit")
	}
	if contents := readFile(t, proposals, checkoutPath); contents != memoryPage {
		t.Fatal("proposal changed canonical memory")
	}
	if status := gitOutput(t, proposals, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("proposal left Git dirty: %s", status)
	}
	if main := gitOutput(t, h.m.Remote, "ls-tree", "-r", "--name-only", "main"); strings.Contains(main, "proposals/") {
		t.Fatalf("proposal reached main: %q", main)
	}
}

func TestClaudeAdapterPromotesCreateProposalForFreshProcessRetrieval(t *testing.T) {
	h := newHarness(t, nil)
	session := h.connect(t, "claude-promotion-harness")
	proposal := decodeInboxResponse(t, e2eCall(t, session, "memory_propose", map[string]any{
		"title": "Settlement retry policy", "content": "Settlement retries stop after three failures.",
	})).Data
	e2eOpenRelease(t, session)
	token := e2eToken(t, e2eCall(t, session, "memory_promote_preview", map[string]any{
		"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/settlement-retry-policy.md",
	}))
	e2eCall(t, session, "memory_promote_apply", map[string]any{"preview_token": token})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	fresh := h.connect(t, "fresh-retrieval-harness")
	searchResult := e2eCall(t, fresh, "memory_search", map[string]any{"query": "settlement retry failures", "budget_bytes": 2048, "source": "release"})
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 || search.Data.Items[0].Handle.Path != "memories/domains/settlement-retry-policy.md" || !strings.Contains(search.Data.Items[0].Content, "Settlement retries stop after three failures") {
		t.Fatalf("fresh search = %+v", search.Data)
	}
	// Until the release pull request is merged, published memory does not change.
	if published := decodeResponse(t, e2eCall(t, fresh, "memory_search", map[string]any{"query": "settlement retry failures", "budget_bytes": 2048})); published.Data == nil || len(published.Data.Items) != 0 {
		t.Fatalf("fresh main search = %+v", published.Data)
	}
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeAdapterPromotesUpdateForFreshProcessRetrieval(t *testing.T) {
	h := newHarness(t, map[string]string{checkoutPath: memoryPage})
	session := h.connect(t, "claude-update-harness")
	searchResult := e2eCall(t, session, "memory_search", map[string]any{"query": "checkout timeout team-b", "budget_bytes": 2048})
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("memory_search = %+v", search.Data)
	}
	handle := search.Data.Items[0].Handle
	proposal := decodeInboxResponse(t, e2eCall(t, session, "memory_propose", map[string]any{
		"title": "Update checkout timeout", "content": "The checkout timeout is five seconds.",
	})).Data
	e2eOpenRelease(t, session)
	token := e2eToken(t, e2eCall(t, session, "memory_promote_preview", map[string]any{
		"operation": "replace_section", "proposal_id": proposal.ID, "target_path": handle.Path,
		"section_id": handle.SectionID, "content_hash": handle.ContentHash,
		"replacement": "- timeout: cinco segundos\n- owner: team-b\n",
	}))
	e2eCall(t, session, "memory_promote_apply", map[string]any{"preview_token": token})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	fresh := h.connect(t, "fresh-update-retrieval")
	freshResult := e2eCall(t, fresh, "memory_search", map[string]any{"query": "checkout cinco team-b", "budget_bytes": 2048, "source": "release"})
	got := decodeResponse(t, freshResult)
	if got.Data == nil || len(got.Data.Items) != 1 || !strings.Contains(got.Data.Items[0].Content, "cinco segundos") || strings.Contains(got.Data.Items[0].Content, "tres segundos") {
		t.Fatalf("fresh update search = %+v", got.Data)
	}
	if published := decodeResponse(t, e2eCall(t, fresh, "memory_search", map[string]any{"query": "checkout timeout team-b", "budget_bytes": 2048})); published.Data == nil || len(published.Data.Items) != 1 || !strings.Contains(published.Data.Items[0].Content, "tres segundos") {
		t.Fatalf("published memory changed before review: %+v", published.Data)
	}
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestE2EReadingTeamPageIsRestructuredByRewrite(t *testing.T) {
	h := newHarness(t, map[string]string{rewriteBaselinePath: rewriteBaselinePage})
	session := h.connect(t, "e2e-rewrite")
	first := e2ePropose(t, session, map[string]any{
		"title": "Wuerike é do time da camada de leitura", "content": "Wuerike faz parte do time da camada de leitura.",
	})
	second := e2ePropose(t, session, map[string]any{
		"title": "Guilherme é membro do time da camada de leitura", "content": "Guilherme é membro do time da camada de leitura.",
	})
	e2eOpenRelease(t, session)
	release := h.m.Config.Paths.Release
	parent := gitOutput(t, release, "rev-parse", "HEAD") // the release branch, cut after both proposals
	read := e2eCall(t, session, "memory_page_read", map[string]any{"path": rewriteBaselinePath, "budget_bytes": 65536, "source": "release"})
	var page pageReadResponse
	decodeStructured(t, read, &page)
	if page.Data == nil || page.Data.ContentHash == "" || page.Data.Markdown != rewriteBaselinePage {
		t.Fatalf("memory_page_read = %+v", page.Data)
	}
	token := e2eBatchPreview(t, session,
		map[string]any{"operation": "rewrite_page", "proposal_id": first.ID, "target_path": rewriteBaselinePath, "content_hash": page.Data.ContentHash, "replacement": rewriteReplacement},
		map[string]any{"operation": "fold_proposal", "proposal_id": second.ID, "fold_into": first.ID},
	)
	e2eCall(t, session, "memory_promote_batch_apply", map[string]any{"preview_token": token})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	e2eAssertTeamPage(t, h, release, parent, first.Path, second.Path)
}

func TestE2ENewTeamPageIsCreatedFromTwoProposalsInOneRun(t *testing.T) {
	h := newHarness(t, nil)
	session := h.connect(t, "e2e-create-fold")
	first := e2ePropose(t, session, map[string]any{
		"title": "Wuerike é do time da camada de leitura", "content": "Wuerike faz parte do time da camada de leitura.",
	})
	second := e2ePropose(t, session, map[string]any{
		"title": "Guilherme é membro do time da camada de leitura", "content": "Guilherme é membro do time da camada de leitura.",
	})
	e2eOpenRelease(t, session)
	release := h.m.Config.Paths.Release
	parent := gitOutput(t, release, "rev-parse", "HEAD")
	token := e2eBatchPreview(t, session,
		map[string]any{"operation": "create_page", "proposal_id": first.ID, "target_path": rewriteBaselinePath, "replacement": rewriteReplacement},
		map[string]any{"operation": "fold_proposal", "proposal_id": second.ID, "fold_into": first.ID},
	)
	if exists(release, rewriteBaselinePath) {
		t.Fatal("preview created the page")
	}
	e2eCall(t, session, "memory_promote_batch_apply", map[string]any{"preview_token": token})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	e2eAssertTeamPage(t, h, release, parent, first.Path, second.Path)
}

type e2eProposal struct{ ID, Path string }

func e2ePropose(t *testing.T, session *mcp.ClientSession, arguments map[string]any) e2eProposal {
	t.Helper()
	proposal := decodeInboxResponse(t, e2eCall(t, session, "memory_propose", arguments)).Data
	return e2eProposal{proposal.ID, proposal.Path}
}

func e2eBatchPreview(t *testing.T, session *mcp.ClientSession, items ...map[string]any) string {
	t.Helper()
	return e2eToken(t, e2eCall(t, session, "memory_promote_batch_preview", map[string]any{"items": items}))
}

// e2eAssertTeamPage checks the committed state of the release and then retrieves the team
// page from a brand-new MCP process.
func e2eAssertTeamPage(t *testing.T, h *harness, release, parent string, proposalPaths ...string) {
	t.Helper()
	golden, err := os.ReadFile("../memory/testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	if page := readFile(t, release, rewriteBaselinePath); page != string(golden) {
		t.Fatalf("page = %q, want %q", page, golden)
	}
	for _, path := range proposalPaths {
		if exists(release, path) {
			t.Fatalf("proposal %s remains", path)
		}
	}
	if got := gitOutput(t, release, "rev-parse", "HEAD^"); got != parent {
		t.Fatalf("promotion commit parent = %s, want %s (one commit)", got, parent)
	}
	if status := gitOutput(t, release, "status", "--porcelain"); status != "" {
		t.Fatalf("dirty status after apply: %q", status)
	}

	fresh := h.connect(t, "e2e-fresh-retrieval")
	search := decodeResponse(t, e2eCall(t, fresh, "memory_search", map[string]any{"query": "membros camada de leitura", "budget_bytes": 2048, "source": "release"}))
	if search.Data == nil || len(search.Data.Items) != 1 || search.Data.Items[0].Handle.Path != rewriteBaselinePath {
		t.Fatalf("fresh search = %+v", search.Data)
	}
	content := search.Data.Items[0].Content
	if !strings.Contains(content, "- Wuerike\n- Guilherme") || strings.Contains(content, "Overview") {
		t.Fatalf("fresh search content = %q", content)
	}
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingClaudeMCPIsVisibleWithoutPersistingPayload(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-harness", Version: "test"}, nil)
	command := exec.Command(filepath.Join(t.TempDir(), "missing-gpm"), "mcp")
	if _, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil); err == nil {
		t.Fatal("Connect() succeeded without the MCP executable")
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state root exists after missing MCP: %v", err)
	}
}
