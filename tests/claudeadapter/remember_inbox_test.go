package claudeadapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/consolidation"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const populatedPage = "# Checkout API\n<!-- @id: checkout-api -->\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout is three seconds.\n"

func TestRememberContractSearchesThenCommitsConfirmedProposalToRemote(t *testing.T) {
	managed := wt.Setup(t, map[string]string{"memories/domains/checkout-api.md": populatedPage})
	proposals := managed.Config.Paths.Proposals
	parent := wt.Git(t, proposals, "rev-parse", "HEAD")
	session, stop := startMCP(t, managed.Config)
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
	if got := wt.Git(t, proposals, "rev-parse", "HEAD"); got != parent {
		t.Fatalf("search/expand moved proposals before confirmation: %s -> %s", parent, got)
	}

	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title":   "Update checkout timeout",
			"content": "# Update checkout timeout\n\nUpdate memories/domains/checkout-api.md: the checkout timeout is five seconds.",
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("confirmed proposal = %+v, %v", proposalResult, err)
	}
	proposal := decodeProposal(t, proposalResult)
	head := wt.Git(t, managed.Remote, "rev-parse", "proposals")
	if proposal.ID == "" || proposal.Commit != head || wt.Git(t, managed.Remote, "rev-parse", "proposals^") != parent {
		t.Fatalf("proposal = %+v", proposal)
	}
	if changed := wt.Git(t, managed.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", head); changed != proposal.Path {
		t.Fatalf("proposal commit changed %q", changed)
	}
	if main := wt.Git(t, managed.Remote, "log", "--format=%s", "main"); strings.Contains(main, "proposal") || strings.Count(main, "\n") != 0 {
		t.Fatalf("remember changed main: %q", main)
	}
	if status := wt.Git(t, managed.Config.Paths.Read, "status", "--porcelain"); status != "" {
		t.Fatalf("remember dirtied the read worktree: %q", status)
	}
}

func TestRememberReadingAndBudgetFailureDoNotModifyProposals(t *testing.T) {
	for _, budget := range []int{2048, 1} {
		t.Run(string(rune('a'+budget%2)), func(t *testing.T) {
			managed := wt.Setup(t, map[string]string{"memories/domains/checkout-api.md": populatedPage})
			proposals := managed.Config.Paths.Proposals
			beforeProposals := wt.Git(t, proposals, "rev-parse", "HEAD")
			beforeRemote := wt.Git(t, managed.Remote, "for-each-ref")
			session, stop := startMCP(t, managed.Config)
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
			if got := wt.Git(t, proposals, "rev-parse", "HEAD"); got != beforeProposals {
				t.Fatalf("reading moved the proposals worktree: %s -> %s", beforeProposals, got)
			}
			if status := wt.Git(t, proposals, "status", "--porcelain"); status != "" {
				t.Fatalf("reading dirtied the proposals worktree: %q", status)
			}
			if got := wt.Git(t, managed.Remote, "for-each-ref"); got != beforeRemote {
				t.Fatalf("reading changed the remote:\n%s\n%s", beforeRemote, got)
			}
		})
	}
}

func TestPromoteCreateFlowUsesFinalFilesAndSendsPR(t *testing.T) {
	managed := wt.Setup(t, nil)
	session, stop := startMCP(t, managed.Config)
	defer stop()
	mainBefore := wt.Git(t, managed.Remote, "rev-parse", "main")
	content := "# Checkout timeout\n\nCreate a memory about checkout timeout: five seconds.\n"
	proposal := decodeProposal(t, callOK(t, session, "memory_propose", map[string]any{"title": "Checkout timeout", "content": content}))
	prepared := decodeEnvelope[consolidation.Prepared](t, callOK(t, session, "memory_promote_prepare", map[string]any{}))
	if prepared.MainSHA != mainBefore || len(prepared.Proposals) != 1 || prepared.Proposals[0].ID != proposal.ID {
		t.Fatalf("prepare = %+v", prepared)
	}
	input, err := os.ReadFile(prepared.Proposals[0].InputPath)
	if err != nil || string(input) != content {
		t.Fatalf("fixed proposal content = %q, %v", input, err)
	}
	refused, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_promote_submit", Arguments: map[string]any{"summary": "No memory prepared yet."}})
	if err != nil || !refused.IsError || errorCode(t, refused) != "no_changes" {
		t.Fatalf("empty submit = %+v, %v", refused, err)
	}
	const target = "memories/domains/checkout-timeout.md"
	body := "# Checkout timeout\n\n## Policy\nThe checkout timeout is five seconds.\n"
	full := filepath.Join(prepared.Worktree, target)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	checked := decodeEnvelope[consolidation.Checked](t, callOK(t, session, "memory_promote_check", map[string]any{}))
	if len(checked.Paths) != 1 || checked.Paths[0] != target || !strings.Contains(checked.Diff, "+The checkout timeout is five seconds.") {
		t.Fatalf("review = %+v", checked)
	}
	submitted := decodeEnvelope[consolidation.Submitted](t, callOK(t, session, "memory_promote_submit", map[string]any{"summary": proposal.ID + " creates " + target}))
	if submitted.PRURL == "" || submitted.PRNumber <= 0 || submitted.Commit == "" {
		t.Fatalf("submit = %+v", submitted)
	}
	if head := wt.Git(t, prepared.Worktree, "rev-parse", "HEAD"); head != submitted.Commit {
		t.Fatalf("worktree HEAD = %s, want %s", head, submitted.Commit)
	}
	if changed := wt.Git(t, managed.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", submitted.Commit); changed != target {
		t.Fatalf("canonical commit changed %q", changed)
	}
	if wt.Git(t, managed.Remote, "rev-parse", "main") != mainBefore {
		t.Fatal("submission moved main before merge")
	}
	if _, err := os.Stat(filepath.Join(managed.Config.Paths.Read, target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unreviewed memory is visible to readers: %v", err)
	}
	if text := wt.Git(t, managed.Remote, "show", "proposals:"+proposal.Path); text != strings.TrimSuffix(content, "\n") {
		t.Fatalf("proposal removed or changed before merge: %q", text)
	}
	if head := wt.Git(t, managed.Remote, "rev-parse", prepared.Branch); head != submitted.Commit {
		t.Fatalf("remote promotion branch = %s", head)
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
	for _, contract := range []string{"two fields", "`title`", "`content`", "Keys, tokens, passwords"} {
		if !strings.Contains(string(contents), contract) {
			t.Fatalf("remember skill is missing proposal contract %q", contract)
		}
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

func callOK(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("%s = %+v, %v", name, result, err)
	}
	return result
}

func decodeEnvelope[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response.Data
}

func errorCode(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response.Error.Code
}

func startMCP(t *testing.T, cfg config.Config) (*mcp.ClientSession, func()) {
	t.Helper()
	// A fresh fetch stamp keeps the server's detached background refresh from racing with the assertions.
	if err := os.WriteFile(filepath.Join(cfg.Paths.State, "fetch-attempt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.New(cfg, "test")
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
