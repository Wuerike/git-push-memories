package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSaveAndLoadGlobalConfiguration(t *testing.T) {
	configHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	memoryRoot := newGitRoot(t, "memory")

	saved, err := Save(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	wantState, err := StatePath(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MemoryRoot != saved.MemoryRoot || loaded.StateRoot != saved.StateRoot || !slices.Equal(loaded.MemoryCategories, saved.MemoryCategories) || loaded.MemoryRoot != memoryRoot || loaded.StateRoot != wantState {
		t.Fatalf("Load() = %+v, want memory %q and state %q", loaded, memoryRoot, wantState)
	}
	if strings.Join(loaded.MemoryCategories, ",") != "domains,decision" {
		t.Fatalf("Load() categories = %v, want domains and decision", loaded.MemoryCategories)
	}
	info, err := os.Stat(filepath.Join(configHome, "git-push-memories", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadRequiresSetup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "run setup first") {
		t.Fatalf("Load() error = %v, want setup instruction", err)
	}
}

func TestResolveRequiresCommittedGitRoot(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := Resolve(""); err == nil || !strings.Contains(err.Error(), "memory root is required") {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, err := Resolve(t.TempDir()); err == nil || !strings.Contains(err.Error(), "with a commit") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestResolveRejectsInvalidRepositoryConfiguration(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	tests := []struct {
		name     string
		contents string
	}{
		{name: "missing", contents: ""},
		{name: "malformed", contents: "{"},
		{name: "unknown field", contents: `{"schema_version":1,"memory_categories":["domains"],"extra":true}`},
		{name: "wrong schema", contents: `{"schema_version":2,"memory_categories":["domains"]}`},
		{name: "empty categories", contents: `{"schema_version":1,"memory_categories":[]}`},
		{name: "empty category", contents: `{"schema_version":1,"memory_categories":[""]}`},
		{name: "non kebab category", contents: `{"schema_version":1,"memory_categories":["Domain Rules"]}`},
		{name: "duplicate category", contents: `{"schema_version":1,"memory_categories":["domains","domains"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newGitRoot(t, test.name)
			configPath := filepath.Join(root, "gpm.json")
			if test.contents == "" {
				if err := os.Remove(configPath); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(configPath, []byte(test.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Resolve(root); err == nil || !strings.Contains(err.Error(), "repository configuration is invalid") {
				t.Fatalf("Resolve() error = %v, want invalid repository configuration", err)
			}
		})
	}
}

func TestConfigRecognizesOnlyConfiguredMemoryPaths(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg, err := Resolve(newGitRoot(t, "memory-paths"))
	if err != nil {
		t.Fatal(err)
	}
	for _, valid := range []string{"memories/domains/charge.md", "memories/decision/platform/retry.md"} {
		if !cfg.IsMemoryPath(valid) {
			t.Fatalf("IsMemoryPath(%q) = false", valid)
		}
	}
	for _, invalid := range []string{"memories/charge.md", "memories/notes/charge.md", "memories/domains/charge.txt", "memories/domains/../decision/charge.md", "../memories/domains/charge.md"} {
		if cfg.IsMemoryPath(invalid) {
			t.Fatalf("IsMemoryPath(%q) = true", invalid)
		}
	}
}

func TestResolveRejectsStateInsideGitWorktree(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	otherRoot := newGitRoot(t, "other")
	t.Setenv("XDG_STATE_HOME", filepath.Join(otherRoot, "private-state"))
	if _, err := Resolve(memoryRoot); err == nil || !strings.Contains(err.Error(), "outside Git worktrees") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestResolveIsolatesStateByMemoryRepository(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	first, err := Resolve(newGitRoot(t, "first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Resolve(newGitRoot(t, "second"))
	if err != nil {
		t.Fatal(err)
	}
	if first.StateRoot == second.StateRoot {
		t.Fatalf("memory repositories share state root %q", first.StateRoot)
	}
}

func TestResolveIgnoresRepositorySelectionEnvironment(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	memoryRoot := newGitRoot(t, "memory")
	otherRoot := newGitRoot(t, "other")
	t.Setenv("GIT_DIR", filepath.Join(otherRoot, ".git"))
	t.Setenv("GIT_WORK_TREE", otherRoot)

	got, err := Resolve(memoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.MemoryRoot != memoryRoot {
		t.Fatalf("Resolve() = %+v, want explicit memory root", got)
	}
}

func newGitRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "gpm.json"), []byte("{\n  \"schema_version\": 1,\n  \"memory_categories\": [\"domains\", \"decision\"]\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "gpm.json")
	runGit(t, root, "commit", "--quiet", "-m", "initial "+name)
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
