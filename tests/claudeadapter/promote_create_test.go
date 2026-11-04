package claudeadapter_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func promoteSkill(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "integrations", "claude-code", "skills", "promote", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func promoteSection(t *testing.T, title string) string {
	t.Helper()
	text := promoteSkill(t)
	marker := "## " + title + "\n"
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatalf("missing contract section %q", title)
	}
	rest := text[start+len(marker):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		return rest[:end]
	}
	return rest
}
func requirePromoteClauses(t *testing.T, text string, clauses ...string) {
	t.Helper()
	for _, clause := range clauses {
		if !strings.Contains(text, clause) {
			t.Errorf("missing policy clause %q", clause)
		}
	}
}

func TestPromoteContractRequiresFullMemoryRead(t *testing.T) {
	section := promoteSection(t, "2. Read the entire set")
	requirePromoteClauses(t, section, "Read every proposal at its `input_path`", "every memory in `memory_paths` in full", "Search is only a navigation aid")
	text := promoteSkill(t)
	if strings.Index(text, "## 2. Read the entire set") > strings.Index(text, "## 3. Discuss the global plan") {
		t.Fatal("reading must precede planning")
	}
}

func TestPromoteContractPresentsGlobalGroups(t *testing.T) {
	section := promoteSection(t, "3. Discuss the global plan")
	requirePromoteClauses(t, section, "topic groups", "proposal IDs and titles", "destination pages", "suggested changes", "Several proposals can contribute to one memory", "one proposal can contribute to several memories")
}

func TestPromoteContractAccountsForEveryProposal(t *testing.T) {
	section := promoteSection(t, "3. Discuss the global plan")
	requirePromoteClauses(t, section, "Account for every initial proposal", "one or more destinations", "free-form justification", "Suggested actions and destinations are hints")
}

func TestPromoteContractWaitsForPlanApproval(t *testing.T) {
	section := promoteSection(t, "3. Discuss the global plan")
	requirePromoteClauses(t, section, "Wait for explicit approval of the global plan before editing any canonical file")
	text := promoteSkill(t)
	if strings.Index(text, "Wait for explicit approval of the global plan before editing any canonical file") > strings.Index(text, "## 4. Write final memories") {
		t.Fatal("plan approval must precede writing")
	}
}

func TestPromoteContractRevisesPlanInConversation(t *testing.T) {
	section := promoteSection(t, "3. Discuss the global plan")
	requirePromoteClauses(t, section, "merge or split groups", "change destinations", "resolve contradictions", "Update the global plan in conversation", "do not create a separate plan file")
}

func TestPromoteContractLimitsReorganization(t *testing.T) {
	section := promoteSection(t, "3. Discuss the global plan")
	requirePromoteClauses(t, section, "Reorganize only memories related to this proposal set", "preserve knowledge that remains valid", "Leave unrelated improvements outside this consolidation")
}

func TestPromoteContractWritesCanonicalKnowledge(t *testing.T) {
	section := promoteSection(t, "4. Write final memories")
	requirePromoteClauses(t, section, "Write canonical knowledge in complete final pages", "Omit proposal instructions about action and destination", "Preserve existing @id markers on retained sections", "new headings without markers")
}

func TestPromoteContractDelegatesByMemory(t *testing.T) {
	section := promoteSection(t, "4. Write final memories")
	requirePromoteClauses(t, section, "After plan approval", "delegate independent memory files", "one owner per file in each round", "current page, all relevant proposals, approved decisions and cross-page context", "Only the principal performs Git, proposal cleanup and publication", "Wait for every delegate to finish")
}

func TestPromoteContractReviewsWholeResult(t *testing.T) {
	section := promoteSection(t, "5. Review the whole result")
	requirePromoteClauses(t, section, "The principal rereads every prepared page", "duplicates, contradictions, references and omissions", "across the resulting pages", "memory_promote_check")
}

func TestPromoteContractReviewsCurrentFilesAndDiff(t *testing.T) {
	section := promoteSection(t, "6. Review with the person")
	requirePromoteClauses(t, section, "global summary", "complete diff for each changed memory", "worktree path", "optional VS Code editing", "do not open the editor automatically")
}

func TestPromoteContractPausesForHumanEditing(t *testing.T) {
	section := promoteSection(t, "6. Review with the person")
	requirePromoteClauses(t, section, "Finish all agent writing before human review", "Stop writing while the person is editing", "Do not run check during human editing", "asks for adjustments or says editing is finished in conversation", "reread the current files", "preserve human edits")
}

func TestPromoteContractDiscussesMaterialPlanChanges(t *testing.T) {
	section := promoteSection(t, "5. Review the whole result")
	requirePromoteClauses(t, section, "If content materially changes the approved plan", "return to discussion", "obtain the person's decision before writing that change")
}

func TestPromoteContractRequiresApprovalOfCurrentResult(t *testing.T) {
	section := promoteSection(t, "7. Publish")
	requirePromoteClauses(t, section, "Ask for explicit approval to publish the current files and diff", "Before submitting, reread the current changed files", "If anything changed, return to human review and obtain a new approval", "After explicit approval, call `memory_promote_submit`", "every initial proposal", "Do not edit files after publication approval")
	submit := strings.Index(section, "call `memory_promote_submit`")
	if strings.Index(section, "Ask for explicit approval") > submit || strings.Index(section, "Before submitting, reread") > submit {
		t.Fatal("submit must follow approval and current-file reread")
	}
}

func TestPromoteContractStopsOnIncompleteContext(t *testing.T) {
	section := promoteSection(t, "2. Read the entire set")
	requirePromoteClauses(t, section, "If the entire set cannot be read within the available context, say so and stop", "Never present a partial analysis as complete")
}

func TestPromoteSkillGrantsFileEditingAndFourPromotionTools(t *testing.T) {
	text := promoteSkill(t)
	end := strings.Index(text[4:], "\n---\n")
	if !strings.HasPrefix(text, "---\n") || end < 0 {
		t.Fatal("invalid frontmatter")
	}
	frontmatter := text[:end+4]
	var granted []string
	for _, line := range strings.Split(frontmatter, "\n") {
		if rest, ok := strings.CutPrefix(line, "allowed-tools: "); ok {
			for _, tool := range strings.Split(rest, ",") {
				granted = append(granted, strings.TrimSpace(tool))
			}
		}
	}
	want := []string{"Read", "Write", "Edit", "Glob", "Grep", "Agent", "mcp__git-push-memories__memory_promote_prepare", "mcp__git-push-memories__memory_promote_check", "mcp__git-push-memories__memory_promote_submit", "mcp__git-push-memories__memory_promote_finish"}
	sort.Strings(granted)
	sort.Strings(want)
	if strings.Join(granted, "|") != strings.Join(want, "|") {
		t.Fatalf("grants = %v, want %v", granted, want)
	}
	requirePromoteClauses(t, frontmatter, "disable-model-invocation: true")
	for _, obsolete := range []string{"memory_promote_open", "memory_promote_status", "memory_promote_preview", "memory_promote_apply", "memory_promote_batch_preview", "memory_promote_batch_apply", "fold_proposal", "preview_token", "release.lock.json"} {
		if strings.Contains(text, obsolete) {
			t.Errorf("retired contract remains: %s", obsolete)
		}
	}
}
