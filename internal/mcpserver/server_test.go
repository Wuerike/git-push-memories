package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/proposal"
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
	server, err := mcpserver.New(config.Config{MemoryRoot: memoryRoot, StateRoot: filepath.Join(t.TempDir(), "state")}, "test")
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

func TestMemoryProposeReplace(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"services/checkout-api.md": memoryPage})
	stateRoot := filepath.Join(t.TempDir(), "state")
	before, err := os.ReadFile(filepath.Join(memoryRoot, "services/checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}

	session, stop := startServer(t, memoryRoot, stateRoot)
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
	if result.IsError || response.Data == nil || response.Data.ID == "" || response.Data.Revision != 1 {
		t.Fatalf("memory_propose_replace result = %+v, response = %+v", result, response)
	}
	if response.Data.Target.Path != "services/checkout-api.md" || response.Data.Target.SectionID != "checkout-api-operation" {
		t.Fatalf("target = %+v", response.Data.Target)
	}
	if response.Data.DiffSummary.AddedLines != 2 || response.Data.DiffSummary.RemovedLines != 2 {
		t.Fatalf("diff summary = %+v", response.Data.DiffSummary)
	}
	entries, err := os.ReadDir(filepath.Join(stateRoot, "proposals"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("proposal entries = %v, err = %v", entries, err)
	}
	after, err := os.ReadFile(filepath.Join(memoryRoot, "services/checkout-api.md"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("memory changed before apply: err = %v", err)
	}
	stop()

	workflow, err := proposal.OpenReadOnly(memoryRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer workflow.Close()
	review, err := workflow.Review(response.Data.ID)
	if err != nil || review.Proposal == nil || review.Proposal.Revision != 1 {
		t.Fatalf("Review() = %+v, %v", review, err)
	}
}

func TestMemoryProposeCreatePageAndSection(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		tool      string
		arguments map[string]any
		operation string
		target    proposal.Target
	}{
		{
			name: "page", tool: "memory_propose_create_page", operation: "create_page",
			arguments: map[string]any{"target_path": "parcel-dispatch.md", "title": "Parcel Dispatch", "body": "Retries use backoff.", "source_paths": []string{"services/parcel.go"}},
			target:    proposal.Target{Path: "parcel-dispatch.md", SectionID: "parcel-dispatch-overview"},
		},
		{
			name: "section", files: map[string]string{"services/checkout-api.md": memoryPage}, tool: "memory_propose_create_section", operation: "create_section",
			arguments: map[string]any{"target_path": "services/checkout-api.md", "heading": "Failure Modes", "body": "Circuit breaker opens.", "source_paths": []string{}},
			target:    proposal.Target{Path: "services/checkout-api.md", SectionID: "checkout-api-failure-modes"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			memoryRoot := newGitRoot(t, test.files)
			stateRoot := filepath.Join(t.TempDir(), "state")
			before := gitOutput(t, memoryRoot, "status", "--porcelain=v1", "--untracked-files=all")
			session, stop := startServer(t, memoryRoot, stateRoot)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.arguments})
			if err != nil {
				t.Fatal(err)
			}
			response := decodeProposalResponse(t, result)
			if result.IsError || response.Data == nil || response.Data.Operation != test.operation || response.Data.Target != test.target || response.Data.ID == "" {
				t.Fatalf("%s result = %+v, response = %+v", test.tool, result, response)
			}
			if after := gitOutput(t, memoryRoot, "status", "--porcelain=v1", "--untracked-files=all"); after != before {
				t.Fatalf("proposal changed memory/Git: before %q, after %q", before, after)
			}
			stop()
			workflow, err := proposal.OpenReadOnly(memoryRoot, stateRoot)
			if err != nil {
				t.Fatal(err)
			}
			review, err := workflow.Review(response.Data.ID)
			_ = workflow.Close()
			if err != nil || review.Proposal == nil || review.Proposal.Operation.Type != test.operation {
				t.Fatalf("Review() = %+v, %v", review, err)
			}
		})
	}
}

func TestInvalidAndSensitiveProposalsAreRejectedWithoutStateOrBlockingSession(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"services/checkout-api.md": memoryPage})
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	defer stop()

	unstructured, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_propose_replace",
		Arguments: map[string]any{"text": "remember the checkout timeout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !unstructured.IsError {
		t.Fatalf("unstructured input result = %+v", unstructured)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state root exists after unstructured input: %v", err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose_replace",
		Arguments: map[string]any{
			"target_path":  "services/checkout-api.md",
			"section_id":   "checkout-api-operation",
			"replacement":  "\n- password=super-secret\n",
			"source_paths": []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := decodeProposalResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "privacy_sensitive_content" {
		t.Fatalf("sensitive result = %+v, response = %+v", result, response)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state root exists after rejection: %v", err)
	}
	search, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "memory_search",
		Arguments: map[string]any{"query": "checkout", "budget_bytes": 2048},
	})
	if err != nil || search.IsError {
		t.Fatalf("session unusable after rejection: result = %+v, err = %v", search, err)
	}
}

type proposalResponse struct {
	SchemaVersion int `json:"schema_version"`
	Data          *struct {
		ID          string          `json:"id"`
		Revision    int             `json:"revision"`
		Operation   string          `json:"operation"`
		Target      proposal.Target `json:"target"`
		DiffSummary struct {
			AddedLines   int `json:"added_lines"`
			RemovedLines int `json:"removed_lines"`
		} `json:"diff_summary"`
	} `json:"data,omitempty"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

func decodeProposalResponse(t *testing.T, result *mcp.CallToolResult) proposalResponse {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded proposalResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func startServer(t *testing.T, memoryRoot, stateRoot string) (*mcp.ClientSession, func()) {
	t.Helper()
	server, err := mcpserver.New(config.Config{MemoryRoot: memoryRoot, StateRoot: stateRoot}, "test")
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
	return session, func() {
		t.Helper()
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
	} else {
		runGit(t, root, "commit", "--quiet", "--allow-empty", "-m", "initial")
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

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return string(output)
}
