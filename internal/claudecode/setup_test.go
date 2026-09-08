package claudecode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupConfiguresGlobalMCPAndSkill(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "claude.log")
	claude := filepath.Join(bin, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CLAUDE_TEST_LOG\"\n"
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLAUDE_TEST_LOG", logPath)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude-config"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	memoryRoot := newMemoryRoot(t)

	result, err := Setup(context.Background(), memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(result.SkillPath)
	if err != nil || !strings.Contains(string(contents), managedMarker) {
		t.Fatalf("installed skill = %q, %v", contents, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "code-root") || !strings.Contains(string(log), "mcp add --transport stdio --scope user git-push-memories") || !strings.Contains(string(log), "mcp get git-push-memories") {
		t.Fatalf("Claude commands = %s", log)
	}
	if _, err := Setup(context.Background(), memoryRoot); err != nil {
		t.Fatalf("repeated Setup() failed: %v", err)
	}
}

func newMemoryRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "commit", "--quiet", "--allow-empty", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
