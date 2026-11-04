// Package codex installs the user-scoped Codex integration.
package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	codexintegration "github.com/Wuerike/git-push-memories/integrations/codex"
)

var skillFiles = []string{
	"remember/SKILL.md",
	"remember/agents/openai.yaml",
	"promote/SKILL.md",
	"promote/agents/openai.yaml",
}

// Preflight checks that Codex can be invoked before preparing the repository.
func Preflight() error {
	if _, err := exec.LookPath("codex"); err != nil {
		return errors.New("Codex is not installed or is not available in PATH")
	}
	return nil
}

// Install registers the MCP server and installs explicit-only skills.
func Install(ctx context.Context) error {
	if err := Preflight(); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return errors.New("executable path could not be resolved")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return errors.New("executable path could not be resolved")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.New("Codex skill path could not be resolved")
	}
	root := filepath.Join(home, ".agents", "skills")
	contents := make(map[string][]byte, len(skillFiles))
	// Check every destination before changing any integration file or registration.
	for _, name := range skillFiles {
		data, err := codexintegration.Skills.ReadFile("skills/" + name)
		if err != nil {
			return fmt.Errorf("bundled Codex skill %s could not be read: %w", name, err)
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := ensureManaged(path, data); err != nil {
			return err
		}
		contents[name] = data
	}
	for _, name := range skillFiles {
		if err := installFile(filepath.Join(root, filepath.FromSlash(name)), contents[name]); err != nil {
			return err
		}
	}

	// Codex updates this named server in place and preserves unrelated settings.
	arguments := []string{"mcp", "add", "git-push-memories"}
	// Codex does not forward arbitrary environment variables to stdio servers.
	// Preserve the non-secret directories used by this setup and by gh.
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "GH_CONFIG_DIR"} {
		if value := os.Getenv(name); value != "" {
			arguments = append(arguments, "--env", name+"="+value)
		}
	}
	arguments = append(arguments, "--", executable, "mcp")
	add := exec.CommandContext(ctx, "codex", arguments...)
	if output, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("Codex MCP registration failed: %w: %s", err, bytes.TrimSpace(output))
	}
	check := exec.CommandContext(ctx, "codex", "mcp", "get", "git-push-memories")
	if output, err := check.CombinedOutput(); err != nil {
		return fmt.Errorf("Codex MCP registration check failed: %w: %s", err, bytes.TrimSpace(output))
	}
	return nil
}

func ensureManaged(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("existing Codex skill file %s could not be inspected: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Codex skill file %s is a symbolic link and is not managed by git-push-memories", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("existing Codex skill file %s could not be read: %w", path, err)
	}
	marker := "<!-- managed-by: git-push-memories -->"
	if filepath.Ext(path) == ".yaml" {
		marker = "# managed-by: git-push-memories"
	}
	if bytes.Equal(contents, expected) || bytes.Contains(contents, []byte(marker)) {
		return nil
	}
	return fmt.Errorf("Codex skill file %s already exists and is not managed by git-push-memories", path)
}

func installFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("Codex skill directory could not be created: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".skill-*.tmp")
	if err != nil {
		return fmt.Errorf("Codex skill file could not be written: %w", err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("Codex skill file could not be written: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("Codex skill file could not be written: %w", err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("Codex skill file could not be installed: %w", err)
	}
	return nil
}
