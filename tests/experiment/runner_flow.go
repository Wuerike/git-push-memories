package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/markdown"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type curationLabels struct {
	Integrity string
	Support   string
}

func (runner *experimentRunner) executor() executor {
	return executor{
		root: runner.temporary, authRoot: runner.authRoot, productBinary: runner.binary, claudeBinary: runner.claudeBinary,
		credential:    runner.options.CredentialPath,
		rememberSkill: filepath.Join(runner.config.Repository, "integrations", "claude-code", "skills", "remember", "SKILL.md"),
		curateSkill:   filepath.Join(runner.config.Repository, "integrations", "claude-code", "skills", "curate", "SKILL.md"),
		goRoot:        runner.goRoot,
	}
}

func (runner *experimentRunner) review(request reviewRequest) (string, error) {
	fmt.Fprintf(runner.output, "\n[%s %s, turn %d]\n%s\n", request.Kind, request.UnitID, request.Turn, request.Text)
	fmt.Fprint(runner.output, "Reviewer response (`/cancel` ends this case): ")
	response, err := runner.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	response = strings.TrimSpace(response)
	if request.Kind == "curation" && response == "Confirm this exact decision and diff." {
		integrity, err := runner.readLabel("Reference integrity shown [verified/unavailable/mismatch/legacy_unverified/absent]: ", []string{"verified", "unavailable", "mismatch", "legacy_unverified", "absent"})
		if err != nil {
			return "", err
		}
		support, err := runner.readLabel("Proposal semantic support [supports/contradicts/unclear]: ", []string{"supports", "contradicts", "unclear"})
		if err != nil {
			return "", err
		}
		runner.reviewLabels[request.UnitID] = curationLabels{Integrity: integrity, Support: support}
	}
	return response, nil
}

func (runner *experimentRunner) readLabel(message string, allowed []string) (string, error) {
	fmt.Fprint(runner.output, message)
	value, err := runner.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", errors.New("reviewer supplied an invalid closed label")
}

func (runner *experimentRunner) runCaptures(memoryRoot, sourceRoot string) error {
	visible := []string{"deploy/current-policy.yaml", "docs/retired-policy.md"}
	cases := make(map[string]struct {
		ID, Class, Fact, Action, Target string
	})
	for _, item := range runner.config.Corpus.CurationCases {
		cases[item.ID] = struct{ ID, Class, Fact, Action, Target string }{item.ID, item.Class, item.ProposalFact, item.SuggestedAction, item.SuggestedTarget}
	}
	for index := 1; index <= 12; index++ {
		caseID := fmt.Sprintf("cur-%02d", index)
		item := cases[caseID]
		unitID := "capture-" + caseID
		before := proposalFiles(memoryRoot)
		prompt := runner.config.Protocol.PromptTemplates.Capture
		prompt = strings.ReplaceAll(prompt, "<proposal_fact>", item.Fact)
		prompt = strings.ReplaceAll(prompt, "<suggested_action>", item.Action)
		target := item.Target
		if target == "" {
			target = "(none)"
		}
		prompt = strings.ReplaceAll(prompt, "<suggested_target>", target)
		observation, duration, runErr := runner.executor().run(runner.ctx, sessionRequest{
			UnitID: unitID, Kind: "capture", Workspace: sourceRoot, MemoryRoot: memoryRoot,
			StateRoot: filepath.Join(runner.temporary, "state", unitID), Prompt: prompt, ExpectedModel: runner.records.ExpectedModel,
			VisiblePaths: visible, MaxBudgetUSD: runner.config.Protocol.Provider.MaximumCaptureUSD,
			Timeout: time.Duration(runner.config.Protocol.Budgets.CaptureTimeoutSeconds) * time.Second,
		}, runner.reviewer)
		seconds := durationSeconds(duration)
		runner.captureDurations[caseID] = seconds
		valid := runErr == nil
		runner.records.Capture.Sessions = append(runner.records.Capture.Sessions, CaptureRecord{
			UnitID: unitID, Completed: runErr == nil, Valid: valid, WithinBudget: withinBudget(observation, runner.config.Protocol.Provider.MaximumCaptureUSD), DurationSeconds: seconds, Observation: observation,
		})
		if err := runner.store.recordAttempt(attemptRecord{UnitID: unitID, Kind: "capture", State: terminalState(valid), Valid: valid, InvalidReason: invalidReason(runErr)}); err != nil {
			return err
		}
		after := proposalFiles(memoryRoot)
		if path := oneAddedPath(before, after); path != "" {
			runner.proposalPath[caseID] = path
		}
		if caseID == "cur-01" {
			passed := false
			if observation.ProposalArguments != nil {
				passed = runner.runExactDuplicate(memoryRoot, observation.ProposalArguments)
			}
			runner.records.Capture.DirectDuplicateCompleted = true
			runner.records.Capture.ExactDuplicatePassed = passed
			if err := runner.store.recordAttempt(attemptRecord{UnitID: "capture-exact-duplicate", Kind: "direct_mcp_control", State: "completed", Valid: true}); err != nil {
				return err
			}
		}
	}
	return runner.runSecretCapture(memoryRoot, sourceRoot, visible)
}

func (runner *experimentRunner) runExactDuplicate(memoryRoot string, arguments map[string]any) bool {
	headBefore, err := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return false
	}
	stateRoot := filepath.Join(runner.temporary, "state", "capture-exact-duplicate")
	result, err := directToolCall(runner.ctx, runner.binary, memoryRoot, stateRoot, "memory_propose", arguments)
	if err != nil {
		return false
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	headAfter, err := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	return err == nil && bytes.Contains(encoded, []byte(`"duplicate":true`)) && headAfter == headBefore
}

func (runner *experimentRunner) runSecretCapture(memoryRoot, sourceRoot string, visible []string) error {
	digest := sha256.Sum256([]byte(runner.config.Protocol.RandomizationSeed + ":normative-secret"))
	secret := "ghp_" + hex.EncodeToString(digest[:])[:36]
	defer zero([]byte(secret))
	prompt := strings.ReplaceAll(runner.config.Protocol.PromptTemplates.CaptureSecret, "<normative-secret-derived-in-memory-from-randomization-seed>", secret)
	objectsBefore, err := gitObjectInventory(runner.ctx, memoryRoot)
	if err != nil {
		return err
	}
	headBefore, err := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	proposalsBefore := proposalFiles(memoryRoot)
	observation, duration, runErr := runner.executor().run(runner.ctx, sessionRequest{
		UnitID: "capture-secret", Kind: "capture", Workspace: sourceRoot, MemoryRoot: memoryRoot,
		StateRoot: filepath.Join(runner.temporary, "state", "capture-secret"), Prompt: prompt, ExpectedModel: runner.records.ExpectedModel,
		VisiblePaths: visible, MaxBudgetUSD: runner.config.Protocol.Provider.MaximumCaptureUSD,
		Timeout: time.Duration(runner.config.Protocol.Budgets.CaptureTimeoutSeconds) * time.Second,
	}, runner.reviewer)
	objectsAfter, inventoryErr := gitObjectInventory(runner.ctx, memoryRoot)
	headAfter, headErr := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	passed := runErr == nil && inventoryErr == nil && headErr == nil && observation.ToolCounts["mcp__git-push-memories__memory_propose"] == 1 &&
		containsString(observation.ErrorCodes, "privacy_sensitive_content") && inventoriesEqual(objectsBefore, objectsAfter) &&
		headBefore == headAfter && slicesEqual(proposalsBefore, proposalFiles(memoryRoot))
	runner.records.Privacy.SecretRejected = passed
	valid := runErr == nil && inventoryErr == nil
	runner.records.Capture.Sessions = append(runner.records.Capture.Sessions, CaptureRecord{
		UnitID: "capture-secret", Completed: runErr == nil, Valid: valid, WithinBudget: withinBudget(observation, runner.config.Protocol.Provider.MaximumCaptureUSD), DurationSeconds: durationSeconds(duration), Observation: observation,
	})
	return runner.store.recordAttempt(attemptRecord{UnitID: "capture-secret", Kind: "capture", State: terminalState(valid), Valid: valid, InvalidReason: invalidReason(runErr)})
}

func (runner *experimentRunner) runCurations(memoryRoot, sourceRoot string) error {
	cases := make(map[string]runnerCurationCase)
	for _, item := range runner.config.Corpus.CurationCases {
		cases[item.ID] = runnerCurationCase{
			ID: item.ID, Class: item.Class, ProposalFact: item.ProposalFact, SuggestedAction: item.SuggestedAction, SuggestedTarget: item.SuggestedTarget,
			ExpectedOperation: item.ExpectedOperation, ExpectedTarget: item.ExpectedTarget, ExpectedHeading: item.ExpectedHeading, ExpectedSectionID: item.ExpectedSectionID,
			ExpectedDisposition: item.ExpectedDisposition, ExpectedReason: item.ExpectedReason, ExpectedEdit: item.ExpectedEdit,
			ExpectedIntegrity: item.ExpectedIntegrity, ExpectedSemanticSupport: item.ExpectedSemanticSupport,
			RequiredPoststate: item.RequiredPoststate, ForbiddenPoststate: item.ForbiddenPoststate,
		}
	}
	visible := []string{"deploy/current-policy.yaml", "docs/retired-policy.md"}
	for _, caseID := range runner.config.Protocol.Execution.CurationOrder {
		item := cases[caseID]
		proposalPath := runner.proposalPath[caseID]
		if proposalPath == "" {
			record := CurationRecord{CaseID: caseID, Class: item.Class, Completed: false, Valid: false, CaptureDurationSeconds: runner.captureDurations[caseID]}
			runner.records.Curations = append(runner.records.Curations, record)
			if err := runner.store.recordAttempt(attemptRecord{UnitID: "curation-" + caseID, Kind: "curation", State: "aborted", Valid: false, InvalidReason: "proposal_unavailable"}); err != nil {
				return err
			}
			continue
		}
		proposalID := proposalIDFromPath(proposalPath)
		unitID := "curation-" + caseID
		headBefore, err := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		beforeTarget, beforeTargetExists := readOptional(filepath.Join(memoryRoot, filepath.FromSlash(item.ExpectedTarget)))
		prompt := strings.ReplaceAll(runner.config.Protocol.PromptTemplates.Curation, "<proposal-id>", proposalID)
		observation, duration, runErr := runner.executor().run(runner.ctx, sessionRequest{
			UnitID: unitID, Kind: "curation", Workspace: sourceRoot, MemoryRoot: memoryRoot,
			StateRoot: filepath.Join(runner.temporary, "state", unitID), Prompt: prompt, ExpectedModel: runner.records.ExpectedModel,
			VisiblePaths: visible, MaxBudgetUSD: runner.config.Protocol.Provider.MaximumCurationUSD,
			Timeout: time.Duration(runner.config.Protocol.Budgets.CurationTimeoutSeconds) * time.Second,
		}, runner.reviewer)
		for _, arguments := range append(observation.ToolArguments["mcp__git-push-memories__memory_curate_apply"], observation.ToolArguments["mcp__git-push-memories__memory_curate_batch_apply"]...) {
			if value, ok := arguments["preview_token"].(string); ok {
				runner.previewValues = append(runner.previewValues, []byte(value))
			}
		}
		record, evaluationErr := evaluateCurationRecord(runner.ctx, memoryRoot, headBefore, beforeTarget, beforeTargetExists, proposalPath, item, observation, runner.reviewLabels[unitID])
		if evaluationErr != nil {
			return evaluationErr
		}
		record.Completed = runErr == nil
		record.Valid = runErr == nil
		record.WithinBudget = withinBudget(observation, runner.config.Protocol.Provider.MaximumCurationUSD)
		record.DurationSeconds = durationSeconds(duration)
		record.CaptureDurationSeconds = runner.captureDurations[caseID]
		record.ToolResultBytes = observation.CurationToolBytes
		record.Observation = observation
		runner.records.Curations = append(runner.records.Curations, record)
		if err := runner.store.recordAttempt(attemptRecord{UnitID: unitID, Kind: "curation", State: terminalState(runErr == nil), Valid: runErr == nil, InvalidReason: invalidReason(runErr)}); err != nil {
			return err
		}
	}
	return nil
}

type runnerCurationCase struct {
	ID, Class, ProposalFact, SuggestedAction, SuggestedTarget, ExpectedOperation, ExpectedTarget, ExpectedHeading, ExpectedSectionID string
	ExpectedDisposition, ExpectedReason, ExpectedEdit, ExpectedIntegrity, ExpectedSemanticSupport                                    string
	RequiredPoststate, ForbiddenPoststate                                                                                            []string
}

func evaluateCurationRecord(ctx context.Context, memoryRoot, headBefore string, beforeTarget []byte, beforeTargetExists bool, proposalPath string, item runnerCurationCase, observation SessionObservation, labels curationLabels) (CurationRecord, error) {
	record := CurationRecord{CaseID: item.ID, Class: item.Class, ReferenceIntegrity: labels.Integrity, SemanticSupport: labels.Support}
	preview := lastPreviewArguments(observation)
	record.Operation, _ = preview["operation"].(string)
	record.Target, _ = preview["target_path"].(string)
	if !containsString([]string{"create_page", "create_section", "replace_section", "remove_section", "remove_page", "discard_proposal"}, record.Operation) {
		record.Operation = "invalid"
	}
	if record.Target != "" && (!regexp.MustCompile(`^memories/(domains|decision)/[a-z0-9][a-z0-9-]*\.md$`).MatchString(record.Target)) {
		record.Target = "invalid"
	}
	if record.Operation == "discard_proposal" {
		record.Disposition = "discarded"
	} else if record.Operation != "" {
		record.Disposition = "consumed"
	}
	headAfter, err := commandText(ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return record, err
	}
	if headAfter != headBefore {
		record.Commit = headAfter
		changedPaths, err := commandText(ctx, memoryRoot, nil, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
		if err != nil {
			return record, err
		}
		record.ChangedPaths = strings.Fields(changedPaths)
		diff, err := commandText(ctx, memoryRoot, nil, "git", "show", "--format=", "--no-ext-diff", "HEAD")
		if err != nil {
			return record, err
		}
		digest := sha256.Sum256([]byte(diff))
		record.DiffHash = hex.EncodeToString(digest[:])
	}
	postTarget, postTargetExists := readOptional(filepath.Join(memoryRoot, filepath.FromSlash(item.ExpectedTarget)))
	integrityCorrect := labels.Integrity == item.ExpectedIntegrity
	if item.ExpectedIntegrity == "absent" {
		integrityCorrect = integrityCorrect && len(observation.ReferenceIntegrities) == 0
	} else {
		integrityCorrect = integrityCorrect && containsString(observation.ReferenceIntegrities, item.ExpectedIntegrity)
	}
	correct := record.Disposition == item.ExpectedDisposition && record.Operation == item.ExpectedOperation && record.Target == item.ExpectedTarget &&
		integrityCorrect && labels.Support == item.ExpectedSemanticSupport && !fileExists(filepath.Join(memoryRoot, filepath.FromSlash(proposalPath)))
	if reason, _ := preview["reason"].(string); reason != item.ExpectedReason {
		correct = false
	}
	if heading, _ := preview["heading"].(string); item.ExpectedHeading != "" && heading != item.ExpectedHeading {
		correct = false
	}
	if sectionID, _ := preview["section_id"].(string); item.ExpectedSectionID != "" && item.ExpectedOperation != "create_page" && item.ExpectedOperation != "create_section" && sectionID != item.ExpectedSectionID {
		correct = false
	}
	if !curationPathsCorrect(record.ChangedPaths, proposalPath, item.ExpectedTarget, item.ExpectedDisposition) {
		correct = false
	}
	for _, required := range item.RequiredPoststate {
		if !bytes.Contains(postTarget, []byte(required)) {
			correct = false
		}
	}
	for _, forbidden := range item.ForbiddenPoststate {
		if forbidden == item.ExpectedTarget {
			if postTargetExists {
				correct = false
			}
		} else if bytes.Contains(postTarget, []byte(forbidden)) {
			correct = false
		}
	}
	if item.ExpectedOperation == "create_page" && (beforeTargetExists || !postTargetExists) {
		correct = false
	}
	if item.ExpectedOperation == "remove_page" && (!beforeTargetExists || postTargetExists) {
		correct = false
	}
	expectedAnchor := []byte("<!-- @id: " + item.ExpectedSectionID + " -->")
	if (item.ExpectedOperation == "create_page" || item.ExpectedOperation == "create_section" || item.ExpectedOperation == "replace_section") && !bytes.Contains(postTarget, expectedAnchor) {
		correct = false
	}
	if item.ExpectedOperation == "remove_section" && bytes.Contains(postTarget, expectedAnchor) {
		correct = false
	}
	if !exactCurationPoststate(item, beforeTarget, postTarget, postTargetExists) {
		correct = false
	}
	record.Correct = correct
	record.WithoutProposalCorrection = correct && item.ExpectedEdit == "without_proposal_correction"
	record.EditClass = item.ExpectedEdit
	record.ReferenceMisrepresented = !integrityCorrect
	record.CanonicalTrustFailure = curationTrustFailure(item, record, beforeTarget, postTarget, postTargetExists)
	_ = beforeTarget
	return record, nil
}

func curationTrustFailure(item runnerCurationCase, record CurationRecord, beforeTarget, postTarget []byte, postTargetExists bool) bool {
	if record.Disposition != "consumed" {
		return false
	}
	if item.Class == "negative" {
		return item.ExpectedReason == "incorrect" || item.ExpectedReason == "sensitive"
	}
	if record.Target != item.ExpectedTarget {
		return true
	}
	if !exactCurationPoststate(item, beforeTarget, postTarget, postTargetExists) {
		return true
	}
	for _, required := range item.RequiredPoststate {
		if !bytes.Contains(postTarget, []byte(required)) {
			return true
		}
	}
	for _, forbidden := range item.ForbiddenPoststate {
		if forbidden == item.ExpectedTarget {
			if postTargetExists {
				return true
			}
		} else if bytes.Contains(postTarget, []byte(forbidden)) {
			return true
		}
	}
	return false
}

func exactCurationPoststate(item runnerCurationCase, beforeTarget, postTarget []byte, postTargetExists bool) bool {
	switch item.ExpectedOperation {
	case "create_page", "create_section", "replace_section":
		sections, err := markdown.ParseSections(postTarget)
		if err != nil {
			return false
		}
		section, exists := markdown.FindByID(sections, item.ExpectedSectionID)
		if !exists {
			return false
		}
		body, err := markdown.SectionContent(postTarget, *section)
		if err != nil || strings.TrimSpace(string(body)) != strings.Join(item.RequiredPoststate, "\n") {
			return false
		}
		return item.ExpectedOperation != "create_page" || len(sections) == 2
	case "remove_section":
		if beforeTarget == nil {
			return true
		}
		sections, err := markdown.ParseSections(beforeTarget)
		if err != nil {
			return false
		}
		section, exists := markdown.FindByID(sections, item.ExpectedSectionID)
		if !exists {
			return false
		}
		expected, err := markdown.Splice(beforeTarget, []markdown.SpliceOp{{ByteStart: section.ByteStart, ByteEnd: section.ByteEnd}})
		return err == nil && bytes.Equal(expected, postTarget)
	case "remove_page":
		return !postTargetExists
	case "discard_proposal":
		return true
	default:
		return false
	}
}

func (runner *experimentRunner) runTasks(memoryRoot, codeRoot string) error {
	scenarios := make(map[string]struct {
		Task, Hidden string
		Omission     controlOmission
		Terms        []string
	})
	for _, scenario := range runner.config.Corpus.ValueScenarios {
		scenarios[scenario.ID] = struct {
			Task, Hidden string
			Omission     controlOmission
			Terms        []string
		}{scenario.Task, scenario.HiddenTest, scenario.ControlOmission, scenario.RelevantTerms}
	}
	visibleOutput, err := commandText(runner.ctx, codeRoot, nil, "git", "ls-files")
	if err != nil {
		return err
	}
	visible := strings.Fields(visibleOutput)
	runNumber := 0
	for _, pair := range runner.config.Protocol.Pairs {
		scenario := scenarios[pair.Scenario]
		for _, arm := range pair.Order {
			runNumber++
			runID := fmt.Sprintf("run-%02d", runNumber)
			workspace := filepath.Join(runner.temporary, "workspaces", runID)
			if err := copyDirectory(codeRoot, workspace); err != nil {
				return err
			}
			objectsBefore, err := gitObjectInventory(runner.ctx, workspace)
			if err != nil {
				return err
			}
			treatmentMemory := filepath.Join(runner.temporary, "task-memory", runID, "treatment")
			controlMemory := filepath.Join(runner.temporary, "task-memory", runID, "control")
			if err := buildTaskArms(memoryRoot, treatmentMemory, controlMemory, scenario.Omission); err != nil {
				return err
			}
			armMemory := controlMemory
			if arm == "treatment" {
				armMemory = treatmentMemory
			}
			prompt, err := os.ReadFile(filepath.Join(runner.config.Fixture, filepath.FromSlash(scenario.Task)))
			if err != nil {
				return err
			}
			observation, duration, runErr := runner.executor().run(runner.ctx, sessionRequest{
				UnitID: runID, Kind: "task", Arm: arm, Workspace: workspace, MemoryRoot: armMemory,
				StateRoot: filepath.Join(runner.temporary, "state", runID), Prompt: string(prompt), ExpectedModel: runner.records.ExpectedModel,
				RelevantTerms: scenario.Terms, VisiblePaths: visible, MaxBudgetUSD: runner.config.Protocol.Provider.MaximumTaskUSD,
				Timeout: time.Duration(runner.config.Protocol.Budgets.TaskTimeoutSeconds) * time.Second,
			}, nil)
			objectsAfter, inventoryErr := gitObjectInventory(runner.ctx, workspace)
			valid := runErr == nil && inventoryErr == nil && inventoriesEqual(objectsBefore, objectsAfter)
			recordErr := runErr
			if recordErr == nil && (inventoryErr != nil || !inventoriesEqual(objectsBefore, objectsAfter)) {
				recordErr = errors.New("task changed Git object store")
			}
			runner.records.Tasks = append(runner.records.Tasks, TaskRecord{
				RunID: runID, PairID: pair.ID, Scenario: pair.Scenario, Arm: arm, Completed: runErr == nil, Valid: valid,
				WithinBudget: withinBudget(observation, runner.config.Protocol.Provider.MaximumTaskUSD), DurationSeconds: durationSeconds(duration), Observation: observation,
			})
			runner.taskWorktrees[runID] = workspace
			if err := runner.store.recordAttempt(attemptRecord{UnitID: runID, Kind: "task", State: terminalState(valid), Valid: valid, InvalidReason: invalidReason(recordErr)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (runner *experimentRunner) evaluateBlindly() (map[string]BlindEvaluationRecord, error) {
	expectedAPI, err := exportedAPISignatures(filepath.Join(runner.config.Fixture, "code-seed", "policy"))
	if err != nil {
		return nil, err
	}
	hiddenByScenario := make(map[string]string)
	for _, scenario := range runner.config.Corpus.ValueScenarios {
		hiddenByScenario[scenario.ID] = scenario.HiddenTest
	}
	records := make([]BlindEvaluationRecord, 0, len(runner.records.Tasks))
	result := make(map[string]BlindEvaluationRecord)
	for _, task := range runner.records.Tasks {
		evaluation := evaluateCandidate(runner.ctx, candidateEvaluationSpec{
			RunID: task.RunID, Scenario: task.Scenario, HiddenTest: hiddenByScenario[task.Scenario], Workspace: runner.taskWorktrees[task.RunID],
			EvaluatorPath: filepath.Join(runner.config.Fixture, "evaluator", "policy_hidden_test.go"), ExpectedAPI: expectedAPI,
		})
		records = append(records, evaluation)
		result[task.RunID] = evaluation
	}
	if err := runner.store.writeJSON("blind-evaluations.json", map[string]any{"schema_version": 1, "evaluations": records}); err != nil {
		return nil, err
	}
	return result, nil
}

func (runner *experimentRunner) writeGitManifest(memoryRoot string) error {
	head, err := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	tree, err := commandText(runner.ctx, memoryRoot, nil, "git", "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	status, err := commandText(runner.ctx, memoryRoot, nil, "git", "status", "--porcelain=v1")
	if err != nil {
		return err
	}
	return runner.store.writeJSON("git-manifest.json", map[string]any{
		"schema_version": 1, "head": head, "tree": tree, "clean": status == "", "remote_count": 0,
	})
}

func (runner *experimentRunner) runPrivacyScan(memoryRoot string) (PrivacyScanResult, error) {
	digest := sha256.Sum256([]byte(runner.config.Protocol.RandomizationSeed + ":normative-secret"))
	secret := []byte("ghp_" + hex.EncodeToString(digest[:])[:36])
	credentials, err := os.ReadFile(runner.options.CredentialPath)
	if err != nil {
		return PrivacyScanResult{}, err
	}
	defer zero(credentials)
	markers := [][]byte{secret, []byte(runner.temporary), []byte(runner.config.Repository), credentials}
	markers = append(markers, runner.previewValues...)
	fileRoots := []string{runner.store.root, runner.temporary}
	result, err := scanPrivacy(runner.ctx, privacyScanSpec{FileRoots: fileRoots, GitRoots: []string{memoryRoot}, Markers: markers})
	zero(secret)
	if err != nil {
		return PrivacyScanResult{}, err
	}
	canary, err := sourceCanaryValue(filepath.Join(runner.config.Fixture, "evidence", "current-policy.yaml"), runner.config.Protocol.PrivacyInputs.SourceCanaryKey, runner.config.Protocol.PrivacyInputs.SourceCanaryHash)
	if err != nil {
		return PrivacyScanResult{}, err
	}
	unexpected, err := markerHitsOutsideTaskSources(fileRoots, canary)
	if err != nil {
		return PrivacyScanResult{}, err
	}
	result.FileHits += unexpected
	configIncidents, err := inspectPrivateConfigs(runner.authRoot, credentials)
	if err != nil {
		return PrivacyScanResult{}, err
	}
	result.FileHits += configIncidents
	sessionStateFiles, err := regularFileCount(filepath.Join(runner.temporary, "units"))
	if err != nil {
		return PrivacyScanResult{}, err
	}
	result.FileHits += sessionStateFiles
	secretScannerHits, err := normativeSecretFileHits(append(fileRoots, runner.authRoot), map[string]bool{
		filepath.Join(runner.store.root, "protocol.json"): true,
	})
	if err != nil {
		return PrivacyScanResult{}, err
	}
	result.FileHits += secretScannerHits
	gitSecretHits, err := normativeSecretGitObjectHits(runner.ctx, memoryRoot)
	if err != nil {
		return PrivacyScanResult{}, err
	}
	result.GitObjectHits += gitSecretHits
	result.Clean = result.FileHits == 0 && result.GitObjectHits == 0
	return result, nil
}

func regularFileCount(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return count, err
}

func normativeSecretGitObjectHits(ctx context.Context, root string) (int, error) {
	guard, err := intake.New()
	if err != nil {
		return 0, err
	}
	inventory, err := gitObjectInventory(ctx, root)
	if err != nil {
		return 0, err
	}
	hits := 0
	for _, id := range inventory.IDs {
		kind, err := commandText(ctx, root, cleanEnvironment(), "git", "cat-file", "-t", id)
		if err != nil {
			return 0, err
		}
		if kind != "blob" {
			continue
		}
		contents, err := gitOutputBytes(ctx, root, "cat-file", "-p", id)
		if err != nil {
			return 0, err
		}
		candidate, _ := json.Marshal(map[string]any{"title": "Privacy scan", "suggested_action": "create", "knowledge": string(contents), "source_paths": []string{}})
		_, inspectErr := guard.InspectProposal(intake.NewProposalCandidate(candidate))
		var rejection intake.Rejection
		if errors.As(inspectErr, &rejection) && rejection.Class == "privacy" && rejection.Reason == "sensitive_content" {
			hits++
		}
	}
	return hits, nil
}

func normativeSecretFileHits(roots []string, excluded map[string]bool) (int, error) {
	guard, err := intake.New()
	if err != nil {
		return 0, err
	}
	hits := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if entry.IsDir() || excluded[path] || filepath.Base(path) == ".credentials.json" {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for start := 0; start < len(contents); start += 64 * 1024 {
				end := min(start+64*1024, len(contents))
				candidate, _ := json.Marshal(map[string]any{"title": "Privacy scan", "suggested_action": "create", "knowledge": string(contents[start:end]), "source_paths": []string{}})
				_, inspectErr := guard.InspectProposal(intake.NewProposalCandidate(candidate))
				var rejection intake.Rejection
				if errors.As(inspectErr, &rejection) && rejection.Class == "privacy" && rejection.Reason == "sensitive_content" {
					hits++
					break
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return hits, nil
}

func inspectPrivateConfigs(root string, credential []byte) (int, error) {
	incidents := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			incidents++
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Base(path) == ".credentials.json" {
			info, statErr := os.Stat(path)
			if statErr != nil || info.Mode().Perm() != 0o600 || !bytes.Equal(contents, credential) {
				incidents++
			}
			return nil
		}
		allowed := strings.HasSuffix(filepath.ToSlash(relative), "/mcp.json") || strings.Contains(filepath.ToSlash(relative), "/git-push-memories/config.json") || strings.Contains(filepath.ToSlash(relative), "/skills/remember/SKILL.md") || strings.Contains(filepath.ToSlash(relative), "/skills/curate/SKILL.md")
		if !allowed || bytes.Contains(contents, credential) {
			incidents++
		}
		return nil
	})
	return incidents, err
}

func markerHitsOutsideTaskSources(roots []string, marker []byte) (int, error) {
	hits := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if entry.IsDir() {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			count := bytes.Count(contents, marker)
			if strings.HasSuffix(filepath.ToSlash(path), "/deploy/current-policy.yaml") {
				count--
			}
			if count > 0 {
				hits += count
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
	}
	return hits, nil
}

func sourceCanaryValue(path, key, expectedHash string) ([]byte, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	prefix := []byte(key + ": ")
	start := bytes.Index(contents, prefix)
	if start < 0 {
		return nil, errors.New("source canary is unavailable")
	}
	value := contents[start+len(prefix):]
	if newline := bytes.IndexByte(value, '\n'); newline >= 0 {
		value = value[:newline]
	}
	digest := sha256.Sum256(value)
	if hex.EncodeToString(digest[:]) != expectedHash {
		return nil, errors.New("source canary hash mismatch")
	}
	return append([]byte(nil), value...), nil
}

func directToolCall(ctx context.Context, binary, memoryRoot, stateRoot, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	configRoot := filepath.Join(filepath.Dir(stateRoot), "direct-config")
	defer os.RemoveAll(configRoot)
	if err := writeRuntimeJSON(filepath.Join(configRoot, "git-push-memories", "config.json"), map[string]any{"schema_version": 1, "memory_root": memoryRoot}); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, binary, "mcp")
	command.Env = append(cleanEnvironment(), "XDG_CONFIG_HOME="+configRoot, "XDG_STATE_HOME="+stateRoot)
	client := mcp.NewClient(&mcp.Implementation{Name: "phase3-value-control", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
}

func lastPreviewArguments(observation SessionObservation) map[string]any {
	values := append(observation.ToolArguments["mcp__git-push-memories__memory_curate_preview"], observation.ToolArguments["mcp__git-push-memories__memory_curate_batch_preview"]...)
	if len(values) == 0 {
		return map[string]any{}
	}
	if items, ok := values[len(values)-1]["items"].([]any); ok && len(items) == 1 {
		if item, ok := items[0].(map[string]any); ok {
			return item
		}
	}
	return values[len(values)-1]
}

func curationPathsCorrect(paths []string, proposalPath, target, disposition string) bool {
	want := []string{proposalPath}
	if disposition == "consumed" {
		want = append(want, target)
	}
	sort.Strings(want)
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	return slicesEqual(sorted, want)
}

func proposalFiles(memoryRoot string) []string {
	root := filepath.Join(memoryRoot, "proposals")
	var result []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			relative, _ := filepath.Rel(memoryRoot, path)
			result = append(result, filepath.ToSlash(relative))
		}
		return nil
	})
	sort.Strings(result)
	return result
}

func oneAddedPath(before, after []string) string {
	known := make(map[string]bool, len(before))
	for _, path := range before {
		known[path] = true
	}
	var added []string
	for _, path := range after {
		if !known[path] {
			added = append(added, path)
		}
	}
	if len(added) == 1 {
		return added[0]
	}
	return ""
}

func proposalIDFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".md")
	if separator := strings.IndexByte(base, '-'); separator > 0 {
		return base[:separator]
	}
	return base
}

func withinBudget(observation SessionObservation, maximum float64) bool {
	return !observation.CostAvailable || observation.CostMicroUSD <= int64(math.Round(maximum*1_000_000))
}

func durationSeconds(duration time.Duration) int64 {
	return int64(math.Ceil(duration.Seconds()))
}

func terminalState(valid bool) string {
	if valid {
		return "completed"
	}
	return "aborted"
}

func invalidReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "infrastructure_timeout_or_crash"
	}
	return "report_parser_failure"
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func readOptional(path string) ([]byte, bool) {
	contents, err := os.ReadFile(path)
	return contents, err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cleanEnvironment() []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=/dev/null", "GOTOOLCHAIN=go1.25.12", "GOPROXY=off", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "TZ=UTC", "LC_ALL=C.UTF-8"}
}

func seedRepository(ctx context.Context, root, message string) error {
	environment := cleanEnvironment()
	if err := runCommandContext(ctx, root, environment, "git", "init", "--quiet", "--initial-branch=main"); err != nil {
		return err
	}
	if err := runCommandContext(ctx, root, environment, "git", "config", "user.name", "Phase 3 Value Experiment"); err != nil {
		return err
	}
	if err := runCommandContext(ctx, root, environment, "git", "config", "user.email", "phase3-value@example.invalid"); err != nil {
		return err
	}
	if err := runCommandContext(ctx, root, environment, "git", "add", "."); err != nil {
		return err
	}
	arguments := []string{"commit", "--quiet", "-m", message}
	return runCommandContext(ctx, root, environment, "git", arguments...)
}

func seedSourceRepository(ctx context.Context, root string) error {
	environment := append(cleanEnvironment(),
		"GIT_AUTHOR_NAME=GPM Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=GPM Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
	)
	if err := runCommandContext(ctx, root, environment, "git", "init", "--quiet", "--initial-branch=main", "--object-format=sha1"); err != nil {
		return err
	}
	if err := runCommandContext(ctx, root, environment, "git", "add", "deploy/current-policy.yaml", "docs/retired-policy.md"); err != nil {
		return err
	}
	return runCommandContext(ctx, root, environment, "git", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture: source evidence")
}

func copyRuntimeFile(source, target string) error {
	contents, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	return os.WriteFile(target, contents, 0o600)
}

func validVerificationTSV(contents []byte) bool {
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) != 5 || lines[0] != "name\tcommand\texit_code\tduration_ms" {
		return false
	}
	want := []string{"tests", "race", "vet", "build"}
	commands := []string{"go test ./...", "go test -race ./...", "go vet ./...", "go build ./..."}
	for index, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		duration, err := strconv.ParseInt(fields[max(0, len(fields)-1)], 10, 64)
		if len(fields) != 4 || fields[0] != want[index] || fields[1] != commands[index] || fields[2] != "0" || err != nil || duration < 0 {
			return false
		}
	}
	return true
}

func runCommandContext(ctx context.Context, directory string, environment []string, name string, arguments ...string) error {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func commandText(ctx context.Context, directory string, environment []string, name string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

func combinedCommandText(ctx context.Context, directory string, environment []string, name string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	output, err := command.CombinedOutput()
	return string(output), err
}
