package setup_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/setup"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func categories(t *testing.T, asked *int) func() ([]string, error) {
	return func() ([]string, error) {
		*asked++
		return []string{"domains", "decision"}, nil
	}
}

func TestEmptyRemoteIsInitializedAndRerunIsIdempotent(t *testing.T) {
	wt.Env(t)
	remote := wt.Remote(t)
	ctx := context.Background()
	asked := 0
	result, err := setup.Prepare(ctx, setup.Options{URL: remote, Categories: categories(t, &asked)})
	if err != nil {
		t.Fatal(err)
	}
	if asked != 1 || !slices.Equal(result.MemoryCategories, []string{"domains", "decision"}) {
		t.Fatalf("result = %+v, asked %d", result, asked)
	}
	refs := wt.Git(t, remote, "rev-parse", "main", "proposals")
	if strings.Count(refs, "\n") != 1 {
		t.Fatalf("remote refs = %q", refs)
	}
	for _, dir := range []string{result.Paths.Read, result.Paths.Proposals} {
		if _, err := os.Stat(filepath.Join(dir, "gpm.json")); err != nil {
			t.Fatalf("worktree %s: %v", dir, err)
		}
	}
	if err := config.Save(remote); err != nil {
		t.Fatal(err)
	}
	again, err := setup.Prepare(ctx, setup.Options{URL: remote, Categories: categories(t, &asked)})
	if err != nil || asked != 1 || !slices.Equal(again.MemoryCategories, result.MemoryCategories) {
		t.Fatalf("rerun = %+v, %v, asked %d", again, err, asked)
	}
	if got := wt.Git(t, remote, "rev-parse", "main", "proposals"); got != refs {
		t.Fatalf("repeated setup changed remote refs: %q, want %q", got, refs)
	}
}

func TestSecondMachineUsesExistingProposals(t *testing.T) {
	wt.Env(t)
	remote := wt.Remote(t)
	wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{\"schema_version\":1,\"memory_categories\":[\"domains\"]}\n"})
	wt.Seed(t, remote, "proposals", map[string]string{"proposals/1-x.md": "p\n"})
	asked := 0
	result, err := setup.Prepare(context.Background(), setup.Options{URL: remote, Categories: categories(t, &asked)})
	if err != nil || asked != 0 {
		t.Fatalf("Prepare = %+v, %v, asked %d", result, err, asked)
	}
	if _, err := os.Stat(filepath.Join(result.Paths.Proposals, "proposals", "1-x.md")); err != nil {
		t.Fatalf("proposals worktree lost remote proposal: %v", err)
	}
}

func TestRemoteWithoutGpmJSONIsRejected(t *testing.T) {
	wt.Env(t)
	remote := wt.Remote(t)
	wt.Seed(t, remote, "main", map[string]string{"README.md": "x\n"})
	if _, err := setup.Prepare(context.Background(), setup.Options{URL: remote}); err == nil || !strings.Contains(err.Error(), "no gpm.json") {
		t.Fatalf("Prepare error = %v", err)
	}
}

func TestDifferentConfiguredURLIsRejected(t *testing.T) {
	wt.Env(t)
	if err := config.Save("git@github.com:o/a.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.Prepare(context.Background(), setup.Options{URL: "git@github.com:o/b.git"}); err == nil || !strings.Contains(err.Error(), "already configured") {
		t.Fatalf("Prepare error = %v", err)
	}
}
