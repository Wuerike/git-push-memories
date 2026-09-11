package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

type streamPolicy struct {
	ExpectedModel string
	RelevantTerms []string
	AllowedTools  map[string]bool
}

type SessionObservation struct {
	Authenticated          bool                        `json:"authenticated"`
	Model                  string                      `json:"model"`
	Turns                  int                         `json:"turns"`
	CostAvailable          bool                        `json:"cost_available"`
	CostMicroUSD           int64                       `json:"cost_micro_usd,omitempty"`
	IsError                bool                        `json:"is_error"`
	Cancelled              bool                        `json:"cancelled"`
	ToolCounts             map[string]int              `json:"tool_counts"`
	ExplorationCalls       int                         `json:"exploration_calls"`
	ExplorationResultBytes int64                       `json:"exploration_result_bytes"`
	MemoryResultBytes      int64                       `json:"memory_result_bytes"`
	CurationToolBytes      int64                       `json:"curation_tool_bytes"`
	MemoryRelevant         bool                        `json:"memory_relevant"`
	ErrorCodes             []string                    `json:"-"`
	ReferenceIntegrities   []string                    `json:"reference_integrities,omitempty"`
	ProposalArguments      map[string]any              `json:"-"`
	ToolArguments          map[string][]map[string]any `json:"-"`
}

type streamObserver struct {
	policy      streamPolicy
	observation SessionObservation
	toolNames   map[string]string
	terminal    int
	failed      error
}

func newStreamObserver(policy streamPolicy) *streamObserver {
	return &streamObserver{
		policy: policy,
		observation: SessionObservation{
			ToolCounts:    make(map[string]int),
			ToolArguments: make(map[string][]map[string]any),
		},
		toolNames: make(map[string]string),
	}
}

func (observer *streamObserver) consume(line []byte) error {
	if observer.failed != nil {
		return observer.failed
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var event map[string]any
	if err := decoder.Decode(&event); err != nil {
		return observer.fail(fmt.Errorf("decode stream event: %w", err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return observer.fail(errors.New("stream event has trailing content"))
	}
	switch event["type"] {
	case "assistant":
		return observer.consumeAssistant(event)
	case "user":
		return observer.consumeToolResults(event)
	case "result":
		return observer.consumeResult(event)
	default:
		return nil
	}
}

func (observer *streamObserver) finish() (SessionObservation, error) {
	if observer.failed != nil {
		return observer.observation, observer.failed
	}
	if observer.terminal != 1 {
		return observer.observation, fmt.Errorf("terminal result count = %d, want 1", observer.terminal)
	}
	if observer.observation.Model == "" {
		return observer.observation, errors.New("authenticated stream did not report a model")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`).MatchString(observer.observation.Model) {
		return observer.observation, errors.New("stream reported an invalid model ID")
	}
	if observer.policy.ExpectedModel != "" && observer.observation.Model != observer.policy.ExpectedModel {
		return observer.observation, errors.New("stream model does not match preflight")
	}
	return observer.observation, nil
}

func (observer *streamObserver) consumeAssistant(event map[string]any) error {
	message, ok := event["message"].(map[string]any)
	if !ok {
		return observer.fail(errors.New("assistant event has no message"))
	}
	if model, ok := message["model"].(string); ok && model != "" {
		if observer.observation.Model != "" && observer.observation.Model != model {
			return observer.fail(errors.New("stream contains mixed model IDs"))
		}
		observer.observation.Model = model
	}
	contents, ok := message["content"].([]any)
	if !ok {
		return nil
	}
	for _, raw := range contents {
		content, ok := raw.(map[string]any)
		if !ok || content["type"] != "tool_use" {
			continue
		}
		id, idOK := content["id"].(string)
		name, nameOK := content["name"].(string)
		if !idOK || id == "" || !nameOK || name == "" {
			return observer.fail(errors.New("tool use is missing ID or name"))
		}
		if _, duplicate := observer.toolNames[id]; duplicate {
			return observer.fail(errors.New("duplicate tool use ID"))
		}
		if input, exists := content["input"]; exists && toolInputEscapesWorkspace(input) {
			return observer.fail(errors.New("tool input escapes /workspace"))
		}
		observer.toolNames[id] = name
		if observer.policy.AllowedTools != nil && !toolAllowed(name, content["input"], observer.policy.AllowedTools) {
			return observer.fail(errors.New("tool use violates the allowlist"))
		}
		observer.observation.ToolCounts[name]++
		if input, ok := content["input"].(map[string]any); ok {
			observer.observation.ToolArguments[name] = append(observer.observation.ToolArguments[name], input)
		}
		if isExplorationTool(name) {
			observer.observation.ExplorationCalls++
		}
		if name == "mcp__git-push-memories__memory_propose" {
			if input, ok := content["input"].(map[string]any); ok {
				observer.observation.ProposalArguments = input
			}
		}
	}
	return nil
}

func (observer *streamObserver) consumeToolResults(event map[string]any) error {
	message, ok := event["message"].(map[string]any)
	if !ok {
		return nil
	}
	contents, ok := message["content"].([]any)
	if !ok {
		return nil
	}
	for _, raw := range contents {
		content, ok := raw.(map[string]any)
		if !ok || content["type"] != "tool_result" {
			continue
		}
		id, _ := content["tool_use_id"].(string)
		name, exists := observer.toolNames[id]
		if !exists {
			return observer.fail(errors.New("tool result has unknown tool use ID"))
		}
		result := content["content"]
		encoded, err := serializedContent(result)
		if err != nil {
			return observer.fail(err)
		}
		size := int64(len(encoded))
		if isExplorationTool(name) {
			observer.observation.ExplorationResultBytes += size
		}
		if isMemoryTool(name) {
			observer.observation.MemoryResultBytes += size
			if containsRelevantTerm(encoded, observer.policy.RelevantTerms) {
				observer.observation.MemoryRelevant = true
			}
		}
		if strings.HasPrefix(name, "mcp__git-push-memories__") {
			observer.observation.CurationToolBytes += size
		}
		observer.collectClosedEvidence(result)
	}
	return nil
}

func (observer *streamObserver) collectClosedEvidence(value any) {
	if text, ok := value.(string); ok {
		var decoded any
		if json.Unmarshal([]byte(text), &decoded) == nil {
			observer.collectClosedEvidence(decoded)
		}
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				switch strings.ToLower(key) {
				case "code":
					if text == "privacy_sensitive_content" {
						observer.observation.ErrorCodes = appendUnique(observer.observation.ErrorCodes, text)
					}
				case "integrity":
					if text == "verified" || text == "unavailable" || text == "mismatch" || text == "legacy_unverified" {
						observer.observation.ReferenceIntegrities = appendUnique(observer.observation.ReferenceIntegrities, text)
					}
				}
			}
			observer.collectClosedEvidence(nested)
		}
	case []any:
		for _, nested := range typed {
			observer.collectClosedEvidence(nested)
		}
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (observer *streamObserver) consumeResult(event map[string]any) error {
	observer.terminal++
	if observer.terminal != 1 {
		return observer.fail(errors.New("duplicate terminal result"))
	}
	observer.observation.Authenticated = true
	observer.observation.IsError, _ = event["is_error"].(bool)
	if turns, ok := event["num_turns"].(json.Number); ok {
		value, err := turns.Int64()
		if err != nil || value < 0 {
			return observer.fail(errors.New("invalid turn count"))
		}
		observer.observation.Turns = int(value)
	}
	if cost, exists := event["total_cost_usd"]; exists && cost != nil {
		number, ok := cost.(json.Number)
		if !ok {
			return observer.fail(errors.New("invalid reported cost"))
		}
		value, err := number.Float64()
		if err != nil || value < 0 {
			return observer.fail(errors.New("invalid reported cost"))
		}
		observer.observation.CostAvailable = true
		observer.observation.CostMicroUSD = int64(math.Round(value * 1_000_000))
	}
	return nil
}

func (observer *streamObserver) fail(err error) error {
	observer.failed = err
	return err
}

func serializedContent(value any) ([]byte, error) {
	if text, ok := value.(string); ok {
		return []byte(text), nil
	}
	return json.Marshal(value)
}

func containsRelevantTerm(contents []byte, terms []string) bool {
	normalized := strings.ToLower(string(contents))
	for _, term := range terms {
		if strings.Contains(normalized, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func isExplorationTool(name string) bool {
	return name == "Read" || name == "Grep" || name == "Glob"
}

func isMemoryTool(name string) bool {
	return name == "mcp__git-push-memories__memory_search" || name == "mcp__git-push-memories__memory_expand"
}

func toolAllowed(name string, input any, allowed map[string]bool) bool {
	if allowed[name] {
		return true
	}
	if name != "Bash" || (!allowed["Bash(gofmt *)"] && !allowed["Bash(go test ./...)"]) {
		return false
	}
	fields, ok := input.(map[string]any)
	if !ok {
		return false
	}
	command, _ := fields["command"].(string)
	if strings.ContainsAny(command, "\n;&|><`") {
		return false
	}
	return allowed["Bash(go test ./...)"] && command == "go test ./..." ||
		allowed["Bash(gofmt *)"] && strings.HasPrefix(command, "gofmt ") && len(strings.Fields(command)) > 1
}

func toolInputEscapesWorkspace(input any) bool {
	object, ok := input.(map[string]any)
	if !ok {
		return false
	}
	for key, raw := range object {
		if nested, ok := raw.(map[string]any); ok && toolInputEscapesWorkspace(nested) {
			return true
		}
		if !strings.Contains(strings.ToLower(key), "path") && !strings.Contains(strings.ToLower(key), "root") {
			continue
		}
		value, ok := raw.(string)
		if !ok || value == "" {
			continue
		}
		cleaned := filepath.Clean(value)
		if filepath.IsAbs(cleaned) {
			if cleaned != "/workspace" && !strings.HasPrefix(cleaned, "/workspace/") {
				return true
			}
			continue
		}
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return true
		}
	}
	return false
}
