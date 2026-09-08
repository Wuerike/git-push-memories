package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wuerike/git-push-memories/internal/proposal"
	"github.com/Wuerike/git-push-memories/internal/protocol"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	query            = "checkout timeout team"
	targetPath       = "services/checkout-api.md"
	targetSectionID  = "checkout-api-operation"
	searchBudget     = 2048
	expandBudget     = 4096
	replacement      = "\n- O timeout de requisicoes e de tres segundos.\n- O servico pertence ao `team-b`.\n"
	proposalToolName = "memory_propose_replace"
)

type commandRecord struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

type manifest struct {
	SchemaVersion       int               `json:"schema_version"`
	ProductRevision     string            `json:"product_revision"`
	ProductWorktree     string            `json:"product_worktree"`
	GoVersion           string            `json:"go_version"`
	GitVersion          string            `json:"git_version"`
	ClaudeCodeVersion   string            `json:"claude_code_version"`
	MCPSDKVersion       string            `json:"mcp_sdk_version"`
	Offline             bool              `json:"offline"`
	Configuration       map[string]string `json:"configuration"`
	RankingCriterion    string            `json:"ranking_criterion"`
	Commands            []commandRecord   `json:"commands"`
	InitialContextBytes int               `json:"initial_context_bytes"`
	InitialExpandBytes  int               `json:"initial_expand_bytes"`
	FinalContextBytes   int               `json:"final_context_bytes"`
	FinalExpandBytes    int               `json:"final_expand_bytes"`
	InitialFileHash     string            `json:"initial_file_hash"`
	FinalFileHash       string            `json:"final_file_hash"`
	ExpectedFileHash    string            `json:"expected_file_hash"`
	PublicationCommit   string            `json:"publication_commit"`
	CommitPaths         []string          `json:"commit_paths"`
	ProposalPayloadGone bool              `json:"proposal_payload_gone"`
	ReceiptBodyFree     bool              `json:"receipt_body_free"`
	PrivacyMarkerHits   int               `json:"privacy_marker_hits_outside_source"`
	PrivacyWriteTargets int               `json:"privacy_write_targets"`
	PrivacyCases        int               `json:"privacy_cases_rejected"`
}

type harness struct {
	t            *testing.T
	ctx          context.Context
	repository   string
	evidenceRoot string
	binary       string
	records      []commandRecord
}

func TestSyntheticWorkflow(t *testing.T) {
	evidenceRoot := os.Getenv("GPM_E2E_EVIDENCE_ROOT")
	if evidenceRoot == "" {
		t.Skip("run tests/e2e/run-phase-2.sh to execute the classificatory E2E")
	}
	h := newHarness(t, evidenceRoot)
	result := h.runBehavioral()
	privacyHits, privacyWrites := h.runPrivacy()
	result.PrivacyMarkerHits = privacyHits
	result.PrivacyWriteTargets = privacyWrites
	result.PrivacyCases = 6
	result.Commands = h.records
	h.writeJSON("manifest.json", result)
}

func newHarness(t *testing.T, evidenceRoot string) *harness {
	t.Helper()
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(evidenceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	h := &harness{t: t, ctx: ctx, repository: repository, evidenceRoot: evidenceRoot}
	h.binary = filepath.Join(t.TempDir(), "gpm")
	h.run("build product", repository, os.Environ(), "go", "build", "-o", h.binary, "./cmd/gpm")
	return h
}

func (h *harness) runBehavioral() manifest {
	t := h.t
	runRoot := t.TempDir()
	memoryRoot := filepath.Join(runRoot, "memory")
	codeRoot := filepath.Join(runRoot, "code")
	home := filepath.Join(runRoot, "home-a")
	configRoot := filepath.Join(runRoot, "config-a")
	for _, root := range []string{memoryRoot, codeRoot, home, configRoot} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	initial := h.fixture("memory-initial", targetPath)
	expected := h.fixture("memory-expected", targetPath)
	h.seedRepository(memoryRoot, map[string][]byte{
		targetPath:           initial,
		"notes/staged.txt":   []byte("base staged\n"),
		"notes/unstaged.txt": []byte("base unstaged\n"),
	})
	h.seedRepository(codeRoot, map[string][]byte{
		"services/checkout-api/service.yaml": h.fixture("code-repo", "services/checkout-api/service.yaml"),
	})
	stateRoot := stateRootFor(t, configRoot, memoryRoot)
	if err := os.WriteFile(filepath.Join(memoryRoot, "notes/staged.txt"), []byte("user staged change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.git(memoryRoot, "add", "notes/staged.txt")
	if err := os.WriteFile(filepath.Join(memoryRoot, "notes/unstaged.txt"), []byte("user unstaged change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	initialHead := h.gitOutput(memoryRoot, "rev-parse", "HEAD")
	indexBefore := h.gitOutput(memoryRoot, "ls-files", "--stage", "--debug")
	stagedBefore := h.gitOutput(memoryRoot, "diff", "--cached", "--", "notes/staged.txt")
	initialHash := hashBytes(initial)
	h.writeFile("index-before.txt", []byte(indexBefore))
	h.writeFile("inventory-before.txt", []byte(h.inventory(runRoot)))

	environment := isolatedEnvironment(home, configRoot)
	session := h.connectMCP("MCP process A", memoryRoot, environment, "client-a")
	initialBundle, initialBytes := h.callBundle(session, "memory_search initial", "memory_search", map[string]any{
		"query": query, "budget_bytes": searchBudget,
	})
	if initialBytes > searchBudget {
		t.Fatalf("initial search exceeded budget: %d", initialBytes)
	}
	assertBundle(t, initialBundle, "cinco segundos", "team-a")
	if len(initialBundle.Items) != 1 {
		t.Fatalf("initial ranking item count = %d", len(initialBundle.Items))
	}
	initialExpanded, expandedBytes := h.callBundle(session, "memory_expand initial", "memory_expand", map[string]any{
		"handle": initialBundle.Items[0].Handle, "budget_bytes": expandBudget,
	})
	if expandedBytes > expandBudget {
		t.Fatalf("initial expand bytes = %d", expandedBytes)
	}
	assertBundle(t, initialExpanded, "cinco segundos", "team-a")
	h.writeJSON("context-initial.json", initialBundle)

	proposalResult := h.callTool(session, "memory_propose_replace", proposalToolName, map[string]any{
		"target_path": targetPath, "section_id": targetSectionID,
		"replacement":  replacement,
		"source_paths": []string{"services/checkout-api/service.yaml"},
	})
	var proposed struct {
		SchemaVersion int `json:"schema_version"`
		Data          struct {
			ID       string          `json:"id"`
			Revision int             `json:"revision"`
			Target   proposal.Target `json:"target"`
		} `json:"data"`
	}
	decodeStructured(t, proposalResult, &proposed)
	if proposalResult.IsError || proposed.Data.ID == "" || proposed.Data.Revision != 1 || proposed.Data.Target.Path != targetPath {
		t.Fatal("proposal response was an error or omitted required summary fields")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	if got := hashFile(t, filepath.Join(memoryRoot, filepath.FromSlash(targetPath))); got != initialHash {
		t.Fatalf("proposal changed Markdown: %s", got)
	}
	if got := h.gitOutput(memoryRoot, "rev-parse", "HEAD"); got != initialHead {
		t.Fatalf("proposal changed HEAD: %s", got)
	}
	if got := h.gitOutput(memoryRoot, "ls-files", "--stage", "--debug"); got != indexBefore {
		t.Fatal("proposal changed index")
	}

	showOutput := h.cli(environment, "proposal show", "proposal", "show", proposed.Data.ID)
	var review protocol.Envelope[proposal.Review]
	decodeJSON(t, showOutput, &review)
	if review.Data.Proposal == nil || review.Data.Proposal.Diff == "" {
		t.Fatal("proposal review omitted the pending diff")
	}
	if !strings.Contains(review.Data.Proposal.Diff, targetPath) || strings.Contains(review.Data.Proposal.Diff, "notes/") {
		t.Fatal("proposal diff escaped the target path")
	}
	h.writeFile("proposal.diff", []byte(review.Data.Proposal.Diff))

	applyArgs := []string{"proposal", "apply", proposed.Data.ID, "--revision", "1"}
	applyOutput := h.cli(environment, "proposal apply", applyArgs...)
	applyAgain := h.cli(environment, "proposal apply repeat", applyArgs...)
	var applied, appliedAgain protocol.Envelope[proposal.ApplyReceipt]
	decodeJSON(t, applyOutput, &applied)
	decodeJSON(t, applyAgain, &appliedAgain)
	if !reflect.DeepEqual(applied.Data, appliedAgain.Data) {
		t.Fatal("apply retry changed its receipt")
	}

	publishArgs := []string{"proposal", "publish", proposed.Data.ID}
	publishOutput := h.cli(environment, "proposal publish", publishArgs...)
	publishAgain := h.cli(environment, "proposal publish repeat", publishArgs...)
	var published, publishedAgain protocol.Envelope[proposal.PublicationReceipt]
	decodeJSON(t, publishOutput, &published)
	decodeJSON(t, publishAgain, &publishedAgain)
	if !reflect.DeepEqual(published.Data, publishedAgain.Data) {
		t.Fatal("publish retry changed its receipt")
	}

	final := readFile(t, filepath.Join(memoryRoot, filepath.FromSlash(targetPath)))
	if !bytes.Equal(final, expected) {
		t.Fatal("final memory differs from expected fixture")
	}
	if !reflect.DeepEqual(anchors(string(initial)), anchors(string(final))) {
		t.Fatalf("anchors changed: %q != %q", anchors(string(initial)), anchors(string(final)))
	}
	commitPaths := fields(h.gitOutput(memoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"))
	if !reflect.DeepEqual(commitPaths, []string{targetPath}) {
		t.Fatalf("commit paths = %v", commitPaths)
	}
	if got := h.gitOutput(memoryRoot, "diff", "--cached", "--", "notes/staged.txt"); got != stagedBefore {
		t.Fatal("unrelated staging changed")
	}
	if got := string(readFile(t, filepath.Join(memoryRoot, "notes/unstaged.txt"))); got != "user unstaged change\n" {
		t.Fatalf("unrelated unstaged content = %q", got)
	}
	indexAfter := h.gitOutput(memoryRoot, "ls-files", "--stage", "--debug")
	h.writeFile("index-after.txt", []byte(indexAfter))
	h.writeFile("commit-tree.txt", []byte(strings.Join(commitPaths, "\n")+"\n"))
	h.writeFile("markdown.diff", []byte(h.gitOutput(memoryRoot, "show", "--format=", "--no-ext-diff", "HEAD", "--", targetPath)))

	proposalGone := missing(filepath.Join(stateRoot, "proposals", proposed.Data.ID+".json"))
	applyReceipt := readFile(t, filepath.Join(stateRoot, "applies", proposed.Data.ID+".json"))
	publicationReceipt := readFile(t, filepath.Join(stateRoot, "publications", proposed.Data.ID+".json"))
	receiptBodyFree := hasExactJSONKeys(t, applyReceipt, []string{
		"applied_at", "baseline", "memory_root", "post_state", "proposal_id", "revision", "schema_version", "status", "target",
	}) && hasExactJSONKeys(t, publicationReceipt, []string{
		"blob", "branch", "commit", "memory_root", "parent", "proposal_id", "published_at", "revision", "schema_version", "status", "target",
	})
	if !proposalGone || !receiptBodyFree || !missing(filepath.Join(stateRoot, "publishing", proposed.Data.ID+".json")) {
		t.Fatalf("terminal state retained payload or private marker")
	}
	h.writeFile("inventory-terminal.txt", []byte(h.inventory(runRoot)))
	for _, discardedRoot := range []string{codeRoot, stateRoot, home, configRoot} {
		if err := os.RemoveAll(discardedRoot); err != nil {
			t.Fatal(err)
		}
	}

	codeRootB := filepath.Join(runRoot, "code-b")
	homeB := filepath.Join(runRoot, "home-b")
	configB := filepath.Join(runRoot, "config-b")
	stateB := stateRootFor(t, configB, memoryRoot)
	for _, root := range []string{homeB, configB} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	h.seedRepository(codeRootB, nil)
	sessionB := h.connectMCP("MCP process B", memoryRoot, isolatedEnvironment(homeB, configB), "client-b")
	finalBundle, finalBytes := h.callBundle(sessionB, "memory_search final", "memory_search", map[string]any{
		"query": query, "budget_bytes": searchBudget,
	})
	if finalBytes > searchBudget {
		t.Fatalf("final search exceeded budget: %d", finalBytes)
	}
	assertBundle(t, finalBundle, "tres segundos", "team-b")
	if len(finalBundle.Items) != 1 || finalBundle.Items[0].Handle.Path != targetPath || finalBundle.Items[0].Handle.SectionID != targetSectionID {
		t.Fatal("final ranking omitted the expected target handle")
	}
	finalExpanded, finalExpandedBytes := h.callBundle(sessionB, "memory_expand final", "memory_expand", map[string]any{
		"handle": finalBundle.Items[0].Handle, "budget_bytes": expandBudget,
	})
	if finalExpandedBytes > expandBudget {
		t.Fatalf("final expand bytes = %d", finalExpandedBytes)
	}
	assertBundle(t, finalExpanded, "tres segundos", "team-b")
	if err := sessionB.Close(); err != nil {
		t.Fatal(err)
	}
	if !missing(stateB) {
		t.Fatal("read-only second process created state")
	}
	h.writeJSON("context-final.json", finalBundle)
	h.writeFile("inventory-after.txt", []byte(h.inventory(runRoot)))

	return manifest{
		SchemaVersion:     1,
		ProductRevision:   strings.TrimSpace(h.commandOutput(h.repository, os.Environ(), "git", "rev-parse", "HEAD")),
		ProductWorktree:   worktreeState(h.repository),
		GoVersion:         runtime.Version(),
		GitVersion:        strings.TrimSpace(h.commandOutput(h.repository, os.Environ(), "git", "--version")),
		ClaudeCodeVersion: optionalVersion("claude", "--version"),
		MCPSDKVersion:     h.binaryDependencyVersion("github.com/modelcontextprotocol/go-sdk"),
		Offline:           os.Getenv("GOPROXY") == "off",
		Configuration: map[string]string{
			"memory_root": "<run-root>/memory", "workspace_a": "<run-root>/code", "state_root_a": "<run-root>/state-a",
			"home_a": "<run-root>/home-a", "config_a": "<run-root>/config-a",
			"workspace_b": "<run-root>/code-b", "state_root_b": "<run-root>/state-b", "home_b": "<run-root>/home-b", "config_b": "<run-root>/config-b",
			"privacy_memory_root": "<privacy-root>/memory", "privacy_workspace": "<privacy-root>/code", "privacy_state_root": "<privacy-root>/state",
			"privacy_home": "<privacy-root>/home", "privacy_config": "<privacy-root>/config",
		},
		RankingCriterion:    "distinct query-token matches descending; path, section order, and section ID ascending",
		InitialContextBytes: initialBytes,
		InitialExpandBytes:  expandedBytes,
		FinalContextBytes:   finalBytes,
		FinalExpandBytes:    finalExpandedBytes,
		InitialFileHash:     initialHash,
		FinalFileHash:       hashBytes(final),
		ExpectedFileHash:    hashBytes(expected),
		PublicationCommit:   published.Data.Commit,
		CommitPaths:         commitPaths,
		ProposalPayloadGone: proposalGone,
		ReceiptBodyFree:     receiptBodyFree,
	}
}

func (h *harness) runPrivacy() (int, int) {
	t := h.t
	runRoot := t.TempDir()
	memoryRoot := filepath.Join(runRoot, "memory")
	codeRoot := filepath.Join(runRoot, "code")
	home := filepath.Join(runRoot, "home")
	configRoot := filepath.Join(runRoot, "config")
	for _, root := range []string{home, configRoot} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	h.seedRepository(memoryRoot, map[string][]byte{targetPath: h.fixture("memory-initial", targetPath)})
	stateRoot := stateRootFor(t, configRoot, memoryRoot)
	h.seedRepository(codeRoot, map[string][]byte{
		"services/checkout-api/service.yaml": h.fixture("code-repo", "services/checkout-api/service.yaml"),
	})
	markerSource := filepath.Join(codeRoot, "private-markers.txt")
	markers := h.phaseOneFixture("code-repo", "private-markers.txt")
	if err := os.WriteFile(markerSource, markers, 0o600); err != nil {
		t.Fatal(err)
	}

	traceDirectory := t.TempDir()
	tracePrefix := filepath.Join(traceDirectory, "privacy.trace")
	environment := isolatedEnvironment(home, configRoot)
	writeRuntimeConfig(t, memoryRoot, environment)
	command := exec.CommandContext(h.ctx, "strace", "-ff", "-qq", "-e", "trace=%file", "-o", tracePrefix,
		h.binary, "mcp",
	)
	command.Dir = runRoot
	command.Env = environment
	client := mcp.NewClient(&mcp.Implementation{Name: "privacy-client", Version: "test"}, nil)
	started := time.Now()
	session, err := client.Connect(h.ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	privacyCases := []struct {
		tool         string
		arguments    map[string]any
		expectedCode string
	}{
		{tool: proposalToolName, arguments: map[string]any{"text": "unstructured candidate"}},
		{tool: proposalToolName, arguments: map[string]any{"target_path": targetPath, "section_id": targetSectionID, "replacement": strings.Repeat("x", 128*1024+1), "source_paths": []string{}}, expectedCode: "limits_limit_exceeded"},
		{tool: proposalToolName, arguments: map[string]any{"target_path": targetPath, "section_id": targetSectionID, "replacement": string(markers), "source_paths": []string{}}, expectedCode: "privacy_sensitive_content"},
		{tool: proposalToolName, arguments: map[string]any{"target_path": targetPath, "section_id": targetSectionID, "replacement": replacement, "source_paths": markerLines(markers)}, expectedCode: "privacy_sensitive_content"},
		{tool: "memory_propose_create_page", arguments: map[string]any{"target_path": "private.md", "title": "Private", "body": string(markers), "source_paths": []string{}}, expectedCode: "privacy_sensitive_content"},
		{tool: "memory_propose_create_section", arguments: map[string]any{"target_path": targetPath, "heading": "Contact person@example.invalid", "body": "Body", "source_paths": []string{}}, expectedCode: "privacy_sensitive_content"},
	}
	for index, testCase := range privacyCases {
		caseStarted := time.Now()
		result, err := session.CallTool(h.ctx, &mcp.CallToolParams{Name: testCase.tool, Arguments: testCase.arguments})
		if err != nil {
			t.Fatal("privacy proposal call failed")
		}
		if !result.IsError {
			t.Fatal("privacy candidate was accepted")
		}
		if testCase.expectedCode != "" {
			var response struct {
				Error *struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			decodeStructured(t, result, &response)
			if response.Error == nil || response.Error.Code != testCase.expectedCode {
				t.Fatalf("privacy case %d returned the wrong rejection code", index+1)
			}
		}
		h.records = append(h.records, commandRecord{
			Name: fmt.Sprintf("privacy rejection %d", index+1), Command: "MCP proposal tool <redacted structured input>",
			ExitCode: 0, DurationMS: time.Since(caseStarted).Milliseconds(),
		})
	}
	search, err := session.CallTool(h.ctx, &mcp.CallToolParams{
		Name: "memory_search", Arguments: map[string]any{"query": query, "budget_bytes": searchBudget},
	})
	if err != nil || search.IsError {
		t.Fatal("privacy rejection blocked the MCP session")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	h.records = append(h.records, commandRecord{Name: "privacy MCP under file trace", Command: "strace -ff -e trace=%file gpm mcp <isolated-roots>", ExitCode: 0, DurationMS: time.Since(started).Milliseconds()})
	if !missing(stateRoot) {
		t.Fatal("privacy rejection created state")
	}

	markerValues := markerValues(markers)
	hits := scanMarkerHits(t, []string{runRoot, traceDirectory, h.evidenceRoot}, markerSource, markerValues)
	writeTargets := tracedWriteTargets(t, tracePrefix)
	h.writeFile("privacy-summary.txt", []byte(fmt.Sprintf(
		"trace=strace trace=%%file\nprivacy_cases_rejected=%d\nmarker_hits_outside_source=%d\nfile_write_targets=%d\nstate_root_created=false\n",
		len(privacyCases), hits, len(writeTargets),
	)))
	if hits != 0 || len(writeTargets) != 0 {
		t.Fatalf("privacy boundary failed: marker hits=%d, write target count=%d", hits, len(writeTargets))
	}
	return hits, len(writeTargets)
}

func (h *harness) connectMCP(name, memoryRoot string, environment []string, clientName string) *mcp.ClientSession {
	h.t.Helper()
	writeRuntimeConfig(h.t, memoryRoot, environment)
	command := exec.CommandContext(h.ctx, h.binary, "mcp")
	command.Env = environment
	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "test"}, nil)
	started := time.Now()
	session, err := client.Connect(h.ctx, &mcp.CommandTransport{Command: command}, nil)
	exitCode := 0
	if err != nil {
		exitCode = 1
	}
	h.records = append(h.records, commandRecord{Name: name, Command: "gpm mcp", ExitCode: exitCode, DurationMS: time.Since(started).Milliseconds()})
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = session.Close() })
	return session
}

func (h *harness) callBundle(session *mcp.ClientSession, recordName, toolName string, arguments map[string]any) (retrieval.ContextBundle, int) {
	h.t.Helper()
	result := h.callTool(session, recordName, toolName, arguments)
	var response struct {
		SchemaVersion int                     `json:"schema_version"`
		Data          retrieval.ContextBundle `json:"data"`
	}
	decodeStructured(h.t, result, &response)
	if result.IsError || response.SchemaVersion != 1 {
		h.t.Fatalf("%s returned an error or invalid schema", toolName)
	}
	canonical, err := json.Marshal(response.Data)
	if err != nil {
		h.t.Fatal(err)
	}
	return response.Data, len(canonical)
}

func (h *harness) callTool(session *mcp.ClientSession, recordName, toolName string, arguments map[string]any) *mcp.CallToolResult {
	h.t.Helper()
	started := time.Now()
	result, err := session.CallTool(h.ctx, &mcp.CallToolParams{Name: toolName, Arguments: arguments})
	exitCode := 0
	if err != nil || (result != nil && result.IsError) {
		exitCode = 1
	}
	h.records = append(h.records, commandRecord{Name: recordName, Command: "MCP tool " + toolName + " <structured-input>", ExitCode: exitCode, DurationMS: time.Since(started).Milliseconds()})
	if err != nil {
		h.t.Fatal(err)
	}
	return result
}

func (h *harness) cli(environment []string, name string, arguments ...string) []byte {
	h.t.Helper()
	started := time.Now()
	command := exec.CommandContext(h.ctx, h.binary, arguments...)
	command.Env = environment
	output, err := command.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitCode = 1
	}
	h.records = append(h.records, commandRecord{Name: name, Command: sanitizedCommand("gpm", arguments), ExitCode: exitCode, DurationMS: time.Since(started).Milliseconds()})
	if err != nil {
		h.t.Fatalf("%s failed: %v", name, err)
	}
	return output
}

func (h *harness) seedRepository(root string, files map[string][]byte) {
	h.t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		h.t.Fatal(err)
	}
	h.git(root, "init", "--quiet")
	h.git(root, "config", "user.name", "Phase 2 E2E")
	h.git(root, "config", "user.email", "phase2-e2e@example.invalid")
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			h.t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			h.t.Fatal(err)
		}
	}
	if len(files) > 0 {
		h.git(root, "add", ".")
		h.git(root, "commit", "--quiet", "-m", "seed synthetic fixture")
	}
}

func (h *harness) git(root string, arguments ...string) {
	h.t.Helper()
	h.run("git "+strings.Join(arguments, " "), root, isolatedEnvironment(filepath.Join(root, ".home"), filepath.Join(root, ".config")), "git", arguments...)
}

func (h *harness) gitOutput(root string, arguments ...string) string {
	h.t.Helper()
	started := time.Now()
	command := exec.CommandContext(h.ctx, "git", arguments...)
	command.Dir = root
	command.Env = isolatedEnvironment(filepath.Join(root, ".home"), filepath.Join(root, ".config"))
	output, err := command.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitCode = 1
	}
	h.records = append(h.records, commandRecord{Name: "git inspect " + strings.Join(arguments, " "), Command: sanitizedCommand("git", arguments), ExitCode: exitCode, DurationMS: time.Since(started).Milliseconds()})
	if err != nil {
		h.t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return string(output)
}

func (h *harness) run(name, directory string, environment []string, executable string, arguments ...string) {
	h.t.Helper()
	started := time.Now()
	command := exec.CommandContext(h.ctx, executable, arguments...)
	command.Dir = directory
	command.Env = environment
	output, err := command.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitCode = 1
	}
	h.records = append(h.records, commandRecord{Name: name, Command: sanitizedCommand(filepath.Base(executable), arguments), ExitCode: exitCode, DurationMS: time.Since(started).Milliseconds()})
	if err != nil {
		h.t.Fatalf("%s: %v: %s", name, err, output)
	}
}

func (h *harness) commandOutput(directory string, environment []string, executable string, arguments ...string) string {
	h.t.Helper()
	command := exec.CommandContext(h.ctx, executable, arguments...)
	command.Dir = directory
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		h.t.Fatalf("%s %v: %v: %s", executable, arguments, err, output)
	}
	return string(output)
}

func (h *harness) binaryDependencyVersion(path string) string {
	h.t.Helper()
	output := h.commandOutput(h.repository, os.Environ(), "go", "version", "-m", h.binary)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "dep" && fields[1] == path {
			return fields[1] + " " + fields[2]
		}
	}
	return "unavailable"
}

func (h *harness) fixture(parts ...string) []byte {
	h.t.Helper()
	return readFile(h.t, filepath.Join(append([]string{h.repository, "testdata", "phase-2"}, parts...)...))
}

func (h *harness) phaseOneFixture(parts ...string) []byte {
	h.t.Helper()
	return readFile(h.t, filepath.Join(append([]string{h.repository, "docs", "phase-1", "corpus"}, parts...)...))
}

func (h *harness) writeJSON(name string, value any) {
	h.t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		h.t.Fatal(err)
	}
	h.writeFile(name, append(encoded, '\n'))
}

func (h *harness) writeFile(name string, contents []byte) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.evidenceRoot, name), contents, 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) inventory(root string) string {
	h.t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "objects" && filepath.Base(filepath.Dir(path)) == ".git" {
			entries = append(entries, filepath.ToSlash(relative)+"/ <git-objects>")
			return filepath.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s %s %d", filepath.ToSlash(relative), info.Mode(), info.Size())
		if info.Mode().IsRegular() {
			line += " " + hashFile(h.t, path)
		}
		entries = append(entries, line)
		return nil
	})
	if err != nil {
		h.t.Fatal(err)
	}
	sort.Strings(entries)
	return strings.Join(entries, "\n") + "\n"
}

func isolatedEnvironment(home, configRoot string) []string {
	path := os.Getenv("PATH")
	return []string{
		"PATH=" + path,
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + configRoot,
		"XDG_STATE_HOME=" + filepath.Join(configRoot, "state-home"),
		"CLAUDE_CONFIG_DIR=" + filepath.Join(configRoot, "claude"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"LANG=C",
	}
}

func stateRootFor(t *testing.T, configRoot, memoryRoot string) string {
	t.Helper()
	command := exec.Command("git", "--no-optional-locks", "-C", memoryRoot, "rev-list", "--max-parents=0", "HEAD")
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	identity := fmt.Sprintf("%x", sha256.Sum256(bytes.TrimSpace(output)))[:16]
	return filepath.Join(configRoot, "state-home", "git-push-memories", identity)
}

func writeRuntimeConfig(t *testing.T, memoryRoot string, environment []string) {
	t.Helper()
	var configRoot string
	for _, entry := range environment {
		if strings.HasPrefix(entry, "XDG_CONFIG_HOME=") {
			configRoot = strings.TrimPrefix(entry, "XDG_CONFIG_HOME=")
			break
		}
	}
	directory := filepath.Join(configRoot, "git-push-memories")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "memory_root": memoryRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertBundle(t *testing.T, bundle retrieval.ContextBundle, expected ...string) {
	t.Helper()
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range expected {
		if !bytes.Contains(encoded, []byte(value)) {
			t.Fatalf("context does not contain expected fixture value %q", value)
		}
	}
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, target any) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	decodeJSON(t, encoded, target)
}

func decodeJSON(t *testing.T, encoded []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	return hashBytes(readFile(t, path))
}

func hashBytes(contents []byte) string {
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:])
}

func anchors(contents string) []string {
	var result []string
	for _, line := range strings.Split(contents, "\n") {
		if strings.Contains(line, "<!-- @id:") {
			result = append(result, line)
		}
	}
	return result
}

func fields(output string) []string {
	return strings.Fields(output)
}

func missing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

func markerValues(contents []byte) [][]byte {
	var values [][]byte
	for _, line := range bytes.Split(contents, []byte("\n")) {
		if separator := bytes.IndexByte(line, '='); separator >= 0 {
			values = append(values, append([]byte(nil), line[separator+1:]...))
		}
	}
	return values
}

func markerLines(contents []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.Contains(line, "=") {
			lines = append(lines, line)
		}
	}
	return lines
}

func scanMarkerHits(t *testing.T, roots []string, allowedSource string, markers [][]byte) int {
	t.Helper()
	hits := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !entry.Type().IsRegular() || path == allowedSource {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, marker := range markers {
				hits += bytes.Count(contents, marker)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return hits
}

func tracedWriteTargets(t *testing.T, tracePrefix string) []string {
	t.Helper()
	paths, err := filepath.Glob(tracePrefix + "*")
	if err != nil || len(paths) == 0 {
		t.Fatalf("privacy trace files = %v, err = %v", paths, err)
	}
	var targets []string
	writeOperations := []string{"O_WRONLY", "O_RDWR", "O_CREAT", "mkdir(", "mkdirat(", "rename(", "renameat(", "unlink(", "unlinkat(", "truncate("}
	for _, path := range paths {
		for _, line := range strings.Split(string(readFile(t, path)), "\n") {
			if strings.Contains(line, `"/dev/null"`) {
				continue
			}
			for _, operation := range writeOperations {
				if strings.Contains(line, operation) {
					targets = append(targets, operation)
					break
				}
			}
		}
	}
	sort.Strings(targets)
	return targets
}

func optionalVersion(executable string, arguments ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(output))
}

func worktreeState(repository string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", repository, "status", "--porcelain=v1")
	output, err := command.Output()
	if err != nil || len(output) != 0 {
		return "dirty"
	}
	return "clean"
}

func hasExactJSONKeys(t *testing.T, encoded []byte, expected []string) bool {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal("receipt is not a JSON object")
	}
	actual := make([]string, 0, len(object))
	for key := range object {
		actual = append(actual, key)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	return reflect.DeepEqual(actual, expected)
}

func sanitizedCommand(executable string, arguments []string) string {
	sanitized := make([]string, len(arguments))
	copy(sanitized, arguments)
	for index := range sanitized {
		if index > 0 {
			switch sanitized[index-1] {
			case "-o":
				sanitized[index] = "<output>"
			}
		}
		if len(sanitized[index]) == 32 {
			if _, err := hex.DecodeString(sanitized[index]); err == nil {
				sanitized[index] = "<proposal-id>"
			}
		}
	}
	return strings.Join(append([]string{executable}, sanitized...), " ")
}
