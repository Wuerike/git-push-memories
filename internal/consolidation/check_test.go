package consolidation_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func preparedManager(t *testing.T) (*consolidation.Manager, *wt.Managed, consolidation.Prepared) {
	t.Helper()
	m := wt.Setup(t, map[string]string{checkoutPath: checkoutPage})
	seedProposals(t, m, 3)
	manager := consolidation.New(m.Config)
	prepared, err := manager.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return manager, m, prepared
}

func writeMemory(t *testing.T, prepared consolidation.Prepared, path, text string) {
	t.Helper()
	full := filepath.Join(prepared.Worktree, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckObservesOrdinaryMarkdownWithoutChangingFilesOrIndex(t *testing.T) {
	for name, body := range map[string]string{
		"plain text":        "A fact without headings.",
		"multiple headings": "# Checkout\n# Related policy\n",
		"frontmatter":       "---\nowner: team-green\n---\n# Checkout\nUpdated ownership.\n",
		"merge markers":     "# Checkout\n<<<<<<< HEAD\nThe agent reviews this content.\n",
		"large CRLF":        strings.Repeat("A durable fact.\r\n", 10000),
	} {
		t.Run(name, func(t *testing.T) {
			manager, m, p := preparedManager(t)
			beforeRemote := wt.Git(t, m.Remote, "show-ref")
			writeMemory(t, p, checkoutPath, body)
			wt.Git(t, p.Worktree, "add", "--", checkoutPath)
			beforeIndex := wt.Git(t, p.Worktree, "write-tree")
			got, err := manager.Check(context.Background())
			if err != nil || !reflect.DeepEqual(got.Paths, []string{checkoutPath}) || !strings.Contains(got.Diff, "diff --git") {
				t.Fatalf("check = %+v, %v", got, err)
			}
			data, err := os.ReadFile(filepath.Join(p.Worktree, checkoutPath))
			if err != nil || string(data) != body {
				t.Fatalf("check changed bytes: %q, %v", data, err)
			}
			if wt.Git(t, p.Worktree, "write-tree") != beforeIndex || wt.Git(t, m.Remote, "show-ref") != beforeRemote {
				t.Fatal("check changed index or remote")
			}
		})
	}
}

func TestCheckIncludesNewFilesInCurrentDiff(t *testing.T) {
	manager, _, p := preparedManager(t)
	const name = "memories/domains/retry.md"
	const body = "# Retry\n\n## Policy\nThree attempts.\n"
	writeMemory(t, p, name, body)
	before := wt.Git(t, p.Worktree, "write-tree")
	got, err := manager.Check(context.Background())
	if err != nil || !reflect.DeepEqual(got.Paths, []string{name}) {
		t.Fatalf("check = %+v, %v", got, err)
	}
	data, err := os.ReadFile(filepath.Join(p.Worktree, name))
	if err != nil || string(data) != body {
		t.Fatalf("check changed new file: %q, %v", data, err)
	}
	for _, expected := range []string{"--- /dev/null", "+++ b/" + name, "+Three attempts."} {
		if !strings.Contains(got.Diff, expected) {
			t.Fatalf("diff lacks %q: %s", expected, got.Diff)
		}
	}
	if wt.Git(t, p.Worktree, "write-tree") != before {
		t.Fatal("check staged the new file")
	}
}

func TestCheckReflectsCurrentHumanEditsAndDeletions(t *testing.T) {
	manager, _, p := preparedManager(t)
	first, err := manager.Check(context.Background())
	if err != nil || len(first.Paths) != 0 || first.Diff != "" {
		t.Fatalf("unchanged = %+v, %v", first, err)
	}
	human := checkoutPage + "\nHuman correction: nine seconds.\n"
	writeMemory(t, p, checkoutPath, human)
	got, err := manager.Check(context.Background())
	if err != nil || !strings.Contains(got.Diff, "+Human correction: nine seconds.") {
		t.Fatalf("human diff = %+v, %v", got, err)
	}
	if err := os.Remove(filepath.Join(p.Worktree, checkoutPath)); err != nil {
		t.Fatal(err)
	}
	got, err = manager.Check(context.Background())
	if err != nil || !reflect.DeepEqual(got.Paths, []string{checkoutPath}) || !strings.Contains(got.Diff, "+++ /dev/null") {
		t.Fatalf("deletion = %+v, %v", got, err)
	}
}

func TestCheckRejectsSymlinkMemory(t *testing.T) {
	manager, _, p := preparedManager(t)
	if err := os.Symlink("/etc/passwd", filepath.Join(p.Worktree, "memories/domains/link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Check(context.Background()); err == nil {
		t.Fatal("symlink accepted for publication")
	}
}

func TestCheckRejectsMemoryDirectorySymlinkOutsideWorktree(t *testing.T) {
	manager, _, p := preparedManager(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "checkout.md"), []byte("Outside private content.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(p.Worktree, "memories/domains")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(p.Worktree, "memories/domains")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Check(context.Background()); err == nil {
		t.Fatal("check followed a memory directory outside the worktree")
	}
}
