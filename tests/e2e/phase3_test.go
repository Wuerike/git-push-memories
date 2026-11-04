package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const githubURL = "git@github.com:o/r.git"
const privateSourceMarker = "PRIVATE-SOURCE-E2E-7f9c1a"
const rejectedSecret = "ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const checkoutPath = "memories/domains/checkout-policy.md"

type toolEnvelope[T any] struct {
	SchemaVersion int        `json:"schema_version"`
	Data          *T         `json:"data,omitempty"`
	Error         *toolError `json:"error,omitempty"`
}
type toolError struct {
	Code string `json:"code"`
}
type e2eEvidence struct {
	SchemaVersion        int    `json:"schema_version"`
	ProductRevision      string `json:"product_revision"`
	GoVersion            string `json:"go_version"`
	PromotionBranch      string `json:"promotion_branch"`
	Commit               string `json:"commit"`
	PullRequest          string `json:"pull_request"`
	InitialProposals     int    `json:"initial_proposals"`
	SetupInitialized     bool   `json:"setup_initialized_empty_remote"`
	HumanEditsPreserved  bool   `json:"human_edits_preserved"`
	PullRequestMerged    bool   `json:"pull_request_merged_finalized"`
	NewProposalPreserved bool   `json:"new_proposal_preserved"`
	FinalFactRecovered   bool   `json:"final_fact_recovered"`
	SecondUserReadOnly   bool   `json:"second_user_read_only"`
}

func TestManagedWorkflowEndToEnd(t *testing.T) {
	testManagedWorkflowEndToEnd(t, "claude-code")
}

func TestCodexManagedWorkflowEndToEnd(t *testing.T) {
	testManagedWorkflowEndToEnd(t, "codex")
}

func testManagedWorkflowEndToEnd(t *testing.T, agent string) {
	t.Helper()
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

	wt.Env(t)
	gh := wt.FakeGH(t)
	runRoot := t.TempDir()
	remote := wt.Remote(t)
	t.Setenv("GPM_FAKE_REMOTE", remote)
	sourceRoot := filepath.Join(runRoot, "source")
	seedRepository(t, realGit, sourceRoot, map[string]string{"services/checkout/config.yaml": "timeout: 3s\nmarker: " + privateSourceMarker + "\n"})
	toolsRoot := filepath.Join(runRoot, "tools")
	if err := os.Mkdir(toolsRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	agentCLI, invocation := "claude", "Use /remember <fact> and /promote"
	if agent == "codex" {
		agentCLI, invocation = "codex", "Use $remember <fact> and $promote"
	}
	writeExecutable(t, filepath.Join(toolsRoot, agentCLI), "#!/bin/sh\nexit 0\n")
	environment := gpmEnvironment(t, runRoot, toolsRoot)
	mapURLToRemote(t, environmentValue(environment, "HOME"), remote)
	setupOutput := commandOutput(t, ctx, repositoryRoot, environment, binary, "setup", githubURL, "--agent", agent, "--yes", "--categories", "domains,decision")
	if !strings.Contains(setupOutput, invocation) {
		t.Fatalf("setup = %q", setupOutput)
	}
	paths, err := workspace.PathsFor(githubURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gitOutput(t, ctx, realGit, remote, environment, "show", "main:gpm.json"), "domains") {
		t.Fatal("setup did not initialize repository config")
	}
	for _, worktree := range []string{paths.Read, paths.Proposals} {
		if missing(filepath.Join(worktree, ".git")) {
			t.Fatalf("missing managed worktree %s", worktree)
		}
	}
	if !missing(paths.Promotions) {
		t.Fatal("setup created a promotion")
	}

	session := connectMCP(t, ctx, binary, environment, "e2e-promoter")
	var proposals []inbox.Proposal
	for n := 0; n < 17; n++ {
		content := "# Checkout fact\nThe checkout timeout is nine seconds and the owner is team-blue.\n"
		if n == 0 {
			content = "# Initial checkout policy\nCreate a memory: checkout timeout is three seconds; owner is team-blue.\n"
		}
		if n == 1 {
			content = "# Updated checkout timeout\nUpdate the timeout to nine seconds; preserve ownership.\n"
		}
		proposals = append(proposals, propose(t, ctx, session, map[string]any{"title": fmt.Sprintf("Checkout fact %d", n), "content": content}))
	}
	if got := proposalFiles(t, ctx, realGit, remote, environment, "proposals"); len(got) != 17 {
		t.Fatalf("remember published %d proposals, want 17", len(got))
	}
	mainBefore := gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "main")
	if strings.Count(gitOutput(t, ctx, realGit, remote, environment, "log", "--format=%H", "main"), "\n") != 0 {
		t.Fatal("remember changed main")
	}

	prepared := requireData(t, decodeTool[consolidation.Prepared](t, callTool(t, ctx, session, "memory_promote_prepare", map[string]any{})))
	if prepared.MainSHA != mainBefore || len(prepared.Proposals) != 17 || len(prepared.MemoryPaths) != 0 {
		t.Fatalf("prepare = %+v", prepared)
	}
	for _, p := range prepared.Proposals {
		input, err := os.ReadFile(p.InputPath)
		if err != nil || !strings.HasPrefix(string(input), "# ") {
			t.Fatalf("free proposal %s unavailable: %v", p.Path, err)
		}
	}

	// A separate writer adds an input after the fixed set was prepared.
	writer := connectMCP(t, ctx, binary, environment, "e2e-later-writer")
	later := propose(t, ctx, writer, map[string]any{"title": "Future settlement retry policy", "content": "# Settlement\nSettlement retries stop after three failures.\n"})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	// The scripted agent prepares canonical knowledge; the human edits that same file.
	full := filepath.Join(prepared.Worktree, checkoutPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Checkout policy\n\n## Operation\nCheckout requests time out after nine seconds. Ownership remains team-blue.\n"
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewed := requireData(t, decodeTool[consolidation.Checked](t, callTool(t, ctx, session, "memory_promote_check", map[string]any{})))
	if len(reviewed.Paths) != 1 || reviewed.Paths[0] != checkoutPath || !strings.Contains(reviewed.Diff, "+Checkout requests time out after nine seconds.") {
		t.Fatalf("initial review = %+v", reviewed)
	}
	canonical, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	human := append(append([]byte(nil), canonical...), []byte("\nHuman clarification: timeout applies to authorization only.\n")...)
	if err := os.WriteFile(full, human, 0o644); err != nil {
		t.Fatal(err)
	}
	finalReview := requireData(t, decodeTool[consolidation.Checked](t, callTool(t, ctx, session, "memory_promote_check", map[string]any{})))
	if !strings.Contains(finalReview.Diff, "+Human clarification: timeout applies to authorization only.") {
		t.Fatal("final diff lost human edit")
	}
	writeExecutable(t, filepath.Join(prepared.Worktree, "local-review-notes"), "Private helper notes, excluded from the memory commit.\n")
	var summary strings.Builder
	summary.WriteString("## Reviewed consolidation\n\n")
	for i, p := range proposals {
		if i < 2 {
			fmt.Fprintf(&summary, "- %s contributes the current state to %s.\n", p.ID, checkoutPath)
		} else {
			fmt.Fprintf(&summary, "- %s is discarded because the approved page already represents the same knowledge.\n", p.ID)
		}
	}
	submitted := requireData(t, decodeTool[consolidation.Submitted](t, callTool(t, ctx, session, "memory_promote_submit", map[string]any{"summary": summary.String()})))
	if submitted.PRURL == "" || submitted.Commit == "" {
		t.Fatalf("submit = %+v", submitted)
	}
	assertCommitPaths(t, ctx, realGit, remote, environment, submitted.Commit, []string{checkoutPath})
	if count := gitOutput(t, ctx, realGit, prepared.Worktree, environment, "rev-list", "--count", prepared.MainSHA+"..HEAD"); count != "1" {
		t.Fatalf("consolidation has %s commits", count)
	}
	if got := proposalFiles(t, ctx, realGit, remote, environment, "proposals"); len(got) != 18 {
		t.Fatalf("submit consumed proposals: %v", got)
	}
	if gitOutput(t, ctx, realGit, remote, environment, "rev-parse", "main") != mainBefore {
		t.Fatal("submit merged its own PR")
	}
	beforeMerge := requireData(t, decodeTool[retrieval.ContextBundle](t, callTool(t, ctx, session, "memory_search", map[string]any{"query": "checkout nine team-blue", "limit": 20})))
	if len(beforeMerge.Items) != 0 {
		t.Fatal("unmerged memory is visible to readers")
	}
	open := requireData(t, decodeTool[consolidation.Finished](t, callTool(t, ctx, session, "memory_promote_finish", map[string]any{})))
	if !open.Found || open.Merged || len(open.Removed) != 0 {
		t.Fatalf("open PR completion = %+v", open)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	mergeOID := squashMerge(t, ctx, realGit, remote, environment, prepared.Branch)
	gh.SetPR(t, "MERGED", mergeOID)
	// Completion in a new process uses only the published PR/input linkage, never an editing session.
	finishSession := connectMCP(t, ctx, binary, environment, "e2e-finish-published-pr")
	finished := requireData(t, decodeTool[consolidation.Finished](t, callTool(t, ctx, finishSession, "memory_promote_finish", map[string]any{})))
	if !finished.Merged || len(finished.Removed) != 17 {
		t.Fatalf("finish = %+v", finished)
	}
	if err := finishSession.Close(); err != nil {
		t.Fatal(err)
	}
	if pending := proposalFiles(t, ctx, realGit, remote, environment, "proposals"); !slices.Equal(pending, []string{later.Path}) {
		t.Fatalf("cleanup lost the new proposal or retained concluded inputs: %v", pending)
	}
	if !missing(paths.Published) {
		t.Fatal("cleanup retained its publication record")
	}
	if got := gitOutput(t, ctx, realGit, remote, environment, "show", "main:"+checkoutPath); got != strings.TrimSuffix(string(human), "\n") {
		t.Fatalf("merged page differs from reviewed human file: %q", got)
	}
	reader := connectMCP(t, ctx, binary, environment, "e2e-published-reader")
	mergedRead := requireData(t, decodeTool[retrieval.ContextBundle](t, callTool(t, ctx, reader, "memory_search", map[string]any{"query": "checkout nine team-blue", "limit": 20})))
	if !bundleContains(*mergedRead, "nine seconds") || !bundleContains(*mergedRead, "authorization only") {
		t.Fatalf("published read lost current facts: %+v", mergedRead)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	for _, value := range []string{privateSourceMarker, rejectedSecret, sourceRoot} {
		if gitOutput(t, ctx, realGit, remote, environment, "log", "--all", "--oneline", "-S"+value) != "" {
			t.Fatal("unrelated private source content entered shared history")
		}
	}
	if scanForValues(t, paths.State, []string{privateSourceMarker, rejectedSecret, sourceRoot}) != 0 || containsTranscriptArtifact(t, paths.State) {
		t.Fatal("private state retained unrelated source/session content")
	}

	// A second person initializes independent state and reads without publishing or preparing.
	freshRoot := filepath.Join(runRoot, "fresh-user")
	freshEnvironment := gpmEnvironment(t, freshRoot, toolsRoot)
	mapURLToRemote(t, environmentValue(freshEnvironment, "HOME"), remote)
	commandOutput(t, ctx, repositoryRoot, freshEnvironment, binary, "setup", githubURL, "--agent", agent, "--yes")
	freshState := filepath.Join(environmentValue(freshEnvironment, "XDG_STATE_HOME"), "git-push-memories", workspace.RepoID(githubURL))
	refsBefore := gitOutput(t, ctx, realGit, remote, freshEnvironment, "for-each-ref")
	inboxBefore := gitOutput(t, ctx, realGit, filepath.Join(freshState, "proposals"), freshEnvironment, "rev-parse", "HEAD")
	fresh := connectMCP(t, ctx, binary, freshEnvironment, "e2e-fresh-reader")
	handle := searchOne(t, ctx, fresh, "checkout nine team-blue")
	expanded := requireData(t, decodeTool[retrieval.FileContent](t, callTool(t, ctx, fresh, "memory_read", map[string]any{"path": handle.Path, "revision": handle.Revision})))
	if !strings.Contains(expanded.Content, "nine seconds") || !strings.Contains(expanded.Content, "authorization only") || strings.Contains(expanded.Content, "three seconds") {
		t.Fatal("fresh reader did not recover the current reviewed fact")
	}
	unpublished := requireData(t, decodeTool[retrieval.ContextBundle](t, callTool(t, ctx, fresh, "memory_search", map[string]any{"query": "settlement retries failures", "limit": 20})))
	if len(unpublished.Items) != 0 {
		t.Fatal("fresh reading returned a pending proposal")
	}
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}
	if gitOutput(t, ctx, realGit, remote, freshEnvironment, "for-each-ref") != refsBefore || gitOutput(t, ctx, realGit, filepath.Join(freshState, "proposals"), freshEnvironment, "rev-parse", "HEAD") != inboxBefore || !missing(filepath.Join(freshState, "promotions")) || !missing(filepath.Join(freshState, "promotion-published.json")) {
		t.Fatal("read-only fresh user prepared or published changes")
	}

	writeEvidence(t, e2eEvidence{SchemaVersion: 1, ProductRevision: gitOutput(t, ctx, realGit, repositoryRoot, os.Environ(), "rev-parse", "HEAD"), GoVersion: runtime.Version(), PromotionBranch: prepared.Branch, Commit: submitted.Commit, PullRequest: submitted.PRURL, InitialProposals: 17, SetupInitialized: true, HumanEditsPreserved: true, PullRequestMerged: true, NewProposalPreserved: true, FinalFactRecovered: true, SecondUserReadOnly: true})
}

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

func searchOne(t *testing.T, ctx context.Context, session *mcp.ClientSession, query string) retrieval.FileContent {
	t.Helper()
	arguments := map[string]any{"query": query, "limit": 20}

	result := callTool(t, ctx, session, "memory_search", arguments)
	bundle := requireData(t, decodeTool[retrieval.ContextBundle](t, result))
	if len(bundle.Items) != 1 {
		t.Fatalf("memory_search(%q) returned %d items", query, len(bundle.Items))
	}
	return retrieval.FileContent{Path: bundle.Items[0].Path, Revision: bundle.Revision}
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

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
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

// gpmEnvironment is the environment of every gpm and git invocation of one person: the
// test process already isolated HOME and the state root for the main user.
func gpmEnvironment(t *testing.T, root, toolsRoot string) []string {
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
		"LANG=C",
	}
}
