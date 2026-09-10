package claudeadapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurateSkillGrantsOnlyCurationToolsAndConfirmsExactPreview(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(repository, "integrations", "claude-code", "skills", "curate", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	frontmatterEnd := strings.Index(text[4:], "\n---\n")
	if frontmatterEnd < 0 {
		t.Fatal("invalid skill frontmatter")
	}
	frontmatter := text[:frontmatterEnd+4]
	for _, tool := range []string{"memory_proposal_list", "memory_search", "memory_expand", "memory_curate_preview", "memory_curate_apply"} {
		if strings.Count(frontmatter, tool) != 1 {
			t.Fatalf("frontmatter tool %q count != 1", tool)
		}
	}
	for _, forbidden := range []string{"memory_propose"} {
		if strings.Contains(frontmatter, forbidden) {
			t.Fatalf("curate skill grants non-curation tool %q", forbidden)
		}
	}
	preview := strings.Index(text, "call `memory_curate_preview` exactly once")
	confirmation := strings.Index(text, "explicit confirmation")
	apply := strings.Index(text, "call `memory_curate_apply` exactly once")
	if preview < 0 || confirmation < preview || apply < confirmation {
		t.Fatalf("skill does not require preview, confirmation, and apply in order")
	}
	for _, contract := range []string{"complete replacement body", "preserving every still-valid fact", "section_id`, `content_hash`, and complete `replacement` only for `replace_section`"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("skill is missing update contract %q", contract)
		}
	}
}
