package mcpserver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/reference"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type previewData struct {
	PreviewToken string   `json:"preview_token"`
	ProposalID   string   `json:"proposal_id"`
	Title        string   `json:"title"`
	Operation    string   `json:"operation"`
	SectionID    string   `json:"section_id"`
	TargetPath   string   `json:"target_path"`
	Reason       string   `json:"reason"`
	SourcePaths  []string `json:"source_paths"`
	Diff         string   `json:"diff"`
	Evidence     struct {
		References []reference.Evidence `json:"references"`
	} `json:"evidence"`
}

func decodePreview(t *testing.T, result *mcp.CallToolResult) previewData {
	t.Helper()
	var preview struct {
		Data previewData `json:"data"`
	}
	decodeStructured(t, result, &preview)
	return preview.Data
}

type appliedData struct {
	ProposalID string `json:"proposal_id"`
	TargetPath string `json:"target_path"`
	SectionID  string `json:"section_id"`
	Commit     string `json:"commit"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
}

func decodeApplied(t *testing.T, result *mcp.CallToolResult) appliedData {
	t.Helper()
	var applied struct {
		Data appliedData `json:"data"`
	}
	decodeStructured(t, result, &applied)
	return applied.Data
}

func TestMemorySearchAndExpandConfiguredCategory(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})

	search := e.search(t, "", "checkout timeout team")
	if search == nil || len(search.Items) != 1 || search.Items[0].Handle.Path != checkoutPath {
		t.Fatalf("memory_search = %+v", search)
	}
	expandResult := e.ok(t, "memory_expand", map[string]any{"handle": search.Items[0].Handle, "budget_bytes": 4096})
	expanded := decodeResponse(t, expandResult)
	if expanded.Data == nil || len(expanded.Data.Items) != 1 || !strings.Contains(expanded.Data.Items[0].Content, "timeout: tres segundos") {
		t.Fatalf("memory_expand response = %+v", expanded)
	}
}

func TestMemoryReadFollowsPublishedMain(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	if search := e.search(t, "main", "settlement retry failures"); len(search.Items) != 0 {
		t.Fatalf("search before publication = %+v", search)
	}
	e.m.PublishMain(t, map[string]string{"memories/domains/settlement.md": "# Settlement\n<!-- @id: settlement -->\n\n## Retry\n<!-- @id: settlement-retry -->\nSettlement retries stop after three failures.\n"})
	// The read worktree answers from the last refresh until the TTL expires.
	if search := e.search(t, "", "settlement retry failures"); len(search.Items) != 0 {
		t.Fatalf("search before refresh = %+v", search)
	}
	if err := os.Remove(filepath.Join(e.m.Config.Paths.State, "fetch-attempt")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Config.Workspace().ResolveRead(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	search := e.search(t, "", "settlement retry failures")
	if len(search.Items) != 1 || search.Items[0].Handle.Path != "memories/domains/settlement.md" {
		t.Fatalf("search after refresh = %+v", search)
	}
}

func TestReadToolsRejectUnavailableOrUnknownSource(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	search := e.call(t, "memory_search", map[string]any{"query": "checkout", "budget_bytes": 2048, "source": "release"})
	if !search.IsError || errorCode(t, search) != "no_release" {
		t.Fatalf("release search without release = %+v, code %q", search, errorCode(t, search))
	}
	expand := e.call(t, "memory_expand", map[string]any{"handle": retrieval.Handle{Path: checkoutPath}, "budget_bytes": 2048, "source": "release"})
	if !expand.IsError || errorCode(t, expand) != "no_release" {
		t.Fatalf("release expand without release = %+v, code %q", expand, errorCode(t, expand))
	}
	if result, page := e.pageRead(t, "release", checkoutPath, 4096); !result.IsError || page.Error == nil || page.Error.Code != "no_release" {
		t.Fatalf("release page_read without release = %+v, %+v", result, page)
	}
	list := e.call(t, "memory_proposal_list", map[string]any{"source": "release"})
	if !list.IsError || errorCode(t, list) != "no_release" {
		t.Fatalf("release proposal list without release = %+v, code %q", list, errorCode(t, list))
	}
	for name, arguments := range map[string]map[string]any{
		"memory_search":        {"source": "elsewhere", "query": "checkout", "budget_bytes": 2048},
		"memory_proposal_list": {"source": "elsewhere"},
	} {
		if result := e.call(t, name, arguments); !result.IsError || errorCode(t, result) != "invalid_source" {
			t.Fatalf("%s with unknown source = %+v, code %q", name, result, errorCode(t, result))
		}
	}
	if result, page := e.pageRead(t, "elsewhere", checkoutPath, 4096); !result.IsError || page.Error == nil || page.Error.Code != "invalid_source" {
		t.Fatalf("page_read with unknown source = %+v, %+v", result, page)
	}
}

func TestMemoryProposeCommitsMarkdownAndPushesToProposals(t *testing.T) {
	e := newEnv(t, nil)
	root := e.proposalsRoot()
	parent := commitHeads(t, root)

	result := e.ok(t, "memory_propose", map[string]any{
		"title":            "Card transactions from authorizer",
		"suggested_action": "update",
		"target":           "memories/domains/charge-entity-payment-methods.md",
		"knowledge":        "Card transactions can originate from the authorizer adapter.",
		"source_paths":     []string{"adapters/authorizer/config.yaml"},
	})
	response := decodeInboxResponse(t, result)
	if response.Data == nil || response.Data.ID == "" || response.Data.SuggestedAction != inbox.ActionUpdate || !response.Pushed || response.Message != "" {
		t.Fatalf("memory_propose response = %+v", response)
	}
	current := commitHeads(t, root)
	if response.Data.Commit != current || gitOutput(t, root, "rev-parse", "HEAD^") != parent {
		t.Fatalf("memory_propose commit = %q, HEAD = %q", response.Data.Commit, current)
	}
	if changed := gitOutput(t, root, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); changed != response.Data.Path {
		t.Fatalf("memory_propose changed %q, want %q", changed, response.Data.Path)
	}
	if pushed := gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals"); pushed != current {
		t.Fatalf("remote proposals = %s, want %s", pushed, current)
	}
	if main := gitOutput(t, e.m.Remote, "ls-tree", "-r", "--name-only", "main"); strings.Contains(main, "proposals/") {
		t.Fatalf("proposal reached main: %q", main)
	}
	if branches := gitOutput(t, e.m.Config.Paths.Repo, "for-each-ref", "--format=%(refname)", "refs/heads/gpm/proposal/"); branches != "" {
		t.Fatalf("memory_propose created legacy branches: %q", branches)
	}
}

func TestMemoryProposeKeepsProposalLocalWhenPushFails(t *testing.T) {
	e := newEnv(t, nil)
	if err := os.Rename(e.m.Remote, e.m.Remote+".offline"); err != nil {
		t.Fatal(err)
	}
	response := decodeInboxResponse(t, e.ok(t, "memory_propose", map[string]any{
		"title": "Offline fact", "suggested_action": "create", "knowledge": "Knowledge.", "source_paths": []string{},
	}))
	if response.Data == nil || response.Pushed || response.Message == "" {
		t.Fatalf("offline propose = %+v", response)
	}
	if commit := gitOutput(t, e.proposalsRoot(), "rev-parse", "HEAD"); commit != response.Data.Commit {
		t.Fatalf("local HEAD = %s, proposal commit = %s", commit, response.Data.Commit)
	}
	if err := os.Rename(e.m.Remote+".offline", e.m.Remote); err != nil {
		t.Fatal(err)
	}
	next := decodeInboxResponse(t, e.ok(t, "memory_propose", map[string]any{
		"title": "Online fact", "suggested_action": "create", "knowledge": "Other knowledge.", "source_paths": []string{},
	}))
	if !next.Pushed {
		t.Fatalf("propose after recovery = %+v", next)
	}
	if pushed := gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals"); pushed != next.Data.Commit {
		t.Fatalf("remote proposals = %s, want both proposals up to %s", pushed, next.Data.Commit)
	}
	if !strings.Contains(gitOutput(t, e.m.Remote, "ls-tree", "-r", "--name-only", "proposals"), response.Data.Path) {
		t.Fatalf("offline proposal %s was not sent with the next call", response.Data.Path)
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

	e := newEnv(t, nil)
	snapshotResult := e.ok(t, "memory_reference_snapshot", map[string]any{
		"repository_root": sourceRoot, "source_paths": []string{"services/checkout/config.yaml"},
	})
	var snapshot struct {
		Data reference.Snapshot `json:"data"`
	}
	decodeStructured(t, snapshotResult, &snapshot)
	if len(snapshot.Data.References) != 1 || snapshot.Data.References[0].Revision != gitOutput(t, sourceRoot, "rev-parse", "HEAD") {
		t.Fatalf("reference snapshot = %+v", snapshot.Data)
	}
	expandResult := e.ok(t, "memory_reference_expand", map[string]any{
		"reference": snapshot.Data.References[0], "start_line": 1, "budget_bytes": 2048,
	})
	var expanded struct {
		Data reference.Excerpt `json:"data"`
	}
	decodeStructured(t, expandResult, &expanded)
	if expanded.Data.Content != "timeout: 5s\n" || expanded.Data.StartLine != 1 || expanded.Data.Truncated {
		t.Fatalf("reference excerpt = %+v", expanded.Data)
	}
	proposal := e.propose(t, map[string]any{
		"title": "Checkout source", "suggested_action": "create", "knowledge": "Checkout timeout comes from committed configuration.",
		"source_paths": []string{}, "source_references": snapshot.Data.References,
	})
	if proposal.SchemaVersion != 2 || len(proposal.SourceReferences) != 1 {
		t.Fatalf("structured proposal = %+v", proposal)
	}
	mismatchedReference := snapshot.Data.References[0]
	mismatchedReference.ContentHash = "sha256:" + strings.Repeat("0", 64)
	mismatchProposal := e.propose(t, map[string]any{
		"title": "Divergent checkout source", "suggested_action": "create", "knowledge": "This proposal carries a divergent source hash.",
		"source_paths": []string{}, "source_references": []reference.Reference{mismatchedReference},
	})
	release := e.openRelease(t)

	preview := func(current *env, id, target string) (reference.Evidence, string) {
		data := decodePreview(t, current.ok(t, "memory_promote_preview", map[string]any{
			"operation": "create_page", "proposal_id": id, "target_path": target,
		}))
		if len(data.Evidence.References) != 1 {
			t.Fatalf("reference evidence = %+v", data.Evidence.References)
		}
		return data.Evidence.References[0], data.PreviewToken
	}
	verified, _ := preview(e, proposal.ID, "memories/domains/checkout-source.md")
	if verified.Integrity.Status != reference.IntegrityVerified || verified.SemanticSupport != reference.SemanticNotAssessed {
		t.Fatalf("verified evidence = %+v", verified)
	}
	mismatch, _ := preview(e, mismatchProposal.ID, "memories/domains/divergent-checkout-source.md")
	if mismatch.Integrity.Status != reference.IntegrityMismatch || mismatch.Integrity.Reason != "content_hash_mismatch" || mismatch.SemanticSupport != reference.SemanticNotAssessed {
		t.Fatalf("mismatch evidence = %+v", mismatch)
	}
	e.stop()

	fresh := &env{m: e.m}
	fresh.session, fresh.stop = startServer(t, e.m)
	defer fresh.stop()
	unavailable, unavailableToken := preview(fresh, proposal.ID, "memories/domains/checkout-source.md")
	if unavailable.Integrity.Status != reference.IntegrityUnavailable || unavailable.Integrity.Reason != "repository_unavailable" {
		t.Fatalf("fresh-process evidence = %+v", unavailable)
	}
	fresh.ok(t, "memory_reference_snapshot", map[string]any{
		"repository_root": sourceRoot, "revision": snapshot.Data.Revision, "source_paths": []string{"services/checkout/config.yaml"},
	})
	applyResult := fresh.call(t, "memory_promote_apply", map[string]any{"preview_token": unavailableToken})
	if !applyResult.IsError || errorCode(t, applyResult) != "promotion_drift" {
		t.Fatalf("apply after reference status drift = %+v", applyResult)
	}
	if exists(release, "memories/domains/checkout-source.md") {
		t.Fatal("reference status drift created memory")
	}
	if !exists(release, proposal.Path) {
		t.Fatal("reference status drift consumed proposal")
	}
}

func TestMemoryProposeRejectsSensitiveKnowledgeWithoutCommit(t *testing.T) {
	e := newEnv(t, nil)
	head := commitHeads(t, e.proposalsRoot())
	result := e.call(t, "memory_propose", map[string]any{
		"title": "Unsafe fact", "suggested_action": "create", "knowledge": "password=super-secret", "source_paths": []string{},
	})
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "privacy_sensitive_content" {
		t.Fatalf("sensitive result = %+v, response = %+v", result, response)
	}
	if current := commitHeads(t, e.proposalsRoot()); current != head {
		t.Fatalf("rejected proposal moved proposals HEAD from %s to %s", head, current)
	}
}

func TestMemoryProposeRejectsTargetOutsideConfiguredCategories(t *testing.T) {
	e := newEnv(t, nil)
	head := commitHeads(t, e.proposalsRoot())
	result := e.call(t, "memory_propose", map[string]any{
		"title": "Fact", "suggested_action": "update", "target": "memories/notes/fact.md", "knowledge": "Knowledge.", "source_paths": []string{},
	})
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "schema_invalid_target" {
		t.Fatalf("unconfigured target result = %+v, response = %+v", result, response)
	}
	if current := commitHeads(t, e.proposalsRoot()); current != head {
		t.Fatalf("rejected target moved proposals HEAD from %s to %s", head, current)
	}
}

func TestMemoryProposeRequiresProposalsBranch(t *testing.T) {
	e := newEnv(t, nil)
	runGit(t, e.proposalsRoot(), "switch", "--quiet", "-c", "feature")
	result := e.call(t, "memory_propose", map[string]any{"title": "Fact", "suggested_action": "create", "knowledge": "Knowledge.", "source_paths": []string{}})
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "wrong_branch" {
		t.Fatalf("wrong-branch result = %+v, response = %+v", result, response)
	}
}

func TestMemoryProposeReportsDirtyWorktree(t *testing.T) {
	e := newEnv(t, nil)
	writeFile(t, e.proposalsRoot(), "stray.txt", "left by someone\n")
	result := e.call(t, "memory_propose", map[string]any{"title": "Fact", "suggested_action": "create", "knowledge": "Knowledge.", "source_paths": []string{}})
	response := decodeInboxResponse(t, result)
	if !result.IsError || response.Error == nil || response.Error.Code != "worktree_dirty" {
		t.Fatalf("dirty worktree result = %+v, response = %+v", result, response)
	}
}

func TestPromoteToolsRequireAnOpenRelease(t *testing.T) {
	e := newEnv(t, nil)
	proposal := e.propose(t, map[string]any{"title": "Fact", "suggested_action": "create", "knowledge": "Knowledge.", "source_paths": []string{}})
	for name, arguments := range map[string]map[string]any{
		"memory_promote_preview":       {"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "duplicate"},
		"memory_promote_batch_preview": {"items": []map[string]any{{"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "duplicate"}}},
	} {
		if result := e.call(t, name, arguments); !result.IsError || errorCode(t, result) != "no_release" {
			t.Fatalf("%s without release = %+v, code %q", name, result, errorCode(t, result))
		}
	}
	for _, name := range []string{"memory_promote_apply", "memory_promote_batch_apply"} {
		if result := e.call(t, name, map[string]any{"preview_token": "unknown"}); !result.IsError || errorCode(t, result) != "invalid_preview" {
			t.Fatalf("%s with unknown token = %+v, code %q", name, result, errorCode(t, result))
		}
	}
}

type listedProposals struct {
	Data struct {
		Categories []string `json:"categories"`
		Proposals  []struct {
			ID              string `json:"id"`
			Title           string `json:"title"`
			SuggestedAction string `json:"suggested_action"`
		} `json:"proposals"`
	} `json:"data"`
}

func TestPromoteCreatePreviewAndApplyPublishesCanonicalMemory(t *testing.T) {
	e := newEnv(t, nil)
	proposal := e.propose(t, map[string]any{
		"title":            "Parcel dispatch retry policy",
		"suggested_action": "create",
		"knowledge":        "Parcel dispatch retries use exponential backoff and stop after three failures.",
		"source_paths":     []string{"services/parcel/dispatch.go"},
	})

	var listed listedProposals
	decodeStructured(t, e.ok(t, "memory_proposal_list", map[string]any{}), &listed)
	if len(listed.Data.Proposals) != 1 || listed.Data.Proposals[0].ID != proposal.ID || listed.Data.Proposals[0].Title != proposal.Title || listed.Data.Proposals[0].SuggestedAction != "create" {
		t.Fatalf("listed proposals = %+v", listed.Data.Proposals)
	}
	if strings.Join(listed.Data.Categories, ",") != "domains,decision" {
		t.Fatalf("listed categories = %v", listed.Data.Categories)
	}

	release := e.openRelease(t)
	var inRelease listedProposals
	decodeStructured(t, e.ok(t, "memory_proposal_list", map[string]any{"source": "release"}), &inRelease)
	if len(inRelease.Data.Proposals) != 1 || inRelease.Data.Proposals[0].ID != proposal.ID {
		t.Fatalf("release proposals = %+v", inRelease.Data.Proposals)
	}
	releaseHead := commitHeads(t, release)

	target := "memories/domains/parcel-dispatch-retry-policy.md"
	preview := decodePreview(t, e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "create_page", "proposal_id": proposal.ID, "target_path": target,
	}))
	if preview.PreviewToken == "" || preview.ProposalID != proposal.ID || preview.Title != proposal.Title || preview.TargetPath != target {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.SourcePaths) != 1 || preview.SourcePaths[0] != "services/parcel/dispatch.go" {
		t.Fatalf("preview sources = %v", preview.SourcePaths)
	}
	if references := preview.Evidence.References; len(references) != 1 || references[0].LegacyPath != "services/parcel/dispatch.go" || references[0].Integrity.Status != reference.IntegrityLegacyUnverified {
		t.Fatalf("legacy reference evidence = %+v", references)
	}
	if !strings.Contains(preview.Diff, "+++ b/"+target) || !strings.Contains(preview.Diff, "--- a/"+proposal.Path) || !strings.Contains(preview.Diff, "+++ /dev/null") {
		t.Fatalf("preview diff = %s", preview.Diff)
	}
	if current := commitHeads(t, release); current != releaseHead {
		t.Fatalf("preview changed HEAD from %s to %s", releaseHead, current)
	}
	if exists(release, target) {
		t.Fatal("preview created target")
	}

	applied := decodeApplied(t, e.ok(t, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken}))
	if applied.ProposalID != proposal.ID || applied.TargetPath != target || applied.SectionID != "parcel-dispatch-retry-policy-overview" || applied.Commit == "" {
		t.Fatalf("apply = %+v", applied)
	}
	if head := commitHeads(t, release); head != applied.Commit {
		t.Fatalf("HEAD = %s, commit = %s", head, applied.Commit)
	}
	changed := gitOutput(t, release, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD")
	if wantChanged := "A\t" + target + "\nD\t" + proposal.Path; changed != wantChanged {
		t.Fatalf("promotion changed %q, want %q", changed, wantChanged)
	}
	if exists(release, proposal.Path) {
		t.Fatal("consumed proposal remains")
	}
	if message := gitOutput(t, release, "show", "-s", "--format=%B", "HEAD"); !strings.Contains(message, "Git-Push-Memories-Decision: consumed") {
		t.Fatalf("consumption commit does not record its decision:\n%s", message)
	}
	if page := readFile(t, release, target); !strings.Contains(page, "Parcel dispatch retries use exponential backoff") {
		t.Fatalf("canonical page = %q", page)
	}

	search := e.search(t, "release", "parcel dispatch exponential backoff")
	if search == nil || len(search.Items) != 1 || search.Items[0].Handle.Path != target {
		t.Fatalf("promoted release search = %+v", search)
	}
	if published := e.search(t, "main", "parcel dispatch exponential backoff"); len(published.Items) != 0 {
		t.Fatalf("unreviewed release leaked into main search: %+v", published)
	}
}

func TestPromoteCreateSectionUpdatesExistingPageAndConsumesProposal(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	target := checkoutPath
	publishExecutable(t, e.m, target)
	proposal := e.propose(t, map[string]any{
		"title": "Checkout retry policy", "suggested_action": "create",
		"knowledge": "Checkout retries stop after three failures.", "source_paths": []string{"services/checkout/retry.go"},
	})
	release := e.openRelease(t)

	preview := decodePreview(t, e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "create_section", "proposal_id": proposal.ID, "target_path": target, "heading": "Retry Policy",
	}))
	if preview.PreviewToken == "" || preview.Operation != "create_section" || preview.SectionID != "checkout-api-retry-policy" {
		t.Fatalf("preview = %+v", preview)
	}
	if !strings.Contains(preview.Diff, "+## Retry Policy\n+<!-- @id: checkout-api-retry-policy -->") || !strings.Contains(preview.Diff, "--- a/"+proposal.Path) {
		t.Fatalf("preview diff = %s", preview.Diff)
	}
	if beforeApply := readFile(t, release, target); beforeApply != memoryPage {
		t.Fatalf("preview changed target: %q", beforeApply)
	}

	applied := decodeApplied(t, e.ok(t, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken}))
	if applied.ProposalID != proposal.ID || applied.TargetPath != target || applied.SectionID != "checkout-api-retry-policy" || applied.Commit == "" {
		t.Fatalf("apply = %+v", applied)
	}
	if changed, want := gitOutput(t, release, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"), "M\t"+target+"\nD\t"+proposal.Path; changed != want {
		t.Fatalf("promotion changed %q, want %q", changed, want)
	}
	if treeEntry := gitOutput(t, release, "ls-tree", "HEAD", "--", target); !strings.HasPrefix(treeEntry, "100755 blob ") {
		t.Fatalf("promotion changed target mode: %q", treeEntry)
	}
	wantSuffix := "\n## Retry Policy\n<!-- @id: checkout-api-retry-policy -->\nCheckout retries stop after three failures.\n"
	if page := readFile(t, release, target); page != memoryPage+wantSuffix {
		t.Fatalf("canonical page = %q, want suffix %q", page, wantSuffix)
	}
	if exists(release, proposal.Path) {
		t.Fatal("consumed proposal remains")
	}

	search := e.search(t, "release", "checkout retries failures")
	found := false
	for _, item := range search.Items {
		if item.Handle.Path == target && item.Handle.SectionID == "checkout-api-retry-policy" && strings.Contains(item.Content, "three failures") {
			found = true
		}
	}
	if !found {
		t.Fatalf("promoted section not retrieved: %+v", search)
	}
}

func TestPromoteCreateSectionRejectsExistingHeadingWithoutWriting(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "Duplicate operation", "suggested_action": "create", "knowledge": "Another operation description.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	head := commitHeads(t, release)

	result := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "create_section", "proposal_id": proposal.ID, "target_path": checkoutPath, "heading": "Operacao",
	})
	if !result.IsError || errorCode(t, result) != "invalid_target" {
		t.Fatalf("collision preview = %+v, code %q", result, errorCode(t, result))
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("collision preview changed HEAD from %s to %s", head, current)
	}
	if page := readFile(t, release, checkoutPath); page != memoryPage {
		t.Fatalf("collision preview changed page: %q", page)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("collision preview removed proposal")
	}
}

func TestPromoteReplaceSectionUpdatesCurrentStateAndConsumesProposal(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	target := checkoutPath
	publishExecutable(t, e.m, target)
	proposal := e.propose(t, map[string]any{
		"title": "Update checkout operation", "suggested_action": "update", "target": checkoutPath,
		"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{"services/checkout/config.yaml"},
	})
	release := e.openRelease(t)
	searchBefore := e.search(t, "release", "checkout timeout team-b")
	if searchBefore == nil || len(searchBefore.Items) != 1 {
		t.Fatalf("memory_search before promotion = %+v", searchBefore)
	}
	contentHash := searchBefore.Items[0].Handle.ContentHash
	invalidConsumption := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "replace_section", "proposal_id": proposal.ID, "target_path": target,
		"section_id": "checkout-api-operation", "content_hash": contentHash,
		"replacement": "Timeout is five seconds.\nOwned by team-b.", "reason": "incorrect",
	})
	if !invalidConsumption.IsError || errorCode(t, invalidConsumption) != "invalid_target" {
		t.Fatalf("consumption with discard reason = %+v, code %q", invalidConsumption, errorCode(t, invalidConsumption))
	}

	preview := decodePreview(t, e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "replace_section", "proposal_id": proposal.ID,
		"target_path": target, "section_id": "checkout-api-operation", "content_hash": contentHash,
		"replacement": "- timeout: cinco segundos\n- owner: team-b\n",
	}))
	if preview.PreviewToken == "" || preview.Operation != "replace_section" || preview.SectionID != "checkout-api-operation" {
		t.Fatalf("preview = %+v", preview)
	}
	for _, fragment := range []string{"-- timeout: tres segundos", "+- timeout: cinco segundos", "--- a/" + proposal.Path} {
		if !strings.Contains(preview.Diff, fragment) {
			t.Fatalf("preview diff missing %q:\n%s", fragment, preview.Diff)
		}
	}
	if strings.Contains(preview.Diff, "-- owner: team-b") || strings.Contains(preview.Diff, "+- owner: team-b") {
		t.Fatalf("preview rewrote unchanged fact:\n%s", preview.Diff)
	}
	if beforeApply := readFile(t, release, target); beforeApply != memoryPage {
		t.Fatalf("preview changed target: %q", beforeApply)
	}

	applied := decodeApplied(t, e.ok(t, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken}))
	if applied.ProposalID != proposal.ID || applied.TargetPath != target || applied.SectionID != "checkout-api-operation" || applied.Commit == "" {
		t.Fatalf("apply = %+v", applied)
	}
	if changed, want := gitOutput(t, release, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"), "M\t"+target+"\nD\t"+proposal.Path; changed != want {
		t.Fatalf("promotion changed %q, want %q", changed, want)
	}
	if treeEntry := gitOutput(t, release, "ls-tree", "HEAD", "--", target); !strings.HasPrefix(treeEntry, "100755 blob ") {
		t.Fatalf("promotion changed target mode: %q", treeEntry)
	}
	wantPage := "# checkout-api\n<!-- @id: checkout-api -->\n\n## Operacao\n<!-- @id: checkout-api-operation -->\n- timeout: cinco segundos\n- owner: team-b\n"
	if page := readFile(t, release, target); page != wantPage {
		t.Fatalf("canonical page = %q, want %q", page, wantPage)
	}
	if exists(release, proposal.Path) {
		t.Fatal("consumed proposal remains")
	}

	search := e.search(t, "release", "checkout cinco team-b")
	if search == nil || len(search.Items) != 1 || search.Items[0].Handle.SectionID != "checkout-api-operation" ||
		strings.Contains(search.Items[0].Content, "tres segundos") || !strings.Contains(search.Items[0].Content, "team-b") {
		t.Fatalf("promoted replacement search = %+v", search)
	}
}

func TestPromoteReplaceSectionRejectsNonLeafTargetWithoutWriting(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "Rewrite checkout page", "suggested_action": "update", "target": checkoutPath, "knowledge": "Replacement.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	head := commitHeads(t, release)

	result := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "replace_section", "proposal_id": proposal.ID, "target_path": checkoutPath, "section_id": "checkout-api",
		"content_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "replacement": "Replacement.",
	})
	if !result.IsError || errorCode(t, result) != "invalid_target" {
		t.Fatalf("non-leaf preview = %+v, code %q", result, errorCode(t, result))
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("non-leaf preview changed HEAD from %s to %s", head, current)
	}
	if page := readFile(t, release, checkoutPath); page != memoryPage {
		t.Fatalf("non-leaf preview changed page: %q", page)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("non-leaf preview removed proposal")
	}
}

func TestPromoteReplaceSectionRejectsStaleContentHashWithoutWriting(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "Update checkout timeout", "suggested_action": "update", "target": checkoutPath,
		"knowledge": "The checkout timeout is five seconds.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	head := commitHeads(t, release)

	result := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "replace_section", "proposal_id": proposal.ID, "target_path": checkoutPath, "section_id": "checkout-api-operation",
		"content_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"replacement":  "- timeout: cinco segundos\n- owner: team-b\n",
	})
	if !result.IsError || errorCode(t, result) != "promotion_drift" {
		t.Fatalf("stale preview = %+v, code %q", result, errorCode(t, result))
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("stale preview changed HEAD from %s to %s", head, current)
	}
	if page := readFile(t, release, checkoutPath); page != memoryPage {
		t.Fatalf("stale preview changed page: %q", page)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("stale preview removed proposal")
	}
}

func TestPromoteReplaceSectionRejectsCreateProposal(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "New checkout fact", "suggested_action": "create", "knowledge": "New fact.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	result := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "replace_section", "proposal_id": proposal.ID, "target_path": checkoutPath, "section_id": "checkout-api-operation",
		"content_hash": "sha256:0000000000000000000000000000000000000000000000000000000000000000", "replacement": "New fact.\n",
	})
	if !result.IsError || errorCode(t, result) != "unsupported_action" {
		t.Fatalf("wrong-action preview = %+v, code %q", result, errorCode(t, result))
	}
	if page := readFile(t, release, checkoutPath); page != memoryPage {
		t.Fatalf("wrong-action preview changed page: %q", page)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("wrong-action preview removed proposal")
	}
}

func TestPromoteRemoveSectionDeletesLeafAndConsumesProposal(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: removableMemoryPage})
	main := e.search(t, "", "operation timeout seconds")
	if main == nil || len(main.Items) != 1 {
		t.Fatalf("memory_search = %+v", main)
	}
	handle := main.Items[0].Handle
	proposal := e.propose(t, map[string]any{
		"title": "Remove obsolete checkout timeout", "suggested_action": "remove", "target": handle.Path,
		"knowledge": "The checkout timeout section is obsolete and must be removed.", "source_paths": []string{},
	})
	release := e.openRelease(t)

	preview := decodePreview(t, e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "remove_section", "proposal_id": proposal.ID, "target_path": handle.Path,
		"section_id": handle.SectionID, "content_hash": handle.ContentHash,
	}))
	if preview.PreviewToken == "" || preview.Operation != "remove_section" {
		t.Fatalf("preview = %+v", preview)
	}
	for _, fragment := range []string{"-## Operation", "-<!-- @id: checkout-api-operation -->", "-Timeout is three seconds.", "--- a/" + proposal.Path} {
		if !strings.Contains(preview.Diff, fragment) {
			t.Fatalf("preview diff missing %q:\n%s", fragment, preview.Diff)
		}
	}
	if beforeApply := readFile(t, release, handle.Path); beforeApply != removableMemoryPage {
		t.Fatalf("preview changed target: %q", beforeApply)
	}

	e.ok(t, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken})
	if changed, want := gitOutput(t, release, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"), "M\t"+handle.Path+"\nD\t"+proposal.Path; changed != want {
		t.Fatalf("promotion changed %q, want %q", changed, want)
	}
	wantPage := "# Checkout API\n<!-- @id: checkout-api -->\n\n## Ownership\n<!-- @id: checkout-api-ownership -->\nOwned by team-b.\n"
	if page := readFile(t, release, handle.Path); page != wantPage {
		t.Fatalf("canonical page = %q, want %q", page, wantPage)
	}
	if exists(release, proposal.Path) {
		t.Fatal("consumed proposal remains")
	}
	if removed := e.search(t, "release", "timeout seconds"); removed == nil || len(removed.Items) != 0 {
		t.Fatalf("removed knowledge remains retrievable: %+v", removed)
	}
}

const legacyRetryPage = `# Legacy Retry Policy
<!-- @id: legacy-retry-policy -->

## Overview
<!-- @id: legacy-retry-policy-overview -->
Retries use the retired queue.
`

func TestPromoteRemovePageDeletesEligiblePageAndConsumesProposal(t *testing.T) {
	e := newEnv(t, map[string]string{"memories/domains/legacy-retry-policy.md": legacyRetryPage})
	found := e.search(t, "", "retired queue")
	if found == nil || len(found.Items) != 1 {
		t.Fatalf("memory_search = %+v", found)
	}
	handle := found.Items[0].Handle
	proposal := e.propose(t, map[string]any{
		"title": "Remove retired retry policy", "suggested_action": "remove", "target": handle.Path,
		"knowledge": "The legacy retry policy page is obsolete and must be removed.", "source_paths": []string{},
	})
	release := e.openRelease(t)

	preview := decodePreview(t, e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "remove_page", "proposal_id": proposal.ID, "target_path": handle.Path,
		"section_id": handle.SectionID, "content_hash": handle.ContentHash,
	}))
	if preview.PreviewToken == "" || preview.Operation != "remove_page" {
		t.Fatalf("preview = %+v", preview)
	}
	for _, fragment := range []string{"-# Legacy Retry Policy", "-Retries use the retired queue.", "--- a/" + proposal.Path} {
		if !strings.Contains(preview.Diff, fragment) {
			t.Fatalf("preview diff missing %q:\n%s", fragment, preview.Diff)
		}
	}
	if !exists(release, handle.Path) {
		t.Fatal("preview removed target")
	}

	e.ok(t, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken})
	if changed, want := gitOutput(t, release, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"), "D\t"+handle.Path+"\nD\t"+proposal.Path; changed != want {
		t.Fatalf("promotion changed %q, want %q", changed, want)
	}
	if exists(release, handle.Path) {
		t.Fatal("removed page remains")
	}
	if exists(release, proposal.Path) {
		t.Fatal("consumed proposal remains")
	}
	if removed := e.search(t, "release", "retired queue"); removed == nil || len(removed.Items) != 0 {
		t.Fatalf("removed page remains retrievable: %+v", removed)
	}
}

func TestPromoteRemovePageRejectsIneligiblePageWithoutWriting(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: removableMemoryPage})
	found := e.search(t, "", "operation timeout seconds")
	if found == nil || len(found.Items) != 1 {
		t.Fatalf("memory_search = %+v", found)
	}
	handle := found.Items[0].Handle
	proposal := e.propose(t, map[string]any{
		"title": "Remove checkout page", "suggested_action": "remove", "target": handle.Path,
		"knowledge": "The selected checkout timeout is obsolete.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	head := commitHeads(t, release)

	result := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "remove_page", "proposal_id": proposal.ID, "target_path": handle.Path,
		"section_id": handle.SectionID, "content_hash": handle.ContentHash,
	})
	if !result.IsError || errorCode(t, result) != "invalid_target" {
		t.Fatalf("ineligible remove_page = %+v, code %q", result, errorCode(t, result))
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("rejected remove_page changed HEAD from %s to %s", head, current)
	}
	if page := readFile(t, release, handle.Path); page != removableMemoryPage {
		t.Fatalf("rejected remove_page changed page: %q", page)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("rejected remove_page removed proposal")
	}
}

func TestPromoteRemoveSectionRejectsApplyTimeDriftAndConsumesToken(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: removableMemoryPage})
	found := e.search(t, "", "operation timeout seconds")
	handle := found.Items[0].Handle
	proposal := e.propose(t, map[string]any{
		"title": "Remove checkout timeout", "suggested_action": "remove", "target": handle.Path,
		"knowledge": "The checkout timeout is obsolete.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	token := e.previewToken(t, map[string]any{
		"operation": "remove_section", "proposal_id": proposal.ID, "target_path": handle.Path,
		"section_id": handle.SectionID, "content_hash": handle.ContentHash,
	})
	head := commitHeads(t, release)
	driftedPage := removableMemoryPage + "external drift\n"
	writeFile(t, release, handle.Path, driftedPage)
	apply := func() (*mcp.CallToolResult, string) {
		result := e.call(t, "memory_promote_apply", map[string]any{"preview_token": token})
		return result, errorCode(t, result)
	}
	if result, code := apply(); !result.IsError || code != "promotion_drift" {
		t.Fatalf("drifted remove apply = %+v, code = %q", result, code)
	}
	if result, code := apply(); !result.IsError || code != "invalid_preview" {
		t.Fatalf("reused remove token = %+v, code = %q", result, code)
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("failed remove changed HEAD from %s to %s", head, current)
	}
	if page := readFile(t, release, handle.Path); page != driftedPage {
		t.Fatalf("failed remove overwrote drift: %q", page)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("failed remove consumed proposal")
	}
}

func TestPromoteDiscardProposalRecordsReasonWithoutChangingMemory(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "Add incorrect checkout note", "suggested_action": "create",
		"knowledge": "This knowledge was found to be incorrect.", "source_paths": []string{},
	})
	release := e.openRelease(t)

	preview := decodePreview(t, e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "incorrect",
	}))
	if preview.PreviewToken == "" || preview.Operation != "discard_proposal" || preview.Reason != "incorrect" {
		t.Fatalf("preview = %+v", preview)
	}
	if !strings.Contains(preview.Diff, "--- a/"+proposal.Path) || strings.Contains(preview.Diff, checkoutPath) {
		t.Fatalf("discard diff changed memory:\n%s", preview.Diff)
	}

	applied := decodeApplied(t, e.ok(t, "memory_promote_apply", map[string]any{"preview_token": preview.PreviewToken}))
	if applied.Decision != "discarded" || applied.Reason != "incorrect" || applied.Commit == "" {
		t.Fatalf("discard result = %+v", applied)
	}
	if changed, want := gitOutput(t, release, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"), "D\t"+proposal.Path; changed != want {
		t.Fatalf("discard changed %q, want %q", changed, want)
	}
	if page := readFile(t, release, checkoutPath); page != memoryPage {
		t.Fatalf("discard changed canonical memory: %q", page)
	}
	if exists(release, proposal.Path) {
		t.Fatal("discarded proposal remains")
	}
	message := gitOutput(t, release, "show", "-s", "--format=%B", "HEAD")
	for _, fragment := range []string{"Discard memory proposal", "Git-Push-Memories-Decision: discarded", "Git-Push-Memories-Reason: incorrect"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("discard commit message missing %q:\n%s", fragment, message)
		}
	}
}

func TestPromoteDiscardRejectsUnknownReasonAndPreviewCancellationKeepsProposal(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "Questionable checkout note", "suggested_action": "update", "target": checkoutPath,
		"knowledge": "This note requires a promotion decision.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	head := commitHeads(t, release)

	invalid := e.call(t, "memory_promote_preview", map[string]any{
		"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "maybe_later",
	})
	if !invalid.IsError || errorCode(t, invalid) != "invalid_discard_reason" {
		t.Fatalf("invalid discard preview = %+v, code %q", invalid, errorCode(t, invalid))
	}
	e.ok(t, "memory_promote_preview", map[string]any{"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "duplicate"})
	if current := commitHeads(t, release); current != head {
		t.Fatalf("cancelled preview changed HEAD from %s to %s", head, current)
	}
	if !exists(release, proposal.Path) {
		t.Fatal("cancelled preview removed proposal")
	}
	if page := readFile(t, release, checkoutPath); page != memoryPage {
		t.Fatalf("discard previews changed memory: %q", page)
	}
}

func TestProposalListIsReadOnly(t *testing.T) {
	const proposalID = "0123456789abcdef0123456789abcdef"
	e := newEnv(t, map[string]string{
		"proposals/" + proposalID + "-retry-policy.md": "---\nschema_version: 1\nsuggested_action: \"create\"\nsource_paths: []\n---\n\n# Retry policy\n\nRetries stop after three failures.\n",
	})
	head := commitHeads(t, e.proposalsRoot())

	result := e.ok(t, "memory_proposal_list", map[string]any{})
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
	if current := commitHeads(t, e.proposalsRoot()); current != head {
		t.Fatalf("proposal list moved proposals HEAD from %s to %s", head, current)
	}
	if status := gitOutput(t, e.proposalsRoot(), "status", "--porcelain"); status != "" {
		t.Fatalf("proposal list dirtied the worktree: %q", status)
	}
}

func TestProposalListRejectsSensitiveCommittedProposalBeforeDisclosure(t *testing.T) {
	const proposalID = "0123456789abcdef0123456789abcdef"
	e := newEnv(t, map[string]string{
		"proposals/" + proposalID + "-unsafe-fact.md": "---\nschema_version: 1\nsuggested_action: \"create\"\nsource_paths: []\n---\n\n# Unsafe fact\n\npassword=super-secret\n",
	})
	result := e.call(t, "memory_proposal_list", map[string]any{})
	var decoded struct {
		Data  any `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeStructured(t, result, &decoded)
	if !result.IsError || decoded.Data != nil || decoded.Error == nil || decoded.Error.Code != "privacy_sensitive_content" {
		t.Fatalf("sensitive proposal list = %+v, response = %+v", result, decoded)
	}
}

func TestPromotePreviewSeparatesCanonicalAndRelatedProposalEvidence(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	related := e.propose(t, map[string]any{
		"title": "Alternative checkout timeout", "suggested_action": "update", "target": checkoutPath,
		"knowledge": "Checkout timeout may be five seconds.", "source_paths": []string{},
	})
	e.propose(t, map[string]any{
		"title": "Storage allocation", "suggested_action": "create", "knowledge": "Partitions follow geography.", "source_paths": []string{},
	})
	proposal := e.propose(t, map[string]any{
		"title": "Checkout timeout ownership", "suggested_action": "create",
		"knowledge": "Checkout timeout is maintained by team-b.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	head := commitHeads(t, release)
	result := e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/checkout-timeout-ownership.md",
	})
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
	decodeStructured(t, result, &preview)
	if candidates := preview.Data.Evidence.CanonicalCandidates; len(candidates.Items) != 1 || candidates.Items[0].Handle.Path != checkoutPath {
		t.Fatalf("canonical candidates = %+v", candidates)
	}
	proposals := preview.Data.Evidence.RelatedProposals
	if len(proposals.Items) != 1 || proposals.Items[0].ID != related.ID || proposals.Items[0].Title != related.Title || proposals.Items[0].Knowledge != related.Knowledge || proposals.Items[0].Score == 0 || proposals.Truncated {
		t.Fatalf("related proposals = %+v", proposals)
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("evidence preview changed HEAD from %s to %s", head, current)
	}
}

func TestPromoteEvidenceCandidatesComeFromTheRelease(t *testing.T) {
	e := newEnv(t, nil)
	first := e.propose(t, map[string]any{"title": "Ledger settlement window", "suggested_action": "create", "knowledge": "Ledger settlement window closes at midnight.", "source_paths": []string{}})
	second := e.propose(t, map[string]any{"title": "Ledger settlement owner", "suggested_action": "create", "knowledge": "Ledger settlement window is owned by finance.", "source_paths": []string{}})
	e.openRelease(t)
	token := e.previewToken(t, map[string]any{"operation": "create_page", "proposal_id": first.ID, "target_path": "memories/domains/ledger-settlement.md"})
	e.ok(t, "memory_promote_apply", map[string]any{"preview_token": token})

	result := e.ok(t, "memory_promote_preview", map[string]any{"operation": "create_page", "proposal_id": second.ID, "target_path": "memories/domains/ledger-owner.md"})
	var preview struct {
		Data struct {
			Evidence struct {
				CanonicalCandidates retrieval.ContextBundle `json:"canonical_candidates"`
			} `json:"evidence"`
		} `json:"data"`
	}
	decodeStructured(t, result, &preview)
	if candidates := preview.Data.Evidence.CanonicalCandidates; len(candidates.Items) != 1 || candidates.Items[0].Handle.Path != "memories/domains/ledger-settlement.md" {
		t.Fatalf("canonical candidates do not include the page consolidated earlier in the release: %+v", candidates)
	}
	if main := e.search(t, "main", "ledger settlement window"); len(main.Items) != 0 {
		t.Fatalf("release page is visible in main search: %+v", main)
	}
}

func TestPromoteApplyRejectsDriftInReviewedEvidence(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	proposal := e.propose(t, map[string]any{
		"title": "Checkout timeout note", "suggested_action": "create",
		"knowledge": "Checkout timeout documentation belongs to operations.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	result := e.ok(t, "memory_promote_preview", map[string]any{
		"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/checkout-timeout-note.md",
	})
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Evidence     struct {
				CanonicalCandidates retrieval.ContextBundle `json:"canonical_candidates"`
			} `json:"evidence"`
		} `json:"data"`
	}
	decodeStructured(t, result, &preview)
	if len(preview.Data.Evidence.CanonicalCandidates.Items) != 1 {
		t.Fatalf("preview evidence = %+v", preview.Data.Evidence)
	}
	head := commitHeads(t, release)
	writeFile(t, release, checkoutPath, strings.Replace(memoryPage, "tres segundos", "quatro segundos", 1))
	applyResult := e.call(t, "memory_promote_apply", map[string]any{"preview_token": preview.Data.PreviewToken})
	if !applyResult.IsError || errorCode(t, applyResult) != "promotion_drift" {
		t.Fatalf("apply with evidence drift = %+v, code %q", applyResult, errorCode(t, applyResult))
	}
	if current := commitHeads(t, release); current != head {
		t.Fatalf("evidence drift changed HEAD from %s to %s", head, current)
	}
	if exists(release, "memories/domains/checkout-timeout-note.md") {
		t.Fatal("evidence drift created target")
	}
	if !exists(release, proposal.Path) {
		t.Fatal("evidence drift consumed proposal")
	}
}

func TestPromoteApplyRejectsDriftAndConsumesPreviewToken(t *testing.T) {
	e := newEnv(t, nil)
	proposal := e.propose(t, map[string]any{
		"title": "Retry policy", "suggested_action": "create", "knowledge": "Retries stop after three failures.", "source_paths": []string{},
	})
	release := e.openRelease(t)
	token := e.previewToken(t, map[string]any{
		"operation": "create_page", "proposal_id": proposal.ID, "target_path": "memories/domains/retry-policy.md",
	})

	writeFile(t, release, "unrelated.txt", "new commit\n")
	runGit(t, release, "add", "unrelated.txt")
	runGit(t, release, "commit", "--quiet", "-m", "advance HEAD")
	advancedHead := commitHeads(t, release)

	apply := func() (*mcp.CallToolResult, string) {
		result := e.call(t, "memory_promote_apply", map[string]any{"preview_token": token})
		code := errorCode(t, result)
		if code == "" {
			t.Fatalf("apply response = %+v", result)
		}
		return result, code
	}
	if result, code := apply(); !result.IsError || code != "promotion_drift" {
		t.Fatalf("drift apply = %+v, code = %q", result, code)
	}
	if result, code := apply(); !result.IsError || code != "invalid_preview" {
		t.Fatalf("reused token = %+v, code = %q", result, code)
	}
	if head := commitHeads(t, release); head != advancedHead {
		t.Fatalf("failed apply changed HEAD from %s to %s", advancedHead, head)
	}
	if exists(release, "memories/domains/retry-policy.md") {
		t.Fatal("failed apply created memory")
	}
	if !exists(release, proposal.Path) {
		t.Fatal("failed apply removed proposal")
	}
}

func TestPromoteBatchPreviewAndApplyUsesOneCommit(t *testing.T) {
	e := newEnv(t, nil)
	proposalIDs := make([]string, 2)
	for index, title := range []string{"First batch memory", "Second batch memory"} {
		proposalIDs[index] = e.propose(t, map[string]any{
			"title": title, "suggested_action": "create", "knowledge": title + " is current.", "source_paths": []string{"source.go"},
		}).ID
	}
	release := e.openRelease(t)
	parent := commitHeads(t, release)
	previewResult := e.ok(t, "memory_promote_batch_preview", map[string]any{"items": []map[string]any{
		{"operation": "create_page", "proposal_id": proposalIDs[0], "target_path": "memories/domains/first-batch.md"},
		{"operation": "create_page", "proposal_id": proposalIDs[1], "target_path": "memories/domains/second-batch.md"},
	}})
	var preview struct {
		Data struct {
			PreviewToken string `json:"preview_token"`
			Items        []struct {
				ProposalID string `json:"proposal_id"`
			} `json:"items"`
			Diff string `json:"diff"`
		} `json:"data"`
	}
	decodeStructured(t, previewResult, &preview)
	if preview.Data.PreviewToken == "" || len(preview.Data.Items) != 2 || !strings.Contains(preview.Data.Diff, "first-batch.md") || !strings.Contains(preview.Data.Diff, "second-batch.md") {
		t.Fatalf("batch preview = %+v", preview.Data)
	}
	applyResult := e.ok(t, "memory_promote_batch_apply", map[string]any{"preview_token": preview.Data.PreviewToken})
	var applied struct {
		Data struct {
			Commit string `json:"commit"`
			Items  []struct {
				ProposalID string `json:"proposal_id"`
				Commit     string `json:"commit"`
			} `json:"items"`
		} `json:"data"`
	}
	decodeStructured(t, applyResult, &applied)
	if applied.Data.Commit == "" || len(applied.Data.Items) != 2 || applied.Data.Items[0].Commit != applied.Data.Commit || applied.Data.Items[1].Commit != applied.Data.Commit {
		t.Fatalf("batch apply = %+v", applied.Data)
	}
	if got := gitOutput(t, release, "rev-parse", "HEAD^"); got != parent {
		t.Fatalf("batch commit parent = %s, want %s", got, parent)
	}
}

type releaseStatusResponse struct {
	Data struct {
		Open   bool `json:"open"`
		Local  bool `json:"local"`
		Marker *struct {
			ReleaseBranch string   `json:"release_branch"`
			Proposals     []string `json:"proposals"`
		} `json:"marker"`
		Pending      []string `json:"pending"`
		PendingKnown bool     `json:"pending_known"`
	} `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (e *env) releaseTool(t *testing.T, name string) (*mcp.CallToolResult, releaseStatusResponse) {
	t.Helper()
	result := e.call(t, name, map[string]any{})
	var decoded releaseStatusResponse
	decodeStructured(t, result, &decoded)
	return result, decoded
}

func TestReleaseOpenStatusConsolidateAndSubmit(t *testing.T) {
	gh := workspacetest.FakeGH(t)
	e := newEnv(t, nil)

	if result, status := e.releaseTool(t, "memory_promote_status"); result.IsError || status.Data.Open {
		t.Fatalf("status without release = %+v, %+v", result, status)
	}
	if result := e.call(t, "memory_promote_open", map[string]any{}); !result.IsError || errorCode(t, result) != "no_proposals" {
		t.Fatalf("open without proposals = %+v, code %q", result, errorCode(t, result))
	}
	ids := make([]string, 2)
	for index, title := range []string{"Settlement window", "Settlement owner"} {
		ids[index] = e.propose(t, map[string]any{"title": title, "suggested_action": "create", "knowledge": title + " is current.", "source_paths": []string{}}).ID
	}

	result, opened := e.releaseTool(t, "memory_promote_open")
	if result.IsError || !opened.Data.Open || !opened.Data.Local || opened.Data.Marker == nil || !strings.HasPrefix(opened.Data.Marker.ReleaseBranch, "release/") || len(opened.Data.Marker.Proposals) != 2 || len(opened.Data.Pending) != 2 {
		t.Fatalf("memory_promote_open = %+v, %+v", result, opened)
	}
	branch := opened.Data.Marker.ReleaseBranch
	if got := gitOutput(t, e.m.Config.Paths.Release, "rev-parse", "--abbrev-ref", "HEAD"); got != branch {
		t.Fatalf("release worktree on %q, want %q", got, branch)
	}
	if _, resumed := e.releaseTool(t, "memory_promote_open"); !resumed.Data.Open || resumed.Data.Marker == nil || resumed.Data.Marker.ReleaseBranch != branch {
		t.Fatalf("second open did not resume the release: %+v", resumed)
	}
	if result, status := e.releaseTool(t, "memory_promote_status"); result.IsError || !status.Data.Open || !status.Data.PendingKnown || len(status.Data.Pending) != 2 {
		t.Fatalf("status = %+v, %+v", result, status)
	}
	if result := e.call(t, "memory_promote_submit", map[string]any{}); !result.IsError || errorCode(t, result) != "proposals_pending" {
		t.Fatalf("submit with pending proposals = %+v, code %q", result, errorCode(t, result))
	}

	token := e.batchToken(t,
		map[string]any{"operation": "create_page", "proposal_id": ids[0], "target_path": "memories/domains/settlement-window.md"},
		map[string]any{"operation": "create_page", "proposal_id": ids[1], "target_path": "memories/domains/settlement-owner.md"},
	)
	e.ok(t, "memory_promote_batch_apply", map[string]any{"preview_token": token})
	if _, status := e.releaseTool(t, "memory_promote_status"); len(status.Data.Pending) != 0 {
		t.Fatalf("pending after consolidation = %v", status.Data.Pending)
	}
	var left listedProposals
	decodeStructured(t, e.ok(t, "memory_proposal_list", map[string]any{"source": "release"}), &left)
	if len(left.Data.Proposals) != 0 {
		t.Fatalf("release still lists proposals: %+v", left.Data.Proposals)
	}

	type submitted struct {
		Data struct {
			PullRequest string `json:"pull_request"`
			Finalized   bool   `json:"finalized"`
		} `json:"data"`
	}
	var first submitted
	decodeStructured(t, e.ok(t, "memory_promote_submit", map[string]any{}), &first)
	if first.Data.PullRequest != "https://github.com/o/r/pull/1" || first.Data.Finalized {
		t.Fatalf("submit = %+v", first)
	}
	releaseHead := commitHeads(t, e.m.Config.Paths.Release)
	if pushed := gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/"+branch); pushed != releaseHead {
		t.Fatalf("remote %s = %s, want %s", branch, pushed, releaseHead)
	}
	var again submitted
	decodeStructured(t, e.ok(t, "memory_promote_submit", map[string]any{}), &again)
	if again.Data.PullRequest != first.Data.PullRequest || again.Data.Finalized {
		t.Fatalf("second submit did not reuse the open PR: %+v", again)
	}

	// Merge the pull request: main takes the release commit and the forge reports it.
	runGit(t, e.m.Config.Paths.Release, "push", "-q", "origin", "HEAD:main")
	gh.SetPR(t, "MERGED", releaseHead)
	var finalized submitted
	decodeStructured(t, e.ok(t, "memory_promote_submit", map[string]any{}), &finalized)
	if !finalized.Data.Finalized || finalized.Data.PullRequest != "" {
		t.Fatalf("submit after merge = %+v", finalized)
	}
	if _, status := e.releaseTool(t, "memory_promote_status"); status.Data.Open {
		t.Fatalf("release still open after finalization: %+v", status)
	}
	if exists(e.m.Config.Paths.Release, ".git") {
		t.Fatal("release worktree remains after finalization")
	}
	if left := gitOutput(t, e.proposalsRoot(), "ls-tree", "-r", "--name-only", "HEAD", "proposals/"); left != "" {
		t.Fatalf("proposals left after finalization: %q", left)
	}
}

func TestReleaseSourceKeepsHashesAndHandlesConsistentAcrossBatches(t *testing.T) {
	e := newEnv(t, map[string]string{rewriteBaselinePath: rewriteBaselinePage})
	rewrite := e.propose(t, map[string]any{
		"title": "Wuerike é do time da camada de leitura", "suggested_action": "update", "target": rewriteBaselinePath,
		"knowledge": "Wuerike faz parte do time da camada de leitura.", "source_paths": []string{},
	})
	later := e.propose(t, map[string]any{
		"title": "Membros trabalham com recuperação", "suggested_action": "update", "target": rewriteBaselinePath,
		"knowledge": "Os membros do time trabalham com recuperação.", "source_paths": []string{},
	})
	release := e.openRelease(t)

	_, before := e.pageRead(t, "release", rewriteBaselinePath, 65536)
	if before.Data == nil || before.Data.ContentHash == "" || before.Data.Markdown != rewriteBaselinePage {
		t.Fatalf("release page_read = %+v", before)
	}
	e.ok(t, "memory_promote_batch_apply", map[string]any{"preview_token": e.batchToken(t,
		map[string]any{"operation": "rewrite_page", "proposal_id": rewrite.ID, "target_path": rewriteBaselinePath, "content_hash": before.Data.ContentHash, "replacement": rewriteReplacement},
	)})

	_, after := e.pageRead(t, "release", rewriteBaselinePath, 65536)
	if after.Data == nil || after.Data.ContentHash == before.Data.ContentHash || !strings.Contains(after.Data.Markdown, "## Membros") {
		t.Fatalf("release page_read after batch 1 = %+v", after)
	}
	if _, mainPage := e.pageRead(t, "main", rewriteBaselinePath, 65536); mainPage.Data == nil || mainPage.Data.Markdown != rewriteBaselinePage {
		t.Fatalf("main page changed before the release is reviewed: %+v", mainPage)
	}
	search := e.search(t, "release", "membros Wuerike Guilherme")
	var handleSection retrieval.Handle
	for _, item := range search.Items {
		if item.Handle.Path == rewriteBaselinePath && strings.Contains(item.Content, "Wuerike") {
			handleSection = item.Handle
		}
	}
	if handleSection.SectionID != "time-camada-de-leitura-membros" {
		t.Fatalf("release search did not return the rewritten section: %+v", search)
	}

	token := e.previewToken(t, map[string]any{
		"operation": "replace_section", "proposal_id": later.ID, "target_path": rewriteBaselinePath,
		"section_id": handleSection.SectionID, "content_hash": handleSection.ContentHash,
		"replacement": "- Wuerike\n- Guilherme\n- Dandara\n",
	})
	result := e.call(t, "memory_promote_apply", map[string]any{"preview_token": token})
	if result.IsError {
		t.Fatalf("batch 2 apply = %+v, code %q (promotion_drift means release hashes are inconsistent)", result, errorCode(t, result))
	}
	if page := readFile(t, release, rewriteBaselinePath); !strings.Contains(page, "- Dandara\n") {
		t.Fatalf("batch 2 page = %q", page)
	}
	// The page hash from batch 1 is stale for batch 2 and must be rejected.
	if _, third := e.pageRead(t, "release", rewriteBaselinePath, 65536); third.Data == nil || third.Data.ContentHash == after.Data.ContentHash {
		t.Fatalf("release page hash did not change after batch 2: %+v", third)
	}
}

func TestStartupRecoveryFailureOnlyBlocksPromotion(t *testing.T) {
	m := workspacetest.Setup(t, map[string]string{checkoutPath: memoryPage})
	quietRefresh(t, m)
	proposal := m.Remember(t, "Checkout fact")
	m.OpenRelease(t)
	// A transaction marker that is not private makes recovery refuse to touch the release.
	if err := os.WriteFile(filepath.Join(m.Config.StateRoot, "promotion-transaction.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	session, stop := startServer(t, m)
	defer stop()
	e := &env{m: m, session: session, stop: stop}

	if search := e.search(t, "", "checkout timeout team"); search == nil || len(search.Items) != 1 {
		t.Fatalf("search after failed recovery = %+v", search)
	}
	e.propose(t, map[string]any{"title": "Another fact", "suggested_action": "create", "knowledge": "Another fact.", "source_paths": []string{}})
	var listed listedProposals
	decodeStructured(t, e.ok(t, "memory_proposal_list", map[string]any{}), &listed)
	if len(listed.Data.Proposals) != 2 {
		t.Fatalf("proposal list after failed recovery = %+v", listed.Data.Proposals)
	}
	for name, arguments := range map[string]map[string]any{
		"memory_promote_preview":       {"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "duplicate"},
		"memory_promote_batch_preview": {"items": []map[string]any{{"operation": "discard_proposal", "proposal_id": proposal.ID, "reason": "duplicate"}}},
		"memory_proposal_list":         {"source": "release"},
	} {
		if result := e.call(t, name, arguments); !result.IsError || errorCode(t, result) != "promotion_recovery_failed" {
			t.Fatalf("%s after failed recovery = %+v, code %q", name, result, errorCode(t, result))
		}
	}
}

func TestToolListIncludesPageReadAndReleaseTools(t *testing.T) {
	e := newEnv(t, nil)
	tools, err := e.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, tool := range tools.Tools {
		listed[tool.Name] = true
	}
	for _, name := range []string{"memory_page_read", "memory_promote_open", "memory_promote_status", "memory_promote_submit"} {
		if !listed[name] {
			t.Fatalf("%s missing from tool list", name)
		}
	}
}

func TestPageReadToolReturnsWholePage(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	result, response := e.pageRead(t, "", checkoutPath, 4096)
	if result.IsError || response.Data == nil || response.Data.Markdown != memoryPage || response.Data.Path != checkoutPath || !strings.HasPrefix(response.Data.ContentHash, "sha256:") {
		t.Fatalf("memory_page_read result = %+v, response = %+v", result, response)
	}
}

func TestPageReadToolFailsWhenPageExceedsBudget(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	result, response := e.pageRead(t, "", checkoutPath, len(memoryPage))
	if !result.IsError || response.Error == nil || response.Error.Code != "budget_too_small" || response.Data != nil {
		t.Fatalf("oversized result = %+v, response = %+v", result, response)
	}
}

func TestPageReadToolRejectsInvalidRequests(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage, "memories/notes/fact.md": memoryPage})
	for _, test := range []struct {
		path   string
		budget int
		code   string
	}{
		{"memories/notes/fact.md", 4096, "invalid_path"},
		{"memories/domains/missing.md", 4096, "invalid_path"},
		{checkoutPath, 0, "invalid_budget"},
	} {
		result, response := e.pageRead(t, "", test.path, test.budget)
		if !result.IsError || response.Error == nil || response.Error.Code != test.code || response.Data != nil {
			t.Fatalf("%+v result = %+v, response = %+v", test, result, response)
		}
	}
}
