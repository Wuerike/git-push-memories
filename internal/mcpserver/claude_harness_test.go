package mcpserver_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClaudeAdapterCommitsProposalToMemoryRepository(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := config.Save(memoryRoot); err != nil {
		t.Fatal(err)
	}
	binary := buildBinary(t)

	client := mcp.NewClient(&mcp.Implementation{Name: "claude-harness", Version: "test"}, nil)
	serverCommand := exec.Command(binary, "mcp")
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: serverCommand}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title":            "Update checkout timeout",
			"suggested_action": "update",
			"target":           "memories/domains/checkout-api.md",
			"knowledge":        "The checkout timeout is five seconds.",
			"source_paths":     []string{"services/checkout.go"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, result)
	if result.IsError || response.Data == nil {
		t.Fatalf("proposal result = %+v, response = %+v", result, response)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	proposalContents, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(response.Data.Path)))
	if err != nil || !strings.Contains(string(proposalContents), "# Update checkout timeout") {
		t.Fatalf("proposal Markdown = %q, %v", proposalContents, err)
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "memories/domains/checkout-api.md"))
	if err != nil || string(contents) != memoryPage {
		t.Fatalf("proposal changed canonical memory: err = %v", err)
	}
	status := exec.Command("git", "-C", memoryRoot, "status", "--porcelain=v1")
	if output, err := status.CombinedOutput(); err != nil || len(output) != 0 {
		t.Fatalf("proposal commit left Git dirty: %v: %s", err, output)
	}
}

func TestClaudeAdapterPromotesCreateProposalForFreshProcessRetrieval(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := config.Save(memoryRoot); err != nil {
		t.Fatal(err)
	}
	binary := buildBinary(t)

	client := mcp.NewClient(&mcp.Implementation{Name: "claude-promotion-harness", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Settlement retry policy", "suggested_action": "create",
			"knowledge": "Settlement retries stop after three failures.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_promote_preview", Arguments: map[string]any{
			"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/settlement-retry-policy.md",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_promote_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_promote_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_promote_apply = %+v, %v", applyResult, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	freshClient := mcp.NewClient(&mcp.Implementation{Name: "fresh-retrieval-harness", Version: "test"}, nil)
	freshSession, err := freshClient.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	searchResult, err := freshSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "settlement retry failures", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("fresh memory_search = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 || search.Data.Items[0].Handle.Path != "memories/domains/settlement-retry-policy.md" || !strings.Contains(search.Data.Items[0].Content, "Settlement retries stop after three failures") {
		t.Fatalf("fresh search = %+v", search.Data)
	}
	if err := freshSession.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeAdapterPromotesUpdateForFreshProcessRetrieval(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := config.Save(memoryRoot); err != nil {
		t.Fatal(err)
	}
	binary := buildBinary(t)
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-update-harness", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "checkout timeout team-b", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("memory_search = %+v", search.Data)
	}
	handle := search.Data.Items[0].Handle
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Update checkout timeout", "suggested_action": "update", "target": handle.Path,
			"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_promote_preview", Arguments: map[string]any{
			"operation": "replace_section", "proposal_id": proposal.ID, "target_path": handle.Path,
			"section_id": handle.SectionID, "content_hash": handle.ContentHash,
			"replacement": "- timeout: cinco segundos\n- owner: team-b\n",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_promote_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_promote_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_promote_apply = %+v, %v", applyResult, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	freshClient := mcp.NewClient(&mcp.Implementation{Name: "fresh-update-retrieval", Version: "test"}, nil)
	freshSession, err := freshClient.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(binary, "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	freshResult, err := freshSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "checkout cinco team-b", "budget_bytes": 2048},
	})
	if err != nil || freshResult.IsError {
		t.Fatalf("fresh memory_search = %+v, %v", freshResult, err)
	}
	fresh := decodeResponse(t, freshResult)
	if fresh.Data == nil || len(fresh.Data.Items) != 1 || !strings.Contains(fresh.Data.Items[0].Content, "cinco segundos") || strings.Contains(fresh.Data.Items[0].Content, "tres segundos") {
		t.Fatalf("fresh update search = %+v", fresh.Data)
	}
	if err := freshSession.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestE2EReadingTeamPageIsRestructuredByRewrite(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{rewriteBaselinePath: rewriteBaselinePage})
	e2eConfigure(t, memoryRoot)
	binary := buildBinary(t)

	session := e2eConnect(t, binary, "e2e-rewrite")
	first := e2ePropose(t, session, map[string]any{
		"title": "Wuerike é do time da camada de leitura", "suggested_action": "update", "target": rewriteBaselinePath,
		"knowledge": "Wuerike faz parte do time da camada de leitura.", "source_paths": []string{},
	})
	second := e2ePropose(t, session, map[string]any{
		"title": "Guilherme é membro do time da camada de leitura", "suggested_action": "update", "target": rewriteBaselinePath,
		"knowledge": "Guilherme é membro do time da camada de leitura.", "source_paths": []string{},
	})
	parent := gitOutput(t, memoryRoot, "rev-parse", "HEAD") // after both proposal commits
	read := e2eCall(t, session, "memory_page_read", map[string]any{"path": rewriteBaselinePath, "budget_bytes": 65536})
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

	e2eAssertTeamPage(t, binary, memoryRoot, parent, first.Path, second.Path)
}

func TestE2ENewTeamPageIsCreatedFromTwoProposalsInOneRun(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	e2eConfigure(t, memoryRoot)
	binary := buildBinary(t)

	session := e2eConnect(t, binary, "e2e-create-fold")
	first := e2ePropose(t, session, map[string]any{
		"title": "Wuerike é do time da camada de leitura", "suggested_action": "create",
		"knowledge": "Wuerike faz parte do time da camada de leitura.", "source_paths": []string{},
	})
	second := e2ePropose(t, session, map[string]any{
		"title": "Guilherme é membro do time da camada de leitura", "suggested_action": "create",
		"knowledge": "Guilherme é membro do time da camada de leitura.", "source_paths": []string{},
	})
	parent := gitOutput(t, memoryRoot, "rev-parse", "HEAD") // after both proposal commits
	token := e2eBatchPreview(t, session,
		map[string]any{"operation": "create_page", "proposal_id": first.ID, "target_path": rewriteBaselinePath, "replacement": rewriteReplacement},
		map[string]any{"operation": "fold_proposal", "proposal_id": second.ID, "fold_into": first.ID},
	)
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(rewriteBaselinePath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview created the page: %v", err)
	}
	e2eCall(t, session, "memory_promote_batch_apply", map[string]any{"preview_token": token})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	e2eAssertTeamPage(t, binary, memoryRoot, parent, first.Path, second.Path)
}

func e2eConfigure(t *testing.T, memoryRoot string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := config.Save(memoryRoot); err != nil {
		t.Fatal(err)
	}
}

func e2eConnect(t *testing.T, binary, name string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: exec.Command(binary, "mcp")}, nil)
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

type e2eProposal struct{ ID, Path string }

func e2ePropose(t *testing.T, session *mcp.ClientSession, arguments map[string]any) e2eProposal {
	t.Helper()
	proposal := decodeInboxResponse(t, e2eCall(t, session, "memory_propose", arguments)).Data
	return e2eProposal{proposal.ID, proposal.Path}
}

func e2eBatchPreview(t *testing.T, session *mcp.ClientSession, items ...map[string]any) string {
	t.Helper()
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, e2eCall(t, session, "memory_promote_batch_preview", map[string]any{"items": items}), &preview)
	if preview.Data.PreviewToken == "" {
		t.Fatal("memory_promote_batch_preview returned no preview_token")
	}
	return preview.Data.PreviewToken
}

// e2eAssertTeamPage checks the committed state and then retrieves the team
// page from a brand-new MCP process.
func e2eAssertTeamPage(t *testing.T, binary, memoryRoot, parent string, proposalPaths ...string) {
	t.Helper()
	golden, err := os.ReadFile("../memory/testdata/page-rewrite/team.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(rewriteBaselinePath)))
	if err != nil || string(page) != string(golden) {
		t.Fatalf("page = %q, want %q, err = %v", page, golden, err)
	}
	for _, path := range proposalPaths {
		if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(path))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("proposal %s remains: %v", path, err)
		}
	}
	if got := gitOutput(t, memoryRoot, "rev-parse", "HEAD^"); got != parent {
		t.Fatalf("promotion commit parent = %s, want %s (one commit)", got, parent)
	}
	if status := gitOutput(t, memoryRoot, "status", "--porcelain"); status != "" {
		t.Fatalf("dirty status after apply: %q", status)
	}

	fresh := e2eConnect(t, binary, "e2e-fresh-retrieval")
	search := decodeResponse(t, e2eCall(t, fresh, "memory_search", map[string]any{"query": "membros camada de leitura", "budget_bytes": 2048}))
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

func buildBinary(t *testing.T) string {
	t.Helper()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "gpm")
	command := exec.Command("go", "build", "-o", binary, "./cmd/gpm")
	command.Dir = repositoryRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build harness binary: %v: %s", err, output)
	}
	return binary
}
