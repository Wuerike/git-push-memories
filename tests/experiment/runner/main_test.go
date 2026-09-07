package main

import (
	"strings"
	"testing"
)

func TestScanClaudeStreamKeepsOnlyMetrics(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"model":"claude-test","content":[{"type":"tool_use","id":"read-1","name":"Read","input":{"file_path":"secret"}},{"type":"tool_use","id":"memory-1","name":"mcp__git-push-memories__memory_search","input":{"query":"secret"}},{"type":"text","text":"runtime response"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"memory-1","content":"Priority parcels use 90 seconds, route-amber, routing-ops"}]}}`,
		`{"type":"result","is_error":false,"total_cost_usd":0.12,"num_turns":2,"result":"must not survive"}`,
	}, "\n")
	metric := sessionMetric{RunID: "run-01", ToolCounts: map[string]int{}}
	if err := scanClaudeStream(strings.NewReader(stream), true, &metric); err != nil {
		t.Fatal(err)
	}
	if !metric.Authenticated || metric.Model != "claude-test" || metric.ExplorationCalls != 1 || metric.ToolCounts["Read"] != 1 || !metric.MemoryRelevant {
		t.Fatalf("metric = %+v", metric)
	}
}

func TestMemoryHashUsesProductEncoding(t *testing.T) {
	if got := memoryHash([]byte("memory")); got != "sha256:c064fbca9d9de8dd9bb0624984403b28d0da807a69365d4f7fb09123ecb0c405" {
		t.Fatalf("memoryHash() = %s", got)
	}
}

func TestDecisionRuleUsesThreeIndependentPairs(t *testing.T) {
	registered := experiment{Threshold: 0.25, RequiredPairs: 2}
	assignments := map[string]string{
		"run-01": "treatment", "run-02": "control", "run-03": "control",
		"run-04": "treatment", "run-05": "treatment", "run-06": "control",
	}
	metrics := []sessionMetric{
		metric("run-01", 1, true), metric("run-02", 4, false), metric("run-03", 4, false),
		metric("run-04", 3, true), metric("run-05", 2, true), metric("run-06", 2, false),
	}
	evaluations := make([]evaluation, 6)
	for index := range evaluations {
		evaluations[index] = evaluation{RunID: metrics[index].RunID, EvaluatorValid: true, Score: 10, Correctness: true}
	}
	comparisons, improved, decision := decide(registered, metrics, evaluations, assignments)
	if len(comparisons) != 3 || improved != 2 || decision != "continue" {
		t.Fatalf("comparisons=%+v improved=%d decision=%s", comparisons, improved, decision)
	}
	evaluations[0].Correctness = false
	_, _, decision = decide(registered, metrics, evaluations, assignments)
	if decision != "did_not_demonstrate_value" {
		t.Fatalf("regression decision = %s", decision)
	}
	metrics[0].ToolCounts["mcp__git-push-memories__memory_search"] = 0
	_, _, decision = decide(registered, metrics, evaluations, assignments)
	if decision != "did_not_demonstrate_value" {
		t.Fatalf("observed regression with operational failure decision = %s", decision)
	}
}

func TestDecisionRuleCanUseTaskDuration(t *testing.T) {
	registered := experiment{Metric: "task_duration_ms", Threshold: 0.25, RequiredPairs: 2}
	assignments := map[string]string{
		"run-01": "treatment", "run-02": "control", "run-03": "control",
		"run-04": "treatment", "run-05": "treatment", "run-06": "control",
	}
	metrics := []sessionMetric{
		metric("run-01", 4, true), metric("run-02", 4, false), metric("run-03", 4, false),
		metric("run-04", 4, true), metric("run-05", 4, true), metric("run-06", 4, false),
	}
	for index, duration := range []int64{70, 100, 100, 75, 90, 100} {
		metrics[index].DurationMS = duration
	}
	evaluations := make([]evaluation, 6)
	for index := range evaluations {
		evaluations[index] = evaluation{RunID: metrics[index].RunID, EvaluatorValid: true, Score: 10, Correctness: true}
	}
	comparisons, improved, decision := decide(registered, metrics, evaluations, assignments)
	if improved != 2 || decision != "continue" || comparisons[0].Improvement != 0.3 || comparisons[1].Improvement != 0.25 {
		t.Fatalf("comparisons=%+v improved=%d decision=%s", comparisons, improved, decision)
	}
}

func metric(runID string, exploration int, treatment bool) sessionMetric {
	tools := map[string]int{}
	if treatment {
		tools["mcp__git-push-memories__memory_search"] = 1
	}
	return sessionMetric{
		RunID: runID, Authenticated: true, Model: "claude-test", ExplorationCalls: exploration,
		ToolCounts: tools, MemoryRelevant: treatment,
	}
}
