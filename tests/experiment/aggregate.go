package experiment

import (
	"errors"
	"fmt"
	"sort"
)

type PrivacyRecord struct {
	Incidents      int  `json:"incidents"`
	RawEvidence    bool `json:"raw_evidence"`
	SecretRejected bool `json:"secret_rejected"`
}

type CaptureRecord struct {
	UnitID       string             `json:"unit_id"`
	Completed    bool               `json:"completed"`
	Valid        bool               `json:"valid"`
	WithinBudget bool               `json:"within_budget"`
	DurationSeconds int64           `json:"duration_seconds"`
	Observation  SessionObservation `json:"observation"`
}

type CaptureSummary struct {
	Sessions                 []CaptureRecord `json:"runs"`
	DirectDuplicateCompleted bool            `json:"direct_duplicate_completed"`
	ExactDuplicatePassed     bool            `json:"exact_duplicate_passed"`
}

type CurationRecord struct {
	CaseID                    string             `json:"case_id"`
	Class                     string             `json:"class"`
	Completed                 bool               `json:"completed"`
	Valid                     bool               `json:"valid"`
	Correct                   bool               `json:"correct"`
	WithoutProposalCorrection bool               `json:"without_proposal_correction"`
	CanonicalTrustFailure     bool               `json:"canonical_trust_failure"`
	ReferenceMisrepresented   bool               `json:"reference_misrepresented"`
	WithinBudget              bool               `json:"within_budget"`
	DurationSeconds           int64              `json:"duration_seconds"`
	CaptureDurationSeconds    int64              `json:"capture_duration_seconds"`
	ToolResultBytes           int64              `json:"tool_result_bytes"`
	Disposition               string             `json:"disposition,omitempty"`
	Operation                 string             `json:"operation,omitempty"`
	Target                    string             `json:"target,omitempty"`
	EditClass                 string             `json:"edit_class,omitempty"`
	ReferenceIntegrity        string             `json:"reference_integrity,omitempty"`
	SemanticSupport           string             `json:"semantic_support,omitempty"`
	Commit                    string             `json:"commit,omitempty"`
	ChangedPaths              []string           `json:"changed_paths,omitempty"`
	DiffHash                  string             `json:"diff_hash,omitempty"`
	Observation               SessionObservation `json:"observation"`
}

type BlindEvaluation struct {
	EvaluatorValid bool `json:"evaluator_valid"`
	Correctness    bool `json:"correctness"`
	Score          int  `json:"score"`
}

type TaskRecord struct {
	RunID           string             `json:"run_id"`
	PairID          string             `json:"pair_id"`
	Scenario        string             `json:"scenario"`
	Arm             string             `json:"arm"`
	Completed       bool               `json:"completed"`
	Valid           bool               `json:"valid"`
	WithinBudget    bool               `json:"within_budget"`
	DurationSeconds int64              `json:"duration_seconds"`
	Observation     SessionObservation `json:"observation"`
	Evaluation      BlindEvaluation    `json:"evaluation"`
}

type ExperimentRecords struct {
	ExpectedModel string           `json:"expected_model"`
	Privacy       PrivacyRecord    `json:"privacy"`
	Capture       CaptureSummary   `json:"capture"`
	Curations     []CurationRecord `json:"curations"`
	Tasks         []TaskRecord     `json:"tasks"`
}

type PairComparison struct {
	PairID                    string `json:"pair_id"`
	Scenario                  string `json:"scenario"`
	TreatmentRunID            string `json:"treatment_run_id"`
	ControlRunID              string `json:"control_run_id"`
	TreatmentCorrect          bool   `json:"treatment_correct"`
	ControlCorrect            bool   `json:"control_correct"`
	TreatmentScore            int    `json:"treatment_score"`
	ControlScore              int    `json:"control_score"`
	TreatmentExplorationCalls int    `json:"treatment_exploration_calls"`
	ControlExplorationCalls   int    `json:"control_exploration_calls"`
	TreatmentExplorationBytes int64  `json:"treatment_exploration_bytes"`
	ControlExplorationBytes   int64  `json:"control_exploration_bytes"`
	UtilitySuccess            bool   `json:"utility_success"`
}

func Aggregate(records ExperimentRecords) (GateOutcome, []PairComparison, error) {
	if records.ExpectedModel == "" || len(records.Capture.Sessions) != 13 || len(records.Curations) != 12 || len(records.Tasks) != 24 {
		return GateOutcome{}, nil, errors.New("record counts do not match the protocol")
	}
	outcome := GateOutcome{
		PrivacyIncidents:             records.Privacy.Incidents,
		RawPrivateEvidence:           records.Privacy.RawEvidence,
		CaptureSecretRejectionPassed: records.Privacy.SecretRejected,
		CaptureExactDuplicatePassed:  records.Capture.ExactDuplicatePassed,
		UtilitySuccessByScenario:     make(map[string]int),
		AllSessionBudgetsRespected:   true,
	}
	costAvailability := make([]bool, 0, 49)
	seen := make(map[string]bool)
	for _, capture := range records.Capture.Sessions {
		if capture.UnitID == "" || seen[capture.UnitID] {
			return GateOutcome{}, nil, errors.New("duplicate or empty capture ID")
		}
		seen[capture.UnitID] = true
		if capture.Completed {
			outcome.CaptureAttemptsCompleted++
		}
		if !capture.Valid {
			outcome.InvalidSessions++
		}
		if capture.Valid && capture.Observation.Model != records.ExpectedModel {
			return GateOutcome{}, nil, errors.New("valid capture has wrong model")
		}
		outcome.AllSessionBudgetsRespected = outcome.AllSessionBudgetsRespected && capture.WithinBudget
		if capture.Completed {
			costAvailability = append(costAvailability, capture.Observation.CostAvailable)
		}
	}
	if records.Capture.DirectDuplicateCompleted {
		outcome.CaptureAttemptsCompleted++
	}

	seen = make(map[string]bool)
	for _, curation := range records.Curations {
		if curation.CaseID == "" || seen[curation.CaseID] {
			return GateOutcome{}, nil, errors.New("duplicate or empty curation case ID")
		}
		seen[curation.CaseID] = true
		if curation.Completed {
			outcome.CurationCasesCompleted++
		}
		if !curation.Valid {
			outcome.InvalidSessions++
		}
		if curation.Valid && curation.Observation.Model != records.ExpectedModel {
			return GateOutcome{}, nil, errors.New("valid curation has wrong model")
		}
		switch curation.Class {
		case "canonical":
			if curation.Correct {
				outcome.CorrectCanonicalCases++
				if curation.WithoutProposalCorrection {
					outcome.AcceptedWithoutCorrection++
				}
			}
		case "negative":
			if curation.Correct {
				outcome.CorrectNegativeDispositions++
			}
		default:
			return GateOutcome{}, nil, errors.New("unknown curation class")
		}
		if curation.CanonicalTrustFailure {
			outcome.CanonicalTrustFailures++
		}
		if curation.ReferenceMisrepresented {
			outcome.ReferenceMisrepresentations++
		}
		outcome.CurationDurationsSeconds = append(outcome.CurationDurationsSeconds, curation.DurationSeconds)
		outcome.CapturePlusCurateSeconds = append(outcome.CapturePlusCurateSeconds, curation.CaptureDurationSeconds+curation.DurationSeconds)
		outcome.CurationToolResultBytes = append(outcome.CurationToolResultBytes, curation.ToolResultBytes)
		outcome.AllSessionBudgetsRespected = outcome.AllSessionBudgetsRespected && curation.WithinBudget
		if curation.Completed {
			costAvailability = append(costAvailability, curation.Observation.CostAvailable)
		}
	}

	tasksByPair := make(map[string]map[string]TaskRecord)
	seen = make(map[string]bool)
	for _, task := range records.Tasks {
		if task.RunID == "" || seen[task.RunID] || (task.Arm != "control" && task.Arm != "treatment") {
			return GateOutcome{}, nil, errors.New("duplicate task ID or invalid arm")
		}
		seen[task.RunID] = true
		if task.Valid && task.Observation.Model != records.ExpectedModel {
			return GateOutcome{}, nil, errors.New("valid task has wrong model")
		}
		if tasksByPair[task.PairID] == nil {
			tasksByPair[task.PairID] = make(map[string]TaskRecord)
		}
		if _, duplicate := tasksByPair[task.PairID][task.Arm]; duplicate {
			return GateOutcome{}, nil, errors.New("pair contains a duplicate arm")
		}
		tasksByPair[task.PairID][task.Arm] = task
		if task.Completed {
			outcome.TaskSessionsCompleted++
		}
		if !task.Valid || !task.Evaluation.EvaluatorValid {
			outcome.InvalidSessions++
		}
		if task.Observation.MemoryResultBytes > outcome.MaximumTaskMemoryResultBytes {
			outcome.MaximumTaskMemoryResultBytes = task.Observation.MemoryResultBytes
		}
		outcome.AllSessionBudgetsRespected = outcome.AllSessionBudgetsRespected && task.WithinBudget
		if task.Completed {
			costAvailability = append(costAvailability, task.Observation.CostAvailable)
		}
	}
	if len(tasksByPair) != 12 {
		return GateOutcome{}, nil, errors.New("pair count does not match the protocol")
	}

	pairIDs := make([]string, 0, len(tasksByPair))
	for pairID := range tasksByPair {
		pairIDs = append(pairIDs, pairID)
	}
	sort.Strings(pairIDs)
	comparisons := make([]PairComparison, 0, 12)
	for index, pairID := range pairIDs {
		arms := tasksByPair[pairID]
		control, controlOK := arms["control"]
		treatment, treatmentOK := arms["treatment"]
		expectedScenario := []string{"lease-renewal", "webhook-grace", "settlement-window", "regional-routing", "feature-rollout", "cache-ttl"}[index/2]
		if !controlOK || !treatmentOK || control.Scenario != treatment.Scenario || control.Scenario != expectedScenario || pairID != fmt.Sprintf("pair-%02d", index+1) {
			return GateOutcome{}, nil, errors.New("pair assignment does not match the protocol")
		}
		if control.Completed && treatment.Completed {
			outcome.CompletedPairs++
		}
		if treatment.Observation.MemoryRelevant {
			outcome.RelevantTreatmentRetrievals++
		}
		if treatment.Evaluation.Correctness {
			outcome.TreatmentCorrectPairs++
		}
		if control.Evaluation.Correctness && !treatment.Evaluation.Correctness {
			outcome.TreatmentCorrectnessRegressions++
		}
		if treatment.Evaluation.Score < control.Evaluation.Score {
			outcome.TreatmentScoreRegressions++
		}
		utility := IsUtilitySuccess(PairOutcome{
			TreatmentCorrect: treatment.Evaluation.Correctness, TreatmentScore: treatment.Evaluation.Score, ControlScore: control.Evaluation.Score,
			TreatmentExploreCalls: treatment.Observation.ExplorationCalls, ControlExploreCalls: control.Observation.ExplorationCalls,
			TreatmentExploreBytes: int(treatment.Observation.ExplorationResultBytes), ControlExploreBytes: int(control.Observation.ExplorationResultBytes),
		})
		if utility {
			outcome.UtilitySuccessPairs++
			outcome.UtilitySuccessByScenario[treatment.Scenario]++
		} else if _, exists := outcome.UtilitySuccessByScenario[treatment.Scenario]; !exists {
			outcome.UtilitySuccessByScenario[treatment.Scenario] = 0
		}
		outcome.TreatmentExplorationCalls += treatment.Observation.ExplorationCalls
		outcome.ControlExplorationCalls += control.Observation.ExplorationCalls
		outcome.TreatmentExplorationBytes += int(treatment.Observation.ExplorationResultBytes)
		outcome.ControlExplorationBytes += int(control.Observation.ExplorationResultBytes)
		outcome.TreatmentTaskDurationsSeconds = append(outcome.TreatmentTaskDurationsSeconds, treatment.DurationSeconds)
		outcome.ControlTaskDurationsSeconds = append(outcome.ControlTaskDurationsSeconds, control.DurationSeconds)
		outcome.TreatmentCostMicroUSD += treatment.Observation.CostMicroUSD
		outcome.ControlCostMicroUSD += control.Observation.CostMicroUSD
		comparisons = append(comparisons, PairComparison{
			PairID: pairID, Scenario: treatment.Scenario, TreatmentRunID: treatment.RunID, ControlRunID: control.RunID,
			TreatmentCorrect: treatment.Evaluation.Correctness, ControlCorrect: control.Evaluation.Correctness,
			TreatmentScore: treatment.Evaluation.Score, ControlScore: control.Evaluation.Score,
			TreatmentExplorationCalls: treatment.Observation.ExplorationCalls, ControlExplorationCalls: control.Observation.ExplorationCalls,
			TreatmentExplorationBytes: treatment.Observation.ExplorationResultBytes, ControlExplorationBytes: control.Observation.ExplorationResultBytes,
			UtilitySuccess: utility,
		})
	}

	available := 0
	for _, value := range costAvailability {
		if value {
			available++
		}
	}
	switch available {
	case 0:
		outcome.CostAvailability = "none"
	case len(costAvailability):
		outcome.CostAvailability = "all"
	default:
		outcome.CostAvailability = "mixed"
		outcome.InvalidSessions++
	}
	return outcome, comparisons, nil
}
