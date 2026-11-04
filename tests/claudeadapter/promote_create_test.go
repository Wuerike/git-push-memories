package claudeadapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func promoteSkill(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "integrations", "claude-code", "skills", "promote", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// These contracts check the instructions shipped to the agent; runtime tests
// exercise the Git operations. Instruction presence does not prove LLM judgment.
func TestPromoteSkillSupportsScopedAdaptiveInvestigation(t *testing.T) {
	text := promoteSkill(t)
	for _, instruction := range []string{"proposal_paths", "Read every selected proposal", "Read every page you will edit in full", "gpm.json", "Use only those categories", "Explain uncertainty and coverage", "ordinary Markdown"} {
		if !strings.Contains(text, instruction) {
			t.Errorf("missing instruction %q", instruction)
		}
	}
}

func TestPromoteSkillRequiresPlanAndCurrentDiffApproval(t *testing.T) {
	text := promoteSkill(t)
	for _, instruction := range []string{"Wait for explicit approval of the global plan", "Ask for explicit approval to publish the current files and diff", "If anything changed, return to human review", "preserve human edits", "Stop writing while the person is", "reviewed memory `paths`"} {
		if !strings.Contains(text, instruction) {
			t.Errorf("missing instruction %q", instruction)
		}
	}
}

func TestPromoteSkillGrantsFileAndPromotionTools(t *testing.T) {
	text := promoteSkill(t)
	frontmatter := strings.SplitN(text, "\n---\n", 2)[0]
	for _, tool := range []string{"Read", "Write", "Edit", "Glob", "Grep", "Agent", "memory_proposal_list", "memory_promote_prepare", "memory_promote_check", "memory_promote_submit", "memory_promote_finish"} {
		if !strings.Contains(frontmatter, tool) {
			t.Errorf("missing tool %s", tool)
		}
	}
	if !strings.Contains(frontmatter, "disable-model-invocation: true") {
		t.Fatal("promotion must be explicitly invoked")
	}
}
