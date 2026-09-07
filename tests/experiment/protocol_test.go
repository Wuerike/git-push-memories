package experiment_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type protocol struct {
	SchemaVersion int      `json:"schema_version"`
	ExperimentID  string   `json:"experiment_id"`
	Seed          string   `json:"randomization_seed"`
	Model         string   `json:"model"`
	ClaudeVersion string   `json:"claude_code_version"`
	Effort        string   `json:"effort"`
	Autocompact   string   `json:"autocompact"`
	MaxBudget     float64  `json:"max_budget_usd_per_session"`
	Bootstrap     bool     `json:"memory_bootstrap"`
	HumanTurns    []string `json:"human_turns"`
	Profile       struct {
		Filesystem        string   `json:"filesystem"`
		PermissionMode    string   `json:"permission_mode"`
		PermissionPrompts string   `json:"permission_prompts"`
		SessionATools     []string `json:"session_a_tools"`
	} `json:"execution_profile"`
	PrimaryMetric string            `json:"primary_metric"`
	Threshold     float64           `json:"improvement_threshold"`
	RequiredPairs int               `json:"required_improved_pairs"`
	FixtureHashes map[string]string `json:"fixture_hashes"`
	Pairs         []struct {
		Pair      int      `json:"pair"`
		OrderHash string   `json:"order_hash"`
		Order     []string `json:"order"`
	} `json:"pairs"`
}

func TestValueExperimentProtocolIsFrozenAndSelfConsistent(t *testing.T) {
	root := repositoryRoot(t)
	fixtureRoot := filepath.Join(root, "testdata", "phase-2", "value-experiment")
	var registered protocol
	decodeFile(t, filepath.Join(fixtureRoot, "experiment.json"), &registered)
	if registered.SchemaVersion != 1 || registered.Model != "sonnet" || registered.Effort != "medium" {
		t.Fatalf("unexpected execution config: %+v", registered)
	}
	if registered.PrimaryMetric != "code_exploration_tool_calls" || registered.Threshold != 0.25 || registered.RequiredPairs != 2 {
		t.Fatalf("unexpected decision config: %+v", registered)
	}
	if len(registered.Pairs) != 3 {
		t.Fatalf("pair count = %d", len(registered.Pairs))
	}
	for index, pair := range registered.Pairs {
		hash := sha256.Sum256([]byte(registered.Seed + ":" + string(rune('1'+index))))
		order := []string{"control", "treatment"}
		if hash[0]%2 == 1 {
			order = []string{"treatment", "control"}
		}
		if pair.Pair != index+1 || pair.OrderHash != hex.EncodeToString(hash[:]) || !reflect.DeepEqual(pair.Order, order) {
			t.Fatalf("pair %d randomization does not match seed", index+1)
		}
	}
	files := map[string]string{
		"session_a": "session-a.md", "session_a_system": "session-a-system.md", "task": "task.md", "task_system": "task-system.md",
		"rubric": "rubric.json", "memory_initial": "memory-initial/domains/parcel-dispatch.md", "memory_expected": "memory-expected/domains/parcel-dispatch.md",
		"code_go_mod": "code-seed/go.mod", "policy_initial": "code-seed/dispatch/policy.go", "policy_public_test": "code-seed/dispatch/policy_test.go",
		"current_alerts": "code-seed/deploy/production-alerts.yaml", "queue_ownership": "code-seed/ops/queues.yaml", "legacy_policy": "code-seed/docs/legacy-policy.md",
		"hidden_evaluator": "evaluator/policy_hidden_test.go",
	}
	for name, relative := range files {
		if got := hashFile(t, filepath.Join(fixtureRoot, filepath.FromSlash(relative))); got != registered.FixtureHashes[name] {
			t.Fatalf("fixture %s hash = %s, registered %s", name, got, registered.FixtureHashes[name])
		}
	}
}

func TestValueExperimentStartsSolvableButUnimplemented(t *testing.T) {
	root := repositoryRoot(t)
	fixtureRoot := filepath.Join(root, "testdata", "phase-2", "value-experiment")
	workspace := filepath.Join(t.TempDir(), "workspace")
	copyTree(t, filepath.Join(fixtureRoot, "code-seed"), workspace)
	run(t, workspace, "go", "test", "./...")
	copyFile(t, filepath.Join(fixtureRoot, "evaluator", "policy_hidden_test.go"), filepath.Join(workspace, "dispatch", "policy_hidden_test.go"))
	command := exec.Command("go", "test", "./...")
	command.Dir = workspace
	if err := command.Run(); err == nil {
		t.Fatal("hidden evaluator passed before the task was implemented")
	}
	initial := readFile(t, filepath.Join(fixtureRoot, "memory-initial", "domains", "parcel-dispatch.md"))
	expected := readFile(t, filepath.Join(fixtureRoot, "memory-expected", "domains", "parcel-dispatch.md"))
	if bytes.Equal(initial, expected) || !reflect.DeepEqual(anchorLines(initial), anchorLines(expected)) {
		t.Fatal("memory fixtures do not represent an anchored replacement")
	}
}

func TestBootstrapValueExperimentProtocolIsFrozenAndSelfConsistent(t *testing.T) {
	root := repositoryRoot(t)
	fixtureRoot := filepath.Join(root, "testdata", "phase-2", "bootstrap-value-experiment")
	var registered protocol
	decodeFile(t, filepath.Join(fixtureRoot, "experiment.json"), &registered)
	if registered.SchemaVersion != 2 || registered.ExperimentID != "parcel-dispatch-bootstrap-value-v1" || registered.Model != "sonnet" || registered.ClaudeVersion != "2.1.263" || registered.Effort != "medium" || registered.Autocompact != "100k" || registered.MaxBudget != 1 || !registered.Bootstrap || registered.PrimaryMetric != "task_duration_ms" || registered.Threshold != 0.25 || registered.RequiredPairs != 2 {
		t.Fatalf("unexpected bootstrap execution config: %+v", registered)
	}
	if !reflect.DeepEqual(registered.HumanTurns, []string{"create a root page", "confirm"}) || registered.Profile.Filesystem != "bubblewrap-workspace-only" || registered.Profile.PermissionMode != "acceptEdits" || registered.Profile.PermissionPrompts != "none" || !reflect.DeepEqual(registered.Profile.SessionATools, []string{"mcp__git-push-memories__memory_search", "mcp__git-push-memories__memory_expand", "mcp__git-push-memories__memory_propose_create_page"}) {
		t.Fatalf("unexpected bootstrap interaction config: %+v", registered)
	}
	if len(registered.Pairs) != 3 {
		t.Fatalf("pair count = %d", len(registered.Pairs))
	}
	for index, pair := range registered.Pairs {
		hash := sha256.Sum256([]byte(registered.Seed + ":" + string(rune('1'+index))))
		order := []string{"control", "treatment"}
		if hash[0]%2 == 1 {
			order = []string{"treatment", "control"}
		}
		if pair.Pair != index+1 || pair.OrderHash != hex.EncodeToString(hash[:]) || !reflect.DeepEqual(pair.Order, order) {
			t.Fatalf("pair %d randomization does not match seed", index+1)
		}
	}
	files := map[string]string{
		"session_a":        "testdata/phase-2/bootstrap-value-experiment/session-a.md",
		"session_a_system": "testdata/phase-2/bootstrap-value-experiment/session-a-system.md",
		"memory_expected":  "testdata/phase-2/bootstrap-value-experiment/memory-expected/parcel-dispatch.md",
		"remember_skill":   "integrations/claude-code/skills/remember/SKILL.md",
		"task":             "testdata/phase-2/value-experiment/task.md", "task_system": "testdata/phase-2/value-experiment/task-system.md",
		"rubric": "testdata/phase-2/value-experiment/rubric.json", "code_go_mod": "testdata/phase-2/value-experiment/code-seed/go.mod",
		"policy_initial": "testdata/phase-2/value-experiment/code-seed/dispatch/policy.go", "policy_public_test": "testdata/phase-2/value-experiment/code-seed/dispatch/policy_test.go",
		"current_alerts": "testdata/phase-2/value-experiment/code-seed/deploy/production-alerts.yaml", "queue_ownership": "testdata/phase-2/value-experiment/code-seed/ops/queues.yaml",
		"legacy_policy": "testdata/phase-2/value-experiment/code-seed/docs/legacy-policy.md", "hidden_evaluator": "testdata/phase-2/value-experiment/evaluator/policy_hidden_test.go",
	}
	for name, relative := range files {
		if got := hashFile(t, filepath.Join(root, filepath.FromSlash(relative))); got != registered.FixtureHashes[name] {
			t.Fatalf("fixture %s hash = %s, registered %s", name, got, registered.FixtureHashes[name])
		}
	}
	expected := string(readFile(t, filepath.Join(fixtureRoot, "memory-expected", "parcel-dispatch.md")))
	if strings.Count(expected, "<!-- @id:") != 2 || !strings.Contains(expected, "90 seconds") || !strings.Contains(expected, "route-amber") || !strings.Contains(expected, "routing-ops") {
		t.Fatal("bootstrap expected memory is not the frozen canonical page")
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

func decodeFile(t *testing.T, path string, target any) {
	t.Helper()
	if err := json.Unmarshal(readFile(t, path), target); err != nil {
		t.Fatal(err)
	}
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	hash := sha256.Sum256(readFile(t, path))
	return hex.EncodeToString(hash[:])
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
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
			return os.MkdirAll(target, 0o755)
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
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, readFile(t, source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, directory, executable string, arguments ...string) {
	t.Helper()
	command := exec.Command(executable, arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s failed: %v: %s", executable, err, output)
	}
}

func anchorLines(contents []byte) []string {
	var anchors []string
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.Contains(line, "<!-- @id:") {
			anchors = append(anchors, line)
		}
	}
	sort.Strings(anchors)
	return anchors
}
