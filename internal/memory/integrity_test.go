package memory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

func TestReadingDoesNotWriteOutsideRootGitIndexOrObjects(t *testing.T) {
	memoryRoot := newMemoryRoot(t, validPage)
	outsideRoot := newMemoryRoot(t, validPage)
	repository := openRepository(t, memoryRoot)
	outsideBefore := fingerprint(t, outsideRoot)
	indexBefore := fingerprint(t, filepath.Join(memoryRoot, ".git", "index"))
	objectsBefore := fingerprint(t, filepath.Join(memoryRoot, ".git", "objects"))
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if got := fingerprint(t, filepath.Join(memoryRoot, ".git", "index")); got != indexBefore {
		t.Fatal("Inspect() modified Git index")
	}
	if got := fingerprint(t, filepath.Join(memoryRoot, ".git", "objects")); got != objectsBefore {
		t.Fatal("Inspect() modified Git objects")
	}

	page, err := repository.ReadPage(context.Background(), snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	if page.Markdown != validPage || page.ContentHash != snapshot.FileHash {
		t.Fatalf("ReadPage() = %+v, want the inspected page", page)
	}
	if got := fingerprint(t, outsideRoot); got != outsideBefore {
		t.Fatal("reading modified an unrelated worktree")
	}
	if got := fingerprint(t, filepath.Join(memoryRoot, ".git", "index")); got != indexBefore {
		t.Fatal("ReadPage() modified Git index")
	}
	if got := fingerprint(t, filepath.Join(memoryRoot, ".git", "objects")); got != objectsBefore {
		t.Fatal("ReadPage() modified Git objects")
	}
	entries, err := os.ReadDir(filepath.Join(memoryRoot, "memories"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "checkout-api.md" {
		t.Fatalf("unexpected files after reading: %v", entries)
	}
	info, err := os.Stat(filepath.Join(memoryRoot, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %o, want 644", info.Mode().Perm())
	}
}

func fingerprint(t *testing.T, root string) string {
	t.Helper()
	digest := sha256.New()
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(digest, "%s\x00%s\x00", filepath.ToSlash(relative), info.Mode())
		if entry.Type().IsRegular() {
			content, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}
			_, _ = digest.Write(content)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(digest.Sum(nil))
}
