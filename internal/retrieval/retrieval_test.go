package retrieval_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/retrieval"
)

func openRetriever(t *testing.T, root string) *retrieval.Retriever {
	t.Helper()
	r, err := retrieval.New(root, "domains", "decision")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r
}

func TestRetrieveSearchesOrdinaryMarkdownWithPathsAndLines(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/checkout.md": "---\nowner: blue\n---\n# Checkout\nIntro timeout policy.\n## Rules\nThe timeout is five seconds.\n",
		"memories/domains/plain.md":    "Plain Unicode cobrança without headings.\n",
		"memories/domains/broken.md":   "# Multiple\n# Titles\n",
	})
	r := openRetriever(t, root)
	result, err := r.Retrieve(context.Background(), "timeout", retrieval.Scope{}, 20)
	if err != nil || len(result.Items) != 1 || result.Revision == "" || result.Truncated {
		t.Fatalf("search = %+v, %v", result, err)
	}
	item := result.Items[0]
	if item.Path != "memories/domains/checkout.md" || item.StartLine != 2 || item.EndLine != 7 || item.Content != "owner: blue\n---\n# Checkout\nIntro timeout policy.\n## Rules\nThe timeout is five seconds.\n" {
		t.Fatalf("excerpt = %+v", item)
	}
	unicode, err := r.Retrieve(context.Background(), "COBRANÇA", retrieval.Scope{}, 20)
	if err != nil || len(unicode.Items) != 1 || unicode.Items[0].Path != "memories/domains/plain.md" {
		t.Fatalf("unicode = %+v, %v", unicode, err)
	}
}

func TestRetrieveScopesCategoriesAndReportsLimitedResults(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"memories/domains/a.md", "memories/domains/nested/b.md", "memories/decision/c.md", "memories/notes/d.md", "proposals/e.md", "README.md"} {
		files[name] = "A common fact.\n"
	}
	root := newMemoryRoot(t, files)
	r := openRetriever(t, root)
	result, err := r.Retrieve(context.Background(), "common", retrieval.Scope{}, 2)
	if err != nil || !result.Truncated || len(result.Items) != 2 {
		t.Fatalf("limited = %+v, %v", result, err)
	}
	if result.Items[0].Path != "memories/decision/c.md" || result.Items[1].Path != "memories/domains/a.md" {
		t.Fatalf("order = %+v", result.Items)
	}
	scoped, err := r.Retrieve(context.Background(), "common", retrieval.Scope{PathPrefix: "memories/domains/nested"}, 20)
	if err != nil || len(scoped.Items) != 1 || scoped.Items[0].Path != "memories/domains/nested/b.md" {
		t.Fatalf("scoped = %+v, %v", scoped, err)
	}
	full, err := r.Retrieve(context.Background(), "common", retrieval.Scope{}, 3)
	if err != nil || full.Truncated || len(full.Items) != 3 {
		t.Fatalf("full = %+v, %v", full, err)
	}
}

func TestReadPreservesBytesAndPaginatesWithoutMarkdownParsing(t *testing.T) {
	const name = "memories/domains/plain.md"
	const body = "---\r\nowner: blue\r\n---\r\n# First\r\n# Second\r\nLast line"
	root := newMemoryRoot(t, map[string]string{name: body})
	r := openRetriever(t, root)
	whole, err := r.Read(context.Background(), name, "", 0, 0)
	if err != nil || whole.Content != body || whole.StartLine != 1 || whole.EndLine != 6 || whole.Truncated {
		t.Fatalf("whole = %+v, %v", whole, err)
	}
	first, err := r.Read(context.Background(), name, whole.Revision, 2, 2)
	if err != nil || first.Content != "owner: blue\r\n---\r\n" || first.StartLine != 2 || first.EndLine != 3 || !first.Truncated {
		t.Fatalf("window = %+v, %v", first, err)
	}
	end, err := r.Read(context.Background(), name, whole.Revision, 6, 1)
	if err != nil || end.Content != "Last line" || end.Truncated {
		t.Fatalf("last = %+v, %v", end, err)
	}
	beyond, err := r.Read(context.Background(), name, whole.Revision, 99, 1)
	if err != nil || beyond.Content != "" || beyond.Truncated {
		t.Fatalf("beyond = %+v, %v", beyond, err)
	}
}

func TestReadSearchRevisionSurvivesUpdatesAndRestart(t *testing.T) {
	const name = "memories/domains/fact.md"
	root := newMemoryRoot(t, map[string]string{name: "Old timeout.\n"})
	r := openRetriever(t, root)
	before, err := r.Retrieve(context.Background(), "timeout", retrieval.Scope{}, 20)
	if err != nil || len(before.Items) != 1 {
		t.Fatalf("before = %+v, %v", before, err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("New timeout.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Working files do not enter published reads.
	unchanged, err := r.Read(context.Background(), name, "", 1, 200)
	if err != nil || unchanged.Content != "Old timeout.\n" {
		t.Fatalf("dirty read = %+v, %v", unchanged, err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-qm", "publish new timeout")
	restarted := openRetriever(t, root)
	old, err := restarted.Read(context.Background(), name, before.Revision, 1, 200)
	if err != nil || old.Content != "Old timeout.\n" || old.Revision != before.Revision {
		t.Fatalf("old revision = %+v, %v", old, err)
	}
	current, err := restarted.Read(context.Background(), name, "", 1, 200)
	if err != nil || current.Content != "New timeout.\n" || current.Revision == old.Revision {
		t.Fatalf("current = %+v, %v", current, err)
	}
}

func TestReadRejectsUnpublishedRevision(t *testing.T) {
	const name = "memories/domains/fact.md"
	root := newMemoryRoot(t, map[string]string{name: "Published.\n"})
	runGit(t, root, "checkout", "-qb", "draft")
	if err := os.WriteFile(filepath.Join(root, name), []byte("Unpublished.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-qam", "draft")
	runGit(t, root, "checkout", "-")
	r := openRetriever(t, root)
	if _, err := r.Read(context.Background(), name, "draft", 1, 200); err == nil {
		t.Fatal("unpublished revision was readable")
	}
}

func TestRetrievalRejectsEscapesAndSymlinksAndDoesNotChangeFiles(t *testing.T) {
	const name = "memories/domains/fact.md"
	root := newMemoryRoot(t, map[string]string{name: "Common fact.\n", "proposals/private.md": "common private\n"})
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "memories/domains/link.md")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-qm", "link")
	before := treeFingerprint(t, root)
	r := openRetriever(t, root)
	result, err := r.Retrieve(context.Background(), "common", retrieval.Scope{}, 20)
	if err != nil || len(result.Items) != 1 || result.Items[0].Path != name {
		t.Fatalf("search = %+v, %v", result, err)
	}
	for _, name := range []string{"../etc/passwd", "/etc/passwd", "memories/domains/../../proposals/private.md", "memories/domains/link.md", "memories/notes/fact.md"} {
		if _, err := r.Read(context.Background(), name, "", 1, 200); err == nil {
			t.Fatalf("read accepted %q", name)
		}
	}
	if after := treeFingerprint(t, root); before != after {
		t.Fatalf("read changed repository: %s -> %s", before, after)
	}
}

func TestRetrievalPinsMemoryRootAcrossPathReplacement(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{"memories/domains/fact.md": "Common fact.\n"})
	r := openRetriever(t, root)
	moved := root + "-moved"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := r.Retrieve(context.Background(), "common", retrieval.Scope{}, 20)
	if err != nil || len(got.Items) != 1 || got.Items[0].Content != "Common fact.\n" {
		t.Fatalf("pinned read = %+v, %v", got, err)
	}
}

func TestRetrievalReportsInvalidArgumentsAndEmptyResults(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{"memories/domains/empty.md": ""})
	r := openRetriever(t, root)
	if _, err := r.Retrieve(context.Background(), " ", retrieval.Scope{}, 20); !errors.Is(err, retrieval.ErrInvalidQuery) {
		t.Fatalf("query = %v", err)
	}
	if _, err := r.Retrieve(context.Background(), "common", retrieval.Scope{PathPrefix: "../private"}, 20); !errors.Is(err, retrieval.ErrInvalidScope) {
		t.Fatalf("scope = %v", err)
	}
	if _, err := r.Read(context.Background(), "memories/domains/empty.md", "", -1, 1); !errors.Is(err, retrieval.ErrInvalidRange) {
		t.Fatalf("range = %v", err)
	}
	result, err := r.Retrieve(context.Background(), "missing", retrieval.Scope{}, 20)
	if err != nil || !reflect.DeepEqual(result.Items, []retrieval.Item{}) || result.Truncated {
		t.Fatalf("empty = %+v, %v", result, err)
	}
	empty, err := r.Read(context.Background(), "memories/domains/empty.md", "", 1, 200)
	if err != nil || empty.Content != "" || empty.EndLine != 0 || empty.Truncated {
		t.Fatalf("empty read = %+v, %v", empty, err)
	}
}

func newMemoryRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	for name, contents := range files {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "initial")
	return root
}

func runGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	// Auto maintenance would run detached after a commit and touch .git while a
	// test fingerprints the root.
	command := exec.Command("git", append([]string{"-C", root, "-c", "maintenance.auto=false"}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func treeFingerprint(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", relative, info.Mode())
		if info.Mode().IsRegular() {
			contents, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}
			hash.Write(contents)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
