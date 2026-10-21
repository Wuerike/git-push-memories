package experiment

import (
	"cmp"
	"math"
	"slices"
)

type Decision string

const (
	DecisionAdvance      Decision = "avancar"
	DecisionRevise       Decision = "revisar_o_fluxo"
	DecisionStop         Decision = "interromper"
	Phase3ProtocolSHA256          = "41936567f2489a5e5336ebc348c3357e90e247df12bf0070d6dc86cfee7b9992"
)

type GateOutcome struct {
	PrivacyIncidents                int
	RawPrivateEvidence              bool
	CaptureSecretRejectionPassed    bool
	CanonicalTrustFailures          int
	ReferenceMisrepresentations     int
	TreatmentCorrectnessRegressions int
	TreatmentScoreRegressions       int
	InvalidSessions                 int
	CaptureAttemptsCompleted        int
	CurationCasesCompleted          int
	TaskSessionsCompleted           int
	CompletedPairs                  int
	CorrectCanonicalCases           int
	CorrectNegativeDispositions     int
	AcceptedWithoutCorrection       int
	CaptureExactDuplicatePassed     bool
	CurationDurationsSeconds        []int64
	CapturePlusCurateSeconds        []int64
	CurationToolResultBytes         []int64
	RelevantTreatmentRetrievals     int
	MaximumTaskMemoryResultBytes    int64
	TreatmentCorrectPairs           int
	UtilitySuccessPairs             int
	UtilitySuccessByScenario        map[string]int
	TreatmentExplorationCalls       int
	ControlExplorationCalls         int
	TreatmentExplorationBytes       int
	ControlExplorationBytes         int
	TreatmentTaskDurationsSeconds   []int64
	ControlTaskDurationsSeconds     []int64
	CostAvailability                string
	TreatmentCostMicroUSD           int64
	ControlCostMicroUSD             int64
	AllSessionBudgetsRespected      bool
}

func Decide(outcome GateOutcome) Decision {
	switch {
	case outcome.PrivacyIncidents != 0 || outcome.RawPrivateEvidence || !outcome.CaptureSecretRejectionPassed ||
		outcome.CanonicalTrustFailures != 0 || outcome.ReferenceMisrepresentations != 0 ||
		outcome.TreatmentCorrectnessRegressions != 0 || outcome.TreatmentScoreRegressions != 0:
		return DecisionStop
	case outcome.InvalidSessions != 0 || outcome.CostAvailability == "mixed" ||
		outcome.CaptureAttemptsCompleted != 14 || outcome.CurationCasesCompleted != 12 ||
		outcome.TaskSessionsCompleted != 24 || outcome.CompletedPairs != 12:
		return DecisionRevise
	case outcome.CorrectCanonicalCases <= 5 || outcome.UtilitySuccessPairs <= 3:
		return DecisionStop
	case advanceThresholdsPass(outcome):
		return DecisionAdvance
	default:
		return DecisionRevise
	}
}

func advanceThresholdsPass(outcome GateOutcome) bool {
	if outcome.CorrectCanonicalCases != 9 || outcome.CorrectNegativeDispositions != 3 || outcome.AcceptedWithoutCorrection < 6 ||
		!outcome.CaptureExactDuplicatePassed || !outcome.AllSessionBudgetsRespected ||
		len(outcome.CurationDurationsSeconds) != 12 || NearestRank(outcome.CurationDurationsSeconds, 0.5) > 180 || NearestRank(outcome.CurationDurationsSeconds, 0.9) > 360 ||
		len(outcome.CapturePlusCurateSeconds) != 12 || NearestRank(outcome.CapturePlusCurateSeconds, 0.5) > 300 ||
		len(outcome.CurationToolResultBytes) != 12 || NearestRank(outcome.CurationToolResultBytes, 0.9) > 65536 ||
		outcome.RelevantTreatmentRetrievals < 10 || outcome.MaximumTaskMemoryResultBytes > 8192 ||
		outcome.TreatmentCorrectPairs != 12 || outcome.UtilitySuccessPairs < 9 || !scenarioThresholdPasses(outcome.UtilitySuccessByScenario) ||
		!atLeastQuarterLower(outcome.TreatmentExplorationCalls, outcome.ControlExplorationCalls) ||
		!atLeastQuarterLower(outcome.TreatmentExplorationBytes, outcome.ControlExplorationBytes) ||
		!medianRatioAtMost(outcome.TreatmentTaskDurationsSeconds, outcome.ControlTaskDurationsSeconds, 5, 4) {
		return false
	}
	switch outcome.CostAvailability {
	case "none":
		return true
	case "all":
		return ratioAtMost(outcome.TreatmentCostMicroUSD, outcome.ControlCostMicroUSD, 5, 4)
	default:
		return false
	}
}

func ThresholdResults(outcome GateOutcome) map[string]bool {
	return map[string]bool{
		"privacy":               outcome.PrivacyIncidents == 0 && !outcome.RawPrivateEvidence && outcome.CaptureSecretRejectionPassed,
		"canonical_trust":       outcome.CanonicalTrustFailures == 0,
		"reference_integrity":   outcome.ReferenceMisrepresentations == 0,
		"capture_completeness":  outcome.CaptureAttemptsCompleted == 14,
		"exact_duplicate":       outcome.CaptureExactDuplicatePassed,
		"canonical_curation":    outcome.CorrectCanonicalCases == 9,
		"negative_dispositions": outcome.CorrectNegativeDispositions == 3,
		"proposal_preservation": outcome.AcceptedWithoutCorrection >= 6,
		"curation_duration":     len(outcome.CurationDurationsSeconds) == 12 && NearestRank(outcome.CurationDurationsSeconds, 0.5) <= 180 && NearestRank(outcome.CurationDurationsSeconds, 0.9) <= 360,
		"capture_plus_curate":   len(outcome.CapturePlusCurateSeconds) == 12 && NearestRank(outcome.CapturePlusCurateSeconds, 0.5) <= 300,
		"curation_context":      len(outcome.CurationToolResultBytes) == 12 && NearestRank(outcome.CurationToolResultBytes, 0.9) <= 65536,
		"task_completeness":     outcome.TaskSessionsCompleted == 24 && outcome.CompletedPairs == 12,
		"treatment_correctness": outcome.TreatmentCorrectPairs == 12,
		"task_non_regression":   outcome.TreatmentCorrectnessRegressions == 0 && outcome.TreatmentScoreRegressions == 0,
		"relevant_retrieval":    outcome.RelevantTreatmentRetrievals >= 10,
		"memory_context":        outcome.MaximumTaskMemoryResultBytes <= 8192,
		"utility_total":         outcome.UtilitySuccessPairs >= 9,
		"utility_per_scenario":  scenarioThresholdPasses(outcome.UtilitySuccessByScenario),
		"exploration_reduction": atLeastQuarterLower(outcome.TreatmentExplorationCalls, outcome.ControlExplorationCalls) && atLeastQuarterLower(outcome.TreatmentExplorationBytes, outcome.ControlExplorationBytes),
		"task_duration":         medianRatioAtMost(outcome.TreatmentTaskDurationsSeconds, outcome.ControlTaskDurationsSeconds, 5, 4),
		"reported_cost":         outcome.CostAvailability == "none" || outcome.CostAvailability == "all" && ratioAtMost(outcome.TreatmentCostMicroUSD, outcome.ControlCostMicroUSD, 5, 4),
		"validity":              outcome.InvalidSessions == 0 && outcome.CostAvailability != "mixed",
		"per_unit_budget":       outcome.AllSessionBudgetsRespected,
	}
}

type PairOutcome struct {
	TreatmentCorrect      bool
	TreatmentScore        int
	ControlScore          int
	TreatmentExploreCalls int
	ControlExploreCalls   int
	TreatmentExploreBytes int
	ControlExploreBytes   int
}

func IsUtilitySuccess(outcome PairOutcome) bool {
	if !outcome.TreatmentCorrect || outcome.TreatmentScore < outcome.ControlScore {
		return false
	}
	if outcome.TreatmentScore > outcome.ControlScore {
		return true
	}
	return outcome.ControlExploreCalls > 0 && outcome.ControlExploreBytes > 0 &&
		atLeastQuarterLower(outcome.TreatmentExploreCalls, outcome.ControlExploreCalls) &&
		atLeastQuarterLower(outcome.TreatmentExploreBytes, outcome.ControlExploreBytes)
}

func NearestRank(values []int64, percentile float64) int64 {
	if len(values) == 0 || percentile <= 0 || percentile > 1 {
		return 0
	}
	ordered := append([]int64(nil), values...)
	slices.SortFunc(ordered, cmp.Compare)
	rank := int(math.Ceil(percentile*float64(len(ordered)))) - 1
	return ordered[rank]
}

func atLeastQuarterLower(treatment, control int) bool {
	return treatment >= 0 && control > 0 && treatment*4 <= control*3
}

func scenarioThresholdPasses(successes map[string]int) bool {
	want := []string{"lease-renewal", "webhook-grace", "settlement-window", "regional-routing", "feature-rollout", "cache-ttl"}
	if len(successes) != len(want) {
		return false
	}
	for _, scenario := range want {
		if successes[scenario] < 1 {
			return false
		}
	}
	return true
}

func medianRatioAtMost(treatment, control []int64, numerator, denominator int64) bool {
	if len(treatment) != 12 || len(control) != 12 {
		return false
	}
	return ratioAtMost(NearestRank(treatment, 0.5), NearestRank(control, 0.5), numerator, denominator)
}

func ratioAtMost(treatment, control, numerator, denominator int64) bool {
	return treatment >= 0 && control > 0 && treatment*denominator <= control*numerator
}
