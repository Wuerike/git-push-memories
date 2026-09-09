package intake_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/intake"
)

func TestGuardAllowsClosedProposalWithOptionalTarget(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := guard.InspectProposal(intake.NewProposalCandidate(proposalJSON(t,
		"Update checkout timeout", "update", "memories\\domains\\checkout-api.md", "The timeout is five seconds.", []string{"services\\checkout-api\\service.yaml"},
	)))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Title() != "Update checkout timeout" || allowed.SuggestedAction() != "update" || allowed.Target() != "memories/domains/checkout-api.md" || allowed.Knowledge() != "The timeout is five seconds." {
		t.Fatalf("InspectProposal() = %+v", allowed)
	}
	if got := allowed.SourcePaths(); len(got) != 1 || got[0] != "services/checkout-api/service.yaml" {
		t.Fatalf("SourcePaths() = %v", got)
	}

	raw := []byte(`{"title":"New fact","suggested_action":"create","knowledge":"Knowledge.","source_paths":[]}`)
	withoutTarget, err := guard.InspectProposal(intake.NewProposalCandidate(raw))
	if err != nil || withoutTarget.Target() != "" {
		t.Fatalf("targetless InspectProposal() = %+v, %v", withoutTarget, err)
	}
}

func TestGuardRejectsUnknownDuplicateInvalidAndSensitiveFieldsWithoutEcho(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unknown session", raw: `{"title":"Fact","suggested_action":"create","knowledge":"ok","source_paths":[],"session":"private-value"}`},
		{name: "duplicate", raw: `{"title":"Fact","title":"Other","suggested_action":"create","knowledge":"ok","source_paths":[]}`},
		{name: "invalid action", raw: `{"title":"Fact","suggested_action":"apply","knowledge":"ok","source_paths":[]}`},
		{name: "invalid target", raw: `{"title":"Fact","suggested_action":"update","target":"../private-value","knowledge":"ok","source_paths":[]}`},
		{name: "secret", raw: `{"title":"Fact","suggested_action":"create","knowledge":"password=private-value","source_paths":[]}`},
		{name: "null collection", raw: `{"title":"Fact","suggested_action":"create","knowledge":"ok","source_paths":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := guard.InspectProposal(intake.NewProposalCandidate([]byte(test.raw)))
			var rejection intake.Rejection
			if !errors.As(err, &rejection) {
				t.Fatalf("InspectProposal() error = %v", err)
			}
			if strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "password") {
				t.Fatalf("InspectProposal() echoed candidate: %q", err)
			}
		})
	}
}

func TestGuardEnforcesProposalLimits(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	raw := proposalJSON(t, "Fact", "create", "", strings.Repeat("x", 128*1024), []string{})
	if _, err := guard.InspectProposal(intake.NewProposalCandidate(raw)); err != nil {
		t.Fatalf("InspectProposal() rejected exact knowledge limit: %v", err)
	}
	raw = proposalJSON(t, "Fact", "create", "", strings.Repeat("x", 128*1024+1), []string{})
	if _, err := guard.InspectProposal(intake.NewProposalCandidate(raw)); err == nil {
		t.Fatal("InspectProposal() accepted knowledge over limit")
	}
	exact := proposalJSON(t, "Fact", "create", "", "ok", []string{})
	exact = append(exact, bytes.Repeat([]byte(" "), 256*1024-len(exact))...)
	if _, err := guard.InspectProposal(intake.NewProposalCandidate(exact)); err != nil {
		t.Fatalf("InspectProposal() rejected exact candidate limit: %v", err)
	}
	if _, err := guard.InspectProposal(intake.NewProposalCandidate(append(exact, ' '))); err == nil {
		t.Fatal("InspectProposal() accepted candidate over limit")
	}
}

func TestGuardRejectsEverySensitiveDetector(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	values := []string{
		"-----BEGIN TEST PRIVATE KEY-----", "Authorization: Bearer value", "api_key=value", "AKIA" + strings.Repeat("A", 16),
		"ghp_" + strings.Repeat("a", 36), "xoxb-" + strings.Repeat("a", 10), "sk-" + strings.Repeat("a", 20),
		"person@example.invalid", "+12025550123", "session_id=value", ".claude/projects/value.jsonl", validCPF("123456789"),
	}
	for index, value := range values {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			_, err := guard.InspectProposal(intake.NewProposalCandidate(proposalJSON(t, "Fact", "create", "", value, nil)))
			var rejection intake.Rejection
			if !errors.As(err, &rejection) || rejection.Reason != "sensitive_content" {
				t.Fatalf("InspectProposal() error = %v, want sensitive_content", err)
			}
		})
	}
}

func proposalJSON(t *testing.T, title, action, target, knowledge string, sources []string) []byte {
	t.Helper()
	if sources == nil {
		sources = []string{}
	}
	candidate := map[string]any{
		"title": title, "suggested_action": action, "knowledge": knowledge, "source_paths": sources,
	}
	if target != "" {
		candidate["target"] = target
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func validCPF(base string) string {
	value := base
	for check := 9; check < 11; check++ {
		sum := 0
		for i := 0; i < check; i++ {
			sum += int(value[i]-'0') * (check + 1 - i)
		}
		digit := 11 - sum%11
		if digit >= 10 {
			digit = 0
		}
		value += string(rune('0' + digit))
	}
	return value
}
