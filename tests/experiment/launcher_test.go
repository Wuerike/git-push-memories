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
}
