package memory

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (repository *Repository) gitIdentity(ctx context.Context, relativePath string) (string, string, error) {
	head, err := repository.gitOutput(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("read Git HEAD: %w", ErrInvalidSchema)
	}
	headID := strings.TrimSpace(string(head))
	treeEntry, err := repository.gitOutput(ctx, "ls-tree", "-z", headID, "--", relativePath)
	if err != nil || len(treeEntry) == 0 {
		return "", "", fmt.Errorf("read Git blob: %w", ErrInvalidSchema)
	}
	tab := bytes.IndexByte(treeEntry, '\t')
	if tab < 0 {
		return "", "", fmt.Errorf("read Git blob: %w", ErrInvalidSchema)
	}
	fields := strings.Fields(string(treeEntry[:tab]))
	if len(fields) != 3 || fields[1] != "blob" {
		return "", "", fmt.Errorf("read Git blob: %w", ErrInvalidSchema)
	}
	return headID, fields[2], nil
}

func (repository *Repository) gitOutput(ctx context.Context, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--no-optional-locks", "-C", "/proc/self/fd/3"}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	command.Env = withoutGitEnvironment(os.Environ())
	command.ExtraFiles = []*os.File{repository.root}
	return command.Output()
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
