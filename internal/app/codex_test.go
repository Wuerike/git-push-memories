package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/config"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

// fakeCodex provides only Git and Codex so setup cannot depend on Claude Code.
func fakeCodex(t *testing.T) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "codex"))
}

func TestSetupCodexWithoutClaudeCode(t *testing.T) {
	wt.Env(t)
	fakeCodex(t)
	remote := wt.Remote(t)
	code, stdout, stderr := run("setup", remote, "--agent", "codex", "--categories", "domains,decision", "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("setup exit = %d, stderr = %q", code, stderr)
	}
	cfg, err := config.Load()
	if err != nil || cfg.RepoURL != remote || !slices.Equal(cfg.MemoryCategories, []string{"domains", "decision"}) {
		t.Fatalf("configuration = %+v, %v", cfg, err)
	}
	if !strings.Contains(stdout, "Use $remember <fact> and $promote in any Codex session") || !strings.Contains(stdout, cfg.StateRoot) {
		t.Fatalf("Codex setup output = %q", stdout)
	}
	for _, name := range []string{"remember", "promote"} {
		path := filepath.Join(os.Getenv("HOME"), ".agents", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("installed %s: %v", name, err)
		}
		policy, err := os.ReadFile(filepath.Join(filepath.Dir(path), "agents", "openai.yaml"))
		if err != nil || !strings.Contains(string(policy), "allow_implicit_invocation: false") {
			t.Fatalf("explicit invocation policy = %q, %v", policy, err)
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude")); !os.IsNotExist(err) {
		t.Fatalf("Codex setup touched Claude Code: %v", err)
	}
	before := wt.Git(t, remote, "show-ref")
	if code, _, stderr := run("setup", remote, "--agent=codex", "--yes"); code != 0 {
		t.Fatalf("repeated Codex setup exit = %d, stderr = %q", code, stderr)
	}
	if after := wt.Git(t, remote, "show-ref"); after != before {
		t.Fatalf("repeated setup changed remote refs: %s -> %s", before, after)
	}
}

func TestSetupAgentsShareMemoryRepository(t *testing.T) {
	for _, first := range []string{"codex", "claude-code"} {
		t.Run(first+" first", func(t *testing.T) {
			wt.Env(t)
			fakeCodex(t)
			fakeClaude(t)
			remote := wt.Remote(t)
			wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{\"schema_version\":1,\"memory_categories\":[\"domains\"]}\n"})
			second := "codex"
			if first == "codex" {
				second = "claude-code"
			}
			for _, agent := range []string{first, second} {
				if code, _, stderr := run("setup", remote, "--agent", agent, "--yes"); code != 0 {
					t.Fatalf("%s setup exit = %d, stderr = %q", agent, code, stderr)
				}
			}
			if url, err := config.StoredURL(); err != nil || url != remote {
				t.Fatalf("shared memory URL = %q, %v", url, err)
			}
			for _, root := range []string{filepath.Join(os.Getenv("HOME"), ".agents", "skills"), filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "skills")} {
				for _, name := range []string{"remember", "promote"} {
					if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
						t.Fatalf("missing %s in %s: %v", name, root, err)
					}
				}
			}
		})
	}
}

func TestSetupRejectsUnknownAgentBeforePreparingRepository(t *testing.T) {
	wt.Env(t)
	t.Setenv("PATH", t.TempDir())
	code, _, stderr := run("setup", "git@github.com:o/r.git", "--agent", "unknown", "--yes")
	if code != 2 || !strings.Contains(stderr, "unknown agent") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if url, err := config.StoredURL(); err != nil || url != "" {
		t.Fatalf("invalid agent saved configuration: %q, %v", url, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "git-push-memories")); !os.IsNotExist(err) {
		t.Fatalf("invalid agent prepared repository: %v", err)
	}
}

func TestSetupRequiresSelectedCodexCLI(t *testing.T) {
	wt.Env(t)
	t.Setenv("PATH", t.TempDir())
	fakeClaude(t)
	code, _, stderr := run("setup", "git@github.com:o/r.git", "--agent", "codex", "--yes")
	if code != 1 || !strings.Contains(stderr, "Codex is not installed") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if url, err := config.StoredURL(); err != nil || url != "" {
		t.Fatalf("missing Codex saved configuration: %q, %v", url, err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "git-push-memories")); !os.IsNotExist(err) {
		t.Fatalf("missing Codex prepared repository: %v", err)
	}
}

func TestCodexRegistrationFailureDoesNotSaveConfiguration(t *testing.T) {
	wt.Env(t)
	fakeCodex(t)
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codex, []byte("#!/bin/sh\necho 'registration denied' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	remote := wt.Remote(t)
	wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{\"schema_version\":1,\"memory_categories\":[\"domains\"]}\n"})
	code, stdout, stderr := run("setup", remote, "--agent", "codex", "--yes")
	if code != 1 || !strings.Contains(stderr, "registration denied") || strings.Contains(stdout, "Configured memory") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if url, err := config.StoredURL(); err != nil || url != "" {
		t.Fatalf("failed registration saved configuration: %q, %v", url, err)
	}
}
