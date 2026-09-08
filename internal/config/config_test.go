package config

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if loaded != saved || loaded.MemoryRoot != memoryRoot || loaded.StateRoot != wantState {
		t.Fatalf("Load() = %+v, want memory %q and state %q", loaded, memoryRoot, wantState)
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
	runGit(t, root, "commit", "--quiet", "--allow-empty", "-m", "initial "+name)
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
