package claudeadapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/promotion"
	"github.com/Wuerike/git-push-memories/internal/release"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const populatedPage = "# Checkout API\n<!-- @id: checkout-api -->\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout is three seconds.\n"

func TestRememberContractSearchesThenCommitsAndPushesConfirmedProposal(t *testing.T) {
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
			"title": "Update checkout timeout", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("confirmed proposal = %+v, %v", proposalResult, err)
	}
	proposal := decodeProposal(t, proposalResult)
	head := wt.Git(t, proposals, "rev-parse", "HEAD")
	if proposal.ID == "" || proposal.Commit != head || wt.Git(t, proposals, "rev-parse", "HEAD^") != parent {
		t.Fatalf("proposal = %+v", proposal)
	}
	if branch := wt.Git(t, proposals, "symbolic-ref", "--short", "HEAD"); branch != workspace.ProposalsBranch {
		t.Fatalf("proposal was written on %q", branch)
	}
	if changed := wt.Git(t, proposals, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); changed != proposal.Path {
		t.Fatalf("proposal commit changed %q", changed)
	}
	if remote := wt.Git(t, managed.Remote, "rev-parse", "proposals"); remote != head {
		t.Fatalf("remote proposals = %s, want the pushed commit %s", remote, head)
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

func TestPromoteCreateFlowRunsOnAReleaseAndSendsItForReview(t *testing.T) {
	managed := wt.Setup(t, nil)
	wt.FakeGH(t)
	proposal := managed.Remember(t, "Checkout timeout")
	session, stop := startMCP(t, managed.Config)
	defer stop()
	mainBefore := wt.Git(t, managed.Remote, "rev-parse", "main")

	opened := decodeEnvelope[release.Status](t, callOK(t, session, "memory_promote_open", map[string]any{}))
	if !opened.Open || opened.Marker == nil || !strings.HasPrefix(opened.Marker.ReleaseBranch, workspace.ReleasePrefix) || len(opened.Pending) != 1 {
		t.Fatalf("memory_promote_open = %+v", opened)
	}
	releaseRoot := managed.Config.Paths.Release
	if branch := wt.Git(t, releaseRoot, "symbolic-ref", "--short", "HEAD"); branch != opened.Marker.ReleaseBranch {
		t.Fatalf("release worktree is on %q", branch)
	}
	listed := decodeEnvelope[promotion.List](t, callOK(t, session, "memory_proposal_list", map[string]any{"source": "release"}))
	if len(listed.Proposals) != 1 || listed.Proposals[0].ID != proposal.ID {
		t.Fatalf("release proposals = %+v", listed.Proposals)
	}

	// Submitting while a proposal is still pending is refused.
	refused, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_promote_submit", Arguments: map[string]any{}})
	if err != nil || !refused.IsError || errorCode(t, refused) != "proposals_pending" {
		t.Fatalf("submit with pending proposals = %+v, %v", refused, err)
	}

	preview := decodeEnvelope[struct {
		PreviewToken string `json:"preview_token"`
	}](t, callOK(t, session, "memory_promote_preview", map[string]any{
		"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/checkout-timeout.md",
	}))
	applied := decodeEnvelope[promotion.Result](t, callOK(t, session, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken}))
	if got := wt.Git(t, releaseRoot, "rev-parse", "HEAD"); got != applied.Commit {
		t.Fatalf("promotion commit %s is not the release head %s", applied.Commit, got)
	}
	changed := strings.Fields(wt.Git(t, releaseRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", applied.Commit))
	if !slices.Contains(changed, "memories/domains/checkout-timeout.md") || !slices.Contains(changed, proposal.Path) {
		t.Fatalf("promotion commit changed %v", changed)
	}
	// Published memory and the shared inbox stay untouched until the PR merges.
	if got := wt.Git(t, managed.Remote, "rev-parse", "main"); got != mainBefore {
		t.Fatal("promotion moved main")
	}
	if _, err := os.Stat(filepath.Join(managed.Config.Paths.Read, "memories", "domains", "checkout-timeout.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unreviewed memory is visible to readers: %v", err)
	}
	if _, err := os.Stat(filepath.Join(managed.Config.Paths.Proposals, proposal.Path)); err != nil {
		t.Fatalf("proposal left the proposals branch before merge: %v", err)
	}

	submitted := decodeEnvelope[struct {
		PullRequest string `json:"pull_request"`
		Finalized   bool   `json:"finalized"`
	}](t, callOK(t, session, "memory_promote_submit", map[string]any{}))
	if submitted.PullRequest == "" || submitted.Finalized {
		t.Fatalf("memory_promote_submit = %+v", submitted)
	}
	if got := wt.Git(t, managed.Remote, "rev-parse", opened.Marker.ReleaseBranch); got != applied.Commit {
		t.Fatalf("remote release branch = %s, want %s", got, applied.Commit)
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
	for _, tool := range []string{"memory_search", "memory_expand", "memory_reference_snapshot", "memory_reference_expand", "memory_propose"} {
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
	for _, contract := range []string{"never invent or edit repository IDs", "source_paths: []", "`supports`, `contradicts`, or `unclear`", "local root is transient", "Never copy source excerpts"} {
		if !strings.Contains(string(contents), contract) {
			t.Fatalf("remember skill is missing reference contract %q", contract)
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
