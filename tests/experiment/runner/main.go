package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type experiment struct {
	ExperimentID  string            `json:"experiment_id"`
	Model         string            `json:"model"`
	ClaudeVersion string            `json:"claude_code_version"`
	Effort        string            `json:"effort"`
	Autocompact   string            `json:"autocompact"`
	MaxBudget     float64           `json:"max_budget_usd_per_session"`
	SearchBudget  int               `json:"memory_search_budget_bytes"`
	Metric        string            `json:"primary_metric"`
	Threshold     float64           `json:"improvement_threshold"`
	RequiredPairs int               `json:"required_improved_pairs"`
	Bootstrap     bool              `json:"memory_bootstrap"`
	HumanTurns    []string          `json:"human_turns"`
	Profile       executionProfile  `json:"execution_profile"`
	FixtureHashes map[string]string `json:"fixture_hashes"`
	Pairs         []pair            `json:"pairs"`
}

type executionProfile struct {
	Filesystem        string   `json:"filesystem"`
	PermissionMode    string   `json:"permission_mode"`
	PermissionPrompts string   `json:"permission_prompts"`
	SessionATools     []string `json:"session_a_tools"`
	SessionADenied    []string `json:"session_a_denied_tools"`
	TaskTools         []string `json:"task_tools"`
	TaskDenied        []string `json:"task_denied_tools"`
}

type pair struct {
	Pair  int      `json:"pair"`
	Order []string `json:"order"`
}

type sessionMetric struct {
	RunID            string         `json:"run_id"`
	ExitCode         int            `json:"exit_code"`
	Authenticated    bool           `json:"authenticated_response"`
	Model            string         `json:"model"`
	DurationMS       int64          `json:"duration_ms"`
	CostUSD          float64        `json:"cost_usd"`
	Turns            int            `json:"turns"`
	ExplorationCalls int            `json:"code_exploration_tool_calls"`
	ToolCounts       map[string]int `json:"tool_counts"`
	MemoryRelevant   bool           `json:"memory_context_relevant"`
	ChoicePresented  bool           `json:"remember_choice_presented,omitempty"`
	ReviewPresented  bool           `json:"remember_review_presented,omitempty"`
	HumanTurns       int            `json:"remember_human_turns,omitempty"`
	SearchEmpty      bool           `json:"remember_search_empty,omitempty"`
}

type commandEvidence struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

type evaluation struct {
	RunID          string   `json:"run_id"`
	EvaluatorValid bool     `json:"evaluator_valid"`
	HiddenTests    bool     `json:"hidden_tests"`
	PublicTests    bool     `json:"public_tests"`
	APIPreserved   bool     `json:"api_preserved"`
	ScopeValid     bool     `json:"scope_valid"`
	Score          int      `json:"score"`
	Correctness    bool     `json:"correctness_gate"`
	ChangedPaths   []string `json:"changed_paths"`
	DiffHash       string   `json:"diff_hash"`
	PublicTestMS   int64    `json:"public_test_duration_ms"`
	APICheckMS     int64    `json:"api_check_duration_ms"`
	HiddenTestMS   int64    `json:"hidden_test_duration_ms"`
}

type comparison struct {
	Pair               int     `json:"pair"`
	TreatmentRun       string  `json:"treatment_run"`
	ControlRun         string  `json:"control_run"`
	TreatmentScore     int     `json:"treatment_score"`
	ControlScore       int     `json:"control_score"`
	TreatmentCalls     int     `json:"treatment_exploration_calls"`
	ControlCalls       int     `json:"control_exploration_calls"`
	TreatmentDuration  int64   `json:"treatment_duration_ms"`
	ControlDuration    int64   `json:"control_duration_ms"`
	Improvement        float64 `json:"improvement"`
	ThresholdMet       bool    `json:"threshold_met"`
	CorrectnessNoWorse bool    `json:"correctness_no_worse"`
	PairValid          bool    `json:"pair_valid"`
}

type report struct {
	SchemaVersion      int               `json:"schema_version"`
	ExperimentID       string            `json:"experiment_id"`
	ProductRevision    string            `json:"product_revision"`
	ProductWorktree    string            `json:"product_worktree"`
	ProtocolHash       string            `json:"protocol_hash"`
	CodeCommit         string            `json:"code_commit"`
	MemoryCommit       string            `json:"memory_commit"`
	Model              string            `json:"model_alias"`
	ClaudeVersion      string            `json:"claude_code_version"`
	GoVersion          string            `json:"go_version"`
	GitVersion         string            `json:"git_version"`
	MCPSDKVersion      string            `json:"mcp_sdk_version"`
	TaskContextBytes   int               `json:"memory_context_bytes"`
	VerificationPassed bool              `json:"verification_passed"`
	VerificationMS     int64             `json:"verification_duration_ms"`
	MemoryInitialTree  string            `json:"memory_initial_tree"`
	MemoryFinalTree    string            `json:"memory_final_tree"`
	MemoryIndexBefore  string            `json:"memory_index_before"`
	MemoryIndexAfter   string            `json:"memory_index_after"`
	MemoryCommitPaths  []string          `json:"memory_commit_paths"`
	MemoryDiffHash     string            `json:"memory_diff_hash"`
	PrivacyMarkerHits  int               `json:"privacy_marker_hits"`
	Verification       []commandEvidence `json:"verification"`
	EffectiveConfig    map[string]any    `json:"effective_configuration"`
	RankingCriterion   string            `json:"ranking_criterion"`
	FixtureHashes      map[string]string `json:"fixture_hashes"`
	CodeInitialPaths   []string          `json:"code_initial_paths"`
	SessionA           sessionMetric     `json:"session_a"`
	Sessions           []sessionMetric   `json:"sessions"`
	BlindEvaluations   []evaluation      `json:"blind_evaluations"`
	Assignments        map[string]string `json:"assignments"`
	Comparisons        []comparison      `json:"comparisons"`
	ImprovedPairs      int               `json:"improved_pairs"`
	Decision           string            `json:"decision"`
	Evidence           map[string]any    `json:"evidence"`
}

type runner struct {
	ctx              context.Context
	repository       string
	fixture          string
	baseFixture      string
	evidence         string
	temporary        string
	authRoot         string
	credentials      string
	verificationFile string
	sourceHash       string
	preregistration  string
	binary           string
	claudeBinary     string
	protocol         experiment
}

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runMain() error {
	printSourceHash := flag.Bool("print-source-hash", false, "print the current non-artifact repository source hash")
	evidence := flag.String("evidence-root", "", "new directory for sanitized evidence")
	credentials := flag.String("credentials", filepath.Join(os.Getenv("HOME"), ".claude", ".credentials.json"), "Claude credential source copied only to tmpfs")
	authRoot := flag.String("auth-root", "", "existing private tmpfs directory controlled by the caller")
	verificationFile := flag.String("verification-file", "", "sanitized pre-run verification TSV")
	protocolRoot := flag.String("protocol-root", "", "alternate experiment fixture root")
	sourceHash := flag.String("source-hash", "", "pre-run hash of bootstrap experiment sources")
	preregistration := flag.String("preregistration-file", "", "read-only bootstrap pre-run receipt")
	flag.Parse()
	if *printSourceHash {
		repository, err := commandOutput(context.Background(), "", nil, "git", "rev-parse", "--show-toplevel")
		if err != nil {
			return errors.New("locate repository")
		}
		sourceHash, err := experimentSourceHash(strings.TrimSpace(repository))
		if err != nil {
			return err
		}
		fmt.Println(sourceHash)
		return nil
	}
	if *evidence == "" {
		return errors.New("evidence root is required")
	}
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalContext, 35*time.Minute)
	defer cancel()
	r, err := newRunner(ctx, *evidence, *credentials, *authRoot, *verificationFile, *protocolRoot, *sourceHash, *preregistration)
	if err != nil {
		return err
	}
	defer os.RemoveAll(r.temporary)
	defer os.RemoveAll(r.authRoot)
	if err := r.execute(); err != nil {
		return err
	}
	fmt.Println(r.evidence)
	return nil
}

func newRunner(ctx context.Context, evidence, credentials, requestedAuthRoot, verificationFile, protocolRoot, sourceHash, preregistration string) (*runner, error) {
	repository, err := commandOutput(ctx, "", nil, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, errors.New("locate repository")
	}
	repository = strings.TrimSpace(repository)
	status, err := commandOutput(ctx, repository, nil, "git", "status", "--porcelain=v1")
	if err != nil || (status != "" && protocolRoot == "") {
		return nil, errors.New("experiment requires a clean product worktree")
	}
	if _, err := os.Stat(evidence); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("evidence root already exists")
	}
	if err := os.MkdirAll(evidence, 0o700); err != nil {
		return nil, errors.New("create evidence root")
	}
	temporary, err := os.MkdirTemp("", "gpm-value-experiment-")
	if err != nil {
		return nil, err
	}
	authRoot := requestedAuthRoot
	if authRoot == "" {
		authRoot, err = os.MkdirTemp("/dev/shm", "gpm-value-auth-")
		if err != nil {
			os.RemoveAll(temporary)
			return nil, errors.New("create tmpfs auth root")
		}
	} else if !strings.HasPrefix(filepath.Clean(authRoot), "/dev/shm/") {
		_ = os.RemoveAll(temporary)
		return nil, errors.New("auth root must be under /dev/shm")
	} else if entries, readErr := os.ReadDir(authRoot); readErr != nil || len(entries) != 0 {
		_ = os.RemoveAll(temporary)
		return nil, errors.New("auth root must be an empty existing directory")
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(temporary)
			_ = os.RemoveAll(authRoot)
		}
	}()
	baseFixture := filepath.Join(repository, "testdata", "phase-2", "value-experiment")
	fixture := baseFixture
	if protocolRoot != "" {
		fixture, err = filepath.Abs(protocolRoot)
		if err != nil || !strings.HasPrefix(filepath.Clean(fixture), repository+string(filepath.Separator)) {
			return nil, errors.New("protocol root must be inside repository")
		}
	}
	r := &runner{
		ctx: ctx, repository: repository, fixture: fixture, baseFixture: baseFixture,
		evidence: evidence, temporary: temporary, authRoot: authRoot, credentials: credentials, verificationFile: verificationFile, sourceHash: sourceHash, preregistration: preregistration,
	}
	if err := decodeFile(filepath.Join(r.fixture, "experiment.json"), &r.protocol); err != nil {
		return nil, err
	}
	if protocolRoot != "" && !r.protocol.Bootstrap {
		return nil, errors.New("alternate protocol must be a bootstrap experiment")
	}
	if r.protocol.Bootstrap {
		actualSourceHash, hashErr := experimentSourceHash(repository)
		if hashErr != nil || sourceHash == "" || sourceHash != actualSourceHash {
			return nil, errors.New("bootstrap experiment source hash mismatch")
		}
		if err := r.validatePreregistration(); err != nil {
			return nil, err
		}
	}
	claudeBinary, err := exec.LookPath("claude")
	if err != nil {
		return nil, errors.New("locate Claude Code")
	}
	r.claudeBinary, err = filepath.EvalSymlinks(claudeBinary)
	if err != nil {
		return nil, errors.New("resolve Claude Code")
	}
	if version := optionalOutput(ctx, r.claudeBinary, "--version"); !strings.HasPrefix(version, r.protocol.ClaudeVersion+" ") {
		return nil, errors.New("Claude Code version does not match pre-registration")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		return nil, errors.New("bubblewrap is required")
	}
	r.binary = filepath.Join(temporary, "git-push-memories")
	if err := run(ctx, repository, nil, "go", "build", "-o", r.binary, "./cmd/git-push-memories"); err != nil {
		return nil, errors.New("build product")
	}
	success = true
	return r, nil
}

func (r *runner) validatePreregistration() error {
	contents, err := os.ReadFile(r.preregistration)
	if err != nil {
		return errors.New("read bootstrap pre-registration")
	}
	revision := strings.TrimSpace(mustOutput(r.ctx, r.repository, nil, "git", "rev-parse", "HEAD"))
	expected := []string{
		"experiment_id=" + r.protocol.ExperimentID,
		"status=started",
		"approved=true",
		"revision_base=" + revision,
		"protocol_hash=" + hashFile(filepath.Join(r.fixture, "experiment.json")),
		"source_hash=" + r.sourceHash,
		"evidence=" + r.evidence,
	}
	if !equalStrings(strings.Split(strings.TrimSpace(string(contents)), "\n"), expected) {
		return errors.New("bootstrap pre-registration mismatch")
	}
	return nil
}

func (r *runner) execute() error {
	if err := r.verifyFixtureHashes(); err != nil {
		return err
	}
	if r.protocol.Profile.Filesystem != "bubblewrap-workspace-only" || r.protocol.Profile.PermissionMode != "acceptEdits" || r.protocol.Profile.PermissionPrompts != "none" {
		return errors.New("unsupported pre-registered execution profile")
	}
	verification, err := readVerification(r.verificationFile)
	if err != nil || len(verification) != 5 {
		return errors.New("invalid pre-run verification evidence")
	}
	verificationMS := int64(0)
	for _, evidence := range verification {
		verificationMS += evidence.DurationMS
		if evidence.ExitCode != 0 {
			return errors.New("pre-run verification failed")
		}
	}
	memoryRoot := filepath.Join(r.temporary, "memory")
	codeRoot := filepath.Join(r.temporary, "code")
	if r.protocol.Bootstrap {
		if err := os.MkdirAll(memoryRoot, 0o755); err != nil {
			return err
		}
	} else {
		if err := copyTree(filepath.Join(r.fixture, "memory-initial"), memoryRoot); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join(r.baseFixture, "code-seed"), codeRoot); err != nil {
		return err
	}
	if err := seedGit(r.ctx, memoryRoot, "seed memory", r.protocol.Bootstrap); err != nil {
		return err
	}
	if err := seedGit(r.ctx, codeRoot, "seed code", false); err != nil {
		return err
	}
	memoryInitialTree := mustOutput(r.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD^{tree}")
	memoryIndexBefore := hashFile(filepath.Join(memoryRoot, ".git", "index"))
	codeCommit := mustOutput(r.ctx, codeRoot, nil, "git", "rev-parse", "HEAD")
	codeInitialPaths := strings.Fields(mustOutput(r.ctx, codeRoot, nil, "git", "ls-tree", "-r", "--name-only", "HEAD"))
	expectedAPI, err := commandOutput(r.ctx, codeRoot, nil, "go", "doc", "-all", "./dispatch")
	if err != nil {
		return errors.New("capture initial exported API")
	}

	sessionAConfig, sessionAEnv, cleanup, err := r.sessionConfig("session-a", codeRoot, memoryRoot, filepath.Join(r.temporary, "state-a"), true)
	if err != nil {
		return err
	}
	var sessionA sessionMetric
	if r.protocol.Bootstrap {
		sessionA, err = r.runRememberSession(codeRoot, sessionAEnv, sessionAConfig)
	} else {
		sessionA, err = r.runClaude("session-a", codeRoot, sessionAEnv, sessionAConfig, filepath.Join(r.fixture, "session-a-system.md"), filepath.Join(r.fixture, "session-a.md"), false)
	}
	cleanup()
	expectedProposalTool := "mcp__git-push-memories__memory_propose_replace"
	if r.protocol.Bootstrap {
		expectedProposalTool = "mcp__git-push-memories__memory_propose_create_page"
	}
	if err != nil || !sessionA.Authenticated || sessionA.ToolCounts[expectedProposalTool] != 1 {
		return errors.New("authenticated discovery session failed")
	}
	if r.protocol.Bootstrap && (!sessionA.ChoicePresented || !sessionA.ReviewPresented || !sessionA.SearchEmpty || sessionA.HumanTurns != 2 || sessionA.ToolCounts["mcp__git-push-memories__memory_search"] != 1) {
		return errors.New("authenticated remember flow did not match pre-registration")
	}
	if dirty := mustOutput(r.ctx, codeRoot, nil, "git", "status", "--porcelain=v1"); dirty != "" {
		return errors.New("discovery session changed code repository")
	}
	proposalID, err := onlyProposalID(filepath.Join(r.temporary, "state-a", "proposals"))
	if err != nil {
		return err
	}
	if err := r.approveProposal(memoryRoot, codeRoot, filepath.Join(r.temporary, "state-a"), proposalID); err != nil {
		return err
	}
	memoryCommit := mustOutput(r.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	memoryFinalTree := mustOutput(r.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD^{tree}")
	memoryIndexAfter := hashFile(filepath.Join(memoryRoot, ".git", "index"))
	memoryCommitPaths := strings.Fields(mustOutput(r.ctx, memoryRoot, nil, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"))
	memoryDiff := mustOutput(r.ctx, memoryRoot, nil, "git", "show", "--format=", "--no-ext-diff", "HEAD")
	contextBytes, err := r.validatePublishedContext(memoryRoot, codeRoot)
	if err != nil {
		return err
	}

	assignments := make(map[string]string, 6)
	worktrees := make(map[string]string, 6)
	var sessions []sessionMetric
	runNumber := 0
	for _, registeredPair := range r.protocol.Pairs {
		for _, arm := range registeredPair.Order {
			runNumber++
			runID := fmt.Sprintf("run-%02d", runNumber)
			workspace := filepath.Join(r.temporary, runID)
			if err := run(r.ctx, "", nil, "git", "clone", "--quiet", "--no-hardlinks", codeRoot, workspace); err != nil {
				return errors.New("create task worktree")
			}
			if mustOutput(r.ctx, workspace, nil, "git", "rev-parse", "HEAD") != codeCommit {
				return errors.New("task worktree commit mismatch")
			}
			withMemory := arm == "treatment"
			mcpConfig, environment, removeAuth, err := r.sessionConfig(runID, workspace, memoryRoot, filepath.Join(r.temporary, "state-"+runID), withMemory)
			if err != nil {
				return err
			}
			metric, sessionErr := r.runClaude(runID, workspace, environment, mcpConfig, filepath.Join(r.baseFixture, "task-system.md"), filepath.Join(r.baseFixture, "task.md"), withMemory)
			removeAuth()
			if sessionErr != nil {
				metric.ExitCode = 1
			}
			sessions = append(sessions, metric)
			assignments[runID] = arm
			worktrees[runID] = workspace
		}
	}

	var evaluations []evaluation
	for index := 1; index <= 6; index++ {
		runID := fmt.Sprintf("run-%02d", index)
		evaluations = append(evaluations, r.evaluate(runID, worktrees[runID], expectedAPI))
	}
	comparisons, improved, decision := decide(r.protocol, sessions, evaluations, assignments)
	modelConsistent := sessionA.Model != ""
	for _, metric := range sessions {
		modelConsistent = modelConsistent && metric.Model == sessionA.Model
	}
	if !modelConsistent && decision == "continue" {
		decision = "revise_flow"
	}
	markerFixture, _ := os.ReadFile(filepath.Join(r.repository, "docs", "phase-1", "corpus", "code-repo", "private-markers.txt"))
	privacyMarkerHits, markerErr := countMarkerHits([]string{r.temporary, r.authRoot, r.evidence}, markerValues(markerFixture))
	if markerErr != nil {
		return errors.New("privacy marker scan failed")
	}
	if privacyMarkerHits != 0 && decision == "continue" {
		decision = "revise_flow"
	}
	revision := strings.TrimSpace(mustOutput(r.ctx, r.repository, nil, "git", "rev-parse", "HEAD"))
	worktreeState := "clean"
	if r.protocol.Bootstrap {
		worktreeState = "source-hash-verified"
	}
	report := report{
		SchemaVersion: 1, ExperimentID: r.protocol.ExperimentID, ProductRevision: revision, ProductWorktree: worktreeState,
		ProtocolHash: hashFile(filepath.Join(r.fixture, "experiment.json")), CodeCommit: strings.TrimSpace(codeCommit), MemoryCommit: strings.TrimSpace(memoryCommit),
		Model: r.protocol.Model, ClaudeVersion: optionalOutput(r.ctx, r.claudeBinary, "--version"), GoVersion: optionalOutput(r.ctx, "go", "version"), GitVersion: optionalOutput(r.ctx, "git", "--version"), MCPSDKVersion: binaryDependencyVersion(r.ctx, r.binary, "github.com/modelcontextprotocol/go-sdk"), TaskContextBytes: contextBytes,
		VerificationPassed: true, VerificationMS: verificationMS, Verification: verification, MemoryInitialTree: memoryInitialTree, MemoryFinalTree: memoryFinalTree,
		MemoryIndexBefore: memoryIndexBefore, MemoryIndexAfter: memoryIndexAfter, MemoryCommitPaths: memoryCommitPaths, MemoryDiffHash: hashBytes([]byte(memoryDiff)), PrivacyMarkerHits: privacyMarkerHits,
		EffectiveConfig:  map[string]any{"model": r.protocol.Model, "claude_code_version": r.protocol.ClaudeVersion, "effort": r.protocol.Effort, "autocompact": r.protocol.Autocompact, "max_budget_usd_per_session": r.protocol.MaxBudget, "permission_mode": r.protocol.Profile.PermissionMode, "permission_prompts": r.protocol.Profile.PermissionPrompts, "filesystem": r.protocol.Profile.Filesystem, "task_tools": r.protocol.Profile.TaskTools, "task_denied_tools": r.protocol.Profile.TaskDenied, "paths": "isolated placeholders only"},
		RankingCriterion: "distinct query-token matches descending; path, section order, and section ID ascending", FixtureHashes: r.protocol.FixtureHashes, CodeInitialPaths: codeInitialPaths,
		SessionA: sessionA, Sessions: sessions, BlindEvaluations: evaluations, Assignments: assignments, Comparisons: comparisons, ImprovedPairs: improved, Decision: decision,
		Evidence: map[string]any{"raw_responses_persisted": false, "credentials_persisted": false, "session_state_persisted": false, "model_consistent": modelConsistent, "pairs": 3, "runs": 6, "memory_bootstrap": r.protocol.Bootstrap, "source_hash": r.sourceHash},
	}
	if err := writeJSON(filepath.Join(r.evidence, "report.json"), report); err != nil {
		return err
	}
	return r.writeChecksums()
}

func (r *runner) verifyFixtureHashes() error {
	paths := map[string]string{
		"session_a": "session-a.md", "session_a_system": "session-a-system.md", "task": "task.md", "task_system": "task-system.md",
		"rubric": "rubric.json", "memory_initial": "memory-initial/domains/parcel-dispatch.md", "memory_expected": "memory-expected/domains/parcel-dispatch.md",
		"code_go_mod": "code-seed/go.mod", "policy_initial": "code-seed/dispatch/policy.go", "policy_public_test": "code-seed/dispatch/policy_test.go",
		"current_alerts": "code-seed/deploy/production-alerts.yaml", "queue_ownership": "code-seed/ops/queues.yaml", "legacy_policy": "code-seed/docs/legacy-policy.md",
		"hidden_evaluator": "evaluator/policy_hidden_test.go",
	}
	if r.protocol.Bootstrap {
		paths = map[string]string{
			"session_a": "session-a.md", "session_a_system": "session-a-system.md", "memory_expected": "memory-expected/parcel-dispatch.md",
		}
		shared := map[string]string{
			"task": "task.md", "task_system": "task-system.md", "rubric": "rubric.json", "code_go_mod": "code-seed/go.mod",
			"policy_initial": "code-seed/dispatch/policy.go", "policy_public_test": "code-seed/dispatch/policy_test.go",
			"current_alerts": "code-seed/deploy/production-alerts.yaml", "queue_ownership": "code-seed/ops/queues.yaml",
			"legacy_policy": "code-seed/docs/legacy-policy.md", "hidden_evaluator": "evaluator/policy_hidden_test.go",
		}
		for name, relative := range shared {
			if hashFile(filepath.Join(r.baseFixture, filepath.FromSlash(relative))) != r.protocol.FixtureHashes[name] {
				return fmt.Errorf("fixture hash mismatch: %s", name)
			}
		}
		if hashFile(filepath.Join(r.repository, "integrations", "claude-code", "skills", "remember", "SKILL.md")) != r.protocol.FixtureHashes["remember_skill"] {
			return errors.New("fixture hash mismatch: remember_skill")
		}
	}
	for name, relative := range paths {
		if hashFile(filepath.Join(r.fixture, filepath.FromSlash(relative))) != r.protocol.FixtureHashes[name] {
			return fmt.Errorf("fixture hash mismatch: %s", name)
		}
	}
	return nil
}

func (r *runner) sessionConfig(name, codeRoot, memoryRoot, stateRoot string, withMemory bool) (string, []string, func(), error) {
	root := filepath.Join(r.authRoot, name)
	configRoot := filepath.Join(root, "config")
	home := filepath.Join(r.temporary, "home-"+name)
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		return "", nil, nil, err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return "", nil, nil, err
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return "", nil, nil, err
	}
	if name == "session-a" && r.protocol.Bootstrap {
		skillTarget := filepath.Join(configRoot, "skills", "remember", "SKILL.md")
		if err := copyFile(filepath.Join(r.repository, "integrations", "claude-code", "skills", "remember", "SKILL.md"), skillTarget); err != nil {
			return "", nil, nil, err
		}
	}
	credentials, err := os.ReadFile(r.credentials)
	if err != nil {
		return "", nil, nil, errors.New("read Claude credentials")
	}
	if err := os.WriteFile(filepath.Join(configRoot, ".credentials.json"), credentials, 0o600); err != nil {
		return "", nil, nil, errors.New("stage credentials in tmpfs")
	}
	for index := range credentials {
		credentials[index] = 0
	}
	mcpConfig := filepath.Join(configRoot, "mcp.json")
	servers := map[string]any{}
	if withMemory {
		servers["git-push-memories"] = map[string]any{
			"type": "stdio", "command": "/opt/git-push-memories",
			"args": []string{"mcp", "--memory-root", "/memory", "--code-root", "/workspace", "--state-root", "/state"},
		}
	}
	if err := writeJSON(mcpConfig, map[string]any{"mcpServers": servers}); err != nil {
		return "", nil, nil, err
	}
	environment := []string{
		"GPM_HOST_CONFIG=" + configRoot, "GPM_HOST_HOME=" + home, "GPM_HOST_STATE=" + stateRoot,
		"GPM_HOST_MEMORY=" + memoryRoot, "GPM_WITH_MEMORY=" + fmt.Sprint(withMemory),
	}
	return "/config/mcp.json", environment, func() { _ = os.RemoveAll(root) }, nil
}

func (r *runner) runClaude(runID, workspace string, environment []string, mcpConfig, systemPath, promptPath string, treatment bool) (sessionMetric, error) {
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return sessionMetric{RunID: runID}, err
	}
	arguments := []string{
		"--print", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--no-chrome", "--restricted",
		"--strict-mcp-config", "--mcp-config", mcpConfig, "--setting-sources", "", "--model", r.protocol.Model,
		"--effort", r.protocol.Effort, "--autocompact", r.protocol.Autocompact, "--max-budget-usd", fmt.Sprintf("%.2f", r.protocol.MaxBudget),
		"--permission-mode", r.protocol.Profile.PermissionMode, "--permission-prompts", r.protocol.Profile.PermissionPrompts, "--append-system-prompt-file", "/input/system.md",
	}
	if runID == "session-a" {
		arguments = append(arguments,
			"--tools", "Read,Grep,Glob",
			"--allowed-tools", strings.Join(r.protocol.Profile.SessionATools, ","),
			"--disallowed-tools", strings.Join(r.protocol.Profile.SessionADenied, ","),
		)
	} else {
		arguments = append(arguments,
			"--tools", "Read,Grep,Glob,Edit,Write,Bash",
			"--allowed-tools", strings.Join(r.protocol.Profile.TaskTools, ","),
			"--disallowed-tools", strings.Join(r.protocol.Profile.TaskDenied, ","),
		)
	}
	configRoot := environmentValue(environment, "GPM_HOST_CONFIG")
	home := environmentValue(environment, "GPM_HOST_HOME")
	stateRoot := environmentValue(environment, "GPM_HOST_STATE")
	memoryRoot := environmentValue(environment, "GPM_HOST_MEMORY")
	withMemory := environmentValue(environment, "GPM_WITH_MEMORY") == "true"
	bwrap := []string{
		"--clearenv", "--die-with-parent", "--new-session", "--unshare-pid", "--unshare-uts", "--unshare-ipc",
		"--ro-bind", "/usr", "/usr", "--ro-bind", "/bin", "/bin", "--ro-bind", "/lib", "/lib", "--ro-bind", "/lib64", "/lib64",
		"--ro-bind", "/etc/ssl", "/etc/ssl", "--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf", "--ro-bind", "/etc/hosts", "/etc/hosts",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--dir", "/home", "--dir", "/opt", "--dir", "/input",
		"--bind", configRoot, "/config", "--bind", home, "/home/experiment", "--bind", stateRoot, "/state",
		"--ro-bind", r.binary, "/opt/git-push-memories", "--ro-bind", r.claudeBinary, "/opt/claude",
		"--ro-bind", systemPath, "/input/system.md",
	}
	if runID == "session-a" {
		bwrap = append(bwrap, "--ro-bind", workspace, "/workspace")
	} else {
		bwrap = append(bwrap, "--bind", workspace, "/workspace")
	}
	if withMemory {
		mount := "--ro-bind"
		if runID == "session-a" {
			mount = "--bind"
		}
		bwrap = append(bwrap, mount, memoryRoot, "/memory")
	}
	bwrap = append(bwrap,
		"--chdir", "/workspace", "--setenv", "PATH", "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
		"--setenv", "HOME", "/home/experiment", "--setenv", "CLAUDE_CONFIG_DIR", "/config", "--setenv", "XDG_CONFIG_HOME", "/config/xdg",
		"--setenv", "TMPDIR", "/tmp", "--setenv", "GOPROXY", "off", "--setenv", "GOTOOLCHAIN", "go1.25.12",
		"--setenv", "GIT_CONFIG_NOSYSTEM", "1", "--setenv", "GIT_CONFIG_GLOBAL", "/dev/null", "--", "/opt/claude",
	)
	bwrap = append(bwrap, arguments...)
	command := exec.CommandContext(r.ctx, "bwrap", bwrap...)
	command.Stdin = bytes.NewReader(prompt)
	command.WaitDelay = 5 * time.Second
	stdout, err := command.StdoutPipe()
	if err != nil {
		return sessionMetric{RunID: runID}, err
	}
	command.Stderr = io.Discard
	started := time.Now()
	if err := command.Start(); err != nil {
		return sessionMetric{RunID: runID}, err
	}
	metric := sessionMetric{RunID: runID, ToolCounts: map[string]int{}}
	scanErr := scanClaudeStream(stdout, treatment, &metric)
	waitErr := command.Wait()
	metric.DurationMS = time.Since(started).Milliseconds()
	if scanErr != nil || waitErr != nil {
		metric.ExitCode = 1
		return metric, errors.New("Claude session failed")
	}
	if metric.ExitCode != 0 || !metric.Authenticated {
		return metric, errors.New("Claude session returned an error")
	}
	return metric, nil
}

func (r *runner) runRememberSession(workspace string, environment []string, mcpConfig string) (sessionMetric, error) {
	prompt, err := os.ReadFile(filepath.Join(r.fixture, "session-a.md"))
	if err != nil {
		return sessionMetric{RunID: "session-a"}, err
	}
	arguments := []string{
		"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--no-chrome", "--restricted",
		"--strict-mcp-config", "--mcp-config", mcpConfig, "--setting-sources", "", "--model", r.protocol.Model,
		"--effort", r.protocol.Effort, "--autocompact", r.protocol.Autocompact, "--max-budget-usd", fmt.Sprintf("%.2f", r.protocol.MaxBudget),
		"--permission-mode", r.protocol.Profile.PermissionMode, "--permission-prompts", r.protocol.Profile.PermissionPrompts,
		"--append-system-prompt-file", "/input/system.md", "--tools", "",
		"--allowed-tools", strings.Join(r.protocol.Profile.SessionATools, ","), "--disallowed-tools", strings.Join(r.protocol.Profile.SessionADenied, ","),
	}
	configRoot := environmentValue(environment, "GPM_HOST_CONFIG")
	home := environmentValue(environment, "GPM_HOST_HOME")
	stateRoot := environmentValue(environment, "GPM_HOST_STATE")
	memoryRoot := environmentValue(environment, "GPM_HOST_MEMORY")
	bwrap := []string{
		"--clearenv", "--die-with-parent", "--new-session", "--unshare-pid", "--unshare-uts", "--unshare-ipc",
		"--ro-bind", "/usr", "/usr", "--ro-bind", "/bin", "/bin", "--ro-bind", "/lib", "/lib", "--ro-bind", "/lib64", "/lib64",
		"--ro-bind", "/etc/ssl", "/etc/ssl", "--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf", "--ro-bind", "/etc/hosts", "/etc/hosts",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--dir", "/home", "--dir", "/opt", "--dir", "/input",
		"--bind", configRoot, "/config", "--bind", home, "/home/experiment", "--bind", stateRoot, "/state",
		"--ro-bind", r.binary, "/opt/git-push-memories", "--ro-bind", r.claudeBinary, "/opt/claude",
		"--ro-bind", filepath.Join(r.fixture, "session-a-system.md"), "/input/system.md", "--ro-bind", workspace, "/workspace", "--bind", memoryRoot, "/memory",
		"--chdir", "/workspace", "--setenv", "PATH", "/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
		"--setenv", "HOME", "/home/experiment", "--setenv", "CLAUDE_CONFIG_DIR", "/config", "--setenv", "XDG_CONFIG_HOME", "/config/xdg",
		"--setenv", "TMPDIR", "/tmp", "--setenv", "GOPROXY", "off", "--setenv", "GOTOOLCHAIN", "go1.25.12",
		"--setenv", "GIT_CONFIG_NOSYSTEM", "1", "--setenv", "GIT_CONFIG_GLOBAL", "/dev/null", "--", "/opt/claude",
	}
	bwrap = append(bwrap, arguments...)
	command := exec.CommandContext(r.ctx, "bwrap", bwrap...)
	command.WaitDelay = 5 * time.Second
	stdin, err := command.StdinPipe()
	if err != nil {
		return sessionMetric{RunID: "session-a"}, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return sessionMetric{RunID: "session-a"}, err
	}
	command.Stderr = io.Discard
	started := time.Now()
	if err := command.Start(); err != nil {
		return sessionMetric{RunID: "session-a"}, err
	}
	encoder := json.NewEncoder(stdin)
	send := func(text string) error {
		return encoder.Encode(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
	}
	if err := send(string(prompt)); err != nil {
		_ = stdin.Close()
		_ = command.Wait()
		return sessionMetric{RunID: "session-a"}, err
	}
	for index := range prompt {
		prompt[index] = 0
	}
	metric := sessionMetric{RunID: "session-a", ToolCounts: map[string]int{}}
	searchToolIDs := map[string]bool{}
	phase := 0
	var responseText strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var event map[string]any
		if json.Unmarshal(line, &event) == nil && event["type"] == "assistant" {
			message, _ := event["message"].(map[string]any)
			if model, ok := message["model"].(string); ok && metric.Model == "" {
				metric.Model = model
			}
			contents, _ := message["content"].([]any)
			for _, raw := range contents {
				content, _ := raw.(map[string]any)
				if text, ok := content["text"].(string); ok {
					responseText.WriteString(text)
				}
				if content["type"] == "tool_use" {
					name, _ := content["name"].(string)
					metric.ToolCounts[name]++
					if name == "mcp__git-push-memories__memory_propose_create_page" && phase != 2 {
						metric.ExitCode = 1
					}
					if name == "mcp__git-push-memories__memory_search" {
						if id, ok := content["id"].(string); ok {
							searchToolIDs[id] = true
						}
					}
				}
			}
		}
		if json.Unmarshal(line, &event) == nil && event["type"] == "user" {
			message, _ := event["message"].(map[string]any)
			contents, _ := message["content"].([]any)
			for _, raw := range contents {
				content, _ := raw.(map[string]any)
				id, _ := content["tool_use_id"].(string)
				if content["type"] == "tool_result" && searchToolIDs[id] {
					encoded, _ := json.Marshal(content["content"])
					metric.SearchEmpty = bytes.Contains(encoded, []byte(`\"items\":[]`)) || bytes.Contains(encoded, []byte(`"items":[]`))
					for index := range encoded {
						encoded[index] = 0
					}
				}
			}
		}
		if json.Unmarshal(line, &event) == nil && event["type"] == "result" {
			metric.Authenticated = true
			metric.CostUSD, _ = event["total_cost_usd"].(float64)
			if turns, ok := event["num_turns"].(float64); ok {
				metric.Turns = int(turns)
			}
			if isError, _ := event["is_error"].(bool); isError {
				metric.ExitCode = 1
				_ = stdin.Close()
				continue
			}
			text := responseText.String()
			switch phase {
			case 0:
				metric.ChoicePresented = strings.Count(text, "create a root page") == 1 && strings.Count(text, "cancel") == 1 && !strings.Contains(text, "create a section") && !strings.Contains(text, "replace")
				if len(r.protocol.HumanTurns) != 2 || !metric.ChoicePresented || !metric.SearchEmpty || send(r.protocol.HumanTurns[0]) != nil {
					metric.ExitCode = 1
					_ = stdin.Close()
				} else {
					metric.HumanTurns++
					phase++
					responseText.Reset()
				}
			case 1:
				expectedBody := "- Priority parcels escalate after 90 seconds through `route-amber`.\n- The active queue is owned by `routing-ops`."
				metric.ReviewPresented = strings.Count(text, "parcel-dispatch.md") == 1 && strings.Count(text, "Parcel dispatch") == 1 && strings.Count(text, expectedBody) == 1 && strings.Count(text, "deploy/production-alerts.yaml") == 1 && strings.Count(text, "ops/queues.yaml") == 1
				if !metric.ReviewPresented || send(r.protocol.HumanTurns[1]) != nil {
					metric.ExitCode = 1
					_ = stdin.Close()
				} else {
					metric.HumanTurns++
					phase++
					responseText.Reset()
				}
			default:
				_ = stdin.Close()
			}
		}
		for index := range line {
			line[index] = 0
		}
	}
	metric.DurationMS = time.Since(started).Milliseconds()
	waitErr := command.Wait()
	if scanner.Err() != nil || waitErr != nil || metric.ExitCode != 0 || phase != 2 || metric.ToolCounts["mcp__git-push-memories__memory_propose_create_page"] != 1 {
		metric.ExitCode = 1
		return metric, errors.New("interactive Claude remember session failed")
	}
	return metric, nil
}

func scanClaudeStream(reader io.Reader, treatment bool, metric *sessionMetric) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	memoryToolIDs := map[string]bool{}
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var event map[string]any
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if event["type"] == "assistant" {
			message, _ := event["message"].(map[string]any)
			if model, ok := message["model"].(string); ok && metric.Model == "" {
				metric.Model = model
			}
			contents, _ := message["content"].([]any)
			for _, raw := range contents {
				content, _ := raw.(map[string]any)
				if content["type"] != "tool_use" {
					continue
				}
				name, _ := content["name"].(string)
				metric.ToolCounts[name]++
				if name == "mcp__git-push-memories__memory_search" {
					if id, ok := content["id"].(string); ok {
						memoryToolIDs[id] = true
					}
				}
				if name == "Read" || name == "Grep" || name == "Glob" {
					metric.ExplorationCalls++
				}
			}
		}
		if treatment && event["type"] == "user" {
			message, _ := event["message"].(map[string]any)
			contents, _ := message["content"].([]any)
			for _, raw := range contents {
				content, _ := raw.(map[string]any)
				toolUseID, _ := content["tool_use_id"].(string)
				if content["type"] != "tool_result" || !memoryToolIDs[toolUseID] {
					continue
				}
				encoded, _ := json.Marshal(content["content"])
				metric.MemoryRelevant = bytes.Contains(encoded, []byte("90 seconds")) && bytes.Contains(encoded, []byte("route-amber")) && bytes.Contains(encoded, []byte("routing-ops"))
				for index := range encoded {
					encoded[index] = 0
				}
			}
		}
		if event["type"] == "result" {
			metric.Authenticated = true
			metric.CostUSD, _ = event["total_cost_usd"].(float64)
			if turns, ok := event["num_turns"].(float64); ok {
				metric.Turns = int(turns)
			}
			if isError, _ := event["is_error"].(bool); isError {
				metric.ExitCode = 1
			}
		}
		for index := range line {
			line[index] = 0
		}
	}
	return scanner.Err()
}

func (r *runner) approveProposal(memoryRoot, codeRoot, stateRoot, id string) error {
	common := []string{"--memory-root", memoryRoot, "--code-root", codeRoot, "--state-root", stateRoot}
	show, err := commandOutput(r.ctx, "", isolatedRuntime(), r.binary, append([]string{"proposal", "show", id}, common...)...)
	if err != nil {
		return errors.New("show discovery proposal")
	}
	var envelope struct {
		Data struct {
			Proposal *struct {
				Revision  int `json:"revision"`
				Operation struct {
					Type   string `json:"type"`
					Target struct {
						Path string `json:"path"`
					} `json:"target"`
					Title       string   `json:"title"`
					Body        string   `json:"body"`
					Replacement string   `json:"replacement"`
					SourcePaths []string `json:"source_paths"`
				} `json:"operation"`
				PostState struct {
					FileHash string `json:"file_hash"`
				} `json:"post_state"`
				SourcePaths []string `json:"source_paths"`
			} `json:"proposal"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(show), &envelope) != nil || envelope.Data.Proposal == nil {
		return errors.New("decode discovery proposal")
	}
	expectedPath := filepath.Join(r.fixture, "memory-expected", "domains", "parcel-dispatch.md")
	if r.protocol.Bootstrap {
		expectedPath = filepath.Join(r.fixture, "memory-expected", "parcel-dispatch.md")
	}
	expectedMemory, _ := os.ReadFile(expectedPath)
	sources := append([]string(nil), envelope.Data.Proposal.SourcePaths...)
	sort.Strings(sources)
	validOperation := envelope.Data.Proposal.Operation.Replacement == sectionBody(string(expectedMemory))
	if r.protocol.Bootstrap {
		expectedBody := "- Priority parcels escalate after 90 seconds through `route-amber`.\n- The active queue is owned by `routing-ops`."
		body := strings.TrimSuffix(envelope.Data.Proposal.Operation.Body, "\n")
		validOperation = envelope.Data.Proposal.Operation.Type == "create_page" && envelope.Data.Proposal.Operation.Target.Path == "parcel-dispatch.md" && envelope.Data.Proposal.Operation.Title == "Parcel dispatch" && body == expectedBody && envelope.Data.Proposal.PostState.FileHash == memoryHash(expectedMemory)
	}
	if envelope.Data.Proposal.Revision != 1 || !validOperation || !equalStrings(sources, []string{"deploy/production-alerts.yaml", "ops/queues.yaml"}) {
		return errors.New("proposal does not match pre-registered memory")
	}
	if _, err := commandOutput(r.ctx, "", isolatedRuntime(), r.binary, append([]string{"proposal", "apply", id, "--revision", "1"}, common...)...); err != nil {
		return errors.New("apply discovery proposal")
	}
	if _, err := commandOutput(r.ctx, "", isolatedRuntime(), r.binary, append([]string{"proposal", "publish", id}, common...)...); err != nil {
		return errors.New("publish discovery proposal")
	}
	actualPath := filepath.Join(memoryRoot, "domains", "parcel-dispatch.md")
	if r.protocol.Bootstrap {
		actualPath = filepath.Join(memoryRoot, "parcel-dispatch.md")
	}
	actual, _ := os.ReadFile(actualPath)
	if !bytes.Equal(actual, expectedMemory) {
		return errors.New("published memory differs from pre-registration")
	}
	return nil
}

func (r *runner) validatePublishedContext(memoryRoot, codeRoot string) (int, error) {
	output, err := commandOutput(r.ctx, "", isolatedRuntime(), r.binary, "retrieve", "priority parcel escalation timeout queue owner", "--memory-root", memoryRoot, "--code-root", codeRoot, "--budget-bytes", fmt.Sprint(r.protocol.SearchBudget))
	if err != nil {
		return 0, errors.New("retrieve published memory")
	}
	if !strings.Contains(output, "90 seconds") || !strings.Contains(output, "route-amber") || !strings.Contains(output, "routing-ops") {
		return 0, errors.New("published context is not relevant")
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal([]byte(output), &envelope) != nil || len(envelope.Data) > r.protocol.SearchBudget {
		return 0, errors.New("published context exceeds budget")
	}
	return len(envelope.Data), nil
}

func (r *runner) evaluate(runID, workspace, expectedAPI string) evaluation {
	evaluation := evaluation{RunID: runID, EvaluatorValid: true}
	publicStarted := time.Now()
	publicErr := run(r.ctx, workspace, nil, "go", "test", "./...")
	evaluation.PublicTestMS = time.Since(publicStarted).Milliseconds()
	evaluation.PublicTests = publicErr == nil
	apiStarted := time.Now()
	actualAPI, apiErr := commandOutput(r.ctx, workspace, nil, "go", "doc", "-all", "./dispatch")
	evaluation.APICheckMS = time.Since(apiStarted).Milliseconds()
	evaluation.APIPreserved = apiErr == nil && actualAPI == expectedAPI
	hidden := filepath.Join(workspace, "dispatch", "zz_gpm_evaluator_test.go")
	if _, err := os.Stat(hidden); !errors.Is(err, os.ErrNotExist) {
		evaluation.EvaluatorValid = false
	} else if err := copyFile(filepath.Join(r.baseFixture, "evaluator", "policy_hidden_test.go"), hidden); err != nil {
		evaluation.EvaluatorValid = false
	} else {
		hiddenStarted := time.Now()
		evaluation.HiddenTests = run(r.ctx, workspace, nil, "go", "test", "./...") == nil
		evaluation.HiddenTestMS = time.Since(hiddenStarted).Milliseconds()
		if err := os.Remove(hidden); err != nil {
			evaluation.EvaluatorValid = false
			evaluation.HiddenTests = false
		}
	}
	diff, _ := commandOutput(r.ctx, workspace, nil, "git", "diff", "--binary")
	paths := strings.Fields(mustOutput(r.ctx, workspace, nil, "git", "diff", "--name-only"))
	untracked := strings.Fields(mustOutput(r.ctx, workspace, nil, "git", "ls-files", "--others", "--exclude-standard"))
	paths = append(paths, untracked...)
	sort.Strings(paths)
	for _, path := range untracked {
		contents, _ := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
		diff += "\x00" + path + "\x00" + string(contents)
	}
	evaluation.ChangedPaths = paths
	evaluation.DiffHash = hashBytes([]byte(diff))
	evaluation.ScopeValid = len(paths) > 0
	for _, path := range paths {
		if !strings.HasPrefix(path, "dispatch/") || !strings.HasSuffix(path, ".go") {
			evaluation.ScopeValid = false
		}
	}
	if evaluation.HiddenTests {
		evaluation.Score += 4
	}
	if evaluation.PublicTests {
		evaluation.Score += 2
	}
	if evaluation.APIPreserved {
		evaluation.Score += 2
	}
	if evaluation.ScopeValid {
		evaluation.Score += 2
	}
	evaluation.Correctness = evaluation.EvaluatorValid && evaluation.HiddenTests && evaluation.PublicTests && evaluation.APIPreserved
	return evaluation
}

func decide(protocol experiment, sessions []sessionMetric, evaluations []evaluation, assignments map[string]string) ([]comparison, int, string) {
	metrics := map[string]sessionMetric{}
	scores := map[string]evaluation{}
	operationalFailure := false
	for _, metric := range sessions {
		metrics[metric.RunID] = metric
	}
	for _, score := range evaluations {
		scores[score.RunID] = score
		if !score.EvaluatorValid {
			operationalFailure = true
		}
	}
	var comparisons []comparison
	improved := 0
	for _, metric := range sessions {
		arm := assignments[metric.RunID]
		memoryCalls := metric.ToolCounts["mcp__git-push-memories__memory_search"]
		if metric.ExitCode != 0 || !metric.Authenticated || metric.Model == "" ||
			(arm == "treatment" && (memoryCalls != 1 || !metric.MemoryRelevant)) ||
			(arm == "control" && memoryCalls != 0) {
			operationalFailure = true
		}
	}
	for pairIndex := 1; pairIndex <= 3; pairIndex++ {
		var treatment, control string
		for runID, arm := range assignments {
			runNumber := 0
			fmt.Sscanf(runID, "run-%02d", &runNumber)
			if (runNumber-1)/2+1 != pairIndex {
				continue
			}
			if arm == "treatment" {
				treatment = runID
			} else {
				control = runID
			}
		}
		treatmentMetric, controlMetric := metrics[treatment], metrics[control]
		treatmentCalls, controlCalls := treatmentMetric.ExplorationCalls, controlMetric.ExplorationCalls
		treatmentValue, controlValue := float64(treatmentCalls), float64(controlCalls)
		if protocol.Metric == "task_duration_ms" {
			treatmentValue, controlValue = float64(treatmentMetric.DurationMS), float64(controlMetric.DurationMS)
		}
		improvement := 0.0
		if controlValue > 0 {
			improvement = (controlValue - treatmentValue) / controlValue
		}
		thresholdMet := improvement >= protocol.Threshold
		if thresholdMet {
			improved++
		}
		noWorse := scores[treatment].Correctness && scores[treatment].Score >= scores[control].Score
		pairValid := scores[treatment].EvaluatorValid && scores[control].EvaluatorValid && metrics[treatment].ExitCode == 0 && metrics[control].ExitCode == 0 && metrics[treatment].Authenticated && metrics[control].Authenticated
		comparisons = append(comparisons, comparison{Pair: pairIndex, TreatmentRun: treatment, ControlRun: control, TreatmentScore: scores[treatment].Score, ControlScore: scores[control].Score, TreatmentCalls: treatmentCalls, ControlCalls: controlCalls, TreatmentDuration: treatmentMetric.DurationMS, ControlDuration: controlMetric.DurationMS, Improvement: improvement, ThresholdMet: thresholdMet, CorrectnessNoWorse: noWorse, PairValid: pairValid})
	}
	for _, comparison := range comparisons {
		if comparison.PairValid && !comparison.CorrectnessNoWorse {
			return comparisons, improved, "did_not_demonstrate_value"
		}
	}
	if operationalFailure {
		return comparisons, improved, "revise_flow"
	}
	if improved < protocol.RequiredPairs {
		return comparisons, improved, "revise_flow"
	}
	return comparisons, improved, "continue"
}

func onlyProposalID(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".json") {
		return "", errors.New("discovery session did not create exactly one proposal")
	}
	return strings.TrimSuffix(entries[0].Name(), ".json"), nil
}

func sectionBody(page string) string {
	anchor := "<!-- @id: parcel-dispatch-priority-escalation -->\n"
	index := strings.Index(page, anchor)
	if index < 0 {
		return ""
	}
	return page[index+len(anchor):]
}

func seedGit(ctx context.Context, root, message string, allowEmpty bool) error {
	commit := []string{"commit", "--quiet", "-m", message}
	if allowEmpty {
		commit = []string{"commit", "--quiet", "--allow-empty", "-m", message}
	}
	commands := [][]string{{"init", "--quiet"}, {"config", "user.name", "Value Experiment"}, {"config", "user.email", "value-experiment@example.invalid"}, {"add", "."}, commit}
	for _, arguments := range commands {
		if err := run(ctx, root, nil, "git", arguments...); err != nil {
			return err
		}
	}
	return nil
}

func isolatedRuntime() []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LANG=C", "GOPROXY=off", "GOTOOLCHAIN=go1.25.12"}
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

func run(ctx context.Context, directory string, environment []string, executable string, arguments ...string) error {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func readVerification(path string) ([]commandEvidence, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) != 6 || lines[0] != "name\tcommand\texit_code\tduration_ms" {
		return nil, errors.New("invalid verification TSV")
	}
	expectedNames := []string{"tests", "race", "vet", "build", "fault-injection"}
	result := make([]commandEvidence, 0, 5)
	for index, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 || fields[0] != expectedNames[index] {
			return nil, errors.New("invalid verification row")
		}
		exitCode, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, err
		}
		duration, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || duration < 0 {
			return nil, errors.New("invalid verification duration")
		}
		result = append(result, commandEvidence{Name: fields[0], Command: fields[1], ExitCode: exitCode, DurationMS: duration})
	}
	return result, nil
}

func commandOutput(ctx context.Context, directory string, environment []string, executable string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = io.Discard
	err := command.Run()
	return strings.TrimSpace(output.String()), err
}

func mustOutput(ctx context.Context, directory string, environment []string, executable string, arguments ...string) string {
	output, err := commandOutput(ctx, directory, environment, executable, arguments...)
	if err != nil {
		panic("required command failed")
	}
	return output
}

func optionalOutput(ctx context.Context, executable string, arguments ...string) string {
	output, err := commandOutput(ctx, "", nil, executable, arguments...)
	if err != nil {
		return "unavailable"
	}
	return output
}

func binaryDependencyVersion(ctx context.Context, binary, dependency string) string {
	output := optionalOutput(ctx, "go", "version", "-m", binary)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "dep" && fields[1] == dependency {
			return fields[1] + " " + fields[2]
		}
	}
	return "unavailable"
}

func decodeFile(path string, target any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, target)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o600)
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(source, destination string) error {
	contents, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, contents, 0o644)
}

func hashFile(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "unavailable"
	}
	return hashBytes(contents)
}

func hashBytes(contents []byte) string {
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:])
}

func memoryHash(contents []byte) string {
	return "sha256:" + hashBytes(contents)
}

func experimentSourceHash(repository string) (string, error) {
	var paths []string
	err := filepath.WalkDir(repository, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, relErr := filepath.Rel(repository, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() && (relative == ".git" || relative == ".artifacts" || strings.HasPrefix(relative, ".git"+string(filepath.Separator)) || strings.HasPrefix(relative, ".artifacts"+string(filepath.Separator))) {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() {
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hasher := sha256.New()
	for _, relative := range paths {
		contents, readErr := os.ReadFile(filepath.Join(repository, filepath.FromSlash(relative)))
		if readErr != nil {
			return "", readErr
		}
		fmt.Fprintf(hasher, "%s\x00%d\x00", relative, len(contents))
		_, _ = hasher.Write(contents)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func markerValues(contents []byte) [][]byte {
	var markers [][]byte
	for _, line := range bytes.Split(contents, []byte("\n")) {
		if separator := bytes.IndexByte(line, '='); separator >= 0 {
			markers = append(markers, append([]byte(nil), line[separator+1:]...))
		}
	}
	return markers
}

func countMarkerHits(roots []string, markers [][]byte) (int, error) {
	hits := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !entry.Type().IsRegular() {
				return nil
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, marker := range markers {
				hits += bytes.Count(contents, marker)
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return hits, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (r *runner) writeChecksums() error {
	entries, err := os.ReadDir(r.evidence)
	if err != nil {
		return err
	}
	var lines []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.sha256" {
			continue
		}
		lines = append(lines, hashFile(filepath.Join(r.evidence, entry.Name()))+"  "+entry.Name())
	}
	sort.Strings(lines)
	return os.WriteFile(filepath.Join(r.evidence, "checksums.sha256"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
