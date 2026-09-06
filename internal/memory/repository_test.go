package memory_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

const validPage = "# Checkout API\n<!-- @id: checkout-api -->\nOverview.\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout: 5 seconds.\n"

func TestInspectReturnsCanonicalSnapshotAndGitIdentity(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)

	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path != "services/checkout-api.md" {
		t.Fatalf("Path = %q", snapshot.Path)
	}
	if snapshot.FileHash != "sha256:43e8ee2e48f3d539cc731b0c5fd196b32bb89e8d10e69e9c6f2ef444af5e4456" {
		t.Fatalf("FileHash = %q", snapshot.FileHash)
	}
	if snapshot.Head != gitOutput(t, root, "rev-parse", "HEAD") {
		t.Fatalf("Head = %q", snapshot.Head)
	}
	if snapshot.Blob != gitOutput(t, root, "rev-parse", "HEAD:services/checkout-api.md") {
		t.Fatalf("Blob = %q", snapshot.Blob)
	}
	if len(snapshot.Sections) != 2 {
		t.Fatalf("Sections = %+v", snapshot.Sections)
	}
	operation := snapshot.Sections[1]
	if operation.ID != "checkout-api-operation" || operation.Heading != "Operation" || !operation.Leaf {
		t.Fatalf("operation section = %+v", operation)
	}
	if operation.Body != "Timeout: 5 seconds.\n" {
		t.Fatalf("operation body = %q", operation.Body)
	}
}

func TestApplyReplacesOnlyLeafBodyAndReturnsReceipt(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	target := memory.Target{Path: snapshot.Path, SectionID: "checkout-api-operation"}
	receipt, err := repository.Apply(context.Background(), memory.ChangeSet{
		Target:       target,
		BaselineHash: snapshot.FileHash,
		Replacement:  "Timeout: 3 seconds.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Target != target || receipt.BaselineHash != snapshot.FileHash || receipt.PostStateHash == snapshot.FileHash {
		t.Fatalf("receipt = %+v", receipt)
	}
	got, err := os.ReadFile(filepath.Join(root, "services", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Checkout API\n<!-- @id: checkout-api -->\nOverview.\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout: 3 seconds.\n"
	if string(got) != want {
		t.Fatalf("applied file mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyReportsDriftBeforeParsingChangedBytes(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "services", "checkout-api.md")
	changed := append([]byte{0xff}, []byte(validPage[1:])...)
	if err := os.WriteFile(filePath, changed, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: snapshot.Path, SectionID: "checkout-api-operation"},
		BaselineHash: snapshot.FileHash,
		Replacement:  "new body\n",
	})
	if !errors.Is(err, memory.ErrDrift) {
		t.Fatalf("Apply() error = %v, want ErrDrift", err)
	}
	got, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(changed) {
		t.Fatal("Apply() changed the drifted file")
	}
}

func TestRepositoryPinsMemoryRootAcrossPathReplacement(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	movedRoot := root + "-moved"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	if err := os.MkdirAll(filepath.Join(root, "services"), 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := strings.Replace(validPage, "Timeout: 5 seconds.", "Decoy root.", 1)
	if err := os.WriteFile(filepath.Join(root, "services", "checkout-api.md"), []byte(decoy), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Sections[1].Body != "Timeout: 5 seconds.\n" {
		t.Fatalf("Inspect() followed replacement root: %q", snapshot.Sections[1].Body)
	}
	_, err = repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: snapshot.Path, SectionID: "checkout-api-operation"},
		BaselineHash: snapshot.FileHash,
		Replacement:  "Pinned root.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	decoyAfter, err := os.ReadFile(filepath.Join(root, "services", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoyAfter) != decoy {
		t.Fatal("Apply() wrote through replacement root path")
	}
	pinnedAfter, err := os.ReadFile(filepath.Join(movedRoot, "services", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pinnedAfter), "Pinned root.\n") {
		t.Fatal("Apply() did not write through pinned root descriptor")
	}
}

func newMemoryRoot(t *testing.T, page string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "services"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "services", "checkout-api.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "services/checkout-api.md")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "initial")
	return root
}

func openRepository(t *testing.T, root string) *memory.Repository {
	t.Helper()
	repository, err := memory.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})
	return repository
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}
