package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const memoryPage = `# checkout-api
<!-- @id: checkout-api -->

## Operacao
<!-- @id: checkout-api-operation -->

- timeout: tres segundos
- owner: team-b
`

func TestMemorySearchAndExpand(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"services/checkout-api.md": memoryPage})
	codeRoot := newGitRoot(t, nil)
	server, err := mcpserver.New(config.Config{MemoryRoot: memoryRoot, CodeRoot: codeRoot}, "test")
	if err != nil {
		t.Fatal(err)
	}
	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- server.Run(context.Background(), serverInput, serverOutput) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.IOTransport{Reader: clientInput, Writer: clientOutput}, nil)
	if err != nil {
		t.Fatal(err)
	}
	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search",
		Arguments: map[string]any{
			"query":        "checkout timeout team",
			"budget_bytes": 2048,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	search := decodeResponse(t, searchResult)
	if searchResult.IsError || search.SchemaVersion != 1 || search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("memory_search result = %+v, response = %+v", searchResult, search)
	}

	expandResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_expand",
		Arguments: map[string]any{
			"handle":       search.Data.Items[0].Handle,
			"budget_bytes": 4096,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expanded := decodeResponse(t, expandResult)
	if expandResult.IsError || expanded.Data == nil || len(expanded.Data.Items) != 1 || expanded.Data.Items[0].Content != "\n- timeout: tres segundos\n- owner: team-b\n" {
		t.Fatalf("memory_expand result = %+v, response = %+v", expandResult, expanded)
	}

	budgetResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_expand",
		Arguments: map[string]any{
			"handle":       search.Data.Items[0].Handle,
			"budget_bytes": 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	budget := decodeResponse(t, budgetResult)
	if !budgetResult.IsError || budget.Error == nil || budget.Error.Code != "budget_too_small" {
		t.Fatalf("memory_expand budget result = %+v, response = %+v", budgetResult, budget)
	}

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP server did not stop")
	}
}

type response struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

func decodeResponse(t *testing.T, result *mcp.CallToolResult) response {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded response
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func newGitRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	for name, contents := range files {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(files) > 0 {
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "--quiet", "-m", "initial")
	}
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
