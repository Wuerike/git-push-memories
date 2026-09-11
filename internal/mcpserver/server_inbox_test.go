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
	"github.com/Wuerike/git-push-memories/internal/reference"
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

const removableMemoryPage = `# Checkout API
<!-- @id: checkout-api -->

## Operation
<!-- @id: checkout-api-operation -->
Timeout is three seconds.

## Ownership
<!-- @id: checkout-api-ownership -->
Owned by team-b.
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

func TestMemoryProposeReturnsExistingExactDuplicateWithoutNewCommit(t *testing.T) {
	memoryRoot := newGitRoot(t, nil)
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	arguments := map[string]any{
		"title": "Checkout timeout", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
		"knowledge": "Checkout requests time out after five seconds.", "source_paths": []string{"services/checkout/config.yaml"},
	}
	firstResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_propose", Arguments: arguments})
	if err != nil || firstResult.IsError {
		t.Fatalf("first memory_propose = %+v, %v", firstResult, err)
	}
	first := decodeInboxResponse(t, firstResult).Data
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	duplicateResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "memory_propose", Arguments: arguments})
	if err != nil || duplicateResult.IsError {
		t.Fatalf("duplicate memory_propose = %+v, %v", duplicateResult, err)
	}
	duplicate := decodeInboxResponse(t, duplicateResult).Data
	if duplicate == nil || !duplicate.Duplicate || duplicate.ID != first.ID || duplicate.Path != first.Path || duplicate.Commit != first.Commit {
		t.Fatalf("duplicate response = %+v, first = %+v", duplicate, first)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("duplicate proposal advanced HEAD from %s to %s", head, current)
	}
}

func TestReferenceSnapshotFlowsIntoProposalAndVerifiedPreviewEvidence(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, sourceRoot, "init", "--quiet", "--initial-branch=main")
	runGit(t, sourceRoot, "config", "user.name", "Test")
	runGit(t, sourceRoot, "config", "user.email", "test@example.invalid")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "services", "checkout"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "services", "checkout", "config.yaml"), []byte("timeout: 5s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sourceRoot, "add", ".")
	runGit(t, sourceRoot, "commit", "--quiet", "-m", "source")

	memoryRoot := newGitRoot(t, nil)
	stateRoot := filepath.Join(t.TempDir(), "state")
	session, stop := startServer(t, memoryRoot, stateRoot)
	snapshotResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_reference_snapshot", Arguments: map[string]any{
			"repository_root": sourceRoot, "source_paths": []string{"services/checkout/config.yaml"},
		},
	})
	if err != nil || snapshotResult.IsError {
		stop()
		t.Fatalf("memory_reference_snapshot = %+v, %v", snapshotResult, err)
	}
	var snapshot struct {
		Data reference.Snapshot `json:"data"`
	}
	decodeStructured(t, snapshotResult, &snapshot)
	if len(snapshot.Data.References) != 1 || snapshot.Data.References[0].Revision != gitOutput(t, sourceRoot, "rev-parse", "HEAD") {
		stop()
		t.Fatalf("reference snapshot = %+v", snapshot.Data)
	}
	expandResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_reference_expand", Arguments: map[string]any{
			"reference": snapshot.Data.References[0], "start_line": 1, "budget_bytes": 2048,
		},
	})
	if err != nil || expandResult.IsError {
		stop()
		t.Fatalf("memory_reference_expand = %+v, %v", expandResult, err)
	}
	var expanded struct {
		Data reference.Excerpt `json:"data"`
	}
	decodeStructured(t, expandResult, &expanded)
	if expanded.Data.Content != "timeout: 5s\n" || expanded.Data.StartLine != 1 || expanded.Data.Truncated {
		stop()
		t.Fatalf("reference excerpt = %+v", expanded.Data)
	}
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Checkout source", "suggested_action": "create", "knowledge": "Checkout timeout comes from committed configuration.",
			"source_paths": []string{}, "source_references": snapshot.Data.References,
		},
	})
	if err != nil || proposalResult.IsError {
		stop()
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	if proposal.SchemaVersion != 2 || len(proposal.SourceReferences) != 1 {
		stop()
		t.Fatalf("structured proposal = %+v", proposal)
	}
	preview := func(current *mcp.ClientSession) (reference.Evidence, string) {
		result, err := current.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "memory_curate_preview", Arguments: map[string]any{
				"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/checkout-source.md",
			},
		})
		if err != nil || result.IsError {
			t.Fatalf("memory_curate_preview = %+v, %v", result, err)
		}
		var response struct {
			Data struct {
				PreviewToken string `json:"preview_token"`
				Evidence     struct {
					References []reference.Evidence `json:"references"`
				} `json:"evidence"`
			} `json:"data"`
		}
		decodeStructured(t, result, &response)
		if len(response.Data.Evidence.References) != 1 {
			t.Fatalf("reference evidence = %+v", response.Data.Evidence.References)
		}
		return response.Data.Evidence.References[0], response.Data.PreviewToken
	}
	verified, _ := preview(session)
	if verified.Integrity.Status != reference.IntegrityVerified || verified.SemanticSupport != reference.SemanticNotAssessed {
		stop()
		t.Fatalf("verified evidence = %+v", verified)
	}
	mismatchedReference := snapshot.Data.References[0]
	mismatchedReference.ContentHash = "sha256:" + strings.Repeat("0", 64)
	mismatchProposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Divergent checkout source", "suggested_action": "create", "knowledge": "This proposal carries a divergent source hash.",
			"source_paths": []string{}, "source_references": []reference.Reference{mismatchedReference},
		},
	})
	if err != nil || mismatchProposalResult.IsError {
		stop()
		t.Fatalf("mismatched memory_propose = %+v, %v", mismatchProposalResult, err)
	}
	mismatchProposal := decodeInboxResponse(t, mismatchProposalResult).Data
	mismatchPreviewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "create_page", "proposal_id": mismatchProposal.ID, "target_path": "memories/domains/divergent-checkout-source.md",
		},
	})
	if err != nil || mismatchPreviewResult.IsError {
		stop()
		t.Fatalf("mismatch preview = %+v, %v", mismatchPreviewResult, err)
	}
	var mismatchPreview struct {
		Data struct {
			Evidence struct {
				References []reference.Evidence `json:"references"`
			} `json:"evidence"`
		} `json:"data"`
	}
	decodeStructured(t, mismatchPreviewResult, &mismatchPreview)
	if evidence := mismatchPreview.Data.Evidence.References; len(evidence) != 1 || evidence[0].Integrity.Status != reference.IntegrityMismatch || evidence[0].Integrity.Reason != "content_hash_mismatch" || evidence[0].SemanticSupport != reference.SemanticNotAssessed {
		stop()
		t.Fatalf("mismatch evidence = %+v", evidence)
	}
	stop()

	freshSession, freshStop := startServer(t, memoryRoot, stateRoot)
	defer freshStop()
	unavailable, unavailableToken := preview(freshSession)
	if unavailable.Integrity.Status != reference.IntegrityUnavailable || unavailable.Integrity.Reason != "repository_unavailable" {
		t.Fatalf("fresh-process evidence = %+v", unavailable)
	}
	reboundResult, err := freshSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_reference_snapshot", Arguments: map[string]any{
			"repository_root": sourceRoot, "revision": snapshot.Data.Revision, "source_paths": []string{"services/checkout/config.yaml"},
		},
	})
	if err != nil || reboundResult.IsError {
		t.Fatalf("rebind memory_reference_snapshot = %+v, %v", reboundResult, err)
	}
	applyResult, err := freshSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": unavailableToken},
	})
	if err != nil || !applyResult.IsError || decodeResponse(t, applyResult).Error.Code != "curation_drift" {
		t.Fatalf("apply after reference status drift = %+v, %v", applyResult, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "memories", "domains", "checkout-source.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reference status drift created memory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("reference status drift consumed proposal: %v", err)
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
			"operation": "create_page", "proposal_id": proposal.ID,
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
			Evidence     struct {
				References []reference.Evidence `json:"references"`
			} `json:"evidence"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.ProposalID != proposal.ID || preview.Data.Title != proposal.Title || preview.Data.TargetPath != target {
		t.Fatalf("preview = %+v", preview.Data)
	}
	if len(preview.Data.SourcePaths) != 1 || preview.Data.SourcePaths[0] != "services/parcel/dispatch.go" {
		t.Fatalf("preview sources = %v", preview.Data.SourcePaths)
	}
	if references := preview.Data.Evidence.References; len(references) != 1 || references[0].LegacyPath != "services/parcel/dispatch.go" || references[0].Integrity.Status != reference.IntegrityLegacyUnverified {
		t.Fatalf("legacy reference evidence = %+v", references)
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
	if message := gitOutput(t, memoryRoot, "show", "-s", "--format=%B", "HEAD"); !strings.Contains(message, "Git-Push-Memories-Decision: consumed") {
		t.Fatalf("consumption commit does not record its decision:\n%s", message)
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

func TestCurateCreateSectionUpdatesExistingPageAndConsumesProposal(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	target := "memories/domains/checkout-api.md"
	if err := os.Chmod(filepath.Join(memoryRoot, filepath.FromSlash(target)), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "add", target)
	runGit(t, memoryRoot, "commit", "--quiet", "-m", "make memory executable")
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()

	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Checkout retry policy", "suggested_action": "create",
			"knowledge": "Checkout retries stop after three failures.", "source_paths": []string{"services/checkout/retry.go"},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	if proposal == nil {
		t.Fatal("memory_propose returned no proposal")
	}

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "create_section", "proposal_id": proposal.ID, "target_path": target, "heading": "Retry Policy",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Operation    string `json:"operation"`
			SectionID    string `json:"section_id"`
			Diff         string `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.Operation != "create_section" || preview.Data.SectionID != "checkout-api-retry-policy" {
		t.Fatalf("preview = %+v", preview.Data)
	}
	if !strings.Contains(preview.Data.Diff, "+## Retry Policy\n+<!-- @id: checkout-api-retry-policy -->") || !strings.Contains(preview.Data.Diff, "--- a/"+proposal.Path) {
		t.Fatalf("preview diff = %s", preview.Data.Diff)
	}
	beforeApply, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(target)))
	if err != nil || string(beforeApply) != memoryPage {
		t.Fatalf("preview changed target: %q, %v", beforeApply, err)
	}

	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
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
	if applied.Data.ProposalID != proposal.ID || applied.Data.TargetPath != target || applied.Data.SectionID != "checkout-api-retry-policy" || applied.Data.Commit == "" {
		t.Fatalf("apply = %+v", applied.Data)
	}
	wantChanged := "M\t" + target + "\nD\t" + proposal.Path
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"); changed != wantChanged {
		t.Fatalf("curation changed %q, want %q", changed, wantChanged)
	}
	if treeEntry := gitOutput(t, memoryRoot, "ls-tree", "HEAD", "--", target); !strings.HasPrefix(treeEntry, "100755 blob ") {
		t.Fatalf("curation changed target mode: %q", treeEntry)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(target)))
	wantSuffix := "\n## Retry Policy\n<!-- @id: checkout-api-retry-policy -->\nCheckout retries stop after three failures.\n"
	if err != nil || string(page) != memoryPage+wantSuffix {
		t.Fatalf("canonical page = %q, want suffix %q, err = %v", page, wantSuffix, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed proposal remains: %v", err)
	}

	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "checkout retries failures", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search after curation = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	found := false
	if search.Data != nil {
		for _, item := range search.Data.Items {
			if item.Handle.Path == target && item.Handle.SectionID == "checkout-api-retry-policy" && strings.Contains(item.Content, "three failures") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("curated section not retrieved: %+v", search.Data)
	}
}

func TestCurateCreateSectionRejectsExistingHeadingWithoutWriting(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Duplicate operation", "suggested_action": "create", "knowledge": "Another operation description.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "create_section", "proposal_id": proposal.ID,
			"target_path": "memories/domains/checkout-api.md", "heading": "Operacao",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, previewResult, &response)
	if !previewResult.IsError || response.Error == nil || response.Error.Code != "invalid_target" {
		t.Fatalf("collision preview = %+v, response = %+v", previewResult, response)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("collision preview changed HEAD from %s to %s", head, current)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"))
	if err != nil || string(page) != memoryPage {
		t.Fatalf("collision preview changed page: %q, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("collision preview removed proposal: %v", err)
	}
}

func TestCurateReplaceSectionUpdatesCurrentStateAndConsumesProposal(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{
		"memories/domains/checkout-api.md": memoryPage,
		"staged.txt":                       "before staged\n", "unstaged.txt": "before unstaged\n",
	})
	target := "memories/domains/checkout-api.md"
	if err := os.Chmod(filepath.Join(memoryRoot, filepath.FromSlash(target)), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "add", target)
	runGit(t, memoryRoot, "commit", "--quiet", "-m", "make memory executable")
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()

	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Update checkout operation", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{"services/checkout/config.yaml"},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	if proposal == nil {
		t.Fatal("memory_propose returned no proposal")
	}
	if err := os.WriteFile(filepath.Join(memoryRoot, "staged.txt"), []byte("after staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(memoryRoot, "unstaged.txt"), []byte("after unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	searchBefore, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "checkout timeout team-b", "budget_bytes": 2048},
	})
	if err != nil || searchBefore.IsError {
		t.Fatalf("memory_search before curation = %+v, %v", searchBefore, err)
	}
	beforeBundle := decodeResponse(t, searchBefore)
	if beforeBundle.Data == nil || len(beforeBundle.Data.Items) != 1 {
		t.Fatalf("memory_search before curation = %+v", beforeBundle.Data)
	}
	contentHash := beforeBundle.Data.Items[0].Handle.ContentHash
	invalidConsumption, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "replace_section", "proposal_id": proposal.ID, "target_path": target,
			"section_id": "checkout-api-operation", "content_hash": contentHash,
			"replacement": "Timeout is five seconds.\nOwned by team-b.", "reason": "incorrect",
		},
	})
	if err != nil || !invalidConsumption.IsError {
		t.Fatalf("consumption with discard reason = %+v, %v", invalidConsumption, err)
	}
	if code := decodeResponse(t, invalidConsumption).Error.Code; code != "invalid_target" {
		t.Fatalf("consumption with discard reason code = %q", code)
	}

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "replace_section", "proposal_id": proposal.ID,
			"target_path": target, "section_id": "checkout-api-operation", "content_hash": contentHash,
			"replacement": "- timeout: cinco segundos\n- owner: team-b\n",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Operation    string `json:"operation"`
			SectionID    string `json:"section_id"`
			Diff         string `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.Operation != "replace_section" || preview.Data.SectionID != "checkout-api-operation" {
		t.Fatalf("preview = %+v", preview.Data)
	}
	for _, fragment := range []string{"-- timeout: tres segundos", "+- timeout: cinco segundos", "--- a/" + proposal.Path} {
		if !strings.Contains(preview.Data.Diff, fragment) {
			t.Fatalf("preview diff missing %q:\n%s", fragment, preview.Data.Diff)
		}
	}
	if strings.Contains(preview.Data.Diff, "-- owner: team-b") || strings.Contains(preview.Data.Diff, "+- owner: team-b") {
		t.Fatalf("preview rewrote unchanged fact:\n%s", preview.Data.Diff)
	}
	beforeApply, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(target)))
	if err != nil || string(beforeApply) != memoryPage {
		t.Fatalf("preview changed target: %q, %v", beforeApply, err)
	}

	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
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
	if applied.Data.ProposalID != proposal.ID || applied.Data.TargetPath != target || applied.Data.SectionID != "checkout-api-operation" || applied.Data.Commit == "" {
		t.Fatalf("apply = %+v", applied.Data)
	}
	wantChanged := "M\t" + target + "\nD\t" + proposal.Path
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"); changed != wantChanged {
		t.Fatalf("curation changed %q, want %q", changed, wantChanged)
	}
	if treeEntry := gitOutput(t, memoryRoot, "ls-tree", "HEAD", "--", target); !strings.HasPrefix(treeEntry, "100755 blob ") {
		t.Fatalf("curation changed target mode: %q", treeEntry)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(target)))
	wantPage := "# checkout-api\n<!-- @id: checkout-api -->\n\n## Operacao\n<!-- @id: checkout-api-operation -->\n- timeout: cinco segundos\n- owner: team-b\n"
	if err != nil || string(page) != wantPage {
		t.Fatalf("canonical page = %q, want %q, err = %v", page, wantPage, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed proposal remains: %v", err)
	}
	if staged := gitOutput(t, memoryRoot, "diff", "--cached", "--name-only"); staged != "staged.txt" {
		t.Fatalf("staged changes = %q", staged)
	}
	if unstaged := gitOutput(t, memoryRoot, "diff", "--name-only"); unstaged != "unstaged.txt" {
		t.Fatalf("unstaged changes = %q", unstaged)
	}

	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "checkout cinco team-b", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search after curation = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 || search.Data.Items[0].Handle.SectionID != "checkout-api-operation" ||
		strings.Contains(search.Data.Items[0].Content, "tres segundos") || !strings.Contains(search.Data.Items[0].Content, "team-b") {
		t.Fatalf("curated replacement search = %+v", search.Data)
	}
}

func TestCurateReplaceSectionRejectsNonLeafTargetWithoutWriting(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Rewrite checkout page", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "Replacement.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "replace_section", "proposal_id": proposal.ID,
			"target_path": "memories/domains/checkout-api.md", "section_id": "checkout-api",
			"content_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "replacement": "Replacement.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, previewResult, &response)
	if !previewResult.IsError || response.Error == nil || response.Error.Code != "invalid_target" {
		t.Fatalf("non-leaf preview = %+v, response = %+v", previewResult, response)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("non-leaf preview changed HEAD from %s to %s", head, current)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"))
	if err != nil || string(page) != memoryPage {
		t.Fatalf("non-leaf preview changed page: %q, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("non-leaf preview removed proposal: %v", err)
	}
}

func TestCurateReplaceSectionRejectsStaleContentHashWithoutWriting(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Update checkout timeout", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "replace_section", "proposal_id": proposal.ID,
			"target_path": "memories/domains/checkout-api.md", "section_id": "checkout-api-operation",
			"content_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			"replacement":  "- timeout: cinco segundos\n- owner: team-b\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, previewResult, &response)
	if !previewResult.IsError || response.Error == nil || response.Error.Code != "curation_drift" {
		t.Fatalf("stale preview = %+v, response = %+v", previewResult, response)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("stale preview changed HEAD from %s to %s", head, current)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"))
	if err != nil || string(page) != memoryPage {
		t.Fatalf("stale preview changed page: %q, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("stale preview removed proposal: %v", err)
	}
}

func TestCurateReplaceSectionRejectsCreateProposal(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "New checkout fact", "suggested_action": "create", "knowledge": "New fact.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "replace_section", "proposal_id": proposal.ID,
			"target_path": "memories/domains/checkout-api.md", "section_id": "checkout-api-operation",
			"content_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "replacement": "New fact.\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, previewResult, &response)
	if !previewResult.IsError || response.Error == nil || response.Error.Code != "unsupported_action" {
		t.Fatalf("wrong-action preview = %+v, response = %+v", previewResult, response)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"))
	if err != nil || string(page) != memoryPage {
		t.Fatalf("wrong-action preview changed page: %q, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("wrong-action preview removed proposal: %v", err)
	}
}

func TestCurateRemoveSectionDeletesLeafAndConsumesProposal(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": removableMemoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "operation timeout seconds", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("memory_search = %+v", search.Data)
	}
	handle := search.Data.Items[0].Handle
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Remove obsolete checkout timeout", "suggested_action": "remove", "target": handle.Path,
			"knowledge": "The checkout timeout section is obsolete and must be removed.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "remove_section", "proposal_id": proposal.ID, "target_path": handle.Path,
			"section_id": handle.SectionID, "content_hash": handle.ContentHash,
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Operation    string `json:"operation"`
			Diff         string `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.Operation != "remove_section" {
		t.Fatalf("preview = %+v", preview.Data)
	}
	for _, fragment := range []string{"-## Operation", "-<!-- @id: checkout-api-operation -->", "-Timeout is three seconds.", "--- a/" + proposal.Path} {
		if !strings.Contains(preview.Data.Diff, fragment) {
			t.Fatalf("preview diff missing %q:\n%s", fragment, preview.Data.Diff)
		}
	}
	beforeApply, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path)))
	if err != nil || string(beforeApply) != removableMemoryPage {
		t.Fatalf("preview changed target: %q, %v", beforeApply, err)
	}

	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_curate_apply = %+v, %v", applyResult, err)
	}
	wantChanged := "M\t" + handle.Path + "\nD\t" + proposal.Path
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"); changed != wantChanged {
		t.Fatalf("curation changed %q, want %q", changed, wantChanged)
	}
	wantPage := "# Checkout API\n<!-- @id: checkout-api -->\n\n## Ownership\n<!-- @id: checkout-api-ownership -->\nOwned by team-b.\n"
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path)))
	if err != nil || string(page) != wantPage {
		t.Fatalf("canonical page = %q, want %q, err = %v", page, wantPage, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed proposal remains: %v", err)
	}
	removedResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "timeout seconds", "budget_bytes": 2048},
	})
	if err != nil || removedResult.IsError {
		t.Fatalf("memory_search after removal = %+v, %v", removedResult, err)
	}
	removed := decodeResponse(t, removedResult)
	if removed.Data == nil || len(removed.Data.Items) != 0 {
		t.Fatalf("removed knowledge remains retrievable: %+v", removed.Data)
	}
}

func TestCurateRemovePageDeletesEligiblePageAndConsumesProposal(t *testing.T) {
	const page = `# Legacy Retry Policy
<!-- @id: legacy-retry-policy -->

## Overview
<!-- @id: legacy-retry-policy-overview -->
Retries use the retired queue.
`
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/legacy-retry-policy.md": page})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "retired queue", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("memory_search = %+v", search.Data)
	}
	handle := search.Data.Items[0].Handle
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Remove retired retry policy", "suggested_action": "remove", "target": handle.Path,
			"knowledge": "The legacy retry policy page is obsolete and must be removed.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "remove_page", "proposal_id": proposal.ID, "target_path": handle.Path,
			"section_id": handle.SectionID, "content_hash": handle.ContentHash,
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Operation    string `json:"operation"`
			Diff         string `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.Operation != "remove_page" {
		t.Fatalf("preview = %+v", preview.Data)
	}
	for _, fragment := range []string{"-# Legacy Retry Policy", "-Retries use the retired queue.", "--- a/" + proposal.Path} {
		if !strings.Contains(preview.Data.Diff, fragment) {
			t.Fatalf("preview diff missing %q:\n%s", fragment, preview.Data.Diff)
		}
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path))); err != nil {
		t.Fatalf("preview removed target: %v", err)
	}

	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_curate_apply = %+v, %v", applyResult, err)
	}
	wantChanged := "D\t" + handle.Path + "\nD\t" + proposal.Path
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"); changed != wantChanged {
		t.Fatalf("curation changed %q, want %q", changed, wantChanged)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed page remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed proposal remains: %v", err)
	}
	removedResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "retired queue", "budget_bytes": 2048},
	})
	if err != nil || removedResult.IsError {
		t.Fatalf("memory_search after removal = %+v, %v", removedResult, err)
	}
	removed := decodeResponse(t, removedResult)
	if removed.Data == nil || len(removed.Data.Items) != 0 {
		t.Fatalf("removed page remains retrievable: %+v", removed.Data)
	}
}

func TestCurateRemovePageRejectsIneligiblePageWithoutWriting(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": removableMemoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "operation timeout seconds", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	if search.Data == nil || len(search.Data.Items) != 1 {
		t.Fatalf("memory_search = %+v", search.Data)
	}
	handle := search.Data.Items[0].Handle
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Remove checkout page", "suggested_action": "remove", "target": handle.Path,
			"knowledge": "The selected checkout timeout is obsolete.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "remove_page", "proposal_id": proposal.ID, "target_path": handle.Path,
			"section_id": handle.SectionID, "content_hash": handle.ContentHash,
		},
	})
	if err != nil || !previewResult.IsError {
		t.Fatalf("ineligible remove_page = %+v, %v", previewResult, err)
	}
	if code := decodeResponse(t, previewResult).Error.Code; code != "invalid_target" {
		t.Fatalf("ineligible remove_page code = %q", code)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("rejected remove_page changed HEAD from %s to %s", head, current)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path)))
	if err != nil || string(page) != removableMemoryPage {
		t.Fatalf("rejected remove_page changed page: %q, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("rejected remove_page removed proposal: %v", err)
	}
}

func TestCurateRemoveSectionRejectsApplyTimeDriftAndConsumesToken(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": removableMemoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	searchResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": "operation timeout seconds", "budget_bytes": 2048},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("memory_search = %+v, %v", searchResult, err)
	}
	search := decodeResponse(t, searchResult)
	handle := search.Data.Items[0].Handle
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Remove checkout timeout", "suggested_action": "remove", "target": handle.Path,
			"knowledge": "The checkout timeout is obsolete.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "remove_section", "proposal_id": proposal.ID, "target_path": handle.Path,
			"section_id": handle.SectionID, "content_hash": handle.ContentHash,
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
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	driftedPage := removableMemoryPage + "external drift\n"
	if err := os.WriteFile(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path)), []byte(driftedPage), 0o644); err != nil {
		t.Fatal(err)
	}
	apply := func() (*mcp.CallToolResult, string) {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result, decodeResponse(t, result).Error.Code
	}
	if result, code := apply(); !result.IsError || code != "curation_drift" {
		t.Fatalf("drifted remove apply = %+v, code = %q", result, code)
	}
	if result, code := apply(); !result.IsError || code != "invalid_preview" {
		t.Fatalf("reused remove token = %+v, code = %q", result, code)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("failed remove changed HEAD from %s to %s", head, current)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, filepath.FromSlash(handle.Path)))
	if err != nil || string(page) != driftedPage {
		t.Fatalf("failed remove overwrote drift: %q, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("failed remove consumed proposal: %v", err)
	}
}

func TestCurateDiscardProposalRecordsReasonWithoutChangingMemory(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{
		"memories/domains/checkout-api.md": memoryPage,
		"staged.txt":                       "before staged\n",
		"unstaged.txt":                     "before unstaged\n",
	})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Add incorrect checkout note", "suggested_action": "create",
			"knowledge": "This knowledge was found to be incorrect.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	if err := os.WriteFile(filepath.Join(memoryRoot, "staged.txt"), []byte("after staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, memoryRoot, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(memoryRoot, "unstaged.txt"), []byte("after unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "incorrect",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Operation    string `json:"operation"`
			Reason       string `json:"reason"`
			Diff         string `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || preview.Data.Operation != "discard_proposal" || preview.Data.Reason != "incorrect" {
		t.Fatalf("preview = %+v", preview.Data)
	}
	if !strings.Contains(preview.Data.Diff, "--- a/"+proposal.Path) || strings.Contains(preview.Data.Diff, "memories/domains/checkout-api.md") {
		t.Fatalf("discard diff changed memory:\n%s", preview.Data.Diff)
	}

	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("memory_curate_apply = %+v, %v", applyResult, err)
	}
	var applied struct {
		Data struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
			Commit   string `json:"commit"`
		} `json:"data"`
	}
	decodeStructured(t, applyResult, &applied)
	if applied.Data.Decision != "discarded" || applied.Data.Reason != "incorrect" || applied.Data.Commit == "" {
		t.Fatalf("discard result = %+v", applied.Data)
	}
	wantChanged := "D\t" + proposal.Path
	if changed := gitOutput(t, memoryRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"); changed != wantChanged {
		t.Fatalf("discard changed %q, want %q", changed, wantChanged)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"))
	if err != nil || string(page) != memoryPage {
		t.Fatalf("discard changed canonical memory: %q, %v", page, err)
	}
	if staged := gitOutput(t, memoryRoot, "diff", "--cached", "--name-only"); staged != "staged.txt" {
		t.Fatalf("discard changed staged paths: %q", staged)
	}
	if unstaged := gitOutput(t, memoryRoot, "diff", "--name-only"); unstaged != "unstaged.txt" {
		t.Fatalf("discard changed unstaged paths: %q", unstaged)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discarded proposal remains: %v", err)
	}
	message := gitOutput(t, memoryRoot, "show", "-s", "--format=%B", "HEAD")
	for _, fragment := range []string{"Discard memory proposal", "Git-Push-Memories-Decision: discarded", "Git-Push-Memories-Reason: incorrect"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("discard commit message missing %q:\n%s", fragment, message)
		}
	}
}

func TestCurateDiscardRejectsUnknownReasonAndPreviewCancellationKeepsProposal(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Questionable checkout note", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "This note requires a curation decision.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")

	invalidResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "maybe_later",
		},
	})
	if err != nil || !invalidResult.IsError {
		t.Fatalf("invalid discard preview = %+v, %v", invalidResult, err)
	}
	var invalid struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, invalidResult, &invalid)
	if invalid.Error.Code != "invalid_discard_reason" {
		t.Fatalf("invalid discard error = %q", invalid.Error.Code)
	}

	validResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "duplicate",
		},
	})
	if err != nil || validResult.IsError {
		t.Fatalf("valid discard preview = %+v, %v", validResult, err)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("cancelled preview changed HEAD from %s to %s", head, current)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("cancelled preview removed proposal: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"))
	if err != nil || string(page) != memoryPage {
		t.Fatalf("discard previews changed memory: %q, %v", page, err)
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

func TestCuratePreviewSeparatesCanonicalAndRelatedProposalEvidence(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	relatedResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Alternative checkout timeout", "suggested_action": "update", "target": "memories/domains/checkout-api.md",
			"knowledge": "Checkout timeout may be five seconds.", "source_paths": []string{},
		},
	})
	if err != nil || relatedResult.IsError {
		t.Fatalf("related memory_propose = %+v, %v", relatedResult, err)
	}
	related := decodeInboxResponse(t, relatedResult).Data
	unrelatedResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Storage allocation", "suggested_action": "create",
			"knowledge": "Partitions follow geography.", "source_paths": []string{},
		},
	})
	if err != nil || unrelatedResult.IsError {
		t.Fatalf("unrelated memory_propose = %+v, %v", unrelatedResult, err)
	}
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
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/checkout-timeout-ownership.md",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			Evidence struct {
				CanonicalCandidates retrieval.ContextBundle `json:"canonical_candidates"`
				RelatedProposals    struct {
					Items []struct {
						ID        string `json:"id"`
						Title     string `json:"title"`
						Knowledge string `json:"knowledge"`
						Score     int    `json:"score"`
					} `json:"items"`
					Truncated bool `json:"truncated"`
				} `json:"related_proposals"`
			} `json:"evidence"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if candidates := preview.Data.Evidence.CanonicalCandidates; len(candidates.Items) != 1 || candidates.Items[0].Handle.Path != "memories/domains/checkout-api.md" {
		t.Fatalf("canonical candidates = %+v", candidates)
	}
	proposals := preview.Data.Evidence.RelatedProposals
	if len(proposals.Items) != 1 || proposals.Items[0].ID != related.ID || proposals.Items[0].Title != related.Title || proposals.Items[0].Knowledge != related.Knowledge || proposals.Items[0].Score == 0 || proposals.Truncated {
		t.Fatalf("related proposals = %+v", proposals)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("evidence preview changed HEAD from %s to %s", head, current)
	}
}

func TestCurateApplyRejectsDriftInReviewedEvidence(t *testing.T) {
	memoryRoot := newGitRoot(t, map[string]string{"memories/domains/checkout-api.md": memoryPage})
	session, stop := startServer(t, memoryRoot, filepath.Join(t.TempDir(), "state"))
	defer stop()
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_propose", Arguments: map[string]any{
			"title": "Checkout timeout note", "suggested_action": "create",
			"knowledge": "Checkout timeout documentation belongs to operations.", "source_paths": []string{},
		},
	})
	if err != nil || proposalResult.IsError {
		t.Fatalf("memory_propose = %+v, %v", proposalResult, err)
	}
	proposal := decodeInboxResponse(t, proposalResult).Data
	previewResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_preview", Arguments: map[string]any{
			"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/checkout-timeout-note.md",
		},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("memory_curate_preview = %+v, %v", previewResult, err)
	}
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Evidence     struct {
				CanonicalCandidates retrieval.ContextBundle `json:"canonical_candidates"`
			} `json:"evidence"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if len(preview.Data.Evidence.CanonicalCandidates.Items) != 1 {
		t.Fatalf("preview evidence = %+v", preview.Data.Evidence)
	}
	head := gitOutput(t, memoryRoot, "rev-parse", "HEAD")
	drifted := strings.Replace(memoryPage, "tres segundos", "quatro segundos", 1)
	if err := os.WriteFile(filepath.Join(memoryRoot, "memories", "domains", "checkout-api.md"), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	applyResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": preview.Data.PreviewToken},
	})
	if err != nil || !applyResult.IsError {
		t.Fatalf("apply with evidence drift = %+v, %v", applyResult, err)
	}
	if code := decodeResponse(t, applyResult).Error.Code; code != "curation_drift" {
		t.Fatalf("apply with evidence drift code = %q", code)
	}
	if current := gitOutput(t, memoryRoot, "rev-parse", "HEAD"); current != head {
		t.Fatalf("evidence drift changed HEAD from %s to %s", head, current)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, "memories", "domains", "checkout-timeout-note.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("evidence drift created target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(memoryRoot, filepath.FromSlash(proposal.Path))); err != nil {
		t.Fatalf("evidence drift consumed proposal: %v", err)
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
			"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/retry-policy.md",
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
