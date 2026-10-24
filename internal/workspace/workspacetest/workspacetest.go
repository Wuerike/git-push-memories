// Package workspacetest builds offline fixtures: a bare "remote" repository,
// an isolated HOME and state root, and a fake gh executable on PATH.
package workspacetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Env isolates HOME, XDG_STATE_HOME and the Git configuration for one test.
func Env(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\n\tname = Test\n\temail = test@example.com\n[init]\n\tdefaultBranch = main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// Git runs git in dir and fails the test on error.
func Git(t *testing.T, dir string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// Remote creates an empty bare repository and returns its path, usable as a Git URL.
func Remote(t *testing.T) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	Git(t, filepath.Dir(remote), "init", "--bare", "-b", "main", remote)
	return remote
}

// Seed commits files onto branch of remote through a throwaway clone.
func Seed(t *testing.T, remote, branch string, files map[string]string) {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "seed")
	Git(t, filepath.Dir(clone), "clone", "-q", remote, clone)
	Git(t, clone, "checkout", "-q", "-B", branch)
	for name, contents := range files {
		path := filepath.Join(clone, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Git(t, clone, "add", "-A")
	Git(t, clone, "commit", "-q", "-m", "seed")
	Git(t, clone, "push", "-q", "origin", branch)
}

// GH is a fake gh: auth always succeeds, and the last PR of any head is whatever the test sets.
type GH struct{ dir string }

const fakeGH = `#!/usr/bin/env bash
dir="$(dirname "$0")"
case "$1 $2" in
  "auth status") exit 0 ;;
  "pr create")
    n=$(( $(cat "$dir/count" 2>/dev/null || echo 0) + 1 )); echo $n > "$dir/count"
    echo OPEN > "$dir/state"; echo "$n" > "$dir/number"; : > "$dir/oid"
    echo "https://github.com/o/r/pull/$n" ;;
  "pr list"|"pr view")
    [ -f "$dir/state" ] || { [ "$2" = list ] && echo '[]' && exit 0; exit 1; }
    pr="{\"number\":$(cat "$dir/number"),\"state\":\"$(cat "$dir/state")\",\"url\":\"https://github.com/o/r/pull/$(cat "$dir/number")\",\"createdAt\":\"2026-01-01T00:00:00Z\",\"mergeCommit\":{\"oid\":\"$(cat "$dir/oid")\"}}"
    if [ "$2" = list ]; then echo "[$pr]"; else echo "$pr"; fi ;;
  *) echo "fake gh: unsupported: $*" >&2; exit 1 ;;
esac
`

// FakeGH installs the fake gh first on PATH.
func FakeGH(t *testing.T) *GH {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(fakeGH), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &GH{dir: dir}
}

// SetPR sets the state (OPEN, MERGED, CLOSED) and merge commit of the current PR.
func (gh *GH) SetPR(t *testing.T, state, mergeOID string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(gh.dir, "number")); err != nil {
		os.WriteFile(filepath.Join(gh.dir, "number"), []byte("1"), 0o644)
	}
	os.WriteFile(filepath.Join(gh.dir, "state"), []byte(state), 0o644)
	os.WriteFile(filepath.Join(gh.dir, "oid"), []byte(mergeOID), 0o644)
}
