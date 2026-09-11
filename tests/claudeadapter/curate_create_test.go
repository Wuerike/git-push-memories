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
	for _, tool := range []string{"memory_proposal_list", "memory_search", "memory_expand", "memory_reference_snapshot", "memory_reference_expand", "memory_curate_preview", "memory_curate_apply", "memory_curate_batch_preview", "memory_curate_batch_apply"} {
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
	for _, contract := range []string{"`remove_section`", "`remove_page`", "`discard_proposal`", "`not_relevant`, `incorrect`, `sensitive`, `wrong_target`, `superseded`, or `duplicate`", "leaves the proposal unchanged"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("skill is missing remove/discard contract %q", contract)
		}
	}
	for _, contract := range []string{"`equivalent`, `contradictory`, `related`, or `unrelated`", "ask the user to choose the resolution", "canonical candidate and related proposal", "limited evidence"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("skill is missing classification contract %q", contract)
		}
	}
	for _, contract := range []string{"legacy path verified", "proves only local repository, revision, blob path and content hash", "`supports`, `contradicts`, or `unclear`", "`unavailable`, `mismatch`, and `legacy_unverified`"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("skill is missing reference evidence contract %q", contract)
		}
	}
	for _, contract := range []string{"one to sixteen", "distinct target paths", "memory_curate_batch_preview` exactly once", "memory_curate_batch_apply` exactly once"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("skill is missing batch contract %q", contract)
		}
	}
}
