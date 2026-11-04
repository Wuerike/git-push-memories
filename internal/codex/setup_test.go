package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	codexintegration "github.com/Wuerike/git-push-memories/integrations/codex"
)

func isolatedCodex(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\000' "$@" >> "$GPM_CODEX_TEST_LOG"
if [ "$2" = "$GPM_CODEX_TEST_FAIL" ]; then
  echo "test failure" >&2
  exit 1
fi
`
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "codex.log")
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(root, "custom-codex"))
	t.Setenv("PATH", bin)
	t.Setenv("GPM_CODEX_TEST_LOG", log)
	t.Setenv("GPM_CODEX_TEST_FAIL", "")
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "GH_CONFIG_DIR"} {
		t.Setenv(name, "")
	}
	return filepath.Join(root, ".agents", "skills"), log
}

func TestInstallRegistersCodexAndRepairsManagedSkills(t *testing.T) {
	root, log := isolatedCodex(t)
	for round := range 2 {
		if err := Install(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, name := range skillFiles {
			path := filepath.Join(root, filepath.FromSlash(name))
			got, err := os.ReadFile(path)
			want, readErr := codexintegration.Skills.ReadFile("skills/" + name)
			if err != nil || readErr != nil || !bytes.Equal(got, want) {
				t.Fatalf("round %d, %s = %q, %v, %v", round, name, got, err, readErr)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("skill permissions: %v, %v", info, err)
			}
			if round == 0 {
				marker := "<!-- managed-by: git-push-memories -->"
				if strings.HasSuffix(name, ".yaml") {
					marker = "# managed-by: git-push-memories"
				}
				if err := os.WriteFile(path, []byte(marker+"\nold version\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	contents, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"mcp", "add", "git-push-memories", "--", executable, "mcp", "mcp", "get", "git-push-memories", ""}, "\x00")
	if string(contents) != want+want {
		t.Fatalf("Codex commands = %q, want %q twice", contents, want)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CODEX_HOME"), "skills")); !os.IsNotExist(err) {
		t.Fatalf("skills must use HOME/.agents/skills, not CODEX_HOME: %v", err)
	}
}

func TestInstallPreservesUnmanagedFilesBeforeAnyChanges(t *testing.T) {
	for _, collision := range skillFiles {
		t.Run(collision, func(t *testing.T) {
			root, log := isolatedCodex(t)
			before := make(map[string][]byte)
			for _, name := range skillFiles {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				data, err := codexintegration.Skills.ReadFile("skills/" + name)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte("\nuser edits to managed version\n")...)
				if name == collision {
					data = []byte("user-owned skill or policy\n")
				}
				before[name] = data
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := Install(context.Background()); err == nil || !strings.Contains(err.Error(), "not managed") {
				t.Fatalf("Install() = %v", err)
			}
			for name, want := range before {
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("conflict changed %s: %q, %v", name, got, err)
				}
			}
			if _, err := os.Stat(log); !os.IsNotExist(err) {
				t.Fatalf("conflict invoked Codex: %v", err)
			}
		})
	}
}

func TestInstallReportsRegistrationFailures(t *testing.T) {
	for _, operation := range []string{"add", "get"} {
		t.Run(operation, func(t *testing.T) {
			_, log := isolatedCodex(t)
			t.Setenv("GPM_CODEX_TEST_FAIL", operation)
			err := Install(context.Background())
			if err == nil || !strings.Contains(err.Error(), "test failure") {
				t.Fatalf("Install() = %v", err)
			}
			contents, readErr := os.ReadFile(log)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if operation == "add" && bytes.Contains(contents, []byte("get\x00")) {
				t.Fatal("checked registration after add failed")
			}
		})
	}
}

func TestInstallPreservesUnmanagedSymlinksBeforeAnyChanges(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		name := "existing target"
		if dangling {
			name = "missing target"
		}
		t.Run(name, func(t *testing.T) {
			root, log := isolatedCodex(t)
			if err := Install(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(log); err != nil {
				t.Fatal(err)
			}
			before := make(map[string][]byte)
			for _, file := range skillFiles {
				path := filepath.Join(root, filepath.FromSlash(file))
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte("\nprevious managed version\n")...)
				before[file] = data
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// A conflict in the final destination must preserve all earlier files.
			collision := filepath.Join(root, "promote", "agents", "openai.yaml")
			target := filepath.Join(t.TempDir(), "user-policy.yaml")
			const userPolicy = "policy:\n  allow_implicit_invocation: true\n"
			if !dangling {
				if err := os.WriteFile(target, []byte(userPolicy), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(collision); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, collision); err != nil {
				t.Fatal(err)
			}
			if err := Install(context.Background()); err == nil || !strings.Contains(err.Error(), "not managed") {
				t.Fatalf("Install() = %v", err)
			}
			if got, err := os.Readlink(collision); err != nil || got != target {
				t.Fatalf("installation replaced unmanaged symlink: %q, %v", got, err)
			}
			for file, want := range before {
				if file == "promote/agents/openai.yaml" {
					continue
				}
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("symlink conflict changed %s: %q, %v", file, got, err)
				}
			}
			if dangling {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("installation created symlink target: %v", err)
				}
			} else if got, err := os.ReadFile(target); err != nil || string(got) != userPolicy {
				t.Fatalf("installation changed symlink target: %q, %v", got, err)
			}
			if _, err := os.Stat(log); !os.IsNotExist(err) {
				t.Fatalf("symlink conflict changed MCP registration: %v", err)
			}
		})
	}
}

func TestInstallCarriesDirectoriesWithoutPersistingCredentials(t *testing.T) {
	_, log := isolatedCodex(t)
	directories := map[string]string{
		"XDG_CONFIG_HOME": filepath.Join(t.TempDir(), "config with spaces"),
		"XDG_STATE_HOME":  filepath.Join(t.TempDir(), "state with spaces"),
		"GH_CONFIG_DIR":   filepath.Join(t.TempDir(), "gh with spaces"),
	}
	for name, value := range directories {
		t.Setenv(name, value)
	}
	const secret = "test-only-token-not-for-config"
	t.Setenv("GH_TOKEN", secret)
	t.Setenv("GITHUB_TOKEN", secret)
	if err := Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range directories {
		want := "--env\x00" + name + "=" + value + "\x00"
		if !bytes.Contains(contents, []byte(want)) {
			t.Fatalf("registration did not preserve %s: %q", name, contents)
		}
	}
	if bytes.Contains(contents, []byte(secret)) {
		t.Fatal("registration persisted a token")
	}
}

func TestMissingCodexDoesNotInstallSkills(t *testing.T) {
	root, log := isolatedCodex(t)
	t.Setenv("PATH", t.TempDir())
	if err := Install(context.Background()); err == nil || !strings.Contains(err.Error(), "Codex is not installed") {
		t.Fatalf("Install() = %v", err)
	}
	for _, path := range []string{root, log} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("missing Codex created %s: %v", path, err)
		}
	}
}
