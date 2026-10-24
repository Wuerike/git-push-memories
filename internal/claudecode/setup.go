package claudecode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	claudeintegration "github.com/Wuerike/git-push-memories/integrations/claude-code"
)

const managedMarker = "<!-- managed-by: git-push-memories -->"

type Result struct {
	SkillPath        string
	PromoteSkillPath string
}

// Preflight fails before any repository work when Claude Code is missing.
func Preflight() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return errors.New("Claude Code is not installed or is not available in PATH")
	}
	return nil
}

// Install registers the MCP server and installs the skills.
func Install(ctx context.Context) (Result, error) {
	if err := Preflight(); err != nil {
		return Result{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Result{}, errors.New("executable path could not be resolved")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return Result{}, errors.New("executable path could not be resolved")
	}
	rememberPath, err := rememberSkillPath()
	if err != nil {
		return Result{}, errors.New("Claude Code skill path could not be resolved")
	}
	promoteSkillPath, err := skillPath("promote")
	if err != nil {
		return Result{}, errors.New("Claude Code skill path could not be resolved")
	}
	if err := ensureSkillCanBeManaged(rememberPath, claudeintegration.RememberSkill, "/remember"); err != nil {
		return Result{}, err
	}
	if err := ensureSkillCanBeManaged(promoteSkillPath, claudeintegration.PromoteSkill, "/promote"); err != nil {
		return Result{}, err
	}
	if err := installSkill(rememberPath, claudeintegration.RememberSkill); err != nil {
		return Result{}, err
	}
	if err := installSkill(promoteSkillPath, claudeintegration.PromoteSkill); err != nil {
		return Result{}, err
	}

	remove := exec.CommandContext(ctx, "claude", "mcp", "remove", "--scope", "user", "git-push-memories")
	_ = remove.Run()
	add := exec.CommandContext(ctx, "claude", "mcp", "add", "--transport", "stdio", "--scope", "user", "git-push-memories", "--", executable, "mcp")
	if output, err := add.CombinedOutput(); err != nil {
		return Result{}, fmt.Errorf("Claude Code MCP registration failed: %s", bytes.TrimSpace(output))
	}
	check := exec.CommandContext(ctx, "claude", "mcp", "get", "git-push-memories")
	if output, err := check.CombinedOutput(); err != nil {
		return Result{}, fmt.Errorf("Claude Code MCP health check failed: %s", bytes.TrimSpace(output))
	}
	return Result{SkillPath: rememberPath, PromoteSkillPath: promoteSkillPath}, nil
}

func rememberSkillPath() (string, error) {
	return skillPath("remember")
}

func skillPath(name string) (string, error) {
	root := os.Getenv("CLAUDE_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".claude")
	}
	return filepath.Join(root, "skills", name, "SKILL.md"), nil
}

func ensureSkillCanBeManaged(path string, expected []byte, name string) error {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("existing Claude Code skill could not be read")
	}
	if bytes.Equal(contents, expected) || bytes.Contains(contents, []byte(managedMarker)) {
		return nil
	}
	return fmt.Errorf("Claude Code %s skill already exists and is not managed by git-push-memories", name)
}

func installSkill(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("Claude Code skill directory could not be created")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".skill-*.tmp")
	if err != nil {
		return errors.New("Claude Code skill could not be written")
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return errors.New("Claude Code skill could not be secured")
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return errors.New("Claude Code skill could not be written")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("Claude Code skill could not be written")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return errors.New("Claude Code skill could not be installed")
	}
	return nil
}
