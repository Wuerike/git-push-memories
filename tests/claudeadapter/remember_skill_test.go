package claudeadapter_test

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
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const populatedPage = "# Checkout API\n<!-- @id: checkout-api -->\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout is three seconds.\n"

func TestRememberContractHarnessBranchesWithoutStateBeforeProposal(t *testing.T) {
	tests := []struct {
		name      string
		page      string
		query     string
		tool      string
		arguments map[string]any
	}{
		{
			name: "empty memory creates page", query: "parcel dispatch retries", tool: "memory_propose_create_page",
			arguments: map[string]any{"target_path": "parcel-dispatch.md", "title": "Parcel Dispatch", "body": "Retries use backoff.", "source_paths": []string{}},
		},
		{
			name: "populated memory creates section", page: populatedPage, query: "checkout failure modes", tool: "memory_propose_create_section",
			arguments: map[string]any{"target_path": "checkout-api.md", "heading": "Failure Modes", "body": "Circuit breaker opens.", "source_paths": []string{}},
		},
		{
			name: "populated memory replaces leaf", page: populatedPage, query: "checkout timeout", tool: "memory_propose_replace",
			arguments: map[string]any{"target_path": "checkout-api.md", "section_id": "checkout-api-operation", "replacement": "Timeout is five seconds.\n", "source_paths": []string{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			memoryRoot := gitRoot(t, test.page)
			stateRoot := filepath.Join(t.TempDir(), "state")
			session, stop := startMCP(t, memoryRoot, stateRoot)
			defer stop()
			searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "memory_search", Arguments: map[string]any{"query": test.query, "budget_bytes": 2048},
			})
			if err != nil || searchResult.IsError {
				t.Fatalf("memory_search = %+v, %v", searchResult, err)
			}
			bundle := decodeBundle(t, searchResult)
			if test.page == "" && len(bundle.Items) != 0 {
				t.Fatalf("empty memory search = %+v", bundle.Items)
			}
			for index, item := range bundle.Items {
				if index == 3 {
					break
				}
				expanded, err := session.CallTool(context.Background(), &mcp.CallToolParams{
					Name: "memory_expand", Arguments: map[string]any{"handle": item.Handle, "budget_bytes": 4096},
				})
				if err != nil || expanded.IsError {
					t.Fatalf("memory_expand = %+v, %v", expanded, err)
				}
			}
			if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("search/expand created state before confirmation: %v", err)
			}
			proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: test.tool, Arguments: test.arguments})
			if err != nil || proposalResult.IsError {
				t.Fatalf("confirmed proposal = %+v, %v", proposalResult, err)
			}
			entries, err := os.ReadDir(filepath.Join(stateRoot, "proposals"))
			if err != nil || len(entries) != 1 {
				t.Fatalf("proposal count = %d, %v", len(entries), err)
			}
		})
	}
}

func TestRememberContractHarnessCancellationAndBudgetFailureCreateNoState(t *testing.T) {
	for _, budget := range []int{2048, 1} {
		t.Run(string(rune('a'+budget%2)), func(t *testing.T) {
			memoryRoot := gitRoot(t, populatedPage)
			stateRoot := filepath.Join(t.TempDir(), "state")
			session, stop := startMCP(t, memoryRoot, stateRoot)
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "memory_search", Arguments: map[string]any{"query": "checkout timeout", "budget_bytes": budget},
			})
			if err != nil {
				t.Fatal(err)
			}
			if budget == 1 && !result.IsError {
				t.Fatal("insufficient budget succeeded")
			}
			stop()
			if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("cancelled/failed flow created state: %v", err)
			}
		})
	}
}

func TestRememberSkillGrantsOnlyReadAndProposalTools(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(repository, "integrations", "claude-code", "skills", "remember", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	frontmatterEnd := strings.Index(string(contents)[4:], "\n---\n")
	if frontmatterEnd < 0 {
		t.Fatal("invalid skill frontmatter")
	}
	frontmatter := string(contents)[:frontmatterEnd+4]
	for _, tool := range []string{"memory_search", "memory_expand", "memory_propose_create_page", "memory_propose_create_section", "memory_propose_replace"} {
		if strings.Count(frontmatter, tool) != 1 {
			t.Fatalf("frontmatter tool %q count != 1", tool)
		}
	}
	if strings.Contains(frontmatter, "proposal_apply") || strings.Contains(frontmatter, "proposal_publish") {
		t.Fatal("skill grants apply or publish authority")
	}
}

func decodeBundle(t *testing.T, result *mcp.CallToolResult) retrieval.ContextBundle {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data retrieval.ContextBundle `json:"data"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func startMCP(t *testing.T, memoryRoot, stateRoot string) (*mcp.ClientSession, func()) {
	t.Helper()
	server, err := mcpserver.New(config.Config{MemoryRoot: memoryRoot, StateRoot: stateRoot}, "test")
	if err != nil {
		t.Fatal(err)
	}
	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- server.Run(context.Background(), serverInput, serverOutput) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "remember-harness", Version: "test"}, nil)
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

func gitRoot(t *testing.T, page string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if page != "" {
		if err := os.WriteFile(filepath.Join(root, "checkout-api.md"), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		runGit(t, root, args...)
	}
	if page != "" {
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "--quiet", "-m", "initial")
	} else {
		runGit(t, root, "commit", "--quiet", "--allow-empty", "-m", "initial")
	}
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
