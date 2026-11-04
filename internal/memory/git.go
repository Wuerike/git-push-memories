package memory

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func (repository *Repository) gitOutput(ctx context.Context, args ...string) ([]byte, error) {
	commandArgs := append([]string{"--no-optional-locks", "--no-replace-objects", "--literal-pathspecs", "-C", "/proc/self/fd/3"}, args...)
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
