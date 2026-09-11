package experiment

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLauncherIsolatesVerificationUmaskFromPrivateRuntime(t *testing.T) {
	root := t.TempDir()
	tools := filepath.Join(root, "tools")
	if err := os.Mkdir(tools, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestExecutable(t, filepath.Join(tools, "git"), `#!/bin/sh
if [ "$1" = rev-parse ]; then printf '%s\n' "$FIXTURE_ROOT"; fi
`)
	// Substituting go is the launcher seam: no authenticated process is started.
	writeTestExecutable(t, filepath.Join(tools, "go"), `#!/bin/sh
set -eu
case "$3" in
  test|vet|build)
    actual=$(umask)
    [ "$actual" = 0022 ] || [ "$actual" = 022 ] || exit 41
    printf 'verified\n'
    ;;
  run)
    actual=$(umask)
    [ "$actual" = 0077 ] || [ "$actual" = 077 ] || exit 42
    if [ -t 0 ]; then
      foreground=$(ps -o tpgid= -p $$)
      group=$(ps -o pgid= -p $$)
      [ "$foreground" = "$group" ] || exit 44
      IFS= read -r confirmation
      [ "$confirmation" = human-confirmation ] || exit 45
    fi
    printf 'private runtime\n'
    ;;
  *) exit 43 ;;
esac
`)
	command := exec.Command("bash", filepath.Join(repositoryRoot(t), "tests/experiment/run-phase-3-value.sh"), "--approved", "launcher-test")
	command.Dir = root
	command.Env = []string{"PATH=" + tools + ":" + os.Getenv("PATH"), "FIXTURE_ROOT=" + root, "HOME=" + root}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("launcher failed: %v: %s", err, output)
	}
	if strings.Count(string(output), "verified\n") != 4 || !strings.Contains(string(output), "private runtime\n") {
		t.Fatalf("launcher did not run four checks before the private runtime: %s", output)
	}
	terminal := exec.Command("script", "--quiet", "--return", "--command", "bash "+shellQuote(filepath.Join(repositoryRoot(t), "tests/experiment/run-phase-3-value.sh"))+" --approved launcher-terminal-test", "/dev/null")
	terminal.Dir, terminal.Env = command.Dir, command.Env
	terminal.Stdin = strings.NewReader("human-confirmation\n")
	terminalOutput, err := terminal.CombinedOutput()
	if err != nil {
		t.Fatalf("launcher cannot read a human confirmation from its terminal: %v: %s", err, terminalOutput)
	}
	if !strings.Contains(string(terminalOutput), "private runtime") {
		t.Fatal("interactive runtime did not complete")
	}
}
