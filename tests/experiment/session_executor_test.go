package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecutorRunsInteractiveTurnInsideIsolatedNamespace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "source")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seedCandidateGit(t, workspace)
	claude := filepath.Join(root, "claude")
	writeTestExecutable(t, claude, `#!/bin/sh
test ! -e /bridge/server.sh || exit 3
IFS= read -r first
printf '%s\n' '{"type":"assistant","message":{"model":"model-test","content":[{"type":"text","text":"Review exact fields"}]}}'
printf '%s\n' '{"type":"result","is_error":false,"total_cost_usd":0.1,"num_turns":1}'
IFS= read -r confirmation
printf '%s\n' '{"type":"assistant","message":{"model":"model-test","content":[{"type":"tool_use","id":"propose-1","name":"mcp__git-push-memories__memory_propose","input":{"title":"Synthetic","suggested_action":"create","knowledge":"fact","source_paths":[]}}]}}'
printf '%s\n' '{"type":"result","is_error":false,"total_cost_usd":0.2,"num_turns":2}'
`)
	product := filepath.Join(root, "gpm")
	writeTestExecutable(t, product, "#!/bin/sh\nexit 0\n")
	credential := filepath.Join(root, "credential.json")
	if err := os.WriteFile(credential, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	memory := filepath.Join(root, "memory")
	state := filepath.Join(root, "state")
	auth := filepath.Join(root, "auth")
	for _, directory := range []string{memory, state, auth} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	reviewed := 0
	observation, _, err := (executor{root: root, authRoot: auth, productBinary: product, claudeBinary: claude, credential: credential, rememberSkill: filepath.Join(repositoryRoot(t), "integrations", "claude-code", "skills", "remember", "SKILL.md")}).run(
		context.Background(),
		sessionRequest{UnitID: "capture-test", Kind: "capture", Workspace: workspace, MemoryRoot: memory, StateRoot: state, Prompt: "/remember fact", ExpectedModel: "model-test", VisiblePaths: []string{"source.txt"}, MaxBudgetUSD: 0.3, Timeout: 20 * time.Second},
		func(request reviewRequest) (string, error) {
			reviewed++
			if request.Text != "Review exact fields" {
				t.Fatalf("review text = %q", request.Text)
			}
			return "Confirm those exact fields.", nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed != 1 || observation.ToolCounts["mcp__git-push-memories__memory_propose"] != 1 || observation.ProposalArguments["knowledge"] != "fact" || observation.CostMicroUSD != 200000 {
		t.Fatalf("observation = %+v, reviewed = %d", observation, reviewed)
	}
}

func writeTestExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}
