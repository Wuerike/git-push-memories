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

func TestCheckAcceptsOnlyCanonicalDocuments(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{"invalid structure", checkoutPath, "# Checkout\n# Another title\n"},
		{"unconfigured category", "memories/other/policy.md", "# Policy\nA valid fact.\n"},
		{"unresolved merge", checkoutPath, "# Checkout\n<<<<<<< HEAD\nA conflicting fact.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, m, prepared := preparedManager(t)
			before := wt.Git(t, m.Remote, "show-ref")
			writeMemory(t, prepared, tc.path, tc.body)
			if _, err := manager.Check(context.Background()); err == nil {
				t.Fatal("check accepted a noncanonical document")
			}
			if wt.Git(t, m.Remote, "show-ref") != before {
				t.Fatal("check published invalid memory")
			}
		})
	}
}

func TestCheckIncludesNewFilesInCurrentDiff(t *testing.T) {
	manager, _, prepared := preparedManager(t)
	const path = "memories/domains/retry.md"
	writeMemory(t, prepared, path, "# Retry\n\n## Policy\nThree attempts.\n")
	got, err := manager.Check(context.Background())
	if err != nil || !reflect.DeepEqual(got.Paths, []string{path}) {
		t.Fatalf("check = %+v, %v", got, err)
	}
	want := "# Retry\n<!-- @id: retry -->\n\n## Policy\n<!-- @id: retry-policy -->\nThree attempts.\n"
	data, err := os.ReadFile(filepath.Join(prepared.Worktree, path))
	if err != nil || string(data) != want {
		t.Fatalf("prepared page = %q, %v; want %q", data, err, want)
	}
	for _, expected := range []string{"--- /dev/null", "+++ b/" + path, "+<!-- @id: retry-policy -->", "+Three attempts."} {
		if !strings.Contains(got.Diff, expected) {
			t.Fatalf("diff lacks %q: %s", expected, got.Diff)
		}
	}
}

func TestCheckReflectsCurrentHumanEdits(t *testing.T) {
	manager, _, prepared := preparedManager(t)
	first, err := manager.Check(context.Background())
	if err != nil || len(first.Paths) != 0 || first.Diff != "" {
		t.Fatalf("unchanged check = %+v, %v", first, err)
	}
	human := checkoutPage + "\nHuman correction: nine seconds.\n"
	writeMemory(t, prepared, checkoutPath, human)
	current, err := manager.Check(context.Background())
	if err != nil || !reflect.DeepEqual(current.Paths, []string{checkoutPath}) || !strings.Contains(current.Diff, "+Human correction: nine seconds.") {
		t.Fatalf("current check = %+v, %v", current, err)
	}
	data, err := os.ReadFile(filepath.Join(prepared.Worktree, checkoutPath))
	if err != nil || string(data) != human {
		t.Fatalf("check changed human text: %q, %v", data, err)
	}
}
