package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/promotion"
	"github.com/Wuerike/git-push-memories/internal/release"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	privateSourceMarker = "PRIVATE-SOURCE-E2E-7f9c1a"
	rejectedSecret      = "ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	initialCheckoutFact = "Checkout requests time out after three seconds. Ownership remains team-blue."
	updatedCheckoutFact = "Checkout requests time out after nine seconds. Ownership remains team-blue."
	legacyCleanupFact   = "Legacy cleanup runs every hour."
)

type toolEnvelope[T any] struct {
	SchemaVersion int        `json:"schema_version"`
	Data          *T         `json:"data,omitempty"`
	Error         *toolError `json:"error,omitempty"`
}

type toolError struct {
	Code string `json:"code"`
}

type previewEvidence struct {
	CanonicalCandidates retrieval.ContextBundle    `json:"canonical_candidates"`
	RelatedProposals    promotion.RelatedProposals `json:"related_proposals"`
}

type batchPreviewItem struct {
	ProposalID string          `json:"proposal_id"`
	Content    string          `json:"content"`
	Evidence   previewEvidence `json:"evidence"`
}

type batchPreview struct {
	PreviewToken string             `json:"preview_token"`
	Items        []batchPreviewItem `json:"items"`
	Diff         string             `json:"diff"`
}

type singlePreview struct {
	PreviewToken string `json:"preview_token"`
}

type e2eEvidence struct {
	SchemaVersion       int      `json:"schema_version"`
	ProductRevision     string   `json:"product_revision"`
	GoVersion           string   `json:"go_version"`
	ReleaseBranch       string   `json:"release_branch"`
	CreateCommit        string   `json:"create_commit"`
	RecoveredCommit     string   `json:"recovered_commit"`
	PullRequest         string   `json:"pull_request"`
	CanonicalOperations []string `json:"canonical_operations"`
	SetupInitialized    bool     `json:"setup_initialized_empty_remote"`
	ProposalsPublished  bool     `json:"proposals_published"`
	ContradictionShown  bool     `json:"contradiction_shown"`
	MarkerContentFree   bool     `json:"marker_content_free"`
	RecoveryConverged   bool     `json:"recovery_converged"`
	BaselineRecovered   bool     `json:"baseline_recovered"`
	PullRequestMerged   bool     `json:"pull_request_merged_finalized"`
	FinalFactRecovered  bool     `json:"final_fact_recovered"`
	SecondUserReadOnly  bool     `json:"second_user_read_only"`
}

const githubURL = "git@github.com:o/r.git"

// proposalFiles lists the proposal Markdown files on a ref of the remote.
func proposalFiles(t *testing.T, ctx context.Context, git string, remote string, environment []string, ref string) []string {
	t.Helper()
	var files []string
	for _, name := range strings.Fields(gitOutput(t, ctx, git, remote, environment, "ls-tree", "-r", "--name-only", ref)) {
		if strings.HasPrefix(name, "proposals/") && strings.HasSuffix(name, ".md") {
			files = append(files, name)
		}
	}
	return files
}

func TestManagedWorkflowEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	realGit, err = filepath.EvalSymlinks(realGit)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "gpm")
	runCommand(t, ctx, repositoryRoot, os.Environ(), "go", "build", "-o", binary, "./cmd/gpm")

	// The test process gets an isolated HOME, state root and Git configuration; every gpm
	// invocation inherits them plus the fake claude, fake gh and a Git shim in front of PATH.
	wt.Env(t)
	gh := wt.FakeGH(t)
	runRoot := t.TempDir()
	remote := wt.Remote(t)
	t.Setenv("GPM_FAKE_REMOTE", remote)
	sourceRoot := filepath.Join(runRoot, "source")
	seedRepository(t, realGit, sourceRoot, map[string]string{
		"services/checkout/config.yaml": "timeout: 3s\nmarker: " + privateSourceMarker + "\n",
	})
	toolsRoot := filepath.Join(runRoot, "tools")
	if err := os.Mkdir(toolsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(toolsRoot, "claude"), "#!/bin/sh\nexit 0\n")
	crashArm := filepath.Join(runRoot, "crash-arm")
	remoteAttemptLog := filepath.Join(runRoot, "remote-attempts")
	writeGitShim(t, filepath.Join(toolsRoot, "git"))
	environment := gpmEnvironment(t, runRoot, toolsRoot, realGit, crashArm, remoteAttemptLog)
	mapURLToRemote(t, environmentValue(environment, "HOME"), remote)

	// Setup against an empty repository initializes main, proposals and the worktrees.
	setupOutput := commandOutput(t, ctx, repositoryRoot, environment, binary, "setup", githubURL, "--yes", "--categories", "domains,decision")
	if !strings.Contains(setupOutput, "Use /remember <fact> and /promote") {
		t.Fatalf("setup output = %q", setupOutput)
	}
	paths, err := workspace.PathsFor(githubURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gitOutput(t, ctx, realGit, remote, environment, "show", "main:gpm.json"), "\"domains\"") ||
		gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "--verify", "proposals") == "" {
		t.Fatal("setup did not initialize main and proposals on the remote")
	}
	for _, worktree := range []string{paths.Read, paths.Proposals} {
		if missing(filepath.Join(worktree, ".git")) {
			t.Fatalf("setup did not create worktree %s", worktree)
		}
	}
	if !missing(paths.Release) {
		t.Fatal("setup created a release worktree")
	}

	// /remember: proposals are committed on proposals and pushed.
	sessionA := connectMCP(t, ctx, binary, environment, "e2e-a")

	createA := propose(t, ctx, sessionA, map[string]any{"title": "Checkout policy", "content": "Create: " + initialCheckoutFact})
	createB := propose(t, ctx, sessionA, map[string]any{"title": "Legacy cleanup", "content": "Create: " + legacyCleanupFact})
	createSection := propose(t, ctx, sessionA, map[string]any{
		"title": "Temporary checkout detail", "content": "Create in memories/domains/checkout-policy.md: Temporary migration detail remains active.",
	})
	removeSection := propose(t, ctx, sessionA, map[string]any{
		"title": "Remove temporary checkout detail", "content": "Remove from memories/domains/checkout-policy.md: Temporary migration detail is retired.",
	})
	updateA := propose(t, ctx, sessionA, map[string]any{
		"title": "Checkout timeout changed", "content": "Update memories/domains/checkout-policy.md: " + updatedCheckoutFact,
	})
	removeB := propose(t, ctx, sessionA, map[string]any{
		"title": "Retire legacy cleanup", "content": "Remove memories/domains/legacy-cleanup.md: Legacy cleanup is retired.",
	})
	discard := propose(t, ctx, sessionA, map[string]any{"title": "Duplicate checkout timeout", "content": "Create: Checkout requests time out after three seconds."})
	proposalIDs := []string{createA.ID, createB.ID, createSection.ID, removeSection.ID, updateA.ID, removeB.ID, discard.ID}
	if gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "proposals") != discard.Commit {
		t.Fatal("remembered proposals were not committed to the remote proposals branch")
	}
	if got := len(proposalFiles(t, ctx, realGit, remote, environment, "proposals")); got != len(proposalIDs) {
		t.Fatalf("remote proposals branch holds %d proposals, want %d", got, len(proposalIDs))
	}
	mainAfterRemember := gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "main")
	if strings.Count(gitOutput(t, ctx, realGit, remote, environment, "log", "--format=%H", "main"), "\n") != 0 {
		t.Fatal("remembering changed main")
	}

	// /promote: open a release (locks promotion, cuts the release branch), then consolidate on it.
	statusBefore := decodeTool[release.Status](t, callTool(t, ctx, sessionA, "memory_promote_status", map[string]any{}))
	if statusBefore.Data == nil || statusBefore.Data.Open {
		t.Fatalf("status before opening = %+v", statusBefore.Data)
	}
	opened := requireData(t, decodeTool[release.Status](t, callTool(t, ctx, sessionA, "memory_promote_open", map[string]any{})))
	if !opened.Open || opened.Marker == nil || len(opened.Pending) != len(proposalIDs) || !strings.HasPrefix(opened.Marker.ReleaseBranch, workspace.ReleasePrefix) {
		t.Fatalf("memory_promote_open = %+v", opened)
	}
	releaseBranch := opened.Marker.ReleaseBranch
	releaseRoot := paths.Release
	if gitOutput(t, ctx, realGit, releaseRoot, environment, "symbolic-ref", "--short", "HEAD") != releaseBranch {
		t.Fatal("release worktree is not on the release branch")
	}
	secondOpen, err := sessionA.CallTool(ctx, &mcp.CallToolParams{Name: "memory_promote_open", Arguments: map[string]any{}})
	if err != nil || secondOpen.IsError {
		t.Fatalf("opening again on the same machine must resume: %+v, %v", secondOpen, err)
	}
	refused, err := sessionA.CallTool(ctx, &mcp.CallToolParams{Name: "memory_promote_submit", Arguments: map[string]any{}})
	if err != nil || !refused.IsError || decodeTool[json.RawMessage](t, refused).Error.Code != "proposals_pending" {
		t.Fatal("submit with pending proposals was not refused")
	}

	createPreviewResult := callTool(t, ctx, sessionA, "memory_promote_batch_preview", map[string]any{"items": []map[string]any{
		{"operation": "create_page", "proposal_id": createA.ID, "target_path": "memories/domains/checkout-policy.md"},
		{"operation": "create_page", "proposal_id": createB.ID, "target_path": "memories/domains/legacy-cleanup.md"},
	}})
	createPreview := requireData(t, decodeTool[batchPreview](t, createPreviewResult))
	if len(createPreview.Items) != 2 {
		t.Fatal("create batch preview omitted decisions")
	}
	createApplyResult := callTool(t, ctx, sessionA, "memory_promote_batch_apply", map[string]any{"preview_token": createPreview.PreviewToken})
	createResult := requireData(t, decodeTool[promotion.BatchResult](t, createApplyResult))
	assertCommitPaths(t, ctx, realGit, releaseRoot, environment, createResult.Commit, []string{
		createA.Path, createB.Path, "memories/domains/checkout-policy.md", "memories/domains/legacy-cleanup.md",
	})
	if gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "HEAD") != createResult.Commit {
		t.Fatal("create batch did not advance the release branch")
	}
	// Published memory and the shared inbox stay untouched while the release is consolidated.
	if gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "main") != mainAfterRemember || !missing(filepath.Join(paths.Read, "memories", "domains", "checkout-policy.md")) {
		t.Fatal("unreviewed memory reached main or the read worktree")
	}
	if err := sessionA.Close(); err != nil {
		t.Fatal(err)
	}

	// A hook that would fail every commit proves promotion publishes through plumbing.
	hookSentinel := filepath.Join(runRoot, "hook-ran")
	writeExecutable(t, filepath.Join(paths.Repo, "hooks", "commit-msg"), "#!/bin/sh\ntouch \"$GPM_E2E_HOOK_SENTINEL\"\nexit 1\n")
	environment = append(environment, "GPM_E2E_HOOK_SENTINEL="+hookSentinel)

	sectionSession := connectMCP(t, ctx, binary, environment, "e2e-sections")
	checkoutOverview := searchOne(t, ctx, sectionSession, "checkout three team-blue", "release")
	createSectionPreviewResult := callTool(t, ctx, sectionSession, "memory_promote_preview", map[string]any{
		"operation": "create_section", "proposal_id": createSection.ID, "target_path": checkoutOverview.Path, "heading": "Temporary detail",
	})
	createSectionPreview := requireData(t, decodeTool[singlePreview](t, createSectionPreviewResult))
	createSectionApplyResult := callTool(t, ctx, sectionSession, "memory_promote_apply", map[string]any{"preview_token": createSectionPreview.PreviewToken})
	createSectionResult := requireData(t, decodeTool[promotion.Result](t, createSectionApplyResult))
	assertCommitPaths(t, ctx, realGit, releaseRoot, environment, createSectionResult.Commit, []string{checkoutOverview.Path, createSection.Path})
	sectionBaselineCommit := gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "HEAD")
	temporaryHandle := searchOne(t, ctx, sectionSession, "temporary migration detail", "release")
	removeSectionPreviewResult := callTool(t, ctx, sectionSession, "memory_promote_preview", map[string]any{
		"operation": "remove_section", "proposal_id": removeSection.ID, "target_path": temporaryHandle.Path,
		"section_id": temporaryHandle.SectionID, "content_hash": temporaryHandle.ContentHash,
	})
	removeSectionPreview := requireData(t, decodeTool[singlePreview](t, removeSectionPreviewResult))
	if err := os.WriteFile(crashArm, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, beforeRefErr := sectionSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "memory_promote_apply", Arguments: map[string]any{"preview_token": removeSectionPreview.PreviewToken},
	})
	if beforeRefErr == nil {
		t.Fatal("pre-update-ref interruption returned a normal MCP response")
	}
	_ = sectionSession.Close()
	if gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "HEAD") != sectionBaselineCommit {
		t.Fatal("pre-update-ref interruption advanced the release branch")
	}
	markerPath := filepath.Join(paths.State, "promotion-transaction.json")
	if missing(markerPath) {
		t.Fatal("pre-update-ref interruption omitted its recovery marker")
	}
	if err := os.Remove(crashArm); err != nil {
		t.Fatal(err)
	}
	baselineRecoverySession := connectMCP(t, ctx, binary, environment, "e2e-baseline-recovery")
	gitDirectory := gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "--absolute-git-dir")
	indexPath := gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "--path-format=absolute", "--git-path", "index")
	baselineArtifacts, _ := filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-promotion-index-*"))
	if !missing(markerPath) || !missing(indexPath+".lock") || len(baselineArtifacts) != 0 || missing(filepath.Join(releaseRoot, filepath.FromSlash(removeSection.Path))) {
		t.Fatal("baseline recovery did not restore proposals or clean controlled artifacts")
	}
	restoredTemporary := searchOne(t, ctx, baselineRecoverySession, "temporary migration detail", "release")
	retryRemovePreviewResult := callTool(t, ctx, baselineRecoverySession, "memory_promote_preview", map[string]any{
		"operation": "remove_section", "proposal_id": removeSection.ID, "target_path": restoredTemporary.Path,
		"section_id": restoredTemporary.SectionID, "content_hash": restoredTemporary.ContentHash,
	})
	retryRemovePreview := requireData(t, decodeTool[singlePreview](t, retryRemovePreviewResult))
	retryRemoveResult := callTool(t, ctx, baselineRecoverySession, "memory_promote_apply", map[string]any{"preview_token": retryRemovePreview.PreviewToken})
	removeSectionResult := requireData(t, decodeTool[promotion.Result](t, retryRemoveResult))
	assertCommitPaths(t, ctx, realGit, releaseRoot, environment, removeSectionResult.Commit, []string{restoredTemporary.Path, removeSection.Path})
	if err := baselineRecoverySession.Close(); err != nil {
		t.Fatal(err)
	}
	decisionBaselineCommit := gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "HEAD")

	decisionSession := connectMCP(t, ctx, binary, environment, "e2e-decisions")
	checkoutHandle := searchOne(t, ctx, decisionSession, "checkout three team-blue", "release")
	cleanupHandle := searchOne(t, ctx, decisionSession, "legacy cleanup hour", "release")
	decisionPreviewResult := callTool(t, ctx, decisionSession, "memory_promote_batch_preview", map[string]any{"items": []map[string]any{
		{"operation": "replace_section", "proposal_id": updateA.ID, "target_path": checkoutHandle.Path, "section_id": checkoutHandle.SectionID, "content_hash": checkoutHandle.ContentHash, "replacement": updatedCheckoutFact},
		{"operation": "remove_page", "proposal_id": removeB.ID, "target_path": cleanupHandle.Path, "section_id": cleanupHandle.SectionID, "content_hash": cleanupHandle.ContentHash},
		{"operation": "discard_proposal", "proposal_id": discard.ID, "reason": "duplicate"},
	}})
	decisionPreview := requireData(t, decodeTool[batchPreview](t, decisionPreviewResult))
	if len(decisionPreview.Items) != 3 {
		t.Fatal("decision batch preview omitted decisions")
	}
	if !bundleContains(decisionPreview.Items[0].Evidence.CanonicalCandidates, "three seconds") || !relatedContains(decisionPreview.Items[0].Evidence.RelatedProposals, discard.ID) || !strings.Contains(decisionPreview.Items[0].Content, "nine seconds") {
		t.Fatal("contradictory current and proposed facts were not observable together")
	}
	for _, expectedPath := range []string{checkoutHandle.Path, cleanupHandle.Path, updateA.Path, removeB.Path, discard.Path} {
		if !strings.Contains(decisionPreview.Diff, expectedPath) {
			t.Fatalf("decision diff omitted %s", expectedPath)
		}
	}
	if err := os.WriteFile(crashArm, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, applyErr := decisionSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "memory_promote_batch_apply", Arguments: map[string]any{"preview_token": decisionPreview.PreviewToken},
	})
	if applyErr == nil {
		t.Fatal("interrupted apply returned a normal MCP response")
	}
	_ = decisionSession.Close()
	interruptedCommit := gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "HEAD")
	if interruptedCommit == decisionBaselineCommit {
		t.Fatal("crash occurred before update-ref")
	}
	assertCommitPaths(t, ctx, realGit, releaseRoot, environment, interruptedCommit, []string{
		checkoutHandle.Path, cleanupHandle.Path, updateA.Path, removeB.Path, discard.Path,
	})
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal("interrupted transaction did not retain its marker")
	}
	markerInfo, err := os.Stat(markerPath)
	if err != nil || markerInfo.Mode().Perm() != 0o600 {
		t.Fatal("transaction marker is not private")
	}
	for _, forbidden := range []string{privateSourceMarker, rejectedSecret, initialCheckoutFact, updatedCheckoutFact, legacyCleanupFact, sourceRoot, decisionPreview.PreviewToken} {
		if bytes.Contains(marker, []byte(forbidden)) {
			t.Fatal("transaction marker retained forbidden content")
		}
	}
	artifacts, err := filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-promotion-index-*"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("interrupted index artifacts = %v, %v", artifacts, err)
	}
	artifactInfo, artifactErr := os.Stat(artifacts[0])
	lockInfo, lockErr := os.Stat(indexPath + ".lock")
	if artifactErr != nil || lockErr != nil || !os.SameFile(artifactInfo, lockInfo) {
		t.Fatal("index lock ownership is not tied to the transaction artifact")
	}
	if err := os.Remove(crashArm); err != nil {
		t.Fatal(err)
	}

	// The next session recovers the interrupted batch, finds nothing pending and sends the release for review.
	recoverySession := connectMCP(t, ctx, binary, environment, "e2e-recovery")
	listResult := callTool(t, ctx, recoverySession, "memory_proposal_list", map[string]any{"source": "release"})
	listed := requireData(t, decodeTool[promotion.List](t, listResult))
	if len(listed.Proposals) != 0 {
		t.Fatalf("recovery retained %d proposals", len(listed.Proposals))
	}
	if gitOutput(t, ctx, realGit, releaseRoot, environment, "rev-parse", "HEAD") != interruptedCommit {
		t.Fatal("startup recovery created a duplicate commit")
	}
	if !missing(markerPath) || !missing(indexPath+".lock") {
		t.Fatal("recovery retained transaction marker or index lock")
	}
	artifacts, _ = filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-promotion-index-*"))
	if len(artifacts) != 0 || !missing(filepath.Join(releaseRoot, "memories", "domains", "legacy-cleanup.md")) {
		t.Fatal("recovery retained controlled transaction artifacts or removed memory")
	}
	if _, err := os.Stat(hookSentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Git commit hook ran during plumbing publication")
	}
	message := gitOutput(t, ctx, realGit, releaseRoot, environment, "show", "-s", "--format=%B", interruptedCommit)
	if strings.Count(message, "Git-Push-Memories-Decision: consumed") != 2 || strings.Count(message, "Git-Push-Memories-Decision: discarded") != 1 || !strings.Contains(message, "Git-Push-Memories-Reason: duplicate") {
		t.Fatal("recovered commit omitted decision trailers")
	}
	if err := os.Remove(filepath.Join(paths.Repo, "hooks", "commit-msg")); err != nil {
		t.Fatal(err)
	}
	readyStatus := requireData(t, decodeTool[release.Status](t, callTool(t, ctx, recoverySession, "memory_promote_status", map[string]any{})))
	if !readyStatus.Open || len(readyStatus.Pending) != 0 || !readyStatus.PendingKnown {
		t.Fatalf("status after consolidation = %+v", readyStatus)
	}
	type submitted struct {
		PullRequest string `json:"pull_request"`
		Finalized   bool   `json:"finalized"`
	}
	submit := requireData(t, decodeTool[submitted](t, callTool(t, ctx, recoverySession, "memory_promote_submit", map[string]any{})))
	if submit.PullRequest == "" || submit.Finalized {
		t.Fatalf("memory_promote_submit = %+v", submit)
	}
	again := requireData(t, decodeTool[submitted](t, callTool(t, ctx, recoverySession, "memory_promote_submit", map[string]any{})))
	if again.PullRequest != submit.PullRequest {
		t.Fatalf("second submit opened another pull request: %q vs %q", again.PullRequest, submit.PullRequest)
	}
	if err := recoverySession.Close(); err != nil {
		t.Fatal(err)
	}
	if gitOutput(t, ctx, realGit, remote, environment, "rev-parse", releaseBranch) != interruptedCommit {
		t.Fatal("the release branch on the remote is not the consolidated release")
	}
	if gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "main") != mainAfterRemember {
		t.Fatal("sending the release for review moved main")
	}
	if got := len(proposalFiles(t, ctx, realGit, remote, environment, "proposals")); got != len(proposalIDs) {
		t.Fatalf("proposals were consumed before the merge: %d left", got)
	}
	// Until the merge, readers still see the old main: no memory at all.
	beforeMerge := commandOutput(t, ctx, repositoryRoot, environment, binary, "retrieve", "checkout nine team-blue", "--budget-bytes", "4096")
	if strings.Contains(beforeMerge, "nine seconds") {
		t.Fatal("unmerged release is visible to readers")
	}

	// `gpm promote --finish` waits for the merge; then the PR is squash-merged on the remote.
	gh.SetPR(t, "OPEN", "")
	finishOpen := exec.CommandContext(ctx, binary, "promote", "--finish", "--yes")
	finishOpen.Env = environment
	finishOpenOutput, finishOpenErr := finishOpen.CombinedOutput()
	if finishOpenErr == nil || !strings.Contains(string(finishOpenOutput), "not merged yet") {
		t.Fatalf("finish with an open pull request = %v: %s", finishOpenErr, finishOpenOutput)
	}
	mergeOID := squashMerge(t, ctx, realGit, remote, environment, releaseBranch)
	gh.SetPR(t, "MERGED", mergeOID)
	finishOutput := commandOutput(t, ctx, repositoryRoot, environment, binary, "promote", "--finish", "--yes")
	if !strings.Contains(finishOutput, "Release finalized.") {
		t.Fatalf("finish output = %q", finishOutput)
	}
	if got := proposalFiles(t, ctx, realGit, remote, environment, "proposals"); len(got) != 0 {
		t.Fatalf("finalizing left consumed proposals: %v", got)
	}
	if strings.Contains(gitOutput(t, ctx, realGit, remote, environment, "ls-tree", "-r", "--name-only", "proposals"), release.MarkerFile) {
		t.Fatal("finalizing left the release marker")
	}
	if branches := gitOutput(t, ctx, realGit, remote, environment, "branch", "--list", workspace.ReleasePrefix+"*"); branches != "" {
		t.Fatalf("finalizing left release branches on the remote: %q", branches)
	}
	if !missing(releaseRoot) {
		t.Fatal("finalizing left the release worktree")
	}
	if !strings.Contains(gitOutput(t, ctx, realGit, remote, environment, "show", "main:"+checkoutHandle.Path), "nine seconds") {
		t.Fatal("merged main does not hold the consolidated memory")
	}
	for _, value := range []string{privateSourceMarker, rejectedSecret, sourceRoot} {
		if gitOutput(t, ctx, realGit, remote, environment, "log", "--all", "--oneline", "-S"+value) != "" {
			t.Fatal("shared repository history retained forbidden source content")
		}
	}
	for _, root := range []string{paths.State} {
		if scanForValues(t, root, []string{privateSourceMarker, rejectedSecret, sourceRoot, decisionPreview.PreviewToken}) != 0 {
			t.Fatal("private state retained forbidden source content or bindings")
		}
	}
	if !missing(remoteAttemptLog) || containsTranscriptArtifact(t, paths.State) {
		t.Fatal("private state retained forbidden source/session content or attempted network access")
	}

	// The same machine now reads the merged memory.
	merged := commandOutput(t, ctx, repositoryRoot, environment, binary, "retrieve", "checkout nine team-blue", "--budget-bytes", "4096")
	if !strings.Contains(merged, "nine seconds") {
		t.Fatal("the read worktree did not move to the merged main")
	}

	// A second person sets up against the same repository and only reads.
	freshRoot := filepath.Join(runRoot, "fresh-user")
	freshRemoteAttemptLog := filepath.Join(freshRoot, "remote-attempts")
	freshEnvironment := gpmEnvironment(t, freshRoot, toolsRoot, realGit, filepath.Join(freshRoot, "unused-arm"), freshRemoteAttemptLog)
	mapURLToRemote(t, environmentValue(freshEnvironment, "HOME"), remote)
	commandOutput(t, ctx, repositoryRoot, freshEnvironment, binary, "setup", githubURL, "--yes")
	freshState := filepath.Join(environmentValue(freshEnvironment, "XDG_STATE_HOME"), "git-push-memories", workspace.RepoID(githubURL))
	remoteRefsBefore := gitOutput(t, ctx, realGit, remote, freshEnvironment, "for-each-ref")
	freshProposalsBefore := gitOutput(t, ctx, realGit, filepath.Join(freshState, "proposals"), freshEnvironment, "rev-parse", "HEAD")
	finalSession := connectMCP(t, ctx, binary, freshEnvironment, "e2e-final")
	finalHandle := searchOne(t, ctx, finalSession, "checkout nine team-blue", "")
	finalResult := callTool(t, ctx, finalSession, "memory_expand", map[string]any{"handle": finalHandle, "budget_bytes": 4096})
	finalBundle := requireData(t, decodeTool[retrieval.ContextBundle](t, finalResult))
	if !bundleContains(*finalBundle, "nine seconds") || bundleContains(*finalBundle, "three seconds") {
		t.Fatal("fresh process did not recover only the updated current fact")
	}
	removedResult := callTool(t, ctx, finalSession, "memory_search", map[string]any{"query": "legacy cleanup hour", "budget_bytes": 2048})
	removedBundle := requireData(t, decodeTool[retrieval.ContextBundle](t, removedResult))
	if len(removedBundle.Items) != 0 {
		t.Fatal("fresh retrieval returned removed memory or proposals")
	}
	if err := finalSession.Close(); err != nil {
		t.Fatal(err)
	}
	if gitOutput(t, ctx, realGit, remote, freshEnvironment, "for-each-ref") != remoteRefsBefore ||
		gitOutput(t, ctx, realGit, filepath.Join(freshState, "proposals"), freshEnvironment, "rev-parse", "HEAD") != freshProposalsBefore ||
		!missing(filepath.Join(freshState, "release")) || !missing(freshRemoteAttemptLog) {
		t.Fatal("read-only fresh process wrote to the repository, created a release or attempted network access")
	}

	writeEvidence(t, e2eEvidence{
		SchemaVersion: 1, ProductRevision: gitOutput(t, ctx, realGit, repositoryRoot, os.Environ(), "rev-parse", "HEAD"), GoVersion: runtime.Version(),
		ReleaseBranch: releaseBranch, CreateCommit: createResult.Commit, RecoveredCommit: interruptedCommit, PullRequest: submit.PullRequest,
		CanonicalOperations: []string{"create_page", "create_section", "replace_section", "remove_section", "remove_page", "discard_proposal"},
		SetupInitialized:    true, ProposalsPublished: true, ContradictionShown: true, MarkerContentFree: true,
		RecoveryConverged: true, BaselineRecovered: true, PullRequestMerged: true, FinalFactRecovered: true, SecondUserReadOnly: true,
	})
}

// squashMerge merges branch into the remote main as a hosting service would and returns the merge commit.
func squashMerge(t *testing.T, ctx context.Context, git, remote string, environment []string, branch string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "merger")
	runCommand(t, ctx, filepath.Dir(clone), environment, git, "clone", "--quiet", remote, clone)
	runCommand(t, ctx, clone, environment, git, "merge", "--squash", "origin/"+branch)
	runCommand(t, ctx, clone, environment, git, "commit", "--quiet", "-m", "memory: promote proposals (#1)")
	runCommand(t, ctx, clone, environment, git, "push", "--quiet", "origin", "main")
	return gitOutput(t, ctx, git, clone, environment, "rev-parse", "HEAD")
}

func commandOutput(t *testing.T, ctx context.Context, directory string, environment []string, executable string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("%s %v: %v: %s%s", filepath.Base(executable), arguments, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// mapURLToRemote makes Git resolve the GitHub-looking URL to the local bare repository.
func mapURLToRemote(t *testing.T, home, remote string) {
	t.Helper()
	configFile := filepath.Join(home, ".gitconfig")
	existing, _ := os.ReadFile(configFile)
	contents := string(existing) + "[user]\n\tname = E2E\n\temail = e2e@example.invalid\n[init]\n\tdefaultBranch = main\n[url \"" + remote + "\"]\n\tinsteadOf = " + githubURL + "\n"
	if err := os.WriteFile(configFile, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func connectMCP(t *testing.T, ctx context.Context, binary string, environment []string, name string) *mcp.ClientSession {
	t.Helper()
	command := exec.CommandContext(ctx, binary, "mcp")
	command.Env = environment
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect %s: %v: %s", name, err, stderr.String())
	}
	return session
}

func propose(t *testing.T, ctx context.Context, session *mcp.ClientSession, arguments map[string]any) inbox.Proposal {
	t.Helper()
	result := callTool(t, ctx, session, "memory_propose", arguments)
	return *requireData(t, decodeTool[inbox.Proposal](t, result))
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		response := decodeTool[json.RawMessage](t, result)
		if response.Error != nil {
			t.Fatalf("%s returned %s", name, response.Error.Code)
		}
		t.Fatalf("%s returned a tool error", name)
	}
	return result
}

func decodeTool[T any](t *testing.T, result *mcp.CallToolResult) toolEnvelope[T] {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var response toolEnvelope[T]
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func requireData[T any](t *testing.T, response toolEnvelope[T]) *T {
	t.Helper()
	if response.SchemaVersion != 1 || response.Data == nil || response.Error != nil {
		t.Fatalf("invalid tool response schema: version=%d error=%+v", response.SchemaVersion, response.Error)
	}
	return response.Data
}

func searchOne(t *testing.T, ctx context.Context, session *mcp.ClientSession, query, source string) retrieval.Handle {
	t.Helper()
	arguments := map[string]any{"query": query, "budget_bytes": 4096}
	if source != "" {
		arguments["source"] = source
	}
	result := callTool(t, ctx, session, "memory_search", arguments)
	bundle := requireData(t, decodeTool[retrieval.ContextBundle](t, result))
	if len(bundle.Items) != 1 {
		t.Fatalf("memory_search(%q) returned %d items", query, len(bundle.Items))
	}
	return bundle.Items[0].Handle
}

func assertCommitPaths(t *testing.T, ctx context.Context, git, root string, environment []string, commit string, expected []string) {
	t.Helper()
	output := gitOutput(t, ctx, git, root, environment, "diff-tree", "--no-commit-id", "--name-only", "-r", commit)
	got := strings.Fields(output)
	slices.Sort(got)
	want := append([]string(nil), expected...)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("commit paths = %v, want %v", got, want)
	}
}

func bundleContains(bundle retrieval.ContextBundle, value string) bool {
	encoded, _ := json.Marshal(bundle)
	return bytes.Contains(encoded, []byte(value))
}

func relatedContains(related promotion.RelatedProposals, proposalID string) bool {
	for _, item := range related.Items {
		if item.ID == proposalID {
			return true
		}
	}
	return false
}

func seedRepository(t *testing.T, git, root string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runCommand(t, context.Background(), root, os.Environ(), git, "init", "--quiet", "--initial-branch=main")
	runCommand(t, context.Background(), root, os.Environ(), git, "config", "user.name", "Phase 3 E2E")
	runCommand(t, context.Background(), root, os.Environ(), git, "config", "user.email", "phase3-e2e@example.invalid")
	for name, contents := range files {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runCommand(t, context.Background(), root, os.Environ(), git, "add", ".")
	runCommand(t, context.Background(), root, os.Environ(), git, "commit", "--quiet", "-m", "seed fixture")
}

func writeGitShim(t *testing.T, path string) {
	t.Helper()
	writeExecutable(t, path, `#!/bin/sh
# The only remote is a local bare repository reached through a url.insteadOf mapping;
# anything that names a real network transport is recorded and refused.
case " $* " in
  *"http://"*|*"https://"*|*"ssh://"*|*"git://"*)
    printf '%s\n' "$*" >>"$GPM_E2E_REMOTE_ATTEMPT_LOG"
    exit 97
    ;;
esac
if [ -f "$GPM_E2E_CRASH_ARM" ]; then
  IFS= read -r crash_mode <"$GPM_E2E_CRASH_ARM"
  case " $* " in
    *" update-ref refs/heads/release/"*)
      if [ "$crash_mode" = "before" ]; then
        kill -KILL "$PPID"
        exit 137
      fi
      "$GPM_E2E_REAL_GIT" "$@"
      status=$?
      if [ "$status" -eq 0 ]; then
        kill -KILL "$PPID"
      fi
      exit "$status"
      ;;
  esac
fi
exec "$GPM_E2E_REAL_GIT" "$@"
`)
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}

// gpmEnvironment is the environment of every gpm and git invocation of one person: the
// test process already isolated HOME and the state root for the main user.
func gpmEnvironment(t *testing.T, root, toolsRoot, realGit, crashArm, remoteAttemptLog string) []string {
	t.Helper()
	home := os.Getenv("HOME")
	configRoot := os.Getenv("XDG_CONFIG_HOME")
	stateRoot := os.Getenv("XDG_STATE_HOME")
	if root != filepath.Dir(toolsRoot) {
		// A second person: own home, configuration and state.
		home, configRoot, stateRoot = filepath.Join(root, "home"), filepath.Join(root, "config"), filepath.Join(root, "state")
	}
	claudeRoot := filepath.Join(root, "claude")
	for _, directory := range []string{home, configRoot, stateRoot, claudeRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return []string{
		"PATH=" + toolsRoot + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + configRoot,
		"XDG_STATE_HOME=" + stateRoot,
		"CLAUDE_CONFIG_DIR=" + claudeRoot,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_SSH_COMMAND=/bin/false",
		"GPM_FAKE_REMOTE=" + os.Getenv("GPM_FAKE_REMOTE"),
		"GPM_FAKE_GH_DIR=" + os.Getenv("GPM_FAKE_GH_DIR"),
		"GPM_E2E_REAL_GIT=" + realGit,
		"GPM_E2E_CRASH_ARM=" + crashArm,
		"GPM_E2E_REMOTE_ATTEMPT_LOG=" + remoteAttemptLog,
		"LANG=C",
	}
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func runCommand(t *testing.T, ctx context.Context, directory string, environment []string, executable string, arguments ...string) {
	t.Helper()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Env = environment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", filepath.Base(executable), arguments, err, output)
	}
}

func gitOutput(t *testing.T, ctx context.Context, git, root string, environment []string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(ctx, git, append([]string{"-C", root}, arguments...)...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func scanForValues(t *testing.T, root string, values []string) int {
	t.Helper()
	hits := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range values {
			hits += bytes.Count(contents, []byte(value))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func containsTranscriptArtifact(t *testing.T, root string) bool {
	t.Helper()
	found := false
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jsonl") || strings.Contains(name, "transcript") || strings.Contains(name, "session") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func missing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func writeEvidence(t *testing.T, evidence e2eEvidence) {
	t.Helper()
	root := os.Getenv("GPM_PHASE3_E2E_EVIDENCE_ROOT")
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
