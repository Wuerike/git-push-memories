package memory_test

import (
	"context"
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

	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path != "memories/checkout-api.md" {
		t.Fatalf("Path = %q", snapshot.Path)
	}
	if snapshot.FileHash != "sha256:43e8ee2e48f3d539cc731b0c5fd196b32bb89e8d10e69e9c6f2ef444af5e4456" {
		t.Fatalf("FileHash = %q", snapshot.FileHash)
	}
	if snapshot.Head != gitOutput(t, root, "rev-parse", "HEAD") {
		t.Fatalf("Head = %q", snapshot.Head)
	}
	if snapshot.Blob != gitOutput(t, root, "rev-parse", "HEAD:memories/checkout-api.md") {
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

func TestRepositoryPinsMemoryRootAcrossPathReplacement(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	movedRoot := root + "-moved"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	if err := os.MkdirAll(filepath.Join(root, "memories"), 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := strings.Replace(validPage, "Timeout: 5 seconds.", "Decoy root.", 1)
	if err := os.WriteFile(filepath.Join(root, "memories", "checkout-api.md"), []byte(decoy), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Sections[1].Body != "Timeout: 5 seconds.\n" {
		t.Fatalf("Inspect() followed replacement root: %q", snapshot.Sections[1].Body)
	}
	page, err := repository.ReadPage(context.Background(), snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	if page.Markdown != validPage {
		t.Fatalf("ReadPage() followed replacement root: %q", page.Markdown)
	}
	decoyAfter, err := os.ReadFile(filepath.Join(root, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoyAfter) != decoy {
		t.Fatal("reading changed the replacement root")
	}
	pinnedAfter, err := os.ReadFile(filepath.Join(movedRoot, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pinnedAfter) != validPage {
		t.Fatal("reading changed the pinned root")
	}
}

func newMemoryRoot(t *testing.T, page string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "memories"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memories", "checkout-api.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "memories/checkout-api.md")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "initial")
	return root
}

func newEmptyMemoryRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "initial")
	return root
}

func openRepository(t *testing.T, root string) *memory.Repository {
	t.Helper()
	repository, err := memory.Open(root)
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
	// Auto maintenance would run detached after a commit and touch .git while a
	// test fingerprints the root.
	command := exec.Command("git", append([]string{"-C", root, "-c", "maintenance.auto=false"}, args...)...)
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
