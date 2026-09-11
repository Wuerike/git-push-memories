package experiment

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type RunOptions struct {
	EvidenceRoot     string
	CredentialPath   string
	VerificationFile string
	RunID            string
	Approved         bool
	Input            io.Reader
	Output           io.Writer
}

type experimentRunner struct {
	ctx               context.Context
	config            runnerConfig
	options           RunOptions
	temporary         string
	authRoot          string
	binary            string
	claudeBinary      string
	goRoot            string
	store             *evidenceStore
	reviewer          reviewFunc
	reader            *bufio.Reader
	output            io.Writer
	records           ExperimentRecords
	proposalPath      map[string]string
	captureDurations  map[string]int64
	reviewLabels      map[string]curationLabels
	taskWorktrees     map[string]string
	previewValues     [][]byte
	blindnessAttested bool
	modelPreflight    SessionObservation
}

type runReport struct {
	SchemaVersion   int              `json:"schema_version"`
	ExperimentID    string           `json:"experiment_id"`
	RunnerRevision  string           `json:"runner_revision"`
	ProductRevision string           `json:"product_revision"`
	ProtocolHash    string           `json:"protocol_hash"`
	ModelID         string           `json:"model_id"`
	ModelPreflight  SessionObservation `json:"model_preflight"`
	Decision        Decision         `json:"decision"`
	Outcome         GateOutcome      `json:"outcome"`
	Pairs           []PairComparison `json:"pairs"`
	Thresholds      map[string]bool  `json:"threshold_results"`
	Capture         CaptureSummary   `json:"capture"`
	Curations       []CurationRecord `json:"curations"`
	Tasks           []TaskRecord     `json:"tasks"`
	Attempts        []attemptRecord  `json:"validity_records"`
}

func Run(ctx context.Context, options RunOptions) (resultErr error) {
	if !options.Approved {
		return errors.New("authenticated evaluation requires explicit approval")
	}
	if options.Input == nil {
		options.Input = os.Stdin
	}
	if options.Output == nil {
		options.Output = os.Stdout
	}
	repository, err := commandText(ctx, "", nil, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("locate repository")
	}
	repository = strings.TrimSpace(repository)
	config, err := loadRunnerConfig(repository)
	if err != nil {
		return err
	}
	if err := validateRunEnvironment(ctx, config, options); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "gpm-phase3-value-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	authRoot, err := os.MkdirTemp("/dev/shm", "gpm-phase3-auth-")
	if err != nil {
		return errors.New("create tmpfs auth root")
	}
	defer os.RemoveAll(authRoot)
	runnerRevision, err := commandText(ctx, repository, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return errors.New("resolve runner revision")
	}
	store, err := createEvidenceStore(options.EvidenceRoot, ledgerIdentity{
		ExperimentID: config.Protocol.ExperimentID, RunID: options.RunID, ProductRevision: config.Protocol.ProductRevision,
		ProtocolHash: Phase3ProtocolSHA256, Approved: true,
	}, time.Now())
	if err != nil {
		return err
	}
	runner := &experimentRunner{
		ctx: ctx, config: config, options: options, temporary: temporary, authRoot: authRoot, store: store,
		reader: bufio.NewReader(options.Input), output: options.Output, proposalPath: make(map[string]string), captureDurations: make(map[string]int64),
		reviewLabels: make(map[string]curationLabels), taskWorktrees: make(map[string]string),
	}
	runner.reviewer = runner.review
	defer func() {
		if resultErr != nil {
			if err := runner.store.finish("aborted", time.Now()); err != nil {
				resultErr = errors.Join(resultErr, err)
			}
			if err := runner.store.writeChecksums(); err != nil {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	if err := runner.attestBlindness(); err != nil {
		return err
	}
	if err := runner.writePreregistration(runnerRevision); err != nil {
		return err
	}
	if err := runner.prepareRuntime(); err != nil {
		return err
	}
	modelID, err := runner.runModelPreflight()
	if err != nil {
		return err
	}
	runner.records.ExpectedModel = modelID
	runner.records.Privacy.SecretRejected = false
	memoryRoot, sourceRoot, codeRoot, err := runner.prepareRepositories()
	if err != nil {
		return err
	}
	if err := runner.runCaptures(memoryRoot, sourceRoot); err != nil {
		return err
	}
	if err := runner.runCurations(memoryRoot, sourceRoot); err != nil {
		return err
	}
	if err := runner.runTasks(memoryRoot, codeRoot); err != nil {
		return err
	}
	evaluations, err := runner.evaluateBlindly()
	if err != nil {
		return err
	}
	for index := range runner.records.Tasks {
		evaluation := evaluations[runner.records.Tasks[index].RunID]
		runner.records.Tasks[index].Evaluation = BlindEvaluation{EvaluatorValid: evaluation.EvaluatorValid, Correctness: evaluation.Correctness, Score: evaluation.Score}
	}
	if err := runner.store.writeJSON("curation-results.json", map[string]any{"schema_version": 1, "results": runner.records.Curations}); err != nil {
		return err
	}
	if err := runner.writeGitManifest(memoryRoot); err != nil {
		return err
	}
	privacy, err := runner.runPrivacyScan(memoryRoot)
	if err != nil {
		return err
	}
	runner.records.Privacy.Incidents = privacy.FileHits + privacy.GitObjectHits
	runner.records.Privacy.RawEvidence = !privacy.Clean
	if err := runner.store.writeJSON("privacy-summary.json", privacy); err != nil {
		return err
	}
	outcome, comparisons, err := Aggregate(runner.records)
	if err != nil {
		return err
	}
	decision := Decide(outcome)
	if err := runner.store.writeJSON("report.json", runReport{
		SchemaVersion: 1, ExperimentID: config.Protocol.ExperimentID, RunnerRevision: runnerRevision, ProductRevision: config.Protocol.ProductRevision,
		ProtocolHash: Phase3ProtocolSHA256, ModelID: modelID, ModelPreflight: runner.modelPreflight, Decision: decision, Outcome: outcome, Pairs: comparisons, Thresholds: ThresholdResults(outcome),
		Capture: runner.records.Capture, Curations: runner.records.Curations, Tasks: runner.records.Tasks, Attempts: runner.store.ledger.Attempts,
	}); err != nil {
		return err
	}
	finalPrivacy, err := runner.runPrivacyScan(memoryRoot)
	if err != nil || finalPrivacy.FileHits != privacy.FileHits || finalPrivacy.GitObjectHits != privacy.GitObjectHits {
		return errors.New("final evidence privacy validation failed")
	}
	if err := runner.store.finish("completed", time.Now()); err != nil {
		return err
	}
	if err := runner.store.writeChecksums(); err != nil {
		return err
	}
	fmt.Fprintf(runner.output, "Decision: %s\nEvidence: %s\n", decision, options.EvidenceRoot)
	return nil
}

func validateRunEnvironment(ctx context.Context, config runnerConfig, options RunOptions) error {
	if options.RunID == "" || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`).MatchString(options.RunID) || options.EvidenceRoot == "" || options.VerificationFile == "" {
		return errors.New("run ID, evidence root and verification file are required")
	}
	expectedParent := filepath.Join(config.Repository, ".artifacts", "phase-3")
	evidenceRoot, err := filepath.Abs(options.EvidenceRoot)
	if err != nil || filepath.Dir(evidenceRoot) != expectedParent || filepath.Base(evidenceRoot) != options.RunID {
		return errors.New("evidence root must be .artifacts/phase-3/<run-id>")
	}
	if attempted, err := experimentAlreadyAttempted(expectedParent, config.Protocol.ExperimentID); err != nil {
		return err
	} else if attempted {
		return errors.New("this pre-registered experiment already has an attempt")
	}
	status, err := commandText(ctx, config.Repository, nil, "git", "status", "--porcelain=v1")
	if err != nil || status != "" {
		return errors.New("evaluation requires a clean worktree")
	}
	checks := []struct {
		executable string
		arguments  []string
		want       string
	}{
		{"go", []string{"version"}, "go version " + config.Protocol.Environment.GoVersion + " linux/amd64"},
		{"git", []string{"--version"}, "git version " + config.Protocol.Environment.GitVersion},
		{"bwrap", []string{"--version"}, "bubblewrap " + config.Protocol.Environment.BubblewrapVersion},
		{"claude", []string{"--version"}, config.Protocol.Provider.ClaudeVersion + " (Claude Code)"},
	}
	for _, check := range checks {
		output, err := commandText(ctx, "", nil, check.executable, check.arguments...)
		if err != nil || strings.TrimSpace(output) != check.want {
			return fmt.Errorf("runtime version mismatch: %s", check.executable)
		}
	}
	socat, err := combinedCommandText(ctx, "", nil, "socat", "-V")
	if err != nil || !strings.Contains(socat, "socat version "+config.Protocol.Environment.SocatVersion) {
		return errors.New("runtime version mismatch: socat")
	}
	verification, err := os.ReadFile(options.VerificationFile)
	if err != nil || !validVerificationTSV(verification) {
		return errors.New("invalid outer verification evidence")
	}
	if _, err := os.Stat(options.CredentialPath); err != nil {
		return errors.New("Claude credential source is unavailable")
	}
	return nil
}

func experimentAlreadyAttempted(parent, experimentID string) (bool, error) {
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(parent, entry.Name(), "ledger.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		var ledger struct {
			ExperimentID string `json:"experiment_id"`
		}
		if json.Unmarshal(contents, &ledger) != nil {
			return false, errors.New("existing phase 3 ledger is invalid")
		}
		if ledger.ExperimentID == experimentID {
			return true, nil
		}
	}
	return false, nil
}

func (runner *experimentRunner) writePreregistration(runnerRevision string) error {
	var protocol any
	if err := decodeStrictFile(filepath.Join(runner.config.Fixture, "protocol.json"), &protocol); err != nil {
		return err
	}
	if err := runner.store.writeJSON("protocol.json", protocol); err != nil {
		return err
	}
	verification, err := os.ReadFile(runner.options.VerificationFile)
	if err != nil {
		return err
	}
	if !validVerificationTSV(verification) {
		return errors.New("verification evidence changed after validation")
	}
	if err := runner.store.writeFile("verification.tsv", verification); err != nil {
		return err
	}
	return runner.store.writeJSON("preregistration.json", map[string]any{
		"schema_version": 1, "experiment_id": runner.config.Protocol.ExperimentID, "run_id": runner.options.RunID,
		"runner_revision": runnerRevision, "product_revision": runner.config.Protocol.ProductRevision, "protocol_hash": Phase3ProtocolSHA256,
		"approved": true, "reviewer_blindness_attested": runner.blindnessAttested, "maximum_total_usd": runner.config.Protocol.Provider.MaximumTotalUSD,
	})
}

func (runner *experimentRunner) attestBlindness() error {
	fmt.Fprintln(runner.output, "Before inference, attest that you have not opened corpus expected_* fields, rubric, assignments, evaluator, or prior outcomes since this ledger started.")
	fmt.Fprint(runner.output, "Type exactly `I attest to reviewer blindness.`: ")
	value, err := runner.reader.ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) != "I attest to reviewer blindness." {
		return errors.New("reviewer blindness was not attested")
	}
	runner.blindnessAttested = true
	return nil
}

func (runner *experimentRunner) prepareRuntime() error {
	claudeBinary, err := exec.LookPath("claude")
	if err != nil {
		return err
	}
	runner.claudeBinary, err = filepath.EvalSymlinks(claudeBinary)
	if err != nil {
		return err
	}
	runner.goRoot, err = commandText(runner.ctx, runner.config.Repository, cleanEnvironment(), "go", "env", "GOROOT")
	if err != nil || !filepath.IsAbs(runner.goRoot) {
		return errors.New("resolve Go toolchain root")
	}
	if err := runCommandContext(runner.ctx, runner.config.Repository, nil, "git", "diff", "--quiet", runner.config.Protocol.ProductRevision, "--", "cmd", "internal", "integrations", "go.mod", "go.sum"); err != nil {
		return errors.New("current product sources differ from the frozen revision")
	}
	runner.binary = filepath.Join(runner.temporary, "bin", "gpm")
	if err := os.MkdirAll(filepath.Dir(runner.binary), 0o700); err != nil {
		return err
	}
	moduleCache, err := commandText(runner.ctx, runner.config.Repository, nil, "go", "env", "GOMODCACHE")
	if err != nil || !filepath.IsAbs(moduleCache) {
		return errors.New("resolve Go module cache")
	}
	environment := append(cleanEnvironment(), "GOMODCACHE="+moduleCache, "GOCACHE="+filepath.Join(runner.temporary, "go-cache"))
	return runCommandContext(runner.ctx, runner.config.Repository, environment, "go", "build", "-trimpath", "-buildvcs=false", "-o", runner.binary, "./cmd/gpm")
}

func (runner *experimentRunner) runModelPreflight() (string, error) {
	empty := filepath.Join(runner.temporary, "preflight-workspace")
	memory := filepath.Join(runner.temporary, "preflight-memory")
	for _, path := range []string{empty, memory} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", err
		}
	}
	execRunner := runner.executor()
	observation, _, err := execRunner.run(runner.ctx, sessionRequest{
		UnitID: "model-preflight", Kind: "model_preflight", Workspace: empty, MemoryRoot: memory,
		StateRoot: filepath.Join(runner.temporary, "state", "model-preflight"), Prompt: "Reply with OK.",
		MaxBudgetUSD: runner.config.Protocol.Provider.MaximumPreflightUSD, Timeout: 2 * time.Minute,
	}, nil)
	valid := err == nil && observation.Authenticated && observation.Model != "" && withinBudget(observation, runner.config.Protocol.Provider.MaximumPreflightUSD)
	runner.modelPreflight = observation
	if ledgerErr := runner.store.recordAttempt(attemptRecord{UnitID: "model-preflight", Kind: "model_preflight", State: terminalState(valid), Valid: valid, InvalidReason: invalidReason(err)}); ledgerErr != nil {
		return "", ledgerErr
	}
	if !valid {
		return "", errors.New("authenticated model preflight failed")
	}
	return observation.Model, nil
}

func (runner *experimentRunner) prepareRepositories() (string, string, string, error) {
	memoryRoot := filepath.Join(runner.temporary, "memory")
	sourceRoot := filepath.Join(runner.temporary, "source")
	codeRoot := filepath.Join(runner.temporary, "code")
	if err := copyDirectory(filepath.Join(runner.config.Fixture, "memory-initial"), memoryRoot); err != nil {
		return "", "", "", err
	}
	if err := seedRepository(runner.ctx, memoryRoot, "seed memory"); err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "deploy"), 0o700); err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "docs"), 0o700); err != nil {
		return "", "", "", err
	}
	if err := copyRuntimeFile(filepath.Join(runner.config.Fixture, "evidence", "current-policy.yaml"), filepath.Join(sourceRoot, "deploy", "current-policy.yaml")); err != nil {
		return "", "", "", err
	}
	if err := copyRuntimeFile(filepath.Join(runner.config.Fixture, "evidence", "retired-policy.md"), filepath.Join(sourceRoot, "docs", "retired-policy.md")); err != nil {
		return "", "", "", err
	}
	if err := seedSourceRepository(runner.ctx, sourceRoot); err != nil {
		return "", "", "", err
	}
	sourceRevision, err := commandText(runner.ctx, sourceRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil || sourceRevision != runner.config.Protocol.SourceRepository.Revision {
		return "", "", "", errors.New("deterministic source revision mismatch")
	}
	if err := copyDirectory(filepath.Join(runner.config.Fixture, "code-seed"), codeRoot); err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Join(codeRoot, "deploy"), 0o700); err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(filepath.Join(codeRoot, "docs"), 0o700); err != nil {
		return "", "", "", err
	}
	if err := copyRuntimeFile(filepath.Join(sourceRoot, "deploy", "current-policy.yaml"), filepath.Join(codeRoot, "deploy", "current-policy.yaml")); err != nil {
		return "", "", "", err
	}
	if err := copyRuntimeFile(filepath.Join(sourceRoot, "docs", "retired-policy.md"), filepath.Join(codeRoot, "docs", "retired-policy.md")); err != nil {
		return "", "", "", err
	}
	if err := seedRepository(runner.ctx, codeRoot, "seed task"); err != nil {
		return "", "", "", err
	}
	return memoryRoot, sourceRoot, codeRoot, nil
}
