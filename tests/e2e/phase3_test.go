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

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/curation"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/reference"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
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
	CanonicalCandidates retrieval.ContextBundle   `json:"canonical_candidates"`
	RelatedProposals    curation.RelatedProposals `json:"related_proposals"`
	References          []reference.Evidence      `json:"references"`
}

type batchPreviewItem struct {
	ProposalID string          `json:"proposal_id"`
	Knowledge  string          `json:"knowledge"`
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
	CreateCommit        string   `json:"create_commit"`
	RecoveredCommit     string   `json:"recovered_commit"`
	ReferenceStatuses   []string `json:"reference_statuses"`
	CanonicalOperations []string `json:"canonical_operations"`
	DuplicateRecognized bool     `json:"duplicate_recognized"`
	ContradictionShown  bool     `json:"contradiction_shown"`
	PrivacyRejected     bool     `json:"privacy_rejected"`
	MarkerContentFree   bool     `json:"marker_content_free"`
	RecoveryConverged   bool     `json:"recovery_converged"`
	BaselineRecovered   bool     `json:"baseline_recovered"`
	FinalFactRecovered  bool     `json:"final_fact_recovered"`
}

func TestPhase3TechnicalWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	runRoot := t.TempDir()
	memoryRoot := filepath.Join(runRoot, "memory")
	sourceRoot := filepath.Join(runRoot, "source")
	seedRepository(t, realGit, memoryRoot, map[string]string{
		"gpm.json":     "{\"schema_version\":1,\"memory_categories\":[\"domains\",\"decision\"]}\n",
		"staged.txt":   "staged baseline\n",
		"unstaged.txt": "unstaged baseline\n",
	})
	seedRepository(t, realGit, sourceRoot, map[string]string{
		"services/checkout/config.yaml": "timeout: 3s\nmarker: " + privateSourceMarker + "\n",
	})
	for _, root := range []string{memoryRoot, sourceRoot} {
		if remote := gitOutput(t, ctx, realGit, root, os.Environ(), "remote"); remote != "" {
			t.Fatalf("synthetic repository has unexpected remote %q", remote)
		}
	}
	toolsRoot := filepath.Join(runRoot, "tools")
	if err := os.Mkdir(toolsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(toolsRoot, "claude"), "#!/bin/sh\nexit 0\n")
	crashArm := filepath.Join(runRoot, "crash-arm")
	remoteAttemptLog := filepath.Join(runRoot, "remote-attempts")
	writeGitShim(t, filepath.Join(toolsRoot, "git"))
	environment := isolatedEnvironment(t, runRoot, toolsRoot, realGit, crashArm, remoteAttemptLog)
	runCommand(t, ctx, repositoryRoot, environment, binary, "setup", memoryRoot)
	t.Setenv("XDG_STATE_HOME", environmentValue(environment, "XDG_STATE_HOME"))
	stateRoot, err := config.StatePath(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(memoryRoot, "staged.txt"), []byte("staged user change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, ctx, memoryRoot, environment, realGit, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(memoryRoot, "unstaged.txt"), []byte("unstaged user change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, ctx, memoryRoot, environment, realGit, "update-index", "--assume-unchanged", "unstaged.txt")
	indexBefore := gitOutput(t, ctx, realGit, memoryRoot, environment, "ls-files", "--stage", "--debug", "--", "staged.txt", "unstaged.txt")
	hookSentinel := filepath.Join(runRoot, "hook-ran")
	writeExecutable(t, filepath.Join(memoryRoot, ".git", "hooks", "commit-msg"), "#!/bin/sh\ntouch \"$GPM_E2E_HOOK_SENTINEL\"\nexit 1\n")
	environment = append(environment, "GPM_E2E_HOOK_SENTINEL="+hookSentinel)

	objectsBeforePrivacy := gitOutput(t, ctx, realGit, memoryRoot, environment, "count-objects", "-v")
	sessionA := connectMCP(t, ctx, binary, environment, "phase3-e2e-a")
	privacyResult, err := sessionA.CallTool(ctx, &mcp.CallToolParams{Name: "memory_propose", Arguments: map[string]any{
		"title": "Rejected secret", "suggested_action": "create", "knowledge": rejectedSecret, "source_paths": []string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	privacy := decodeTool[json.RawMessage](t, privacyResult)
	if !privacyResult.IsError || privacy.Error == nil || privacy.Error.Code != "privacy_sensitive_content" || !missing(stateRoot) ||
		gitOutput(t, ctx, realGit, memoryRoot, environment, "count-objects", "-v") != objectsBeforePrivacy {
		t.Fatal("privacy rejection wrote state or returned the wrong status")
	}

	snapshotResult := callTool(t, ctx, sessionA, "memory_reference_snapshot", map[string]any{
		"repository_root": sourceRoot, "source_paths": []string{"services/checkout/config.yaml"},
	})
	snapshot := requireData(t, decodeTool[reference.Snapshot](t, snapshotResult))
	if len(snapshot.References) != 1 {
		t.Fatal("source snapshot omitted its reference")
	}
	expandResult := callTool(t, ctx, sessionA, "memory_reference_expand", map[string]any{
		"reference": snapshot.References[0], "start_line": 1, "budget_bytes": 2048,
	})
	excerpt := requireData(t, decodeTool[reference.Excerpt](t, expandResult))
	if !strings.Contains(excerpt.Content, privateSourceMarker) {
		t.Fatal("source expansion did not inspect the committed bytes")
	}
	mismatch := snapshot.References[0]
	mismatch.ContentHash = "sha256:" + strings.Repeat("0", 64)

	createAArgs := map[string]any{
		"title": "Checkout policy", "suggested_action": "create", "knowledge": initialCheckoutFact,
		"source_paths": []string{}, "source_references": []reference.Reference{snapshot.References[0], mismatch},
	}
	createA := propose(t, ctx, sessionA, createAArgs)
	headBeforeDuplicate := gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD")
	duplicate := propose(t, ctx, sessionA, createAArgs)
	if !duplicate.Duplicate || duplicate.ID != createA.ID || gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD") != headBeforeDuplicate {
		t.Fatal("exact duplicate created another proposal or commit")
	}
	createB := propose(t, ctx, sessionA, map[string]any{
		"title": "Legacy cleanup", "suggested_action": "create", "knowledge": legacyCleanupFact,
		"source_paths": []string{"jobs/legacy-cleanup.yaml"},
	})
	createSection := propose(t, ctx, sessionA, map[string]any{
		"title": "Temporary checkout detail", "suggested_action": "create", "target": "memories/domains/checkout-policy.md",
		"knowledge": "Temporary migration detail remains active.", "source_paths": []string{},
	})
	removeSection := propose(t, ctx, sessionA, map[string]any{
		"title": "Remove temporary checkout detail", "suggested_action": "remove", "target": "memories/domains/checkout-policy.md",
		"knowledge": "Temporary migration detail is retired.", "source_paths": []string{},
	})
	updateA := propose(t, ctx, sessionA, map[string]any{
		"title": "Checkout timeout changed", "suggested_action": "update", "target": "memories/domains/checkout-policy.md",
		"knowledge": updatedCheckoutFact, "source_paths": []string{"services/checkout/config.yaml"},
	})
	removeB := propose(t, ctx, sessionA, map[string]any{
		"title": "Retire legacy cleanup", "suggested_action": "remove", "target": "memories/domains/legacy-cleanup.md",
		"knowledge": "Legacy cleanup is retired.", "source_paths": []string{},
	})
	discard := propose(t, ctx, sessionA, map[string]any{
		"title": "Duplicate checkout timeout", "suggested_action": "create", "knowledge": "Checkout requests time out after three seconds.",
		"source_paths": []string{}, "source_references": []reference.Reference{snapshot.References[0]},
	})

	createPreviewResult := callTool(t, ctx, sessionA, "memory_curate_batch_preview", map[string]any{"items": []map[string]any{
		{"operation": "create_page", "proposal_id": createA.ID, "target_path": "memories/domains/checkout-policy.md"},
		{"operation": "create_page", "proposal_id": createB.ID, "target_path": "memories/domains/legacy-cleanup.md"},
	}})
	createPreview := requireData(t, decodeTool[batchPreview](t, createPreviewResult))
	if len(createPreview.Items) != 2 {
		t.Fatal("create batch preview omitted decisions")
	}
	assertReferenceStatuses(t, createPreview.Items[0].Evidence.References, reference.IntegrityVerified, reference.IntegrityMismatch)
	assertReferenceStatuses(t, createPreview.Items[1].Evidence.References, reference.IntegrityLegacyUnverified)
	createApplyResult := callTool(t, ctx, sessionA, "memory_curate_batch_apply", map[string]any{"preview_token": createPreview.PreviewToken})
	createResult := requireData(t, decodeTool[curation.BatchResult](t, createApplyResult))
	assertCommitPaths(t, ctx, realGit, memoryRoot, environment, createResult.Commit, []string{
		createA.Path, createB.Path, "memories/domains/checkout-policy.md", "memories/domains/legacy-cleanup.md",
	})
	if err := sessionA.Close(); err != nil {
		t.Fatal(err)
	}

	sectionSession := connectMCP(t, ctx, binary, environment, "phase3-e2e-sections")
	checkoutOverview := searchOne(t, ctx, sectionSession, "checkout three team-blue")
	createSectionPreviewResult := callTool(t, ctx, sectionSession, "memory_curate_preview", map[string]any{
		"operation": "create_section", "proposal_id": createSection.ID, "target_path": checkoutOverview.Path, "heading": "Temporary detail",
	})
	createSectionPreview := requireData(t, decodeTool[singlePreview](t, createSectionPreviewResult))
	createSectionApplyResult := callTool(t, ctx, sectionSession, "memory_curate_apply", map[string]any{"preview_token": createSectionPreview.PreviewToken})
	createSectionResult := requireData(t, decodeTool[curation.Result](t, createSectionApplyResult))
	assertCommitPaths(t, ctx, realGit, memoryRoot, environment, createSectionResult.Commit, []string{checkoutOverview.Path, createSection.Path})
	sectionBaselineCommit := gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD")
	temporaryHandle := searchOne(t, ctx, sectionSession, "temporary migration detail")
	removeSectionPreviewResult := callTool(t, ctx, sectionSession, "memory_curate_preview", map[string]any{
		"operation": "remove_section", "proposal_id": removeSection.ID, "target_path": temporaryHandle.Path,
		"section_id": temporaryHandle.SectionID, "content_hash": temporaryHandle.ContentHash,
	})
	removeSectionPreview := requireData(t, decodeTool[singlePreview](t, removeSectionPreviewResult))
	if err := os.WriteFile(crashArm, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, beforeRefErr := sectionSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "memory_curate_apply", Arguments: map[string]any{"preview_token": removeSectionPreview.PreviewToken},
	})
	if beforeRefErr == nil {
		t.Fatal("pre-update-ref interruption returned a normal MCP response")
	}
	_ = sectionSession.Close()
	if gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD") != sectionBaselineCommit {
		t.Fatal("pre-update-ref interruption advanced HEAD")
	}
	markerPath := filepath.Join(stateRoot, "curation-transaction.json")
	if missing(markerPath) {
		t.Fatal("pre-update-ref interruption omitted its recovery marker")
	}
	if err := os.Remove(crashArm); err != nil {
		t.Fatal(err)
	}
	baselineRecoverySession := connectMCP(t, ctx, binary, environment, "phase3-e2e-baseline-recovery")
	gitDirectory := gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "--absolute-git-dir")
	indexPath := gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "--path-format=absolute", "--git-path", "index")
	baselineArtifacts, _ := filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-curation-index-*"))
	if !missing(markerPath) || !missing(indexPath+".lock") || len(baselineArtifacts) != 0 || missing(filepath.Join(memoryRoot, filepath.FromSlash(removeSection.Path))) {
		t.Fatal("baseline recovery did not restore proposals or clean controlled artifacts")
	}
	restoredTemporary := searchOne(t, ctx, baselineRecoverySession, "temporary migration detail")
	retryRemovePreviewResult := callTool(t, ctx, baselineRecoverySession, "memory_curate_preview", map[string]any{
		"operation": "remove_section", "proposal_id": removeSection.ID, "target_path": restoredTemporary.Path,
		"section_id": restoredTemporary.SectionID, "content_hash": restoredTemporary.ContentHash,
	})
	retryRemovePreview := requireData(t, decodeTool[singlePreview](t, retryRemovePreviewResult))
	retryRemoveResult := callTool(t, ctx, baselineRecoverySession, "memory_curate_apply", map[string]any{"preview_token": retryRemovePreview.PreviewToken})
	removeSectionResult := requireData(t, decodeTool[curation.Result](t, retryRemoveResult))
	assertCommitPaths(t, ctx, realGit, memoryRoot, environment, removeSectionResult.Commit, []string{restoredTemporary.Path, removeSection.Path})
	if err := baselineRecoverySession.Close(); err != nil {
		t.Fatal(err)
	}
	decisionBaselineCommit := gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD")

	decisionSession := connectMCP(t, ctx, binary, environment, "phase3-e2e-decisions")
	checkoutHandle := searchOne(t, ctx, decisionSession, "checkout three team-blue")
	cleanupHandle := searchOne(t, ctx, decisionSession, "legacy cleanup hour")
	decisionPreviewResult := callTool(t, ctx, decisionSession, "memory_curate_batch_preview", map[string]any{"items": []map[string]any{
		{"operation": "replace_section", "proposal_id": updateA.ID, "target_path": checkoutHandle.Path, "section_id": checkoutHandle.SectionID, "content_hash": checkoutHandle.ContentHash, "replacement": updatedCheckoutFact},
		{"operation": "remove_page", "proposal_id": removeB.ID, "target_path": cleanupHandle.Path, "section_id": cleanupHandle.SectionID, "content_hash": cleanupHandle.ContentHash},
		{"operation": "discard_proposal", "proposal_id": discard.ID, "reason": "duplicate"},
	}})
	decisionPreview := requireData(t, decodeTool[batchPreview](t, decisionPreviewResult))
	if len(decisionPreview.Items) != 3 {
		t.Fatal("decision batch preview omitted decisions")
	}
	if !bundleContains(decisionPreview.Items[0].Evidence.CanonicalCandidates, "three seconds") || !relatedContains(decisionPreview.Items[0].Evidence.RelatedProposals, discard.ID) || !strings.Contains(decisionPreview.Items[0].Knowledge, "nine seconds") {
		t.Fatal("contradictory current and proposed facts were not observable together")
	}
	assertReferenceStatuses(t, decisionPreview.Items[0].Evidence.References, reference.IntegrityLegacyUnverified)
	assertReferenceStatuses(t, decisionPreview.Items[2].Evidence.References, reference.IntegrityUnavailable)
	for _, expectedPath := range []string{checkoutHandle.Path, cleanupHandle.Path, updateA.Path, removeB.Path, discard.Path} {
		if !strings.Contains(decisionPreview.Diff, expectedPath) {
			t.Fatalf("decision diff omitted %s", expectedPath)
		}
	}
	if err := os.WriteFile(crashArm, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, applyErr := decisionSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "memory_curate_batch_apply", Arguments: map[string]any{"preview_token": decisionPreview.PreviewToken},
	})
	if applyErr == nil {
		t.Fatal("interrupted apply returned a normal MCP response")
	}
	_ = decisionSession.Close()
	interruptedCommit := gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD")
	if interruptedCommit == decisionBaselineCommit {
		t.Fatal("crash occurred before update-ref")
	}
	assertCommitPaths(t, ctx, realGit, memoryRoot, environment, interruptedCommit, []string{
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
	artifacts, err := filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-curation-index-*"))
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

	recoverySession := connectMCP(t, ctx, binary, environment, "phase3-e2e-recovery")
	listResult := callTool(t, ctx, recoverySession, "memory_proposal_list", map[string]any{})
	listed := requireData(t, decodeTool[curation.List](t, listResult))
	if len(listed.Proposals) != 0 {
		t.Fatalf("recovery retained %d proposals", len(listed.Proposals))
	}
	if err := recoverySession.Close(); err != nil {
		t.Fatal(err)
	}
	if gitOutput(t, ctx, realGit, memoryRoot, environment, "rev-parse", "HEAD") != interruptedCommit {
		t.Fatal("startup recovery created a duplicate commit")
	}
	if got := gitOutput(t, ctx, realGit, memoryRoot, environment, "ls-files", "--stage", "--debug", "--", "staged.txt", "unstaged.txt"); got != indexBefore {
		t.Fatal("recovery changed unrelated index entries")
	}
	if got, err := os.ReadFile(filepath.Join(memoryRoot, "unstaged.txt")); err != nil || string(got) != "unstaged user change\n" {
		t.Fatal("recovery changed unrelated unstaged bytes")
	}
	if !missing(markerPath) || !missing(indexPath+".lock") {
		t.Fatal("recovery retained transaction marker or index lock")
	}
	artifacts, _ = filepath.Glob(filepath.Join(gitDirectory, ".git-push-memories-curation-index-*"))
	if len(artifacts) != 0 || !missing(filepath.Join(memoryRoot, "memories", "domains", "legacy-cleanup.md")) {
		t.Fatal("recovery retained controlled transaction artifacts or removed memory")
	}
	if _, err := os.Stat(hookSentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Git commit hook ran during plumbing publication")
	}
	message := gitOutput(t, ctx, realGit, memoryRoot, environment, "show", "-s", "--format=%B", interruptedCommit)
	if strings.Count(message, "Git-Push-Memories-Decision: consumed") != 2 || strings.Count(message, "Git-Push-Memories-Decision: discarded") != 1 || !strings.Contains(message, "Git-Push-Memories-Reason: duplicate") {
		t.Fatal("recovered commit omitted decision trailers")
	}
	for _, root := range []string{memoryRoot, stateRoot} {
		if scanForValues(t, root, []string{privateSourceMarker, rejectedSecret, sourceRoot, decisionPreview.PreviewToken}) != 0 {
			t.Fatal("memory repository or private state retained forbidden source content or bindings")
		}
	}
	if !missing(remoteAttemptLog) || containsTranscriptArtifact(t, memoryRoot) || containsTranscriptArtifact(t, stateRoot) {
		t.Fatal("memory repository or private state retained forbidden source/session content")
	}

	freshRoot := filepath.Join(runRoot, "fresh-user")
	freshRemoteAttemptLog := filepath.Join(freshRoot, "remote-attempts")
	freshEnvironment := isolatedEnvironment(t, freshRoot, toolsRoot, realGit, filepath.Join(freshRoot, "unused-arm"), freshRemoteAttemptLog)
	runCommand(t, ctx, repositoryRoot, freshEnvironment, binary, "setup", memoryRoot)
	t.Setenv("XDG_STATE_HOME", environmentValue(freshEnvironment, "XDG_STATE_HOME"))
	freshStateRoot, err := config.StatePath(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	finalSession := connectMCP(t, ctx, binary, freshEnvironment, "phase3-e2e-final")
	finalHandle := searchOne(t, ctx, finalSession, "checkout nine team-blue")
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
	if !missing(freshStateRoot) || !missing(freshRemoteAttemptLog) {
		t.Fatal("read-only fresh process created private state or attempted remote access")
	}

	writeEvidence(t, e2eEvidence{
		SchemaVersion: 1, ProductRevision: gitOutput(t, ctx, realGit, repositoryRoot, os.Environ(), "rev-parse", "HEAD"), GoVersion: runtime.Version(),
		CreateCommit: createResult.Commit, RecoveredCommit: interruptedCommit,
		ReferenceStatuses:   []string{"verified", "mismatch", "legacy_unverified", "unavailable"},
		CanonicalOperations: []string{"create_page", "create_section", "replace_section", "remove_section", "remove_page", "discard_proposal"},
		DuplicateRecognized: true, ContradictionShown: true, PrivacyRejected: true, MarkerContentFree: true, RecoveryConverged: true, BaselineRecovered: true, FinalFactRecovered: true,
	})
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

func searchOne(t *testing.T, ctx context.Context, session *mcp.ClientSession, query string) retrieval.Handle {
	t.Helper()
	result := callTool(t, ctx, session, "memory_search", map[string]any{"query": query, "budget_bytes": 4096})
	bundle := requireData(t, decodeTool[retrieval.ContextBundle](t, result))
	if len(bundle.Items) != 1 {
		t.Fatalf("memory_search(%q) returned %d items", query, len(bundle.Items))
	}
	return bundle.Items[0].Handle
}

func assertReferenceStatuses(t *testing.T, evidence []reference.Evidence, statuses ...reference.IntegrityStatus) {
	t.Helper()
	got := make([]reference.IntegrityStatus, len(evidence))
	for index, item := range evidence {
		got[index] = item.Integrity.Status
		if item.SemanticSupport != reference.SemanticNotAssessed {
			t.Fatal("reference verifier asserted semantic support")
		}
	}
	if !slices.Equal(got, statuses) {
		t.Fatalf("reference statuses = %v, want %v", got, statuses)
	}
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

func relatedContains(related curation.RelatedProposals, proposalID string) bool {
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
case " $* " in
  *" fetch "*|*" pull "*|*" push "*|*" clone "*|*" ls-remote "*|*" merge "*|*"http://"*|*"https://"*|*"ssh://"*|*"git@"*)
    printf '%s\n' "$*" >>"$GPM_E2E_REMOTE_ATTEMPT_LOG"
    exit 97
    ;;
esac
if [ -f "$GPM_E2E_CRASH_ARM" ]; then
  IFS= read -r crash_mode <"$GPM_E2E_CRASH_ARM"
  case " $* " in
    *" update-ref refs/heads/main "*)
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

func isolatedEnvironment(t *testing.T, root, toolsRoot, realGit, crashArm, remoteAttemptLog string) []string {
	t.Helper()
	home := filepath.Join(root, "home")
	configRoot := filepath.Join(root, "config")
	stateRoot := filepath.Join(root, "state")
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
		"GIT_CONFIG_GLOBAL=/dev/null",
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
