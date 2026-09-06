package memory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (repository *Repository) ensureTargetClean(ctx context.Context, relativePath, expectedHead string) error {
	for _, args := range [][]string{
		{"diff", "--quiet", "--no-ext-diff", "--cached", expectedHead, "--", relativePath},
		{"diff", "--quiet", "--no-ext-diff", "--", relativePath},
	} {
		commandArgs := append([]string{"--no-optional-locks", "-C", "/proc/self/fd/3"}, args...)
		command := exec.CommandContext(ctx, "git", commandArgs...)
		command.Env = withoutGitEnvironment(os.Environ())
		command.ExtraFiles = []*os.File{repository.root}
		if err := command.Run(); err != nil {
			var exitError *exec.ExitError
			if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
				return ErrTargetDirty
			}
			return fmt.Errorf("inspect Git target: %w", ErrInvalidSchema)
		}
	}
	head, err := repository.gitOutput(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != expectedHead {
		return ErrTargetDirty
	}
	return nil
}

func (repository *Repository) confirmPreviewState(ctx context.Context, expected Snapshot) error {
	canonicalPath, source, _, err := repository.read(expected.Path)
	if err != nil || canonicalPath != expected.Path || hash(source) != expected.FileHash {
		return ErrTargetDirty
	}
	if err := repository.ensureTargetClean(ctx, expected.Path, expected.Head); err != nil {
		return err
	}
	head, blob, err := repository.gitIdentity(ctx, expected.Path)
	if err != nil || head != expected.Head || blob != expected.Blob {
		return ErrTargetDirty
	}
	_, finalSource, _, err := repository.read(expected.Path)
	if err != nil || hash(finalSource) != expected.FileHash {
		return ErrTargetDirty
	}
	return nil
}

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
