package experiment

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluateCandidateScoresOpaqueRunWithoutAssignments(t *testing.T) {
	root := repositoryRoot(t)
	fixture := filepath.Join(root, "testdata/phase-3/value-experiment")
	workspace := filepath.Join(t.TempDir(), "candidate")
	copyTree(t, filepath.Join(fixture, "code-seed"), workspace)
	seedCandidateGit(t, workspace)
	policyPath := filepath.Join(workspace, "policy", "policy.go")
	contents, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(contents), "return 1\n}\n\nfunc WebhookKeyAccepted", "if failures*2 > 10 {\n\t\treturn 10\n\t}\n\treturn failures * 2\n}\n\nfunc WebhookKeyAccepted", 1)
	if err := os.WriteFile(policyPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	expectedAPI, err := exportedAPISignatures(filepath.Join(fixture, "code-seed", "policy"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	evaluation := evaluateCandidate(ctx, candidateEvaluationSpec{
		RunID: "opaque-01", Scenario: "lease-renewal", HiddenTest: "TestHiddenLeaseRenewal",
		Workspace: workspace, EvaluatorPath: filepath.Join(fixture, "evaluator", "policy_hidden_test.go"), ExpectedAPI: expectedAPI,
	})
	if evaluation.RunID != "opaque-01" || !evaluation.EvaluatorValid || !evaluation.PublicTests || !evaluation.HiddenTest || !evaluation.APIPreserved || !evaluation.ScopeValid || !evaluation.Correctness || evaluation.Score != 10 {
		t.Fatalf("evaluation = %+v", evaluation)
	}
	if len(evaluation.ChangedPaths) != 1 || evaluation.ChangedPaths[0] != "policy/policy.go" || evaluation.DiffHash == "" {
		t.Fatalf("diff evidence = %+v", evaluation)
	}
	if _, err := os.Stat(filepath.Join(workspace, "policy", "policy_hidden_test.go")); !os.IsNotExist(err) {
		t.Fatal("hidden evaluator remained in the candidate")
	}
}

func TestEvaluateCandidateRejectsAPIDriftAndOutOfScopeChanges(t *testing.T) {
	root := repositoryRoot(t)
	fixture := filepath.Join(root, "testdata/phase-3/value-experiment")
	workspace := filepath.Join(t.TempDir(), "candidate")
	copyTree(t, filepath.Join(fixture, "code-seed"), workspace)
	seedCandidateGit(t, workspace)
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("not allowed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(workspace, "policy", "policy.go")
	contents, _ := os.ReadFile(policyPath)
	if err := os.WriteFile(policyPath, []byte(strings.Replace(string(contents), "LeaseRenewalDelay(failures int)", "LeaseRenewalDelay(failures int64)", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	expectedAPI, err := exportedAPISignatures(filepath.Join(fixture, "code-seed", "policy"))
	if err != nil {
		t.Fatal(err)
	}
	evaluation := evaluateCandidate(context.Background(), candidateEvaluationSpec{
		RunID: "opaque-02", Scenario: "lease-renewal", HiddenTest: "TestHiddenLeaseRenewal",
		Workspace: workspace, EvaluatorPath: filepath.Join(fixture, "evaluator", "policy_hidden_test.go"), ExpectedAPI: expectedAPI,
	})
	if evaluation.APIPreserved || evaluation.ScopeValid || evaluation.Correctness || evaluation.Score >= 10 {
		t.Fatalf("evaluation = %+v", evaluation)
	}
}

func seedCandidateGit(t *testing.T, root string) {
	t.Helper()
	environment := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "TZ=UTC", "LC_ALL=C.UTF-8"}
	runCommand(t, root, environment, "git", "init", "--quiet", "--initial-branch=main")
	runCommand(t, root, environment, "git", "add", ".")
	commitEnvironment := append(append([]string(nil), environment...), "GIT_AUTHOR_NAME=Evaluator", "GIT_AUTHOR_EMAIL=evaluator@example.invalid", "GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_NAME=Evaluator", "GIT_COMMITTER_EMAIL=evaluator@example.invalid", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z")
	runCommand(t, root, commitEnvironment, "git", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "seed")
}
