package e2e_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/proposal"
	"github.com/Wuerike/git-push-memories/internal/protocol"
)

func TestBootstrapWorkflow(t *testing.T) {
	evidenceRoot := os.Getenv("GPM_BOOTSTRAP_E2E_EVIDENCE_ROOT")
	if evidenceRoot == "" {
		t.Skip("run tests/e2e/run-phase-2-bootstrap.sh to execute the bootstrap E2E")
	}
	h := newHarness(t, evidenceRoot)
	runRoot := t.TempDir()
	memoryRoot := filepath.Join(runRoot, "memory")
	codeRoot := filepath.Join(runRoot, "code")
	stateRoot := filepath.Join(runRoot, "state-a")
	home := filepath.Join(runRoot, "home-a")
	configRoot := filepath.Join(runRoot, "config-a")
	h.seedRepository(memoryRoot, nil)
	h.git(memoryRoot, "commit", "--quiet", "--allow-empty", "-m", "empty memory")
	h.seedRepository(codeRoot, map[string][]byte{"services/parcel/service.yaml": []byte("retry_limit: 3\nowner: fulfillment\n")})
	for _, root := range []string{home, configRoot} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if paths := markdownPaths(t, memoryRoot); len(paths) != 0 {
		t.Fatalf("initial memory contains Markdown: %v", paths)
	}
	initialHead := strings.TrimSpace(h.gitOutput(memoryRoot, "rev-parse", "HEAD"))
	h.writeFile("bootstrap-inventory-before.txt", []byte(h.inventory(runRoot)))

	environment := isolatedEnvironment(home, configRoot)
	session := h.connectMCP("bootstrap MCP", memoryRoot, codeRoot, stateRoot, environment, "bootstrap-client")
	common := []string{"--memory-root", memoryRoot, "--code-root", codeRoot, "--state-root", stateRoot}
	commits := make([]string, 0, 3)
	proposeApplyPublish := func(tool string, arguments map[string]any, wantDiff string) proposal.PublicationReceipt {
		t.Helper()
		result := h.callTool(session, tool, tool, arguments)
		var response struct {
			Data struct {
				ID        string `json:"id"`
				Revision  int    `json:"revision"`
				Operation string `json:"operation"`
			} `json:"data"`
		}
		decodeStructured(t, result, &response)
		if result.IsError || response.Data.ID == "" || response.Data.Revision != 1 {
			t.Fatalf("%s did not create a proposal", tool)
		}
		show := h.cli(environment, tool+" show", append([]string{"proposal", "show", response.Data.ID}, common...)...)
		var review protocol.Envelope[proposal.Review]
		decodeJSON(t, show, &review)
		if review.Data.Proposal == nil || !strings.Contains(review.Data.Proposal.Diff, wantDiff) {
			t.Fatalf("%s diff = %+v", tool, review.Data.Proposal)
		}
		apply := h.cli(environment, tool+" apply", append([]string{"proposal", "apply", response.Data.ID, "--revision", "1"}, common...)...)
		var applied protocol.Envelope[proposal.ApplyReceipt]
		decodeJSON(t, apply, &applied)
		publish := h.cli(environment, tool+" publish", append([]string{"proposal", "publish", response.Data.ID}, common...)...)
		var published protocol.Envelope[proposal.PublicationReceipt]
		decodeJSON(t, publish, &published)
		if published.Data.Commit == "" || published.Data.Parent != applied.Data.Baseline.Head {
			t.Fatalf("%s publication = %+v", tool, published.Data)
		}
		if paths := fields(h.gitOutput(memoryRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", published.Data.Commit)); !reflect.DeepEqual(paths, []string{"parcel-dispatch.md"}) {
			t.Fatalf("%s commit paths = %v", tool, paths)
		}
		commits = append(commits, published.Data.Commit)
		return published.Data
	}

	proposeApplyPublish("memory_propose_create_page", map[string]any{
		"target_path": "parcel-dispatch.md", "title": "Parcel Dispatch", "body": "Retries use exponential backoff.",
		"source_paths": []string{"services/parcel/service.yaml"},
	}, "--- /dev/null")
	proposeApplyPublish("memory_propose_create_section", map[string]any{
		"target_path": "parcel-dispatch.md", "heading": "Failure Modes", "body": "Circuit breaker opens after five failures.",
		"source_paths": []string{"services/parcel/service.yaml"},
	}, "+## Failure Modes")
	proposeApplyPublish("memory_propose_replace", map[string]any{
		"target_path": "parcel-dispatch.md", "section_id": "parcel-dispatch-failure-modes", "replacement": "Circuit breaker opens after three failures.\n",
		"source_paths": []string{"services/parcel/service.yaml"},
	}, "+Circuit breaker opens after three failures.")
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	final := readFile(t, filepath.Join(memoryRoot, "parcel-dispatch.md"))
	if !strings.Contains(string(final), "<!-- @id: parcel-dispatch-overview -->") || !strings.Contains(string(final), "<!-- @id: parcel-dispatch-failure-modes -->") ||
		!strings.Contains(string(final), "after three failures") || strings.Contains(string(final), "after five failures") {
		t.Fatalf("final memory = %s", final)
	}
	if initialHead == strings.TrimSpace(h.gitOutput(memoryRoot, "rev-parse", "HEAD")) || len(commits) != 3 {
		t.Fatal("bootstrap did not publish three revisions")
	}

	stateB := filepath.Join(runRoot, "state-b")
	codeB := filepath.Join(runRoot, "code-b")
	h.seedRepository(codeB, nil)
	h.git(codeB, "commit", "--quiet", "--allow-empty", "-m", "empty code")
	sessionB := h.connectMCP("bootstrap MCP fresh process", memoryRoot, codeB, stateB, isolatedEnvironment(filepath.Join(runRoot, "home-b"), filepath.Join(runRoot, "config-b")), "bootstrap-client-b")
	bundle, bytes := h.callBundle(sessionB, "bootstrap final search", "memory_search", map[string]any{"query": "circuit breaker failures", "budget_bytes": 2048})
	if bytes > 2048 || len(bundle.Items) == 0 || bundle.Items[0].Handle.SectionID != "parcel-dispatch-failure-modes" || !strings.Contains(bundle.Items[0].Content, "three failures") {
		t.Fatalf("fresh retrieval = %+v, bytes=%d", bundle, bytes)
	}
	if err := sessionB.Close(); err != nil {
		t.Fatal(err)
	}
	if !missing(stateB) {
		t.Fatal("fresh retrieval created state")
	}

	privacyHits, privacyWrites := h.runPrivacy()
	sourceHashes := map[string]string{}
	for _, source := range []string{"tests/e2e/bootstrap_test.go", "tests/e2e/e2e_test.go", "tests/e2e/run-phase-2-bootstrap.sh"} {
		sourceHashes[source] = hashFile(t, filepath.Join(h.repository, filepath.FromSlash(source)))
	}
	h.writeJSON("bootstrap-manifest.json", map[string]any{
		"schema_version": 1, "initial_markdown_files": 0, "operations": []string{"create_page", "create_section", "replace_section_content"},
		"publication_commits": commits, "final_file_hash": hashBytes(final), "fresh_context_bytes": bytes,
		"privacy_marker_hits": privacyHits, "privacy_write_targets": privacyWrites, "commands": h.records,
		"product_revision": strings.TrimSpace(h.commandOutput(h.repository, os.Environ(), "git", "rev-parse", "HEAD")),
		"product_worktree": worktreeState(h.repository), "evaluated_source_hashes": sourceHashes, "binary_hash": hashFile(t, h.binary),
	})
	h.writeFile("bootstrap-final.md", final)
	h.writeFile("bootstrap-inventory-after.txt", []byte(h.inventory(runRoot)))
}

func markdownPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.IsDir() && filepath.Ext(path) == ".md" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
