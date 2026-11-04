package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

func TestInspectRejectsPathsOutsideMemoryRoot(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(validPage), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "memories", "escape.md")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	internalSymlink := filepath.Join(root, "memories", "alias.md")
	if err := os.Symlink("checkout-api.md", internalSymlink); err != nil {
		t.Fatal(err)
	}

	for _, requestedPath := range []string{outside, "../outside.md", "memories/../memories/checkout-api.md", "memories/escape.md", "memories/alias.md", "memories/checkout-api.txt"} {
		t.Run(requestedPath, func(t *testing.T) {
			_, err := repository.Inspect(context.Background(), memory.Scope{Path: requestedPath})
			if !errors.Is(err, memory.ErrInvalidPath) {
				t.Fatalf("Inspect() error = %v, want ErrInvalidPath", err)
			}
		})
	}
}

func TestInspectRequiresCommittedMemory(t *testing.T) {
	for _, withHead := range []bool{false, true} {
		name := "without HEAD"
		if withHead {
			name = "untracked page"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init", "--quiet")
			if withHead {
				runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "initial")
			}
			if err := os.Mkdir(filepath.Join(root, "memories"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "memories", "checkout-api.md"), []byte(validPage), 0o644); err != nil {
				t.Fatal(err)
			}
			repository := openRepository(t, root)
			if _, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"}); !errors.Is(err, memory.ErrInvalidSchema) {
				t.Fatalf("Inspect() error = %v, want ErrInvalidSchema", err)
			}
		})
	}
}

func TestInspectAcceptsNestedMemoryPages(t *testing.T) {
	root := newEmptyMemoryRoot(t)
	for _, name := range []string{"memories/page.md", "memories/nested/page.md", "page.md"} {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(validPage), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "pages")
	repository := openRepository(t, root)

	for _, requestedPath := range []string{"memories/page.md", "memories/nested/page.md"} {
		if _, err := repository.Inspect(context.Background(), memory.Scope{Path: requestedPath}); err != nil {
			t.Fatalf("Inspect(%q) rejected memory page: %v", requestedPath, err)
		}
	}
	if _, err := repository.Inspect(context.Background(), memory.Scope{Path: "page.md"}); !errors.Is(err, memory.ErrInvalidPath) {
		t.Fatalf("Inspect(page.md) error = %v, want ErrInvalidPath", err)
	}
}

func TestInspectAllowsConflictExamplesInsideTextAndCodeFences(t *testing.T) {
	page := "# Page\n<!-- @id: page -->\nThe token <<<<<<< is documented inline.\n\n```text\n<<<<<<< ours\n=======\n>>>>>>> theirs\n```\n"
	root := newMemoryRoot(t, page)
	repository := openRepository(t, root)
	if _, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"}); err != nil {
		t.Fatalf("Inspect() rejected conflict examples: %v", err)
	}
}

func TestInspectIgnoresHeadingsInsideFrontmatter(t *testing.T) {
	page := "---\ntitle: Page\n# YAML comment\n---\n# Page\n<!-- @id: page -->\nbody\n"
	root := newMemoryRoot(t, page)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sections) != 1 || snapshot.Sections[0].ID != "page" {
		t.Fatalf("Sections = %+v, want only page", snapshot.Sections)
	}
}

func TestInspectRejectsInvalidMarkdownSchema(t *testing.T) {
	tests := []struct {
		name string
		page []byte
	}{
		{name: "invalid UTF-8", page: []byte{0xff}},
		{name: "missing heading", page: []byte("body\n")},
		{name: "missing ID", page: []byte("# Page\nbody\n")},
		{name: "invalid ID", page: []byte("# Page\n<!-- @id: Invalid -->\n")},
		{name: "duplicate ID", page: []byte("# Page\n<!-- @id: page -->\n## Child\n<!-- @id: page -->\n")},
		{name: "multiple H1", page: []byte("# One\n<!-- @id: one -->\n# Two\n<!-- @id: two -->\n")},
		{name: "conflict marker", page: []byte("# Page\n<!-- @id: page -->\n<<<<<<< ours\n")},
		{name: "merge anchor", page: []byte("# Page\n<!-- @id: page -->\n<!-- @merge-conflict page -->\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newMemoryRoot(t, string(test.page))
			repository := openRepository(t, root)
			_, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
			if !errors.Is(err, memory.ErrInvalidSchema) {
				t.Fatalf("Inspect() error = %v, want ErrInvalidSchema", err)
			}
		})
	}
}
