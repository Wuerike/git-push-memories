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
	symlink := filepath.Join(root, "services", "escape.md")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	internalSymlink := filepath.Join(root, "services", "alias.md")
	if err := os.Symlink("checkout-api.md", internalSymlink); err != nil {
		t.Fatal(err)
	}

	for _, requestedPath := range []string{outside, "../outside.md", "services/../services/checkout-api.md", "services/escape.md", "services/alias.md", "services/checkout-api.txt"} {
		t.Run(requestedPath, func(t *testing.T) {
			_, err := repository.Inspect(context.Background(), memory.Scope{Path: requestedPath})
			if !errors.Is(err, memory.ErrInvalidPath) {
				t.Fatalf("Inspect() error = %v, want ErrInvalidPath", err)
			}
		})
	}
}

func TestInspectAllowsConflictExamplesInsideTextAndCodeFences(t *testing.T) {
	page := "# Page\n<!-- @id: page -->\nThe token <<<<<<< is documented inline.\n\n```text\n<<<<<<< ours\n=======\n>>>>>>> theirs\n```\n"
	root := newMemoryRoot(t, page)
	repository := openRepository(t, root)
	if _, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"}); err != nil {
		t.Fatalf("Inspect() rejected conflict examples: %v", err)
	}
}

func TestInspectIgnoresHeadingsInsideFrontmatter(t *testing.T) {
	page := "---\ntitle: Page\n# YAML comment\n---\n# Page\n<!-- @id: page -->\nbody\n"
	root := newMemoryRoot(t, page)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
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
			_, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
			if !errors.Is(err, memory.ErrInvalidSchema) {
				t.Fatalf("Inspect() error = %v, want ErrInvalidSchema", err)
			}
		})
	}
}

func TestApplyRejectsUnknownNonLeafAndStructuralReplacementWithoutMutation(t *testing.T) {
	tests := []struct {
		name        string
		sectionID   string
		replacement string
		want        error
	}{
		{name: "unknown section", sectionID: "unknown", replacement: "body\n", want: memory.ErrSectionNotFound},
		{name: "non-leaf section", sectionID: "checkout-api", replacement: "body\n", want: memory.ErrSectionNotLeaf},
		{name: "heading in replacement", sectionID: "checkout-api-operation", replacement: "## Injected\n", want: memory.ErrInvalidSchema},
		{name: "anchor in replacement", sectionID: "checkout-api-operation", replacement: "<!-- @id: injected -->\n", want: memory.ErrInvalidSchema},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newMemoryRoot(t, validPage)
			repository := openRepository(t, root)
			snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "services/checkout-api.md"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = repository.Apply(context.Background(), memory.ChangeSet{
				Target:       memory.Target{Path: snapshot.Path, SectionID: test.sectionID},
				BaselineHash: snapshot.FileHash,
				Replacement:  test.replacement,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("Apply() error = %v, want %v", err, test.want)
			}
			got, readErr := os.ReadFile(filepath.Join(root, "services", "checkout-api.md"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != validPage {
				t.Fatal("Apply() mutated file after rejection")
			}
		})
	}
}
