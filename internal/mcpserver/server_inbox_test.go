package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/inbox"
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

func TestMemorySearchAndExpandConfiguredCategory(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()

	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_search",
		Arguments: map[string]any{"query": "checkout timeout team", "budget_bytes": 2048},
	})
	if err != nil {
		t.Fatal(err)
	}
	search := decodeResponse(t, searchResult)
	if searchResult.IsError || search.Data == nil || len(search.Data.Items) != 1 || search.Data.Items[0].Handle.Path != "memories/domains/checkout-api.md" {
		t.Fatalf("memory_search result = %+v, response = %+v", searchResult, search)
	}
	expandResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_expand",
		Arguments: map[string]any{"handle": search.Data.Items[0].Handle, "budget_bytes": 4096},
	})
	if err != nil {
		t.Fatal(err)
	}
	expanded := decodeResponse(t, expandResult)
	if expandResult.IsError || expanded.Data == nil || len(expanded.Data.Items) != 1 || !strings.Contains(expanded.Data.Items[0].Content, "timeout: tres segundos") {
		t.Fatalf("memory_expand result = %+v, response = %+v", expandResult, expanded)
	}
}

func TestMemoryProposeCommitsMarkdownToInbox(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	parent := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title":            "Card transactions from authorizer",
			"suggested_action": "update",
			"target":           "memories/domains/charge-entity-payment-methods.md",
			"knowledge":        "Card transactions can originate from the authorizer adapter.",
			"source_paths":     []string{"adapters/authorizer/config.yaml"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, result)
	if result.IsError || response.Data == nil || response.Data.ID == "" || response.Data.SuggestedAction != inbox.ActionUpdate {
		t.Fatalf("memory_propose result = %+v, response = %+v", result, response)
	}
	current := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	if response.Data.Commit != current || gitOutput(t, memoryRoot, "rev-parse", "HEAD^") != parent {
		t.Fatalf("memory_propose commit = %q, HEAD = %q", response.Data.Commit, current)
	}
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); changed != response.Data.Path {
		t.Fatalf("memory_propose changed %q, want %q", changed, response.Data.Path)
	}
	if branches := gitOutput(t, memoryRoot, "for-each-ref", "--format=%(refname)", "refs/heads/gpm/proposal/"); branches != "" {
		t.Fatalf("memory_propose created legacy branches: %q", branches)
	}
}

func TestMemoryProposeRejectsSensitiveKnowledgeWithoutState(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	defer stop()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title": "Unsafe fact", "suggested_action": "create", "knowledge": "password=super-secret", "source_paths": []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "privacy_sensitive_content" {
		t.Fatalf("sensitive result = %+v, response = %+v", result, response)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected proposal created state: %v", err)
	}
}

func TestMemoryProposeRejectsTargetOutsideConfiguredCategories(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	defer stop()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title": "Fact", "suggested_action": "update", "target": "memories/notes/fact.md", "knowledge": "Knowledge.", "source_paths": []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "schema_invalid_target" {
		t.Fatalf("unconfigured target result = %+v, response = %+v", result, response)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected target created state: %v", err)
	}
}

func TestMemoryProposeRequiresMain(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	runGit(t, memoryRoot, "switch", "--quiet", "-c", "feature")
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_propose",
		Arguments: map[string]any{"title": "Fact", "suggested_action": "create", "knowledge": "Knowledge.", "source_paths": []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "not_on_main" {
		t.Fatalf("non-main result = %+v, response = %+v", result, response)
	}
}

func TestMemoryProposeReportsGitIndexLock(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	lockPath := filepath.Join(memoryRoot, ".git", "index.lock")
	if err := os.WriteFile(lockPath, []byte("other process"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(lockPath)
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_propose",
		Arguments: map[string]any{"title": "Fact", "suggested_action": "create", "knowledge": "Knowledge.", "source_paths": []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "git_index_locked" {
		t.Fatalf("locked index result = %+v, response = %+v", result, response)
	}
}

type response struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

type inboxResponse struct {
	SchemaVersion int             `json:"schema_version"`
	Data          *inbox.Proposal `json:"data,omitempty"`
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

func decodeInboxResponse(t *testing.T, result *mcp.CallToolResult) inboxResponse {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded inboxResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func startServer(t *testing.T, memoryRoot, stateRoot string) (*mcp.ClientSession, func()) {
	t.Helper()
	server, err := mcpserver.New(config.Config{
		MemoryRoot: memoryRoot, StateRoot: stateRoot, MemoryCategories: []string{"domains", "decision"},
	}, "test")
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
	stopped := false
	return session, func() {
		if stopped {
			return
		}
		stopped = true
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
}

func newGitRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	if files == nil {
		files = make(map[string]string)
	}
	if _, configured := files["gpm.json"]; !configured {
		files["gpm.json"] = `{"schema_version":1,"memory_categories":["domains","decision"]}`
	}
	for name, contents := range files {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return strings.TrimSpace(string(output))
}
