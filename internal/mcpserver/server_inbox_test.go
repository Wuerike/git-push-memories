package mcpserver_test

import (
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type listedProposals struct {
	Data struct {
		Categories []string `json:"categories"`
		Proposals  []struct {
			Path string `json:"path"`
			Blob string `json:"blob"`
		} `json:"proposals"`
	} `json:"data"`
}

func TestMemorySearchAndReadConfiguredCategory(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})

	search := e.search(t, "checkout timeout team")
	if search == nil || len(search.Items) != 1 || search.Items[0].Path != checkoutPath {
		t.Fatalf("memory_search = %+v", search)
	}
	result := e.ok(t, "memory_read", map[string]any{"path": search.Items[0].Path, "revision": search.Revision})
	read := decodePromotion[retrieval.FileContent](t, result)
	if read.Path != checkoutPath || read.Revision != search.Revision || !strings.Contains(read.Content, "timeout: tres segundos") {
		t.Fatalf("memory_read = %+v", read)
	}
}

func TestMemoryReadFollowsPublishedMain(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	if search := e.search(t, "settlement retry failures"); len(search.Items) != 0 {
		t.Fatalf("search before publication = %+v", search)
	}
	e.m.PublishMain(t, map[string]string{"memories/domains/settlement.md": "# Settlement\n<!-- @id: settlement -->\n\n## Retry\n<!-- @id: settlement-retry -->\nSettlement retries stop after three failures.\n"})
	// The read worktree answers from the last refresh until the TTL expires.
	if search := e.search(t, "settlement retry failures"); len(search.Items) != 0 {
		t.Fatalf("search before refresh = %+v", search)
	}
	if err := os.Remove(filepath.Join(e.m.Config.Paths.State, "fetch-attempt")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	search := e.search(t, "settlement retry failures")
	for len(search.Items) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		search = e.search(t, "settlement retry failures")
	}
	if len(search.Items) != 1 || search.Items[0].Path != "memories/domains/settlement.md" {
		t.Fatalf("search after refresh = %+v", search)
	}
}

func TestReadToolsRejectPathsOutsideConfiguredMemory(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	for _, name := range []string{"../private.md", "proposals/fact.md", "memories/other/fact.md"} {
		result := e.call(t, "memory_read", map[string]any{"path": name})
		if !result.IsError {
			t.Fatalf("read accepted %s", name)
		}
	}
}

func TestMemoryProposeCommitsMarkdownToRemoteProposals(t *testing.T) {
	e := newEnv(t, nil)
	parent := gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals")

	result := e.ok(t, "memory_propose", map[string]any{
		"title":   "Card transactions from authorizer",
		"content": "Card transactions can originate from the authorizer adapter.",
	})
	response := decodeInboxResponse(t, result)
	if response.Data == nil || response.Data.ID == "" {
		t.Fatalf("memory_propose response = %+v", response)
	}
	remote := gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals")
	if response.Data.Commit != remote || gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals^") != parent {
		t.Fatalf("memory_propose commit = %q, remote = %q", response.Data.Commit, remote)
	}
	if changed := gitOutput(t, e.m.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", remote); changed != response.Data.Path {
		t.Fatalf("memory_propose changed %q, want %q", changed, response.Data.Path)
	}
	if main := gitOutput(t, e.m.Remote, "ls-tree", "-r", "--name-only", "main"); strings.Contains(main, "proposals/") {
		t.Fatalf("proposal reached main: %q", main)
	}
	if status := gitOutput(t, e.proposalsRoot(), "status", "--porcelain"); status != "" {
		t.Fatalf("memory_propose left the local worktree dirty: %s", status)
	}
}

func TestMemoryProposeReportsApiFailuresWithoutWriting(t *testing.T) {
	e := newEnv(t, nil)
	before := gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals")
	for failure, code := range map[string]string{"connection refused": "operation_failed", "Forbidden (HTTP 403)": "push_denied"} {
		t.Setenv("GPM_FAKE_API_FAIL", failure)
		result := e.call(t, "memory_propose", map[string]any{"title": "Fact", "content": "Knowledge."})
		response := decodeInboxResponse(t, result)
		if !result.IsError || response.Error == nil || response.Error.Code != code {
			t.Fatalf("propose with %q = %+v, want %s", failure, response, code)
		}
	}
	if gitOutput(t, e.m.Remote, "rev-parse", "refs/heads/proposals") != before {
		t.Fatal("a failed memory_propose changed the remote")
	}
}

func TestProposalListRefreshesFromRemote(t *testing.T) {
	e := newEnv(t, nil)
	e.propose(t, map[string]any{"title": "Fact", "content": "Knowledge."})
	var listed listedProposals
	decodeStructured(t, e.ok(t, "memory_proposal_list", map[string]any{}), &listed)
	if len(listed.Data.Proposals) != 1 {
		t.Fatalf("memory_proposal_list = %+v, want the proposal just pushed", listed.Data.Proposals)
	}
}

func TestProposalListIsReadOnly(t *testing.T) {
	const proposalID = "0123456789abcdef0123456789abcdef"
	e := newEnv(t, map[string]string{
		"proposals/" + proposalID + "-retry-policy.md": "Retries stop after three failures.",
	})
	head := commitHeads(t, e.proposalsRoot())

	result := e.ok(t, "memory_proposal_list", map[string]any{})
	var listed struct {
		Data struct {
			Proposals []struct {
				Path    string `json:"path"`
				Blob    string `json:"blob"`
				Content string `json:"content"`
			} `json:"proposals"`
		} `json:"data"`
	}
	decodeStructured(t, result, &listed)
	if len(listed.Data.Proposals) != 1 || listed.Data.Proposals[0].Path != "proposals/"+proposalID+"-retry-policy.md" || listed.Data.Proposals[0].Blob == "" || listed.Data.Proposals[0].Content != "Retries stop after three failures." {
		t.Fatalf("listed proposals = %+v", listed.Data.Proposals)
	}
	if current := commitHeads(t, e.proposalsRoot()); current != head {
		t.Fatalf("proposal list moved proposals HEAD from %s to %s", head, current)
	}
	if status := gitOutput(t, e.proposalsRoot(), "status", "--porcelain"); status != "" {
		t.Fatalf("proposal list dirtied the worktree: %q", status)
	}
}
