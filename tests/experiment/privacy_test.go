package experiment

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanPrivacyFindsMarkersInFilesAndUnreachableGitObjects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seedCandidateGit(t, root)
	files := filepath.Join(t.TempDir(), "files")
	if err := os.MkdirAll(files, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(files, "safe"), []byte("marker-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := runCommandWithInput(t, root, []byte("marker-object"), "git", "hash-object", "-w", "--stdin")
	if strings.TrimSpace(command) == "" {
		t.Fatal("unreachable object was not created")
	}
	result, err := scanPrivacy(context.Background(), privacyScanSpec{
		FileRoots: []string{files}, GitRoots: []string{root}, Markers: [][]byte{[]byte("marker-file"), []byte("marker-object")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FileHits != 1 || result.GitObjectHits != 1 || result.Clean {
		t.Fatalf("privacy result = %+v", result)
	}
}

func TestObjectInventoryDetectsUnreachableObjectCreation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seedCandidateGit(t, root)
	before, err := gitObjectInventory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	runCommandWithInput(t, root, []byte("secret"), "git", "hash-object", "-w", "--stdin")
	after, err := gitObjectInventory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if inventoriesEqual(before, after) {
		t.Fatal("object creation was not detected")
	}
}

func runCommandWithInput(t *testing.T, directory string, input []byte, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	command.Stdin = strings.NewReader(string(input))
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}
