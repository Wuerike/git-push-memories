package experiment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateCurationRecordAcceptsGroundedSectionRemoval(t *testing.T) {
	root := t.TempDir()
	target := "memories/domains/cache.md"
	proposal := "proposals/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-remove-cache.md"
	if err := os.MkdirAll(filepath.Join(root, "memories", "domains"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "proposals"), 0o700); err != nil {
		t.Fatal(err)
	}
	before := "# Cache\n<!-- @id: cache-policy -->\n\n## Current TTL\n<!-- @id: cache-policy-current-ttl -->\nCache entries remain valid for at most 30 minutes.\n\n## Retired capacity\n<!-- @id: cache-policy-retired-capacity -->\nCache eviction uses an LRU capacity of 100 entries.\n"
	if err := os.WriteFile(filepath.Join(root, target), []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, proposal), []byte("proposal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	seedCandidateGit(t, root)
	headBefore, err := commandText(context.Background(), root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	post := "# Cache\n<!-- @id: cache-policy -->\n\n## Current TTL\n<!-- @id: cache-policy-current-ttl -->\nCache entries remain valid for at most 30 minutes.\n\n"
	if err := os.WriteFile(filepath.Join(root, target), []byte(post), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, proposal)); err != nil {
		t.Fatal(err)
	}
	runCommand(t, root, nil, "git", "add", "-A")
	runCommand(t, root, nil, "git", "-c", "user.name=Curator", "-c", "user.email=curator@example.invalid", "commit", "--quiet", "-m", "curate")
	observation := SessionObservation{
		ToolCounts: map[string]int{}, ReferenceIntegrities: []string{"verified"},
		ToolArguments: map[string][]map[string]any{"mcp__git-push-memories__memory_curate_preview": {{
			"operation": "remove_section", "target_path": target, "section_id": "cache-policy-retired-capacity",
		}}},
	}
	record, err := evaluateCurationRecord(context.Background(), root, headBefore, []byte(before), true, proposal, runnerCurationCase{
		ID: "cur-08", Class: "canonical", ProposalFact: "The LRU capacity policy is retired; cache validity is governed only by the thirty-minute TTL.",
		SuggestedAction: "remove", SuggestedTarget: target, ExpectedOperation: "remove_section", ExpectedTarget: target,
		ExpectedSectionID: "cache-policy-retired-capacity", ExpectedDisposition: "consumed", ExpectedEdit: "without_proposal_correction",
		ExpectedIntegrity: "verified", ExpectedSemanticSupport: "supports", RequiredPoststate: []string{"Cache entries remain valid for at most 30 minutes."}, ForbiddenPoststate: []string{"Retired capacity", "LRU capacity"},
	}, observation, curationLabels{Integrity: "verified", Support: "supports"})
	if err != nil {
		t.Fatal(err)
	}
	if !record.Correct || record.CanonicalTrustFailure || !record.WithoutProposalCorrection || record.Operation != "remove_section" {
		t.Fatalf("record = %+v", record)
	}
}
