package experiment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const phase3ProtocolSHA256 = "964e5a988af33b10f135a26258e66aeec33cf9b0e5694304434f70ab99bc149f"

type protocolFixture struct {
	SchemaVersion     int    `json:"schema_version"`
	ExperimentID      string `json:"experiment_id"`
	Status            string `json:"status"`
	ProductRevision   string `json:"product_revision"`
	CorpusPath        string `json:"corpus_path"`
	RubricPath        string `json:"rubric_path"`
	RandomizationSeed string `json:"randomization_seed"`
	SourceRepository  struct {
		Revision           string `json:"revision"`
		Tree               string `json:"tree"`
		RepositoryIdentity string `json:"repository_identity"`
	} `json:"source_repository"`
	PrivacyInputs struct {
		NormativeSecretSHA256 string `json:"normative_secret_sha256"`
		SourceCanaryKey       string `json:"source_canary_key"`
		SourceCanarySHA256    string `json:"source_canary_value_sha256"`
	} `json:"privacy_inputs"`
	Execution struct {
		CaptureAttempts int      `json:"capture_attempts"`
		CurationCases   int      `json:"curation_cases"`
		ValueScenarios  int      `json:"value_scenarios"`
		TaskPairs       int      `json:"task_pairs"`
		TaskSessions    int      `json:"task_sessions"`
		CurationOrder   []string `json:"curation_order"`
	} `json:"execution"`
	Thresholds struct {
		CorrectCanonicalCases int     `json:"correct_canonical_cases_required"`
		CorrectNegativeCases  int     `json:"correct_negative_dispositions_required"`
		AcceptedWithoutEdit   int     `json:"accepted_without_proposal_correction_min"`
		TreatmentCorrect      int     `json:"treatment_correct_pairs_required"`
		UtilityPairs          int     `json:"utility_success_pairs_min"`
		ExplorationReduction  float64 `json:"exploration_reduction_min"`
	} `json:"thresholds"`
	Pairs []struct {
		ID        string   `json:"id"`
		Scenario  string   `json:"scenario"`
		OrderHash string   `json:"order_hash"`
		Order     []string `json:"order"`
	} `json:"pairs"`
	FixtureHashes         map[string]string `json:"fixture_hashes"`
	ProductionAssetHashes map[string]string `json:"production_asset_hashes"`
}

type corpusFixture struct {
	SchemaVersion  int    `json:"schema_version"`
	CorpusID       string `json:"corpus_id"`
	ValueScenarios []struct {
		ID              string `json:"id"`
		CurationCase    string `json:"curation_case"`
		Task            string `json:"task"`
		HiddenTest      string `json:"hidden_test"`
		Operation       string `json:"operation"`
		ControlOmission struct {
			Path      string `json:"path"`
			SectionID string `json:"section_id"`
		} `json:"control_omission"`
		RelevantTerms []string `json:"relevant_terms"`
	} `json:"value_scenarios"`
	CurationCases []struct {
		ID                  string   `json:"id"`
		Class               string   `json:"class"`
		ExpectedOperation   string   `json:"expected_operation"`
		ExpectedDisposition string   `json:"expected_disposition"`
		ExpectedEdit        string   `json:"expected_edit"`
		ExpectedReason      string   `json:"expected_reason"`
		ExpectedTarget      string   `json:"expected_target"`
		ExpectedSectionID   string   `json:"expected_section_id"`
		ReferenceIntegrity  string   `json:"expected_reference_integrity"`
		ProposalSupport     string   `json:"expected_proposal_semantic_support"`
		RequiredPoststate   []string `json:"required_poststate"`
		ForbiddenPoststate  []string `json:"forbidden_poststate"`
	} `json:"curation_cases"`
	CaptureControls []struct {
		ID         string `json:"id"`
		SourceCase string `json:"source_case"`
		Method     string `json:"method"`
	} `json:"capture_controls"`
}

func TestPhase3ValueProtocolIsFrozenAndSelfConsistent(t *testing.T) {
	root := repositoryRoot(t)
	protocolPath := filepath.Join(root, "testdata/phase-3/value-experiment/protocol.json")
	if got := fileHash(t, protocolPath); got != phase3ProtocolSHA256 {
		t.Fatalf("protocol hash = %s, want %s; observed criteria cannot be edited in place", got, phase3ProtocolSHA256)
	}
	protocol := decodeJSON[protocolFixture](t, protocolPath)
	corpus := decodeJSON[corpusFixture](t, filepath.Join(root, protocol.CorpusPath))
	if protocol.SchemaVersion != 1 || protocol.ExperimentID != "phase3-curated-memory-value-v1" || protocol.Status != "pre_registered_not_started" ||
		protocol.ProductRevision != "e92a96b6bc4c6f2f169b8b8e96a419c9d9a1b9b2" || corpus.SchemaVersion != 1 || corpus.CorpusID != "phase3-held-out-policies-v1" {
		t.Fatal("protocol identity is not frozen")
	}
	if protocol.Execution.CaptureAttempts != 14 || protocol.Execution.CurationCases != 12 || protocol.Execution.ValueScenarios != 6 ||
		protocol.Execution.TaskPairs != 12 || protocol.Execution.TaskSessions != 24 || len(corpus.ValueScenarios) != 6 || len(corpus.CurationCases) != 12 || len(corpus.CaptureControls) != 2 {
		t.Fatal("protocol counts are inconsistent")
	}
	if protocol.Thresholds.CorrectCanonicalCases != 9 || protocol.Thresholds.CorrectNegativeCases != 3 || protocol.Thresholds.AcceptedWithoutEdit != 6 ||
		protocol.Thresholds.TreatmentCorrect != 12 || protocol.Thresholds.UtilityPairs != 9 || protocol.Thresholds.ExplorationReduction != 0.25 {
		t.Fatal("decision thresholds changed")
	}

	expectedOperations := map[string]int{"create_page": 2, "create_section": 2, "replace_section": 3, "remove_section": 1, "remove_page": 1, "discard_proposal": 3}
	actualOperations := make(map[string]int)
	operationByCase := make(map[string]string)
	targetByCase := make(map[string]string)
	sectionByCase := make(map[string]string)
	canonical, negative, withoutEdit := 0, 0, 0
	caseIDs := make([]string, len(corpus.CurationCases))
	for index, item := range corpus.CurationCases {
		caseIDs[index] = item.ID
		actualOperations[item.ExpectedOperation]++
		operationByCase[item.ID] = item.ExpectedOperation
		targetByCase[item.ID] = item.ExpectedTarget
		sectionByCase[item.ID] = item.ExpectedSectionID
		if item.Class == "canonical" {
			canonical++
		} else if item.Class == "negative" {
			negative++
		}
		if item.ExpectedEdit == "without_proposal_correction" {
			withoutEdit++
		}
		if item.ExpectedDisposition == "discarded" && item.ExpectedReason == "" {
			t.Fatalf("discard case %s has no reason", item.ID)
		}
		if item.ExpectedTarget != "" && item.ExpectedSectionID == "" {
			t.Fatalf("canonical case %s has no expected section", item.ID)
		}
		if item.ReferenceIntegrity == "" || item.ProposalSupport == "" || item.RequiredPoststate == nil || item.ForbiddenPoststate == nil {
			t.Fatalf("case %s has incomplete curation gold", item.ID)
		}
	}
	if !mapsEqual(actualOperations, expectedOperations) || canonical != 9 || negative != 3 || withoutEdit != 6 {
		t.Fatal("curation corpus does not match its pre-registered classes")
	}
	for _, scenario := range corpus.ValueScenarios {
		if operationByCase[scenario.CurationCase] != scenario.Operation || scenario.ControlOmission.Path == "" || scenario.ControlOmission.Path != targetByCase[scenario.CurationCase] ||
			(scenario.ControlOmission.SectionID != "" && scenario.ControlOmission.SectionID != sectionByCase[scenario.CurationCase]) || len(scenario.RelevantTerms) == 0 {
			t.Fatalf("scenario %s is not bound to complete curation gold", scenario.ID)
		}
	}
	slices.SortFunc(caseIDs, func(left, right string) int {
		leftHash := sha256.Sum256([]byte(protocol.RandomizationSeed + ":curation:" + left))
		rightHash := sha256.Sum256([]byte(protocol.RandomizationSeed + ":curation:" + right))
		return bytes.Compare(leftHash[:], rightHash[:])
	})
	if !slices.Equal(caseIDs, protocol.Execution.CurationOrder) {
		t.Fatal("curation order does not match the frozen seed")
	}
	if slices.Index(protocol.Execution.CurationOrder, "cur-01") > slices.Index(protocol.Execution.CurationOrder, "cur-10") {
		t.Fatal("semantic duplicate is curated before its canonical source")
	}
	if corpus.CaptureControls[0].SourceCase != "cur-01" || corpus.CaptureControls[0].Method != "replay_exact_memory_propose_arguments" || corpus.CaptureControls[1].Method != "fresh_remember_session" {
		t.Fatal("capture controls are not executable from frozen inputs")
	}
	assertPrivacyInputs(t, root, protocol)

	if len(protocol.Pairs) != 12 {
		t.Fatal("pair count changed")
	}
	scenarioPairs := make(map[string]int)
	treatmentFirst := 0
	for index, pair := range protocol.Pairs {
		wantID := fmt.Sprintf("pair-%02d", index+1)
		hash := sha256.Sum256([]byte(protocol.RandomizationSeed + ":" + strings.TrimPrefix(wantID, "pair-")))
		wantHash := hex.EncodeToString(hash[:])
		var wantOrder []string
		if index%2 == 0 {
			wantOrder = []string{"control", "treatment"}
			if hash[0]%2 == 1 {
				wantOrder = []string{"treatment", "control"}
			}
		} else {
			wantOrder = []string{"treatment", "control"}
			if protocol.Pairs[index-1].Order[0] == "treatment" {
				wantOrder = []string{"control", "treatment"}
			}
		}
		if wantOrder[0] == "treatment" {
			treatmentFirst++
		}
		if pair.ID != wantID || pair.OrderHash != wantHash || !slices.Equal(pair.Order, wantOrder) {
			t.Fatalf("pair %d does not match its frozen assignment", index+1)
		}
		scenarioPairs[pair.Scenario]++
	}
	if treatmentFirst != 6 || len(scenarioPairs) != 6 {
		t.Fatal("pair assignment is not balanced across six scenarios")
	}
	for scenario, count := range scenarioPairs {
		if count != 2 {
			t.Fatalf("scenario %s has %d pairs", scenario, count)
		}
	}
	for index := 0; index < len(protocol.Pairs); index += 2 {
		if protocol.Pairs[index].Scenario != protocol.Pairs[index+1].Scenario || protocol.Pairs[index].Order[0] == protocol.Pairs[index+1].Order[0] {
			t.Fatalf("scenario %s does not have one pair starting in each arm", protocol.Pairs[index].Scenario)
		}
	}

	for path, want := range protocol.FixtureHashes {
		if got := fileHash(t, filepath.Join(root, filepath.FromSlash(path))); got != want {
			t.Fatalf("fixture %s hash = %s, want %s", path, got, want)
		}
	}
	assertEveryFixtureIsHashed(t, root, protocol.FixtureHashes)
	for path, want := range protocol.ProductionAssetHashes {
		if got := fileHash(t, filepath.Join(root, filepath.FromSlash(path))); got != want {
			t.Fatalf("production asset %s hash = %s, want %s", path, got, want)
		}
	}
	assertSourceRepositoryIdentity(t, root, protocol)

	seed := filepath.Join(root, "testdata/phase-3/value-experiment/code-seed")
	runGoTest(t, seed, "./...")
	hidden := filepath.Join(root, "testdata/phase-3/value-experiment/evaluator/policy_hidden_test.go")
	for _, scenario := range corpus.ValueScenarios {
		if _, err := os.Stat(filepath.Join(root, "testdata/phase-3/value-experiment", filepath.FromSlash(scenario.Task))); err != nil {
			t.Fatalf("scenario %s task is unavailable", scenario.ID)
		}
		worktree := filepath.Join(t.TempDir(), scenario.ID)
		copyTree(t, seed, worktree)
		copyFile(t, hidden, filepath.Join(worktree, "policy", "policy_hidden_test.go"))
		command := exec.Command("go", "test", "./policy", "-run", "^"+scenario.HiddenTest+"$", "-count=1")
		command.Dir = worktree
		if err := command.Run(); err == nil {
			t.Fatalf("scenario %s hidden evaluator passes before the task", scenario.ID)
		}
	}
}

func assertSourceRepositoryIdentity(t *testing.T, root string, protocol protocolFixture) {
	t.Helper()
	sourceRoot := t.TempDir()
	copyFile(t, filepath.Join(root, "testdata/phase-3/value-experiment/evidence/current-policy.yaml"), filepath.Join(sourceRoot, "deploy/current-policy.yaml"))
	copyFile(t, filepath.Join(root, "testdata/phase-3/value-experiment/evidence/retired-policy.md"), filepath.Join(sourceRoot, "docs/retired-policy.md"))
	sourceEnvironment := []string{
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=" + t.TempDir(),
		"TZ=UTC",
		"LC_ALL=C.UTF-8",
	}
	runCommand(t, sourceRoot, sourceEnvironment, "git", "init", "--quiet", "--initial-branch=main", "--object-format=sha1")
	runCommand(t, sourceRoot, sourceEnvironment, "git", "add", "deploy/current-policy.yaml", "docs/retired-policy.md")
	commitEnvironment := append(append([]string(nil), sourceEnvironment...),
		"GIT_AUTHOR_NAME=GPM Fixture",
		"GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=GPM Fixture",
		"GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
	)
	runCommand(t, sourceRoot, commitEnvironment, "git", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture: source evidence")
	revision := strings.TrimSpace(runCommand(t, sourceRoot, sourceEnvironment, "git", "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runCommand(t, sourceRoot, sourceEnvironment, "git", "rev-parse", "HEAD^{tree}"))
	identityHash := sha256.Sum256([]byte(revision))
	identity := "git-lineage-sha256:" + hex.EncodeToString(identityHash[:])
	if revision != protocol.SourceRepository.Revision || tree != protocol.SourceRepository.Tree || identity != protocol.SourceRepository.RepositoryIdentity {
		t.Fatalf("source repository identity changed: revision=%s tree=%s identity=%s", revision, tree, identity)
	}
}

func assertPrivacyInputs(t *testing.T, root string, protocol protocolFixture) {
	t.Helper()
	seedHash := sha256.Sum256([]byte(protocol.RandomizationSeed + ":normative-secret"))
	secret := "ghp_" + hex.EncodeToString(seedHash[:])[:36]
	secretHash := sha256.Sum256([]byte(secret))
	if got := hex.EncodeToString(secretHash[:]); got != protocol.PrivacyInputs.NormativeSecretSHA256 {
		t.Fatalf("normative secret hash = %s, want %s", got, protocol.PrivacyInputs.NormativeSecretSHA256)
	}
	contents, err := os.ReadFile(filepath.Join(root, "testdata/phase-3/value-experiment/evidence/current-policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	prefix := []byte(protocol.PrivacyInputs.SourceCanaryKey + ": ")
	start := bytes.Index(contents, prefix)
	if start < 0 {
		t.Fatal("source privacy canary is unavailable")
	}
	value := contents[start+len(prefix):]
	if newline := bytes.IndexByte(value, '\n'); newline >= 0 {
		value = value[:newline]
	}
	canaryHash := sha256.Sum256(value)
	if got := hex.EncodeToString(canaryHash[:]); got != protocol.PrivacyInputs.SourceCanarySHA256 {
		t.Fatalf("source canary hash = %s, want %s", got, protocol.PrivacyInputs.SourceCanarySHA256)
	}
}

func assertEveryFixtureIsHashed(t *testing.T, root string, frozen map[string]string) {
	t.Helper()
	fixtureRoot := filepath.Join(root, "testdata/phase-3/value-experiment")
	err := filepath.WalkDir(fixtureRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() == "protocol.json" {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		canonical := filepath.ToSlash(relative)
		if _, ok := frozen[canonical]; !ok {
			return fmt.Errorf("fixture %s is not frozen by hash", canonical)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDecisionPrecedenceIsExhaustive(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*GateOutcome)
		want   Decision
	}{
		{name: "advance", want: DecisionAdvance},
		{name: "privacy", mutate: func(value *GateOutcome) { value.PrivacyIncidents = 1 }, want: DecisionStop},
		{name: "raw private evidence", mutate: func(value *GateOutcome) { value.RawPrivateEvidence = true }, want: DecisionStop},
		{name: "secret accepted", mutate: func(value *GateOutcome) { value.CaptureSecretRejectionPassed = false }, want: DecisionStop},
		{name: "trust", mutate: func(value *GateOutcome) { value.CanonicalTrustFailures = 1 }, want: DecisionStop},
		{name: "reference", mutate: func(value *GateOutcome) { value.ReferenceMisrepresentations = 1 }, want: DecisionStop},
		{name: "correctness regression", mutate: func(value *GateOutcome) { value.TreatmentCorrectnessRegressions = 1 }, want: DecisionStop},
		{name: "score regression", mutate: func(value *GateOutcome) { value.TreatmentScoreRegressions = 1 }, want: DecisionStop},
		{name: "invalid", mutate: func(value *GateOutcome) { value.InvalidSessions = 1 }, want: DecisionRevise},
		{name: "mixed cost", mutate: func(value *GateOutcome) { value.CostAvailability = "mixed" }, want: DecisionRevise},
		{name: "capture incomplete", mutate: func(value *GateOutcome) { value.CaptureAttemptsCompleted = 13 }, want: DecisionRevise},
		{name: "curation incomplete", mutate: func(value *GateOutcome) { value.CurationCasesCompleted = 11 }, want: DecisionRevise},
		{name: "task incomplete", mutate: func(value *GateOutcome) { value.TaskSessionsCompleted = 23 }, want: DecisionRevise},
		{name: "pair incomplete", mutate: func(value *GateOutcome) { value.CompletedPairs = 11 }, want: DecisionRevise},
		{name: "low curation", mutate: func(value *GateOutcome) { value.CorrectCanonicalCases = 5 }, want: DecisionStop},
		{name: "low utility", mutate: func(value *GateOutcome) { value.UtilitySuccessPairs = 3 }, want: DecisionStop},
		{name: "curation below advance", mutate: func(value *GateOutcome) { value.CorrectCanonicalCases = 8 }, want: DecisionRevise},
		{name: "negative dispositions", mutate: func(value *GateOutcome) { value.CorrectNegativeDispositions = 2 }, want: DecisionRevise},
		{name: "proposal corrections", mutate: func(value *GateOutcome) { value.AcceptedWithoutCorrection = 5 }, want: DecisionRevise},
		{name: "dedupe", mutate: func(value *GateOutcome) { value.CaptureExactDuplicatePassed = false }, want: DecisionRevise},
		{name: "session budget", mutate: func(value *GateOutcome) { value.AllSessionBudgetsRespected = false }, want: DecisionRevise},
		{name: "curation median", mutate: func(value *GateOutcome) { value.CurationDurationsSeconds = repeated(181) }, want: DecisionRevise},
		{name: "curation p90", mutate: func(value *GateOutcome) {
			value.CurationDurationsSeconds[10], value.CurationDurationsSeconds[11] = 361, 361
		}, want: DecisionRevise},
		{name: "combined duration", mutate: func(value *GateOutcome) { value.CapturePlusCurateSeconds = repeated(301) }, want: DecisionRevise},
		{name: "curation bytes", mutate: func(value *GateOutcome) {
			value.CurationToolResultBytes[10], value.CurationToolResultBytes[11] = 65537, 65537
		}, want: DecisionRevise},
		{name: "retrieval", mutate: func(value *GateOutcome) { value.RelevantTreatmentRetrievals = 9 }, want: DecisionRevise},
		{name: "memory bytes", mutate: func(value *GateOutcome) { value.MaximumTaskMemoryResultBytes = 8193 }, want: DecisionRevise},
		{name: "treatment correctness", mutate: func(value *GateOutcome) { value.TreatmentCorrectPairs = 11 }, want: DecisionRevise},
		{name: "utility below advance", mutate: func(value *GateOutcome) { value.UtilitySuccessPairs = 8 }, want: DecisionRevise},
		{name: "scenario utility", mutate: func(value *GateOutcome) { value.UtilitySuccessByScenario["cache-ttl"] = 0 }, want: DecisionRevise},
		{name: "exploration calls", mutate: func(value *GateOutcome) { value.TreatmentExplorationCalls = 76 }, want: DecisionRevise},
		{name: "exploration bytes", mutate: func(value *GateOutcome) { value.TreatmentExplorationBytes = 751 }, want: DecisionRevise},
		{name: "task duration", mutate: func(value *GateOutcome) { value.TreatmentTaskDurationsSeconds = repeated(126) }, want: DecisionRevise},
		{name: "reported cost", mutate: func(value *GateOutcome) { value.CostAvailability = "all"; value.TreatmentCostMicroUSD = 126 }, want: DecisionRevise},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := passingGateOutcome()
			if test.mutate != nil {
				test.mutate(&value)
			}
			if got := Decide(value); got != test.want {
				t.Fatalf("Decide() = %s, want %s", got, test.want)
			}
		})
	}
}

func passingGateOutcome() GateOutcome {
	return GateOutcome{
		CaptureSecretRejectionPassed:  true,
		CaptureAttemptsCompleted:      14,
		CurationCasesCompleted:        12,
		TaskSessionsCompleted:         24,
		CompletedPairs:                12,
		CorrectCanonicalCases:         9,
		CorrectNegativeDispositions:   3,
		AcceptedWithoutCorrection:     6,
		CaptureExactDuplicatePassed:   true,
		CurationDurationsSeconds:      repeated(180),
		CapturePlusCurateSeconds:      repeated(300),
		CurationToolResultBytes:       repeated(65536),
		RelevantTreatmentRetrievals:   10,
		MaximumTaskMemoryResultBytes:  8192,
		TreatmentCorrectPairs:         12,
		UtilitySuccessPairs:           9,
		UtilitySuccessByScenario:      map[string]int{"lease-renewal": 2, "webhook-grace": 2, "settlement-window": 2, "regional-routing": 1, "feature-rollout": 1, "cache-ttl": 1},
		TreatmentExplorationCalls:     75,
		ControlExplorationCalls:       100,
		TreatmentExplorationBytes:     750,
		ControlExplorationBytes:       1000,
		TreatmentTaskDurationsSeconds: repeated(125),
		ControlTaskDurationsSeconds:   repeated(100),
		CostAvailability:              "none",
		ControlCostMicroUSD:           100,
		AllSessionBudgetsRespected:    true,
	}
}

func repeated(value int64) []int64 {
	result := make([]int64, 12)
	for index := range result {
		result[index] = value
	}
	return result
}

func TestUtilityAndPercentileBoundaries(t *testing.T) {
	base := PairOutcome{TreatmentCorrect: true, TreatmentScore: 10, ControlScore: 10, TreatmentExploreCalls: 3, ControlExploreCalls: 4, TreatmentExploreBytes: 750, ControlExploreBytes: 1000}
	if !IsUtilitySuccess(base) {
		t.Fatal("exact 25 percent exploration reduction did not pass")
	}
	base.TreatmentExploreBytes = 751
	if IsUtilitySuccess(base) {
		t.Fatal("sub-threshold byte reduction passed")
	}
	base = PairOutcome{TreatmentCorrect: true, TreatmentScore: 9, ControlScore: 8}
	if !IsUtilitySuccess(base) {
		t.Fatal("score improvement did not pass")
	}
	base.ControlScore = 10
	if IsUtilitySuccess(base) {
		t.Fatal("score regression passed")
	}
	if got := NearestRank([]int64{50, 10, 40, 20, 30}, 0.9); got != 50 {
		t.Fatalf("NearestRank(p90) = %d, want 50", got)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func decodeJSON[T any](t *testing.T, path string) T {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatal("JSON fixture contains trailing content")
	}
	return value
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:])
}

func mapsEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func runGoTest(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("go", append([]string{"test"}, arguments...)...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go test %v: %v: %s", arguments, err, output)
	}
}

func runCommand(t *testing.T, directory string, environment []string, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, arguments, err, output)
	}
	return string(output)
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		copyFile(t, path, target)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}
