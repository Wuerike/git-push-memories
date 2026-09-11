package experiment

import "testing"

func TestAggregateProducesPassingGateFromIndependentRecords(t *testing.T) {
	records := passingExperimentRecords()
	outcome, pairs, err := Aggregate(records)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 12 || Decide(outcome) != DecisionAdvance {
		t.Fatalf("outcome=%+v pairs=%+v", outcome, pairs)
	}
	if outcome.UtilitySuccessPairs != 12 || outcome.TreatmentCorrectPairs != 12 || outcome.RelevantTreatmentRetrievals != 12 {
		t.Fatalf("derived outcome = %+v", outcome)
	}
}

func TestAggregateRejectsInconsistentRecords(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*ExperimentRecords)
	}{
		{name: "duplicate task", mutate: func(records *ExperimentRecords) { records.Tasks[1].RunID = records.Tasks[0].RunID }},
		{name: "missing arm", mutate: func(records *ExperimentRecords) { records.Tasks = records.Tasks[:23] }},
		{name: "scenario mismatch", mutate: func(records *ExperimentRecords) { records.Tasks[0].Scenario = "cache-ttl" }},
		{name: "duplicate curation", mutate: func(records *ExperimentRecords) { records.Curations[1].CaseID = records.Curations[0].CaseID }},
	} {
		t.Run(test.name, func(t *testing.T) {
			records := passingExperimentRecords()
			test.mutate(&records)
			if _, _, err := Aggregate(records); err == nil {
				t.Fatal("inconsistent records were accepted")
			}
		})
	}
}

func TestAggregatePreservesMixedCostAsInvalidOutcome(t *testing.T) {
	records := passingExperimentRecords()
	records.Tasks[0].Observation.CostAvailable = true
	outcome, _, err := Aggregate(records)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.CostAvailability != "mixed" || outcome.InvalidSessions == 0 || Decide(outcome) != DecisionRevise {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func passingExperimentRecords() ExperimentRecords {
	records := ExperimentRecords{
		ExpectedModel: "model",
		Privacy:       PrivacyRecord{SecretRejected: true},
		Capture:       CaptureSummary{DirectDuplicateCompleted: true, ExactDuplicatePassed: true},
	}
	for index := 1; index <= 13; index++ {
		records.Capture.Sessions = append(records.Capture.Sessions, CaptureRecord{
			UnitID: "capture-" + twoDigits(index), Completed: true, Valid: true, WithinBudget: true,
			Observation: SessionObservation{Authenticated: true, Model: "model", ToolCounts: map[string]int{}},
		})
	}
	for index := 1; index <= 12; index++ {
		class := "canonical"
		correctWithoutCorrection := index <= 6
		if index > 9 {
			class = "negative"
			correctWithoutCorrection = false
		}
		records.Curations = append(records.Curations, CurationRecord{
			CaseID: "cur-" + twoDigits(index), Class: class, Completed: true, Valid: true, Correct: true,
			WithoutProposalCorrection: correctWithoutCorrection, DurationSeconds: 100, CaptureDurationSeconds: 100, ToolResultBytes: 1000,
			WithinBudget: true, Observation: SessionObservation{Authenticated: true, Model: "model", ToolCounts: map[string]int{}},
		})
	}
	scenarios := []string{"lease-renewal", "webhook-grace", "settlement-window", "regional-routing", "feature-rollout", "cache-ttl"}
	for pairIndex := 1; pairIndex <= 12; pairIndex++ {
		pairID := "pair-" + twoDigits(pairIndex)
		scenario := scenarios[(pairIndex-1)/2]
		for _, arm := range []string{"control", "treatment"} {
			explorationCalls, explorationBytes := 4, int64(1000)
			memoryRelevant := false
			if arm == "treatment" {
				explorationCalls, explorationBytes, memoryRelevant = 3, 750, true
			}
			records.Tasks = append(records.Tasks, TaskRecord{
				RunID: pairID + "-" + arm, PairID: pairID, Scenario: scenario, Arm: arm, Completed: true, Valid: true, WithinBudget: true, DurationSeconds: 100,
				Observation: SessionObservation{Authenticated: true, Model: "model", ToolCounts: map[string]int{}, ExplorationCalls: explorationCalls, ExplorationResultBytes: explorationBytes, MemoryResultBytes: 1000, MemoryRelevant: memoryRelevant},
				Evaluation:  BlindEvaluation{EvaluatorValid: true, Correctness: true, Score: 10},
			})
		}
	}
	return records
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return "" + string(rune('0'+value/10)) + string(rune('0'+value%10))
}
