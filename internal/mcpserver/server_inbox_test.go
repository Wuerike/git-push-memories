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

func TestCurateCreatePreviewAndApplyPublishesCanonicalMemory(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{
		"staged.txt":   "before staged\n",
		"unstaged.txt": "before unstaged\n",
	})
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	defer stop()

	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose",
		Arguments: map[string]any{
			"title":            "Parcel dispatch retry policy",
			"suggested_action": "create",
			"knowledge":        "Parcel dispatch retries use exponential backoff and stop after three failures.",
			"source_paths":     []string{"services/parcel/dispatch.go"},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	if proposal == nil {
		t.Fatal("memory_propose returned no proposal")
	}
	proposalHead := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(memoryRoot, "staged.txt"), []byte("after staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(memoryRoot, "unstaged.txt"), []byte("after unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	listResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_proposal_list", Arguments: map[string]any{},
	})
	if err != nil || listResult.IsError {
		t.Fatalf("memory_proposal_list = %+v, %v", listResult, err)
	}
	var listed struct {
		Data struct {
			Categories []string `json:"categories"`
			Proposals  []struct {
				ID              string `json:"id"`
				Title           string `json:"title"`
				SuggestedAction string `json:"suggested_action"`
			} `json:"proposals"`
		} `json:"data"`
	}
	decodeStructured(t, listResult, &listed)
	if len(listed.Data.Proposals) != 1 || listed.Data.Proposals[0].ID != proposal.ID || listed.Data.Proposals[0].Title != proposal.Title || listed.Data.Proposals[0].SuggestedAction != "create" {
		t.Fatalf("listed proposals = %+v", listed.Data.Proposals)
	}
	if strings.Join(listed.Data.Categories, ",") != "domains,decision" {
		t.Fatalf("listed categories = %v", listed.Data.Categories)
	}

	target := "memories/domains/parcel-dispatch-retry-policy.md"
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview",
		Arguments: map[string]any{
			"proposal_id": proposal.ID,
			"target_path": target,
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string   `json:"preview_token"`
			ProposalID   string   `json:"proposal_id"`
			Title        string   `json:"title"`
			SourcePaths  []string `json:"source_paths"`
			TargetPath   string   `json:"target_path"`
			Diff         string   `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.ProposalID != proposal.ID || preview.Data.Title != proposal.Title || preview.Data.TargetPath != target {
		t.Fatalf("preview = %+v", preview.Data)
	}
	if len(preview.Data.SourcePaths) != 1 || preview.Data.SourcePaths[0] != "services/parcel/dispatch.go" {
		t.Fatalf("preview sources = %v", preview.Data.SourcePaths)
	}
	if !strings.Contains(preview.Data.Diff, "+++ b/"+target) || !strings.Contains(preview.Data.Diff, "--- a/"+proposal.Path) || !strings.Contains(preview.Data.Diff, "+++ /dev/null") {
		t.Fatalf("preview diff = %s", preview.Data.Diff)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != proposalHead {
		t.Fatalf("preview changed HEAD from %s to %s", proposalHead, current)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(target))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview created target: %v", err)
	}

	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply",
		Arguments: map[string]any{
			"preview_token": preview.Data.PreviewToken,
		},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_curate_apply = %+v, %v", applyResult, err)
	}
	var applied struct {
		Data struct {
			ProposalID string `json:"proposal_id"`
			TargetPath string `json:"target_path"`
			SectionID  string `json:"section_id"`
			Commit     string `json:"commit"`
		} `json:"data"`
	}
	decodeStructured(t, applyResult, &applied)
	if applied.Data.ProposalID != proposal.ID || applied.Data.TargetPath != target || applied.Data.SectionID != "parcel-dispatch-retry-policy-overview" || applied.Data.Commit == "" {
		t.Fatalf("apply = %+v", applied.Data)
	}
	if head := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); head != applied.Data.Commit {
		t.Fatalf("HEAD = %s, commit = %s", head, applied.Data.Commit)
	}
	changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD")
	wantChanged := "A\t" + target + "\nD\t" + proposal.Path
	if changed != wantChanged {
		t.Fatalf("curation changed %q, want %q", changed, wantChanged)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed proposal remains: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(target)))
	if err != nil || !strings.Contains(string(page), "Parcel dispatch retries use exponential backoff") {
		t.Fatalf("canonical page = %q, %v", page, err)
	}
	if staged := gitOutput(t, memoryRoot, "diff", "--cached", "--name-only"); staged != "staged.txt" {
		t.Fatalf("staged changes = %q", staged)
	}
	if unstaged := gitOutput(t, memoryRoot, "diff", "--name-only"); unstaged != "unstaged.txt" {
		t.Fatalf("unstaged changes = %q", unstaged)
	}

	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "parcel dispatch exponential backoff", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search after curation = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 || search.Data.Items[0].Handle.Path != target {
		t.Fatalf("curated search = %+v", search.Data)
	}
}

func TestProposalListIsReadOnly(t *testing.T) {
	const proposalID = "0123456789abcdef0123456789abcdef"
	proposalPath := "proposals/" + proposalID + "-retry-policy.md"
	memoryRoot := newGitRoot(t, map[string]string{
		proposalPath: "---\nschema_version: 1\nsuggested_action: \"create\"\nsource_paths: []\n---\n\n# Retry policy\n\nRetries stop after three failures.\n",
	})
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	defer stop()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_proposal_list", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("memory_proposal_list = %+v, %v", result, err)
	}
	var listed struct {
		Data struct {
			Proposals []struct {
				ID          string   `json:"id"`
				Knowledge   string   `json:"knowledge"`
				SourcePaths []string `json:"source_paths"`
			} `json:"proposals"`
		} `json:"data"`
	}
	decodeStructured(t, result, &listed)
	if len(listed.Data.Proposals) != 1 || listed.Data.Proposals[0].ID != proposalID || listed.Data.Proposals[0].Knowledge != "Retries stop after three failures.\n" || len(listed.Data.Proposals[0].SourcePaths) != 0 {
		t.Fatalf("listed proposals = %+v", listed.Data.Proposals)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("proposal list created private state: %v", err)
	}
}

func TestProposalListRejectsSensitiveCommittedProposalBeforeDisclosure(t *testing.T) {
	const proposalID = "0123456789abcdef0123456789abcdef"
	memoryRoot := newGitRoot(t, map[string]string{
		"proposals/" + proposalID + "-unsafe-fact.md": "---\nschema_version: 1\nsuggested_action: \"create\"\nsource_paths: []\n---\n\n# Unsafe fact\n\npassword=super-secret\n",
	})
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	defer stop()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_proposal_list", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data  any `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, result, &response)
	if !result.IsError || response.Data != nil || response.Error == nil || response.Error.Code != "privacy_sensitive_content" {
		t.Fatalf("sensitive proposal list = %+v, response = %+v", result, response)
	}
	if _, err := os.Stat(stateRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected proposal list created private state: %v", err)
	}
}

func TestCuratePreviewReturnsRelatedCanonicalMemory(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Checkout timeout ownership", "suggested_action": "create",
			"knowledge": "Checkout timeout is maintained by team-b.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"proposal_id": proposal.ID, "target_path": "memories/domains/checkout-timeout-ownership.md",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			RelatedMemory retrieval.ContextBundle `json:"related_memory"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if len(preview.Data.RelatedMemory.Items) != 1 || preview.Data.RelatedMemory.Items[0].Handle.Path != "memories/domains/checkout-api.md" {
		t.Fatalf("related memory = %+v", preview.Data.RelatedMemory)
	}
}

func TestCurateApplyRejectsDriftAndConsumesPreviewToken(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()

	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Retry policy", "suggested_action": "create", "knowledge": "Retries stop after three failures.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"proposal_id": proposal.ID, "target_path": "memories/domains/retry-policy.md",
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

	if err := os.WriteFile(filepath.Join(memoryRoot, "unrelated.txt"), []byte("new commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "add", "unrelated.txt")
	runGit(t, memoryRoot, "commit", "--quiet", "-m", "advance HEAD")
	advancedHead := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	apply := func() (*mcp.CallToolResult, string) {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
		})
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		decodeStructured(t, result, &response)
		if response.Error == nil {
			t.Fatalf("apply response = %+v", result)
		}
		return result, response.Error.Code
	}
	if result, code := apply(); !result.IsError || code != "curation_drift" {
		t.Fatalf("drift apply = %+v, code = %q", result, code)
	}
	if result, code := apply(); !result.IsError || code != "invalid_preview" {
		t.Fatalf("reused token = %+v, code = %q", result, code)
	}
	if head := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); head != advancedHead {
		t.Fatalf("failed apply changed HEAD from %s to %s", advancedHead, head)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "memories", "domains", "retry-policy.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed apply created memory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("failed apply removed proposal: %v", err)
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

func decodeStructured(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatal(err)
	}
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
