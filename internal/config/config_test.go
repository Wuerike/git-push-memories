package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/workspace"
)

const url = "git@github.com:o/r.git"

func prepare(t *testing.T, contents string) workspace.Paths {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	paths, err := workspace.PathsFor(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Read, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Read, ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Read, "gpm.json"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return paths
}

const validRepo = "{\n  \"schema_version\": 1,\n  \"memory_categories\": [\"domains\", \"decision\"]\n}\n"

func TestSaveAndLoadGlobalConfiguration(t *testing.T) {
	paths := prepare(t, validRepo)
	if err := Save(url); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RepoURL != url || loaded.Paths != paths || loaded.MemoryRoot != paths.Read || loaded.StateRoot != paths.State || !slices.Equal(loaded.MemoryCategories, []string{"domains", "decision"}) {
		t.Fatalf("Load() = %+v", loaded)
	}
	configPath, _ := Path()
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config file = %v, %v", info, err)
	}
}

func TestLoadRequiresSetup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "run setup first") {
		t.Fatalf("Load() error = %v, want setup instruction", err)
	}
}

func TestResolveRequiresPreparedRepository(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := Resolve(""); err == nil || !strings.Contains(err.Error(), "URL is required") {
		t.Fatalf("Resolve(\"\") error = %v", err)
	}
	if _, err := Resolve(url); err == nil || !strings.Contains(err.Error(), "run setup") {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestResolveIsolatesStateByURL(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	first, _ := workspace.PathsFor("git@github.com:o/a.git")
	second, _ := workspace.PathsFor("git@github.com:o/b.git")
	if first.State == second.State {
		t.Fatalf("repositories share state %q", first.State)
	}
}

func TestParseRepositoryConfigRejectsInvalid(t *testing.T) {
	for name, contents := range map[string]string{
		"malformed":          "{",
		"unknown field":      `{"schema_version":1,"memory_categories":["domains"],"extra":true}`,
		"wrong schema":       `{"schema_version":2,"memory_categories":["domains"]}`,
		"empty categories":   `{"schema_version":1,"memory_categories":[]}`,
		"empty category":     `{"schema_version":1,"memory_categories":[""]}`,
		"non kebab category": `{"schema_version":1,"memory_categories":["Domain Rules"]}`,
		"duplicate category": `{"schema_version":1,"memory_categories":["domains","domains"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRepositoryConfig([]byte(contents)); err == nil || !strings.Contains(err.Error(), "repository configuration is invalid") {
				t.Fatalf("ParseRepositoryConfig() error = %v", err)
			}
		})
	}
}

func TestRepositoryConfigJSONRoundTrips(t *testing.T) {
	payload, err := RepositoryConfigJSON([]string{"domains", "decision"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseRepositoryConfig(payload)
	if err != nil || !slices.Equal(got, []string{"domains", "decision"}) {
		t.Fatalf("round trip = %v, %v", got, err)
	}
	if _, err := RepositoryConfigJSON([]string{"Bad Name"}); err == nil {
		t.Fatal("invalid category accepted")
	}
}

func TestConfigRecognizesOnlyConfiguredMemoryPaths(t *testing.T) {
	cfg := Config{MemoryCategories: []string{"domains", "decision"}}
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
