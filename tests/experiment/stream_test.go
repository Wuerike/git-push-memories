package experiment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStreamObserverReducesAuthenticatedEvents(t *testing.T) {
	observer := newStreamObserver(streamPolicy{
		ExpectedModel: "claude-sonnet-test",
		RelevantTerms: []string{"three business days", "settlement"},
	})
	stream := strings.Join([]string{
		`{"type":"assistant","message":{"model":"claude-sonnet-test","content":[{"type":"tool_use","id":"read-1","name":"Read","input":{"file_path":"/workspace/policy/policy.go"}},{"type":"tool_use","id":"memory-1","name":"mcp__git-push-memories__memory_search","input":{"query":"settlement"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"read-1","content":"package policy"},{"type":"tool_result","tool_use_id":"memory-1","content":"BR settlement completes in THREE BUSINESS DAYS."}]}}`,
		`{"type":"result","is_error":false,"total_cost_usd":0.125,"num_turns":3}`,
	}, "\n")
	for _, line := range strings.Split(stream, "\n") {
		if err := observer.consume([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	observation, err := observer.finish()
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Authenticated || observation.Model != "claude-sonnet-test" || observation.Turns != 3 || !observation.CostAvailable || observation.CostMicroUSD != 125000 {
		t.Fatalf("terminal observation = %+v", observation)
	}
	if observation.ToolCounts["Read"] != 1 || observation.ToolCounts["mcp__git-push-memories__memory_search"] != 1 || observation.ExplorationCalls != 1 {
		t.Fatalf("tool metrics = %+v", observation)
	}
	if observation.ExplorationResultBytes != int64(len("package policy")) || observation.MemoryResultBytes != int64(len("BR settlement completes in THREE BUSINESS DAYS.")) || !observation.MemoryRelevant {
		t.Fatalf("result metrics = %+v", observation)
	}
}

func TestStreamObserverRejectsInvalidOrLeakingStreams(t *testing.T) {
	for _, test := range []struct {
		name  string
		lines []string
	}{
		{name: "malformed", lines: []string{`{"type":`, `{"type":"result","is_error":false}`}},
		{name: "missing result", lines: []string{`{"type":"assistant","message":{"model":"expected","content":[]}}`}},
		{name: "duplicate result", lines: []string{`{"type":"result","is_error":false}`, `{"type":"result","is_error":false}`}},
		{name: "wrong model", lines: []string{`{"type":"assistant","message":{"model":"other","content":[]}}`, `{"type":"result","is_error":false}`}},
		{name: "absolute path leak", lines: []string{`{"type":"assistant","message":{"model":"expected","content":[{"type":"tool_use","id":"read-1","name":"Read","input":{"file_path":"/home/private/gold.json"}}]}}`, `{"type":"result","is_error":false}`}},
		{name: "parent path leak", lines: []string{`{"type":"assistant","message":{"model":"expected","content":[{"type":"tool_use","id":"glob-1","name":"Glob","input":{"path":"../testdata"}}]}}`, `{"type":"result","is_error":false}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer := newStreamObserver(streamPolicy{ExpectedModel: "expected"})
			var consumeErr error
			for _, line := range test.lines {
				if err := observer.consume([]byte(line)); err != nil {
					consumeErr = err
					break
				}
			}
			if _, finishErr := observer.finish(); consumeErr == nil && finishErr == nil {
				t.Fatal("invalid stream was accepted")
			}
		})
	}
}

func TestStreamObserverEnforcesBashCommandPatterns(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		valid   bool
	}{
		{name: "tests", command: "go test ./...", valid: true},
		{name: "format", command: "gofmt -w policy/policy.go", valid: true},
		{name: "extra command", command: "go test ./...; cat /config/.credentials.json", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer := newStreamObserver(streamPolicy{ExpectedModel: "expected", AllowedTools: map[string]bool{"Bash(go test ./...)": true, "Bash(gofmt *)": true}})
			line := `{"type":"assistant","message":{"model":"expected","content":[{"type":"tool_use","id":"bash-1","name":"Bash","input":{"command":` + quotedJSON(test.command) + `}}]}}`
			err := observer.consume([]byte(line))
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && err == nil {
				t.Fatal("invalid Bash command was accepted")
			}
		})
	}
}

func quotedJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
