package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
	wt "github.com/Wuerike/git-push-memories/internal/workspace/workspacetest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func decodePromotion[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
		Data          *T  `json:"data"`
	}
	decodeStructured(t, result, &envelope)
	if envelope.SchemaVersion != 1 || envelope.Data == nil {
		t.Fatalf("invalid promotion envelope: %+v", envelope)
	}
	return *envelope.Data
}

func writePreparedFile(t *testing.T, p consolidation.Prepared, path, body string) {
	t.Helper()
	full := filepath.Join(p.Worktree, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func squashPublished(t *testing.T, m *wt.Managed, branch string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "merge")
	wt.Git(t, filepath.Dir(clone), "clone", "-q", "-b", "main", m.Remote, clone)
	wt.Git(t, clone, "merge", "--squash", "origin/"+branch)
	wt.Git(t, clone, "commit", "-q", "-m", "Reviewed consolidation")
	wt.Git(t, clone, "push", "-q", "origin", "main")
	merged := wt.Git(t, clone, "rev-parse", "HEAD")
	wt.FakeGH(t).SetPR(t, "MERGED", merged)
	return merged
}

func TestPromotionToolContractUsesFiles(t *testing.T) {
	e := newEnv(t, nil)
	listed, err := e.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if strings.HasPrefix(tool.Name, "memory_promote_") {
			data, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			for _, obsolete := range []string{"proposal_id", "operation", "preview_token", "fold_into", "items"} {
				if strings.Contains(string(data), `"`+obsolete+`"`) {
					t.Fatalf("old input %s in %s", obsolete, tool.Name)
				}
			}
		}
	}
	sort.Strings(names)
	want := []string{"memory_expand", "memory_promote_check", "memory_promote_finish", "memory_promote_prepare", "memory_promote_submit", "memory_proposal_list", "memory_propose", "memory_search"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
}

func TestConsolidationSupportsManyToManyMapping(t *testing.T) {
	e := newEnv(t, nil)
	for _, title := range []string{"Alice owns checkout", "Bob joins team", "Ownership policy"} {
		e.propose(t, map[string]any{"title": title, "content": "# " + title + "\n\nCreate or update the relevant memory.\n"})
	}
	p := decodePromotion[consolidation.Prepared](t, e.ok(t, "memory_promote_prepare", map[string]any{}))
	if len(p.Proposals) != 3 {
		t.Fatalf("proposal count = %d", len(p.Proposals))
	}
	const team = "memories/domains/team.md"
	const owners = "memories/domains/owners.md"
	writePreparedFile(t, p, team, "# Team\n\n## Members\n- Alice\n- Bob\n")
	writePreparedFile(t, p, owners, "# Owners\n\n## Checkout\nAlice owns checkout. The team reviews ownership changes.\n")
	checked := decodePromotion[consolidation.Checked](t, e.ok(t, "memory_promote_check", map[string]any{}))
	if !reflect.DeepEqual(checked.Paths, []string{owners, team}) {
		t.Fatalf("changed paths = %v", checked.Paths)
	}
	summary := p.Proposals[0].ID + " contributes to " + team + " and " + owners + "; " + p.Proposals[1].ID + " also contributes to " + team + "; " + p.Proposals[2].ID + " also contributes to " + owners + "."
	submitted := decodePromotion[consolidation.Submitted](t, e.ok(t, "memory_promote_submit", map[string]any{"summary": summary}))
	actual := strings.Fields(wt.Git(t, e.m.Remote, "diff-tree", "--no-commit-id", "--name-only", "-r", submitted.Commit))
	if !reflect.DeepEqual(actual, []string{owners, team}) {
		t.Fatalf("commit paths = %v", actual)
	}
	data, err := os.ReadFile(e.m.Config.Paths.Published)
	if err != nil {
		t.Fatal(err)
	}
	var record consolidation.Published
	if err := json.Unmarshal(data, &record); err != nil || len(record.Proposals) != 3 {
		t.Fatalf("record = %s, %v", data, err)
	}
}

func TestConsolidationPreservesRememberAndPublishedReads(t *testing.T) {
	e := newEnv(t, map[string]string{checkoutPath: memoryPage})
	publishExecutable(t, e.m, checkoutPath)
	proposal := e.propose(t, map[string]any{"title": "Checkout timeout", "content": "# Checkout timeout\nUpdate the timeout to nine seconds.\n"})
	p := decodePromotion[consolidation.Prepared](t, e.ok(t, "memory_promote_prepare", map[string]any{}))
	if !reflect.DeepEqual(p.MemoryPaths, []string{checkoutPath}) {
		t.Fatalf("inventory omitted a readable executable Markdown page: %v", p.MemoryPaths)
	}
	current := strings.ReplaceAll(memoryPage, "tres segundos", "nove segundos")
	writePreparedFile(t, p, checkoutPath, current)
	e.ok(t, "memory_promote_check", map[string]any{})
	before := e.search(t, "", "checkout timeout")
	if len(before.Items) != 1 || !strings.Contains(before.Items[0].Content, "tres segundos") {
		t.Fatalf("unpublished read = %+v", before)
	}
	e.ok(t, "memory_promote_submit", map[string]any{"summary": proposal.ID + " updates " + checkoutPath})
	open := decodePromotion[consolidation.Finished](t, e.ok(t, "memory_promote_finish", map[string]any{}))
	if !open.Found || open.Merged || len(open.Removed) != 0 {
		t.Fatalf("open PR = %+v", open)
	}
	if !strings.Contains(wt.Git(t, e.m.Remote, "ls-tree", "-r", "--name-only", "proposals"), proposal.Path) {
		t.Fatal("proposal removed before merge")
	}
	merged := squashPublished(t, e.m, p.Branch)
	finished := decodePromotion[consolidation.Finished](t, e.ok(t, "memory_promote_finish", map[string]any{}))
	if !finished.Merged || !reflect.DeepEqual(finished.Removed, []string{proposal.Path}) {
		t.Fatalf("finish = %+v", finished)
	}
	after := e.search(t, "", "checkout timeout")
	if len(after.Items) != 1 || !strings.Contains(after.Items[0].Content, "nove segundos") || strings.Contains(after.Items[0].Content, "tres segundos") {
		t.Fatalf("published read = %+v", after)
	}
	later := e.propose(t, map[string]any{"title": "Later knowledge", "content": "# Later\nA fresh proposal after merge.\n"})
	if later.Commit == "" || wt.Git(t, e.m.Remote, "rev-parse", "main") != merged {
		t.Fatal("remember stopped working or changed main")
	}
}

func TestPromotionRequiresPreparedFilesAndReportsLimits(t *testing.T) {
	e := newEnv(t, nil)
	if result := e.call(t, "memory_promote_check", map[string]any{}); !result.IsError || errorCode(t, result) != "promotion_not_prepared" {
		t.Fatalf("unprepared check = %+v", result)
	}
	if result := e.call(t, "memory_promote_prepare", map[string]any{}); !result.IsError || errorCode(t, result) != "no_proposals" {
		t.Fatalf("empty preparation = %+v", result)
	}
	e.propose(t, map[string]any{"title": "Fact", "content": "# Fact\nA new fact.\n"})
	p := decodePromotion[consolidation.Prepared](t, e.ok(t, "memory_promote_prepare", map[string]any{}))
	if result := e.call(t, "memory_promote_submit", map[string]any{"summary": "No files yet."}); !result.IsError || errorCode(t, result) != "no_changes" {
		t.Fatalf("empty publication = %+v", result)
	}
	writePreparedFile(t, p, "memories/domains/fact.md", "# One\n# Two\n")
	if result := e.call(t, "memory_promote_check", map[string]any{}); !result.IsError || errorCode(t, result) != "invalid_memory" {
		t.Fatalf("invalid document = %+v", result)
	}
}

func TestPrepareFinishesMergedPublicationBeforeNewSnapshot(t *testing.T) {
	e := newEnv(t, nil)
	first := e.propose(t, map[string]any{"title": "First", "content": "# First\nFirst fact.\n"})
	p := decodePromotion[consolidation.Prepared](t, e.ok(t, "memory_promote_prepare", map[string]any{}))
	writePreparedFile(t, p, "memories/domains/first.md", "# First\nFirst canonical fact.\n")
	e.ok(t, "memory_promote_check", map[string]any{})
	e.ok(t, "memory_promote_submit", map[string]any{"summary": first.ID + " creates first.md"})
	later := e.propose(t, map[string]any{"title": "Later", "content": "# Later\nLater fact.\n"})
	merged := squashPublished(t, e.m, p.Branch)
	next := decodePromotion[consolidation.Prepared](t, e.ok(t, "memory_promote_prepare", map[string]any{}))
	if next.Worktree == p.Worktree || next.MainSHA != merged || len(next.Proposals) != 1 || next.Proposals[0].ID != later.ID {
		t.Fatalf("next preparation = %+v", next)
	}
	if _, err := os.Stat(e.m.Config.Paths.Published); !os.IsNotExist(err) {
		t.Fatalf("cleanup record remains: %v", err)
	}
}
