package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
)

func page(timeout string) string {
	return "# checkout-api\n<!-- @id: checkout-api -->\n\n## Operacao\n<!-- @id: checkout-api-operation -->\n\n- O timeout e de " + timeout + " segundos.\n"
}

func TestReadFollowsMainAndStaleHandleIsDetected(t *testing.T) {
	ctx := context.Background()
	wt.Env(t)
	remote := wt.Remote(t)
	wt.Seed(t, remote, "main", map[string]string{"gpm.json": "{}\n", "memories/domains/checkout-api.md": page("cinco")})
	w, _ := workspace.Open(remote)
	if err := w.Clone(ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.EnsureRead(ctx); err != nil {
		t.Fatal(err)
	}
	root, err := w.ResolveRead(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	retriever, err := retrieval.New(root, "domains")
	if err != nil {
		t.Fatal(err)
	}
	defer retriever.Close()
	first, err := retriever.Retrieve(ctx, "timeout cinco", retrieval.Scope{}, 4096)
	if err != nil || len(first.Items) != 1 {
		t.Fatalf("first search = %+v, %v", first, err)
	}

	wt.Seed(t, remote, "main", map[string]string{"memories/domains/checkout-api.md": page("dez")})
	os.Remove(filepath.Join(w.Paths.State, "fetch-attempt")) // TTL expired
	if _, err := w.ResolveRead(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := retriever.Expand(ctx, first.Items[0].Handle, 4096); !errors.Is(err, retrieval.ErrStaleHandle) {
		t.Fatalf("Expand old handle = %v, want ErrStaleHandle", err)
	}
	second, err := retriever.Retrieve(ctx, "timeout dez", retrieval.Scope{}, 4096)
	if err != nil || len(second.Items) != 1 || !strings.Contains(second.Items[0].Content, "dez") {
		t.Fatalf("second search = %+v, %v", second, err)
	}
}

func TestResolveReadOfflineKeepsLastCheckout(t *testing.T) {
	ctx := context.Background()
	w, remote := setup(t)
	if err := w.EnsureRead(ctx); err != nil {
		t.Fatal(err)
	}
	os.Rename(remote, remote+".gone")
	if root, err := w.ResolveRead(ctx, true); err != nil || root != w.Paths.Read {
		t.Fatalf("ResolveRead offline = %q, %v", root, err)
	}
	if _, err := os.Stat(filepath.Join(w.Paths.Read, "memories", "a.md")); err != nil {
		t.Fatal(err)
	}
}
