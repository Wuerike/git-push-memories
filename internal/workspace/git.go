package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrNetwork    = errors.New("network operation failed")
	ErrPermission = errors.New("remote refused the operation (permission denied)")
	ErrRejected   = errors.New("remote branch moved (push rejected)")
	ErrDirty      = errors.New("worktree has uncommitted changes")
	ErrConflict   = errors.New("merge conflict")
)

// NetworkTimeout bounds every call that talks to the remote.
var NetworkTimeout = 60 * time.Second

// Git runs Git in Dir with the GPM environment: no ambient repository
// selection, no prompts, no hooks, no replace refs.
type Git struct {
	Dir string
}

// ponytail: denylist of the GIT_* variables that retarget or reconfigure Git; add more if one leaks.
var deniedEnvironment = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_NAMESPACE": true,
	"GIT_OBJECT_DIRECTORY": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true,
	"GIT_PREFIX": true, "GIT_CEILING_DIRECTORIES": true, "GIT_REPLACE_REF_BASE": true,
	"GIT_CONFIG":       true,
	"GIT_CONFIG_COUNT": true, "GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_NOSYSTEM": true,
	"GIT_AUTHOR_NAME": true, "GIT_AUTHOR_EMAIL": true, "GIT_AUTHOR_DATE": true,
	"GIT_COMMITTER_NAME": true, "GIT_COMMITTER_EMAIL": true, "GIT_COMMITTER_DATE": true,
	"GIT_EDITOR": true, "GIT_SEQUENCE_EDITOR": true, "GIT_PAGER": true,
}

func environment() []string {
	var out []string
	sshCommand := "ssh"
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		switch {
		case name == "GIT_SSH_COMMAND":
			sshCommand = value
		case strings.HasPrefix(name, "GIT_CONFIG_KEY_"), strings.HasPrefix(name, "GIT_CONFIG_VALUE_"), deniedEnvironment[name]:
		case name == "GIT_TERMINAL_PROMPT":
		default:
			out = append(out, entry)
		}
	}
	return append(out, "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND="+sshCommand+" -o BatchMode=yes")
}

var baseArguments = []string{"--no-optional-locks", "--no-replace-objects", "-c", "core.hooksPath=/dev/null"}

func (git Git) run(ctx context.Context, input []byte, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append(append([]string{}, baseArguments...), arguments...)...)
	command.Dir = git.Dir
	command.Env = environment()
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", arguments[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Output runs a local Git command and returns trimmed stdout.
func (git Git) Output(ctx context.Context, arguments ...string) (string, error) {
	return git.run(ctx, nil, arguments...)
}

// Net runs a Git command that talks to the remote, bounded by NetworkTimeout.
// Any failure, including the timeout, is reported as ErrNetwork.
func (git Git) Net(ctx context.Context, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, NetworkTimeout)
	defer cancel()
	out, err := git.run(ctx, nil, arguments...)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	return out, nil
}

// Ok reports whether a local Git command exits zero.
func (git Git) Ok(ctx context.Context, arguments ...string) bool {
	_, err := git.run(ctx, nil, arguments...)
	return err == nil
}
