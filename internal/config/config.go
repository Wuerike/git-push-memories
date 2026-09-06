package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	MemoryRoot string
	CodeRoot   string
}

func Load(memoryRoot, codeRoot string) (Config, error) {
	if memoryRoot == "" {
		return Config{}, errors.New("memory root is required")
	}
	if codeRoot == "" {
		return Config{}, errors.New("code root is required")
	}

	memoryRoot, err := validateGitRoot("memory", memoryRoot)
	if err != nil {
		return Config{}, err
	}
	codeRoot, err = validateGitRoot("code", codeRoot)
	if err != nil {
		return Config{}, err
	}
	if memoryRoot == codeRoot {
		return Config{}, errors.New("memory root and code root must be different")
	}

	return Config{MemoryRoot: memoryRoot, CodeRoot: codeRoot}, nil
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
		return "", fmt.Errorf("%s root is not a Git worktree root", name)
	}
	topLevel, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil || topLevel != canonical {
		return "", fmt.Errorf("%s root is not a Git worktree root", name)
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
