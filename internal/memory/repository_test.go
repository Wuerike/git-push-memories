package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/memory"
)

const validPage = "# Checkout API\n<!-- @id: checkout-api -->\nOverview.\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout: 5 seconds.\n"

func TestInspectReturnsCanonicalSnapshotAndGitIdentity(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)

	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path != "memories/checkout-api.md" {
		t.Fatalf("Path = %q", snapshot.Path)
	}
	if snapshot.FileHash != "sha256:43e8ee2e48f3d539cc731b0c5fd196b32bb89e8d10e69e9c6f2ef444af5e4456" {
		t.Fatalf("FileHash = %q", snapshot.FileHash)
	}
	if snapshot.Head != gitOutput(t, root, "rev-parse", "HEAD") {
		t.Fatalf("Head = %q", snapshot.Head)
	}
	if snapshot.Blob != gitOutput(t, root, "rev-parse", "HEAD:memories/checkout-api.md") {
		t.Fatalf("Blob = %q", snapshot.Blob)
	}
	if len(snapshot.Sections) != 2 {
		t.Fatalf("Sections = %+v", snapshot.Sections)
	}
	operation := snapshot.Sections[1]
	if operation.ID != "checkout-api-operation" || operation.Heading != "Operation" || !operation.Leaf {
		t.Fatalf("operation section = %+v", operation)
	}
	if operation.Body != "Timeout: 5 seconds.\n" {
		t.Fatalf("operation body = %q", operation.Body)
	}
}

func TestPreviewCreatePageRendersCanonicalDocumentWithoutWriting(t *testing.T) {
	root := newEmptyMemoryRoot(t)
	repository := openRepository(t, root)
	before := fingerprint(t, root)

	preview, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{
		TargetPath: "memories/parcel-dispatch.md",
		Title:      "Parcel Dispatch",
		Body:       "Dispatch retries use exponential backoff.\r\nOwner: Fulfillment.",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantPage := "# Parcel Dispatch\n<!-- @id: parcel-dispatch -->\n\n## Overview\n<!-- @id: parcel-dispatch-overview -->\nDispatch retries use exponential backoff.\nOwner: Fulfillment.\n"
	if preview.Target != (memory.Target{Path: "memories/parcel-dispatch.md", SectionID: "parcel-dispatch-overview"}) {
		t.Fatalf("Target = %+v", preview.Target)
	}
	if preview.PageID != "parcel-dispatch" || preview.Head != gitOutput(t, root, "rev-parse", "HEAD") {
		t.Fatalf("preview identity = %+v", preview)
	}
	if string(preview.PostState) != wantPage {
		t.Fatalf("PostState mismatch\ngot:\n%s\nwant:\n%s", preview.PostState, wantPage)
	}
	if preview.PostStateHash != "sha256:10b8a1d74f0605c17708900c2750c3db78305afb067db81fc75a68426841dfb0" {
		t.Fatalf("PostStateHash = %q", preview.PostStateHash)
	}
	wantDiff := "--- /dev/null\n+++ b/memories/parcel-dispatch.md\n@@ -0,0 +1,7 @@\n+# Parcel Dispatch\n+<!-- @id: parcel-dispatch -->\n+\n+## Overview\n+<!-- @id: parcel-dispatch-overview -->\n+Dispatch retries use exponential backoff.\n+Owner: Fulfillment.\n"
	if preview.Diff != wantDiff {
		t.Fatalf("Diff mismatch\ngot:\n%s\nwant:\n%s", preview.Diff, wantDiff)
	}
	if got := fingerprint(t, root); got != before {
		t.Fatal("PreviewCreatePage() modified the memory root")
	}
}

func TestPreviewCreatePageRejectsExistingTargetInHeadIndexOrWorktree(t *testing.T) {
	for _, location := range []string{"head", "index", "worktree"} {
		t.Run(location, func(t *testing.T) {
			root := newEmptyMemoryRoot(t)
			pagePath := filepath.Join(root, "memories", "existing.md")
			if err := os.MkdirAll(filepath.Dir(pagePath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pagePath, []byte(validPage), 0o644); err != nil {
				t.Fatal(err)
			}
			switch location {
			case "head":
				runGit(t, root, "add", "memories/existing.md")
				runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "existing")
			case "index":
				runGit(t, root, "add", "memories/existing.md")
			}
			repository := openRepository(t, root)
			_, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{
				TargetPath: "memories/existing.md", Title: "Existing", Body: "Body.",
			})
			if !errors.Is(err, memory.ErrTargetExists) {
				t.Fatalf("PreviewCreatePage() error = %v, want ErrTargetExists", err)
			}
		})
	}
}

func TestPreviewCreatePageRejectsInvalidStructure(t *testing.T) {
	tests := []struct {
		name    string
		request memory.CreatePageRequest
		want    error
	}{
		{name: "root path", request: memory.CreatePageRequest{TargetPath: "page.md", Title: "Page", Body: "Body."}, want: memory.ErrInvalidPath},
		{name: "uppercase directory", request: memory.CreatePageRequest{TargetPath: "memories/Nested/page.md", Title: "Page", Body: "Body."}, want: memory.ErrInvalidPath},
		{name: "missing extension", request: memory.CreatePageRequest{TargetPath: "page", Title: "Page", Body: "Body."}, want: memory.ErrInvalidPath},
		{name: "uppercase filename", request: memory.CreatePageRequest{TargetPath: "Page.md", Title: "Page", Body: "Body."}, want: memory.ErrInvalidPath},
		{name: "reserved filename", request: memory.CreatePageRequest{TargetPath: "con.md", Title: "Page", Body: "Body."}, want: memory.ErrInvalidPath},
		{name: "long generated ID", request: memory.CreatePageRequest{TargetPath: "memories/" + strings.Repeat("a", 56) + ".md", Title: "Page", Body: "Body."}, want: memory.ErrInvalidID},
		{name: "title control", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page\x00", Body: "Body."}, want: memory.ErrInvalidSchema},
		{name: "title heading", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "# Page", Body: "Body."}, want: memory.ErrInvalidSchema},
		{name: "title merge marker", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "<<<<<<< ours", Body: "Body."}, want: memory.ErrInvalidSchema},
		{name: "title anchor", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "<!-- @id: page -->", Body: "Body."}, want: memory.ErrInvalidSchema},
		{name: "title invalid UTF-8", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: string([]byte{0xff}), Body: "Body."}, want: memory.ErrInvalidSchema},
		{name: "body control", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page", Body: "Body\x00"}, want: memory.ErrInvalidSchema},
		{name: "body invalid UTF-8", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page", Body: string([]byte{0xff})}, want: memory.ErrInvalidSchema},
		{name: "ATX heading", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page", Body: "## Body"}, want: memory.ErrInvalidSchema},
		{name: "setext heading", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page", Body: "Body\n===="}, want: memory.ErrInvalidSchema},
		{name: "anchor", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page", Body: "<!-- @id: body -->"}, want: memory.ErrInvalidSchema},
		{name: "merge marker", request: memory.CreatePageRequest{TargetPath: "memories/page.md", Title: "Page", Body: "<<<<<<< ours"}, want: memory.ErrInvalidSchema},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newEmptyMemoryRoot(t)
			repository := openRepository(t, root)
			before := fingerprint(t, root)
			_, err := repository.PreviewCreatePage(context.Background(), test.request)
			if !errors.Is(err, test.want) {
				t.Fatalf("PreviewCreatePage() error = %v, want %v", err, test.want)
			}
			if got := fingerprint(t, root); got != before {
				t.Fatal("rejected preview modified the memory root")
			}
		})
	}
}

func TestPreviewCreatePageIsDeterministicAcrossRepositories(t *testing.T) {
	request := memory.CreatePageRequest{TargetPath: "memories/stable.md", Title: "Stable", Body: "Stable body."}
	firstRoot := newEmptyMemoryRoot(t)
	secondRoot := newEmptyMemoryRoot(t)
	first, err := openRepository(t, firstRoot).PreviewCreatePage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "preview.json")
	command := exec.Command(os.Args[0], "-test.run=^TestPreviewCreatePageHelperProcess$")
	command.Env = append(os.Environ(), "MEMORY_PREVIEW_HELPER=1", "MEMORY_PREVIEW_ROOT="+secondRoot, "MEMORY_PREVIEW_RESULT="+resultPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("helper process: %v: %s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var second memory.CreatePagePreview
	if err := json.Unmarshal(encoded, &second); err != nil {
		t.Fatal(err)
	}
	if first.PageID != second.PageID || first.PostStateHash != second.PostStateHash || first.Diff != second.Diff || string(first.PostState) != string(second.PostState) {
		t.Fatalf("previews differ\nfirst: %+v\nsecond: %+v", first, second)
	}
}

func TestPreviewCreatePageHelperProcess(t *testing.T) {
	if os.Getenv("MEMORY_PREVIEW_HELPER") != "1" {
		return
	}
	repository, err := memory.New(os.Getenv("MEMORY_PREVIEW_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	preview, err := repository.PreviewCreatePage(context.Background(), memory.CreatePageRequest{
		TargetPath: "memories/stable.md", Title: "Stable", Body: "Stable body.",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("MEMORY_PREVIEW_RESULT"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewCreateSectionAppendsCanonicalH2WithoutWriting(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	before := fingerprint(t, root)
	preview, err := repository.PreviewCreateSection(context.Background(), memory.CreateSectionRequest{
		TargetPath: "memories/checkout-api.md", Heading: "Failure Modes", Body: "Circuit breaker opens after five failures.",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSuffix := "\n## Failure Modes\n<!-- @id: checkout-api-failure-modes -->\nCircuit breaker opens after five failures.\n"
	if preview.Target != (memory.Target{Path: "memories/checkout-api.md", SectionID: "checkout-api-failure-modes"}) {
		t.Fatalf("Target = %+v", preview.Target)
	}
	if string(preview.Baseline) != validPage || string(preview.PostState) != validPage+wantSuffix {
		t.Fatalf("preview changed baseline unexpectedly\nbaseline:\n%s\npost-state:\n%s", preview.Baseline, preview.PostState)
	}
	if !strings.Contains(preview.Diff, "+## Failure Modes\n+<!-- @id: checkout-api-failure-modes -->") {
		t.Fatalf("Diff = %q", preview.Diff)
	}
	if got := fingerprint(t, root); got != before {
		t.Fatal("PreviewCreateSection() modified memory")
	}
}

func TestPreviewCreateSectionRejectsCollisionInvalidContentAndDirtyTarget(t *testing.T) {
	for _, test := range []struct {
		name    string
		heading string
		body    string
		want    error
	}{
		{name: "existing heading", heading: "Operation", body: "Body.", want: memory.ErrSectionExists},
		{name: "generated ID collision", heading: "Operation!", body: "Body.", want: memory.ErrSectionExists},
		{name: "empty slug", heading: "---", body: "Body.", want: memory.ErrInvalidID},
		{name: "generated ID over limit", heading: strings.Repeat("a", 60), body: "Body.", want: memory.ErrInvalidID},
		{name: "body heading", heading: "New", body: "## Nested", want: memory.ErrInvalidSchema},
		{name: "body anchor", heading: "New", body: "<!-- @id: new -->", want: memory.ErrInvalidSchema},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newMemoryRoot(t, validPage)
			repository := openRepository(t, root)
			before := fingerprint(t, root)
			_, err := repository.PreviewCreateSection(context.Background(), memory.CreateSectionRequest{
				TargetPath: "memories/checkout-api.md", Heading: test.heading, Body: test.body,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("PreviewCreateSection() error = %v, want %v", err, test.want)
			}
			if got := fingerprint(t, root); got != before {
				t.Fatal("rejected section preview modified memory")
			}
		})
	}

	for _, staged := range []bool{false, true} {
		t.Run(fmt.Sprintf("dirty staged=%v", staged), func(t *testing.T) {
			root := newMemoryRoot(t, validPage)
			pagePath := filepath.Join(root, "memories", "checkout-api.md")
			if err := os.WriteFile(pagePath, []byte(validPage+"dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if staged {
				runGit(t, root, "add", "memories/checkout-api.md")
			}
			repository := openRepository(t, root)
			_, err := repository.PreviewCreateSection(context.Background(), memory.CreateSectionRequest{
				TargetPath: "memories/checkout-api.md", Heading: "New", Body: "Body.",
			})
			if !errors.Is(err, memory.ErrTargetDirty) {
				t.Fatalf("PreviewCreateSection() error = %v, want ErrTargetDirty", err)
			}
		})
	}

	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	if _, err := repository.PreviewCreateSection(context.Background(), memory.CreateSectionRequest{TargetPath: "missing.md", Heading: "New", Body: "Body."}); err == nil {
		t.Fatal("PreviewCreateSection() accepted missing page")
	}
}

func TestApplyReplacesOnlyLeafBodyAndReturnsReceipt(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	target := memory.Target{Path: snapshot.Path, SectionID: "checkout-api-operation"}
	receipt, err := repository.Apply(context.Background(), memory.ChangeSet{
		Target:       target,
		BaselineHash: snapshot.FileHash,
		Replacement:  "Timeout: 3 seconds.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Target != target || receipt.BaselineHash != snapshot.FileHash || receipt.PostStateHash == snapshot.FileHash {
		t.Fatalf("receipt = %+v", receipt)
	}
	got, err := os.ReadFile(filepath.Join(root, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Checkout API\n<!-- @id: checkout-api -->\nOverview.\n\n## Operation\n<!-- @id: checkout-api-operation -->\nTimeout: 3 seconds.\n"
	if string(got) != want {
		t.Fatalf("applied file mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyReportsDriftBeforeParsingChangedBytes(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(root, "memories", "checkout-api.md")
	changed := append([]byte{0xff}, []byte(validPage[1:])...)
	if err := os.WriteFile(filePath, changed, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: snapshot.Path, SectionID: "checkout-api-operation"},
		BaselineHash: snapshot.FileHash,
		Replacement:  "new body\n",
	})
	if !errors.Is(err, memory.ErrDrift) {
		t.Fatalf("Apply() error = %v, want ErrDrift", err)
	}
	got, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(changed) {
		t.Fatal("Apply() changed the drifted file")
	}
}

func TestRepositoryPinsMemoryRootAcrossPathReplacement(t *testing.T) {
	root := newMemoryRoot(t, validPage)
	repository := openRepository(t, root)
	movedRoot := root + "-moved"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(movedRoot) })
	if err := os.MkdirAll(filepath.Join(root, "memories"), 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := strings.Replace(validPage, "Timeout: 5 seconds.", "Decoy root.", 1)
	if err := os.WriteFile(filepath.Join(root, "memories", "checkout-api.md"), []byte(decoy), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := repository.Inspect(context.Background(), memory.Scope{Path: "memories/checkout-api.md"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Sections[1].Body != "Timeout: 5 seconds.\n" {
		t.Fatalf("Inspect() followed replacement root: %q", snapshot.Sections[1].Body)
	}
	_, err = repository.Apply(context.Background(), memory.ChangeSet{
		Target:       memory.Target{Path: snapshot.Path, SectionID: "checkout-api-operation"},
		BaselineHash: snapshot.FileHash,
		Replacement:  "Pinned root.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	decoyAfter, err := os.ReadFile(filepath.Join(root, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoyAfter) != decoy {
		t.Fatal("Apply() wrote through replacement root path")
	}
	pinnedAfter, err := os.ReadFile(filepath.Join(movedRoot, "memories", "checkout-api.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pinnedAfter), "Pinned root.\n") {
		t.Fatal("Apply() did not write through pinned root descriptor")
	}
}

func newMemoryRoot(t *testing.T, page string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "memories"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memories", "checkout-api.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "memories/checkout-api.md")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "initial")
	return root
}

func newEmptyMemoryRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "-m", "initial")
	return root
}

func openRepository(t *testing.T, root string) *memory.Repository {
	t.Helper()
	repository, err := memory.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})
	return repository
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}
