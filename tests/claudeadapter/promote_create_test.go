package claudeadapter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromoteSkillGrantsOnlyPromotionToolsAndConfirmsExactPreview(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(repository, "integrations", "claude-code", "skills", "promote", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	frontmatterEnd := strings.Index(text[4:], "\n---\n")
	if frontmatterEnd < 0 {
		t.Fatal("invalid skill frontmatter")
	}
	frontmatter := text[:frontmatterEnd+4]
	for _, tool := range []string{"memory_promote_open", "memory_promote_status", "memory_promote_submit", "memory_page_read", "memory_proposal_list", "memory_search", "memory_expand", "memory_reference_snapshot", "memory_reference_expand", "memory_promote_preview", "memory_promote_apply", "memory_promote_batch_preview", "memory_promote_batch_apply"} {
		if strings.Count(frontmatter, tool) != 1 {
			t.Fatalf("frontmatter tool %q count != 1", tool)
		}
	}
	for _, forbidden := range []string{"memory_propose"} {
		if strings.Contains(frontmatter, forbidden) {
			t.Fatalf("promote skill grants non-promotion tool %q", forbidden)
		}
	}
	preview := strings.Index(text, "call `memory_promote_preview` exactly once")
	// The first "explicit confirmation" guards opening the release; the one that matters follows the preview.
	if !strings.Contains(text, "after explicit confirmation, call `memory_promote_open`") {
		t.Fatal("skill does not require confirmation before opening a release")
	}
	confirmation := -1
	if preview >= 0 {
		if offset := strings.Index(text[preview:], "explicit confirmation"); offset >= 0 {
			confirmation = preview + offset
		}
	}
	apply := strings.Index(text, "call `memory_promote_apply` exactly once")
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
	for _, contract := range []string{"batches of at most sixteen proposals", "distinct target paths", "memory_promote_batch_preview` exactly once", "memory_promote_batch_apply` exactly once"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("skill is missing batch contract %q", contract)
		}
	}
}
