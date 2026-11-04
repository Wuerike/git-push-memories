package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

func TestReadPageReturnsFullMarkdownAndFileHash(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	before := fingerprint(t, root)
	page, err := repository.ReadPage(context.Background(), "memories/checkout-api.md")
	if err != nil {
		t.Fatal(err)
	}
	if page.Path != "memories/checkout-api.md" || page.Markdown != validPage || page.ContentHash != snapshot.FileHash {
		t.Fatalf("ReadPage() = %+v, want full page with hash %s", page, snapshot.FileHash)
	}
	if after := fingerprint(t, root); after != before {
		t.Fatal("ReadPage() changed the repository")
	}
}

func TestReadPageRejectsPathOutsideCategories(t *testing.T) {
	repository := openRepository(t, newMemoryRoot(t, validPage))
	for _, requested := range []string{"../outside.md", "/etc/passwd", "notes/checkout-api.md", "memories/checkout-api.txt", "memories/missing.md"} {
		if _, err := repository.ReadPage(context.Background(), requested); !errors.Is(err, memory.ErrInvalidPath) {
			t.Fatalf("ReadPage(%q) error = %v, want ErrInvalidPath", requested, err)
		}
	}
}
