package claudecode

import (
	"context"
	"os"
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
	if err := Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(root, "claude-config", "skills", "remember", "SKILL.md"))
	if err != nil || !strings.Contains(string(contents), managedMarker) {
		t.Fatalf("installed skill = %q, %v", contents, err)
	}
	promoteContents, err := os.ReadFile(filepath.Join(root, "claude-config", "skills", "promote", "SKILL.md"))
	if err != nil || !strings.Contains(string(promoteContents), managedMarker) {
		t.Fatalf("installed promote skill = %q, %v", promoteContents, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "code-root") || !strings.Contains(string(log), "mcp add --transport stdio --scope user git-push-memories") || !strings.Contains(string(log), "mcp get git-push-memories") {
		t.Fatalf("Claude commands = %s", log)
	}
	if err := Install(context.Background()); err != nil {
		t.Fatalf("repeated Setup() failed: %v", err)
	}
}
