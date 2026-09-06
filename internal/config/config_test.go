package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRequiresValidDistinctGitRoots(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	codeRoot := newGitRoot(t, "code")

	tests := []struct {
		name       string
		memoryRoot string
		codeRoot   string
		wantError  string
	}{
		{name: "valid", memoryRoot: memoryRoot, codeRoot: codeRoot},
		{name: "memory missing", codeRoot: codeRoot, wantError: "memory root is required"},
		{name: "code missing", memoryRoot: memoryRoot, wantError: "code root is required"},
		{name: "same root", memoryRoot: memoryRoot, codeRoot: memoryRoot, wantError: "must be different"},
		{name: "memory is not git", memoryRoot: t.TempDir(), codeRoot: codeRoot, wantError: "memory root is not a Git worktree root"},
		{name: "code is below git root", memoryRoot: memoryRoot, codeRoot: filepath.Join(codeRoot, ".git"), wantError: "code root is not a Git worktree root"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Load(test.memoryRoot, test.codeRoot)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Load() error = %v, want containing %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.MemoryRoot != memoryRoot || got.CodeRoot != codeRoot {
				t.Fatalf("Load() = %+v, want roots %q and %q", got, memoryRoot, codeRoot)
			}
		})
	}
}

func TestLoadIgnoresRepositorySelectionEnvironment(t *testing.T) {
	memoryRoot := newGitRoot(t, "memory")
	codeRoot := newGitRoot(t, "code")
	otherRoot := newGitRoot(t, "other")
	t.Setenv("GIT_DIR", filepath.Join(otherRoot, ".git"))
	t.Setenv("GIT_WORK_TREE", otherRoot)

	got, err := Load(memoryRoot, codeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.MemoryRoot != memoryRoot || got.CodeRoot != codeRoot {
		t.Fatalf("Load() = %+v, want explicit roots", got)
	}
}

func newGitRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "init", "--quiet", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return root
}
