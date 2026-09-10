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
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const populatedPage = "# Checkout API\n<!-- @id: checkout-api -->\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout is three seconds.\n"

func TestRememberContractSearchesThenCommitsConfirmedProposal(t *testing.T) {
	memoryRoot := gitRoot(t, populatedPage)
	stateRoot := filepath.Join(t.TempDir(), "state")
	parent := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	session, stop := startMCP(t, memoryRoot, stateRoot)
	defer stop()

	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "checkout timeout", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search = %+v, %v", searchResult, err)
	}
	bundle := decodeBundle(t, searchResult)
	if len(bundle.Items) != 1 {
		t.Fatalf("memory_search items = %+v", bundle.Items)
	}
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_expand", Arguments: map[string]any{"handle": bundle.Items[0].Handle, "budget_bytes": 4096},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("search/expand created state before confirmation: %v", err)
	}

	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title": "Update checkout timeout", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("confirmed proposal = %+v, %v", proposalResult, err)
	}
	proposal := decodeProposal(t, proposalResult)
	if proposal.ID == "" || proposal.Commit != gitOutput(t, memoryRoot, "rev-parse", "HEAD") || gitOutput(t, memoryRoot, "rev-parse", "HEAD^") != parent {
		t.Fatalf("proposal = %+v", proposal)
	}
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); changed != proposal.Path {
		t.Fatalf("proposal commit changed %q", changed)
	}
}

func TestRememberCancellationAndBudgetFailureCreateNoState(t *testing.T) {
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

func TestRememberSkillGrantsOnlyReadAndInboxTools(t *testing.T) {
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
	for _, tool := range []string{"memory_search", "memory_expand", "memory_propose"} {
		if strings.Count(frontmatter, tool) != 1 {
			t.Fatalf("frontmatter tool %q count != 1", tool)
		}
	}
	for _, removed := range []string{"memory_propose_create_page", "memory_propose_create_section", "memory_propose_replace", "proposal_apply", "proposal_publish"} {
		if strings.Contains(frontmatter, removed) {
			t.Fatalf("skill grants removed tool %q", removed)
		}
	}
	if !strings.Contains(string(contents), "If `duplicate` is true") || !strings.Contains(string(contents), "no new file or commit was created") {
		t.Fatal("remember skill does not explain exact proposal deduplication")
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

func decodeProposal(t *testing.T, result *mcp.CallToolResult) inbox.Proposal {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data inbox.Proposal `json:"data"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func startMCP(t *testing.T, memoryRoot, stateRoot string) (*mcp.ClientSession, func()) {
	t.Helper()
	server, err := mcpserver.New(config.Config{MemoryRoot: memoryRoot, StateRoot: stateRoot, MemoryCategories: []string{"domains", "decision"}}, "test")
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
	runGit(t, root, "init", "--quiet", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "gpm.json"), []byte(`{"schema_version":1,"memory_categories":["domains","decision"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if page != "" {
		pagePath := filepath.Join(root, "memories", "domains", "checkout-api.md")
		if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pagePath, []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}
