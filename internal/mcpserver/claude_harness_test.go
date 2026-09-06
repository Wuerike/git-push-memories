package mcpserver_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClaudeAdapterPersistsProposalForNewCLIProcess(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"services/checkout-api.md": memoryPage})
	codeRoot := newGitRoot(t, nil)
	stateRoot := filepath.Join(t.TempDir(), "state")
	binary := buildBinary(t)

	client := mcp.NewClient(&mcp.Implementation{Name: "claude-harness", Version: "test"}, nil)
	serverCommand := exec.Command(binary, "mcp",
		"--memory-root", memoryRoot,
		"--code-root", codeRoot,
		"--state-root", stateRoot,
	)
	session, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: serverCommand}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose_replace",
		Arguments: map[string]any{
			"target_path":  "services/checkout-api.md",
			"section_id":   "checkout-api-operation",
			"replacement":  "\n- timeout: cinco segundos\n- owner: team-a\n",
			"source_paths": []string{"services/checkout.go"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeProposalResponse(t, result)
	if result.IsError || response.Data == nil {
		t.Fatalf("proposal result = %+v, response = %+v", result, response)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	show := exec.Command(binary, "proposal", "show", response.Data.ID,
		"--memory-root", memoryRoot,
		"--code-root", codeRoot,
		"--state-root", stateRoot,
	)
	output, err := show.CombinedOutput()
	if err != nil {
		t.Fatalf("proposal show: %v: %s", err, output)
	}
	if !strings.Contains(string(output), `"id":"`+response.Data.ID+`"`) || !strings.Contains(string(output), `"status":"pending"`) {
		t.Fatalf("proposal show output = %s", output)
	}
	contents, err := os.ReadFile(filepath.Join(memoryRoot, "services/checkout-api.md"))
	if err != nil || string(contents) != memoryPage {
		t.Fatalf("memory changed before apply: err = %v", err)
	}
	status := exec.Command("git", "-C", memoryRoot, "status", "--porcelain=v1")
	if output, err := status.CombinedOutput(); err != nil || len(output) != 0 {
		t.Fatalf("Git changed before apply: %v: %s", err, output)
	}
}

func TestMissingClaudeMCPIsVisibleWithoutPersistingPayload(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-harness", Version: "test"}, nil)
	command := exec.Command(filepath.Join(t.TempDir(), "missing-git-push-memories"),
		"mcp", "--state-root", stateRoot,
	)
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
	binary := filepath.Join(t.TempDir(), "git-push-memories")
	command := exec.Command("go", "build", "-o", binary, "./cmd/git-push-memories")
	command.Dir = repositoryRoot
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build harness binary: %v: %s", err, output)
	}
	return binary
}
