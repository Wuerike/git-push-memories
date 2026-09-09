package retrieval_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
)

const initialMemory = `# checkout-api
<!-- @id: checkout-api -->

## Operacao
<!-- @id: checkout-api-operation -->

- O timeout de requisicoes e de cinco segundos.
- O servico pertence ao ` + "`team-a`" + `.
`

func TestRetrieveFindsOperationSection(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/checkout-api.md": initialMemory,
	})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := retriever.Close(); err != nil {
			t.Error(err)
		}
	})

	bundle, err := retriever.Retrieve(context.Background(), "checkout timeout team", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Truncated {
		t.Fatal("Retrieve() unexpectedly truncated the corpus result")
	}
	if len(bundle.Items) != 1 {
		t.Fatalf("Retrieve() returned %d items, want 1", len(bundle.Items))
	}
	item := bundle.Items[0]
	if item.Handle.Path != "memories/domains/checkout-api.md" || item.Handle.SectionID != "checkout-api-operation" {
		t.Fatalf("Retrieve() handle = %+v", item.Handle)
	}
	if item.Heading != "Operacao" || item.Score != 3 {
		t.Fatalf("Retrieve() item = %+v", item)
	}
	wantExcerpt := "- O timeout de requisicoes e de cinco segundos.\n- O servico pertence ao `team-a`."
	if item.Content != wantExcerpt {
		t.Fatalf("Retrieve() content = %q, want %q", item.Content, wantExcerpt)
	}
}

func TestRetrieveSearchesRecursivelyOnlyInConfiguredCategories(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"README.md":                        "",
		"root.md":                          initialMemory,
		"memories/checkout-api.md":         initialMemory,
		"memories/domains/checkout-api.md": initialMemory,
		"memories/decision/platform/checkout-api.md": initialMemory,
		"memories/notes/checkout-api.md":             initialMemory,
		"proposals/draft.md":                         "not a memory page",
	})
	retriever, err := retrieval.New(root, "domains", "decision")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })

	bundle, err := retriever.Retrieve(context.Background(), "checkout timeout team", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(bundle.Items))
	for index, item := range bundle.Items {
		got[index] = item.Handle.Path
	}
	want := []string{"memories/decision/platform/checkout-api.md", "memories/domains/checkout-api.md"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Retrieve() paths = %v, want %v", got, want)
	}
}

func TestExpandWorksAfterRestart(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/checkout-api.md": initialMemory,
	})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := retriever.Retrieve(context.Background(), "timeout", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	handle := bundle.Items[0].Handle
	if err := retriever.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	expanded, err := restarted.Expand(context.Background(), handle, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded.Items) != 1 || expanded.Truncated {
		t.Fatalf("Expand() = %+v", expanded)
	}
	want := "\n- O timeout de requisicoes e de cinco segundos.\n- O servico pertence ao `team-a`.\n"
	if expanded.Items[0].Content != want {
		t.Fatalf("Expand() content = %q, want %q", expanded.Items[0].Content, want)
	}
}

func TestRetrieveUsesCurrentStateAndStalesOldHandle(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{"memories/domains/checkout-api.md": initialMemory})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })
	before, err := retriever.Retrieve(context.Background(), "cinco team-a", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	oldHandle := before.Items[0].Handle

	repository, err := memory.New(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: oldHandle.Path})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: oldHandle.Path, SectionID: oldHandle.SectionID},
		BaselineHash: snapshot.FileHash,
		Replacement:  "\n- O timeout de requisicoes e de tres segundos.\n- O servico pertence ao `team-b`.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	current, err := retriever.Retrieve(context.Background(), "tres team-b", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != 1 || strings.Contains(current.Items[0].Content, "cinco") || strings.Contains(current.Items[0].Content, "team-a") {
		t.Fatalf("Retrieve() returned stale authority: %+v", current)
	}
	if _, err := retriever.Expand(context.Background(), oldHandle, 4096); !errors.Is(err, retrieval.ErrStaleHandle) {
		t.Fatalf("Expand() error = %v, want ErrStaleHandle", err)
	}
}

func TestRetrieveTieOrderIsDeterministic(t *testing.T) {
	page := func(name, firstID, secondID string) string {
		return fmt.Sprintf("# %s\n<!-- @id: %s -->\n\n## First\n<!-- @id: %s -->\ncommon\n\n## Second\n<!-- @id: %s -->\ncommon\n", name, name, firstID, secondID)
	}
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/b.md": page("b", "b-first", "b-second"),
		"memories/domains/a.md": page("a", "a-first", "a-second"),
	})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })

	bundle, err := retriever.Retrieve(context.Background(), "common", retrieval.Scope{}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(bundle.Items))
	for i, item := range bundle.Items {
		got[i] = item.Handle.Path + ":" + item.Handle.SectionID
	}
	want := []string{"memories/domains/a.md:a-first", "memories/domains/a.md:a-second", "memories/domains/b.md:b-first", "memories/domains/b.md:b-second"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Retrieve() order = %v, want %v", got, want)
	}
}

func TestRetrieveReportsWholeResultTruncation(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/a.md": "# a\n<!-- @id: a -->\n\n## One\n<!-- @id: a-one -->\ncommon first\n",
		"memories/domains/b.md": "# b\n<!-- @id: b -->\n\n## Two\n<!-- @id: b-two -->\ncommon second\n",
	})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })
	full, err := retriever.Retrieve(context.Background(), "common", retrieval.Scope{}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	oneItem := retrieval.ContextBundle{Items: full.Items[:1], Truncated: true}
	oneItemJSON, err := json.Marshal(oneItem)
	if err != nil {
		t.Fatal(err)
	}

	limited, err := retriever.Retrieve(context.Background(), "common", retrieval.Scope{}, len(oneItemJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Truncated || len(limited.Items) != 1 || limited.Items[0].Handle.Path != "memories/domains/a.md" {
		t.Fatalf("Retrieve() limited bundle = %+v", limited)
	}
	encoded, err := json.Marshal(limited)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > len(oneItemJSON) {
		t.Fatalf("Retrieve() used %d bytes with budget %d", len(encoded), len(oneItemJSON))
	}
}

func TestRetrieveScopesToCanonicalPathPrefix(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/a.md": "# a\n<!-- @id: a -->\n\n## One\n<!-- @id: a-one -->\ncommon\n",
		"memories/domains/b.md": "# b\n<!-- @id: b -->\n\n## Two\n<!-- @id: b-two -->\ncommon\n",
	})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })

	bundle, err := retriever.Retrieve(context.Background(), "common", retrieval.Scope{PathPrefix: "memories/domains/a.md"}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Items) != 1 || bundle.Items[0].Handle.Path != "memories/domains/a.md" {
		t.Fatalf("Retrieve() scoped bundle = %+v", bundle)
	}
	if _, err := retriever.Retrieve(context.Background(), "common", retrieval.Scope{PathPrefix: "../services"}, 2048); !errors.Is(err, retrieval.ErrInvalidScope) {
		t.Fatalf("Retrieve() scope error = %v, want ErrInvalidScope", err)
	}
}

func TestRetrieveAndExpandEnforceExactJSONBudget(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{"memories/domains/checkout-api.md": initialMemory})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })

	search, err := retriever.Retrieve(context.Background(), "timeout", retrieval.Scope{}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	searchJSON, err := json.Marshal(search)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Retrieve(context.Background(), "timeout", retrieval.Scope{}, len(searchJSON)); err != nil {
		t.Fatalf("Retrieve() rejected exact budget: %v", err)
	}
	if _, err := retriever.Retrieve(context.Background(), "timeout", retrieval.Scope{}, len(searchJSON)-1); !errors.Is(err, retrieval.ErrBudgetTooSmall) {
		t.Fatalf("Retrieve() error = %v, want ErrBudgetTooSmall", err)
	}

	expanded, err := retriever.Expand(context.Background(), search.Items[0].Handle, 4096)
	if err != nil {
		t.Fatal(err)
	}
	expandJSON, err := json.Marshal(expanded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Expand(context.Background(), search.Items[0].Handle, len(expandJSON)); err != nil {
		t.Fatalf("Expand() rejected exact budget: %v", err)
	}
	if _, err := retriever.Expand(context.Background(), search.Items[0].Handle, len(expandJSON)-1); !errors.Is(err, retrieval.ErrBudgetTooSmall) {
		t.Fatalf("Expand() error = %v, want ErrBudgetTooSmall", err)
	}
}

func TestRetrievalIsReadOnlyAndExcludesReservedState(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/checkout-api.md":                         initialMemory,
		"proposals/private.md":                                     "not valid memory",
		"receipts/private.md":                                      "not valid memory",
		".git-push-memories/private.md":                            "not valid memory",
		"notes.txt":                                                "timeout team",
		".git-push-memories-0123456789abcdef.tmp":                  "interrupted",
		"memories/domains/.git-push-memories-abcdefabcdef1234.tmp": "interrupted",
	})
	before := treeFingerprint(t, root)
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := retriever.Retrieve(context.Background(), "timeout team", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Items) != 1 {
		t.Fatalf("Retrieve() returned %d items, want 1", len(bundle.Items))
	}
	if _, err := retriever.Expand(context.Background(), bundle.Items[0].Handle, 4096); err != nil {
		t.Fatal(err)
	}
	if err := retriever.Close(); err != nil {
		t.Fatal(err)
	}
	if after := treeFingerprint(t, root); after != before {
		t.Fatalf("retrieval changed memory root: before %s, after %s", before, after)
	}
}

func TestRetrievalPinsMemoryRootAcrossPathReplacement(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{"memories/domains/checkout-api.md": initialMemory})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })
	movedRoot := root + "-moved"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	bundle, err := retriever.Retrieve(context.Background(), "checkout timeout team", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Items) != 1 || bundle.Items[0].Handle.Path != "memories/domains/checkout-api.md" {
		t.Fatalf("Retrieve() followed replacement root: %+v", bundle)
	}
}

func TestRetrieveDoesNotTraverseReservedDirectories(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{
		"memories/domains/checkout-api.md": initialMemory,
		"proposals/private.md":             "not valid memory",
	})
	proposals := filepath.Join(root, "proposals")
	if err := os.Chmod(proposals, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(proposals, 0o755) })
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })

	bundle, err := retriever.Retrieve(context.Background(), "timeout", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Items) != 1 {
		t.Fatalf("Retrieve() returned %d items, want 1", len(bundle.Items))
	}
}

func TestExpandTreatsMissingTargetAsStaleHandle(t *testing.T) {
	root := newMemoryRoot(t, map[string]string{"memories/domains/checkout-api.md": initialMemory})
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retriever.Close() })
	bundle, err := retriever.Retrieve(context.Background(), "timeout", retrieval.Scope{}, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "memories", "domains", "checkout-api.md")); err != nil {
		t.Fatal(err)
	}

	if _, err := retriever.Expand(context.Background(), bundle.Items[0].Handle, 4096); !errors.Is(err, retrieval.ErrStaleHandle) {
		t.Fatalf("Expand() error = %v, want ErrStaleHandle", err)
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
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
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
