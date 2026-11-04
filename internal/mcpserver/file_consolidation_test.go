package mcpserver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/consolidation"
)

const teamPath = "memories/domains/time-camada-de-leitura.md"
const teamBaseline = "# Wuerike é do time da camada de leitura\n<!-- @id: time-camada-de-leitura -->\n\n## Overview\n<!-- @id: time-camada-de-leitura-overview -->\nO usuário se chama Wuerike e faz parte do time da camada de leitura.\n"
const teamCurrent = "# Time da camada de leitura\n\n## Membros\n- Wuerike\n- Guilherme\n"

func filePromotionRoundTrip(t *testing.T, baseline map[string]string, proposals []string, path, body, query string, wanted, absent []string) {
	t.Helper()
	h := newHarness(t, baseline)
	session := h.connect(t, "file-promotion")
	for _, content := range proposals {
		e2eCall(t, session, "memory_propose", map[string]any{"title": "Consolidation input", "content": content})
	}
	p := decodePromotion[consolidation.Prepared](t, e2eCall(t, session, "memory_promote_prepare", map[string]any{}))
	if len(p.Proposals) != len(proposals) {
		t.Fatalf("prepared %d proposals, want %d", len(p.Proposals), len(proposals))
	}
	writePreparedFile(t, p, path, body)
	checked := decodePromotion[consolidation.Checked](t, e2eCall(t, session, "memory_promote_check", map[string]any{}))
	if len(checked.Paths) != 1 || checked.Paths[0] != path || !strings.Contains(checked.Diff, path) {
		t.Fatalf("check = %+v", checked)
	}
	summary := "The reviewed proposal set consolidates into " + path + "."
	for _, proposal := range p.Proposals {
		summary += "\n" + proposal.Path + " contributes to the final page."
	}
	e2eCall(t, session, "memory_promote_submit", map[string]any{"summary": summary})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	squashPublished(t, h.m, p.Branch)
	completion := h.connect(t, "finish-published-pr")
	finished := decodePromotion[consolidation.Finished](t, e2eCall(t, completion, "memory_promote_finish", map[string]any{}))
	if !finished.Merged || len(finished.Removed) != len(proposals) {
		t.Fatalf("finish = %+v", finished)
	}
	if err := completion.Close(); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(p.Worktree, path))
	if err != nil {
		t.Fatal(err)
	}
	published, err := h.m.Config.Workspace().Repo().Bytes(context.Background(), "show", "origin/main:"+path)
	if err != nil || string(published) != string(current) {
		t.Fatalf("published page differs from reviewed file: %q, %v", published, err)
	}
	fresh := h.connect(t, "fresh-published-reader")
	defer fresh.Close()
	response := decodeResponse(t, e2eCall(t, fresh, "memory_search", map[string]any{"query": query, "limit": 20}))
	if response.Data == nil || len(response.Data.Items) == 0 {
		t.Fatalf("fresh search = %+v", response)
	}
	var found strings.Builder
	for _, item := range response.Data.Items {
		if item.Path != path {
			t.Fatalf("unexpected retrieved path %s", item.Path)
		}
		found.WriteString(item.Content)
	}
	for _, value := range wanted {
		if !strings.Contains(found.String(), value) {
			t.Fatalf("fresh content lacks %q: %s", value, found.String())
		}
	}
	for _, value := range absent {
		if strings.Contains(found.String(), value) {
			t.Fatalf("fresh content retains %q: %s", value, found.String())
		}
	}
}

func TestClaudeAdapterPromotesCreateProposalForFreshProcessRetrieval(t *testing.T) {
	filePromotionRoundTrip(t, nil, []string{"# Checkout policy\nCreate a page: timeout is nine seconds and owner is team-blue.\n"},
		"memories/domains/checkout-policy.md", "# Checkout policy\n\n## Policy\nCheckout timeout is nine seconds. Owner is team-blue.\n", "checkout timeout nine team-blue", []string{"nine seconds", "team-blue"}, []string{"Create a page"})
}

func TestClaudeAdapterPromotesUpdateForFreshProcessRetrieval(t *testing.T) {
	filePromotionRoundTrip(t, map[string]string{checkoutPath: memoryPage}, []string{"# Checkout timeout\nUpdate timeout to nove segundos.\n"},
		checkoutPath, strings.ReplaceAll(memoryPage, "tres segundos", "nove segundos"), "checkout timeout nove", []string{"nove segundos", "team-b"}, []string{"tres segundos"})
}

func TestE2EReadingTeamPageIsRestructuredThroughFiles(t *testing.T) {
	filePromotionRoundTrip(t, map[string]string{teamPath: teamBaseline}, []string{"# Guilherme\nGuilherme também faz parte do time da camada de leitura. Organizar a memória por time.\n"},
		teamPath, teamCurrent, "membros camada leitura", []string{"Wuerike", "Guilherme"}, []string{"Overview"})
}

func TestE2ENewTeamPageIsCreatedFromTwoProposalsInOneRun(t *testing.T) {
	filePromotionRoundTrip(t, nil, []string{"# Wuerike\nWuerike faz parte do time da camada de leitura.\n", "# Guilherme\nGuilherme faz parte do time da camada de leitura.\n"},
		teamPath, teamCurrent, "membros camada leitura", []string{"Wuerike", "Guilherme"}, []string{"Overview"})
}
