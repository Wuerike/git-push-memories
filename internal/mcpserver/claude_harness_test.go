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

func TestClaudeAdapterCuratesCreateProposalForFreshProcessRetrieval(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := config.Save(memoryRoot); err != nil {
		t.Fatal(err)
	}
	binary := buildBinary(t)

	client := mcp.NewClient(&mcp.Implementation{Name: "claude-curation-harness", Version: "test"}, nil)
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
		Name: "memory_curate_preview", Arguments: map[string]any{
			"proposal_id": proposal.ID, "target_path": "memories/domains/settlement-retry-policy.md",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_curate_apply = %+v, %v", applyResult, err)
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
