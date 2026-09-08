package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const schemaVersion = 1

type Config struct {
	MemoryRoot string `json:"memory_root"`
	StateRoot  string `json:"-"`
}

type fileConfig struct {
	SchemaVersion int    `json:"schema_version"`
	MemoryRoot    string `json:"memory_root"`
}

func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, errors.New("configuration path is unavailable")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, errors.New("git-push-memories is not configured; run setup first")
		}
		return Config{}, errors.New("configuration could not be read")
	}
	var stored fileConfig
	if err := json.Unmarshal(contents, &stored); err != nil || stored.SchemaVersion != schemaVersion {
		return Config{}, errors.New("configuration is invalid")
	}
	return Resolve(stored.MemoryRoot)
}

func Resolve(memoryRoot string) (Config, error) {
	if memoryRoot == "" {
		return Config{}, errors.New("memory root is required")
	}
	canonical, err := validateGitRoot("memory", memoryRoot)
	if err != nil {
		return Config{}, err
	}
	stateRoot, err := StatePath(canonical)
	if err != nil {
		return Config{}, errors.New("state root is unavailable")
	}
	if pathsOverlap(canonical, stateRoot) || insideGitWorktree(stateRoot) {
		return Config{}, errors.New("state root must be outside Git worktrees")
	}
	return Config{MemoryRoot: canonical, StateRoot: stateRoot}, nil
}

func Save(memoryRoot string) (Config, error) {
	cfg, err := Resolve(memoryRoot)
	if err != nil {
		return Config{}, err
	}
	path, err := Path()
	if err != nil {
		return Config{}, errors.New("configuration path is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Config{}, errors.New("configuration directory could not be created")
	}
	payload, err := json.MarshalIndent(fileConfig{SchemaVersion: schemaVersion, MemoryRoot: cfg.MemoryRoot}, "", "  ")
	if err != nil {
		return Config{}, errors.New("configuration could not be encoded")
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return Config{}, errors.New("configuration could not be written")
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return Config{}, errors.New("configuration could not be secured")
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return Config{}, errors.New("configuration could not be written")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return Config{}, errors.New("configuration could not be synchronized")
	}
	if err := temporary.Close(); err != nil {
		return Config{}, errors.New("configuration could not be written")
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return Config{}, errors.New("configuration could not be installed")
	}
	return cfg, nil
}

func Path() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "git-push-memories", "config.json"), nil
}

func StatePath(memoryRoot string) (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "state")
	}
	command := exec.Command("git", "--no-optional-locks", "-C", memoryRoot, "rev-list", "--max-parents=0", "HEAD")
	command.Env = withoutGitEnvironment(os.Environ())
	output, err := command.Output()
	if err != nil || len(strings.TrimSpace(string(output))) == 0 {
		return "", errors.New("memory repository identity is unavailable")
	}
	identity := fmt.Sprintf("%x", sha256.Sum256(bytes.TrimSpace(output)))[:16]
	absolute, err := filepath.Abs(filepath.Join(root, "git-push-memories", identity))
	if err != nil {
		return "", err
	}
	return absolute, nil
}

func validateGitRoot(name, root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%s root is invalid", name)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("%s root is invalid", name)
	}

	command := exec.Command("git", "--no-optional-locks", "-C", canonical, "rev-parse", "--show-toplevel")
	command.Env = withoutGitEnvironment(os.Environ())
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("%s root is not a Git worktree root with a commit", name)
	}
	topLevel, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil || topLevel != canonical {
		return "", fmt.Errorf("%s root is not a Git worktree root with a commit", name)
	}
	command = exec.Command("git", "--no-optional-locks", "-C", canonical, "rev-parse", "--verify", "HEAD")
	command.Env = withoutGitEnvironment(os.Environ())
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%s root is not a Git worktree root with a commit", name)
	}
	return canonical, nil
}

func withoutGitEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "GIT_") {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func pathsOverlap(left, right string) bool {
	return within(left, right) || within(right, left)
}

func within(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func insideGitWorktree(path string) bool {
	existing := path
	for {
		if _, err := os.Stat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return false
		}
		existing = parent
	}
	command := exec.Command("git", "--no-optional-locks", "-C", existing, "rev-parse", "--is-inside-work-tree")
	command.Env = withoutGitEnvironment(os.Environ())
	output, err := command.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}
