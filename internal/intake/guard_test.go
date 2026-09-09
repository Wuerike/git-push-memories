package intake_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Wuerike/git-push-memories/internal/intake"
)

func TestGuardAllowsOnlyCanonicalClosedCandidate(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"target_path":"memories\\checkout-api.md","section_id":"checkout-api-operation","replacement":"\ncurrent state\n","source_paths":["services\\checkout-api\\service.yaml"]}`)
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.TargetPath() != "memories/checkout-api.md" || allowed.SectionID() != "checkout-api-operation" || allowed.Replacement() != "\ncurrent state\n" {
		t.Fatalf("Inspect() = target %q, section %q, replacement %q", allowed.TargetPath(), allowed.SectionID(), allowed.Replacement())
	}
	if got := allowed.SourcePaths(); len(got) != 1 || got[0] != "services/checkout-api/service.yaml" {
		t.Fatalf("SourcePaths() = %v", got)
	}
}

func TestGuardAllowsClosedCreatePageCandidate(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"operation":"create_page","target_path":"memories/parcel-dispatch.md","title":"Parcel Dispatch","body":"Retries use backoff.\n","source_paths":["services/parcel/service.yaml"]}`)
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Operation() != "create_page" || allowed.TargetPath() != "memories/parcel-dispatch.md" || allowed.Title() != "Parcel Dispatch" || allowed.Body() != "Retries use backoff.\n" {
		t.Fatalf("Inspect() = operation %q, target %q, title %q, body %q", allowed.Operation(), allowed.TargetPath(), allowed.Title(), allowed.Body())
	}
	if allowed.SectionID() != "" || allowed.Replacement() != "" {
		t.Fatal("create candidate retained replace fields")
	}
}

func TestGuardAllowsClosedCreateSectionCandidate(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"operation":"create_section","target_path":"memories/checkout-api.md","heading":"Failure Modes","body":"Circuit breaker opens.","source_paths":[]}`)
	allowed, err := guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Operation() != "create_section" || allowed.Heading() != "Failure Modes" || allowed.Body() != "Circuit breaker opens." || allowed.SectionID() != "" {
		t.Fatalf("Inspect() = %+v", allowed)
	}
}

func TestGuardRejectsTargetsOutsideMemoryDirectory(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"page.md", "memories/nested/page.md", "proposals/page.md"} {
		_, err := guard.Inspect(intake.NewCandidate(candidateJSON(t, target, "page", "body", []string{})))
		var rejection intake.Rejection
		if !errors.As(err, &rejection) || rejection.Class != "schema" || rejection.Reason != "invalid_path" {
			t.Fatalf("Inspect(%q) error = %v, want schema:invalid_path", target, err)
		}
	}
}

func TestGuardRejectsUnknownDuplicateAndSensitiveFieldsWithoutEcho(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unknown session", raw: `{"target_path":"a.md","section_id":"a","replacement":"ok","source_paths":[],"session":"private-value"}`},
		{name: "duplicate", raw: `{"target_path":"a.md","target_path":"b.md","section_id":"a","replacement":"ok","source_paths":[]}`},
		{name: "secret", raw: `{"target_path":"a.md","section_id":"a","replacement":"password=private-value","source_paths":[]}`},
		{name: "null collection", raw: `{"target_path":"a.md","section_id":"a","replacement":"ok","source_paths":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := guard.Inspect(intake.NewCandidate([]byte(test.raw)))
			var rejection intake.Rejection
			if !errors.As(err, &rejection) {
				t.Fatalf("Inspect() error = %v", err)
			}
			if strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "password") {
				t.Fatalf("Inspect() echoed candidate: %q", err)
			}
		})
	}
}

func TestGuardEnforcesExactByteAndCollectionLimits(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	candidate := map[string]any{
		"target_path": "memories/a.md", "section_id": "a", "replacement": strings.Repeat("x", 128*1024), "source_paths": []string{},
	}
	raw, _ := json.Marshal(candidate)
	if _, err := guard.Inspect(intake.NewCandidate(raw)); err != nil {
		t.Fatalf("Inspect() rejected exact replacement limit: %v", err)
	}
	candidate["replacement"] = strings.Repeat("x", 128*1024+1)
	raw, _ = json.Marshal(candidate)
	if _, err := guard.Inspect(intake.NewCandidate(raw)); err == nil {
		t.Fatal("Inspect() accepted replacement over limit")
	}

	exactPath := "memories/" + strings.Repeat("a", 1012) + ".md"
	exactSources := make([]string, 16)
	for i := range exactSources {
		exactSources[i] = "source"
	}
	raw = candidateJSON(t, exactPath, "a", "ok", exactSources)
	if _, err := guard.Inspect(intake.NewCandidate(raw)); err != nil {
		t.Fatalf("Inspect() rejected exact path/source count limits: %v", err)
	}
	overSources := make([]string, 17)
	for i := range overSources {
		overSources[i] = "source"
	}
	raw = candidateJSON(t, "a.md", "a", "ok", overSources)
	if _, err := guard.Inspect(intake.NewCandidate(raw)); err == nil {
		t.Fatal("Inspect() accepted too many source paths")
	}

	base := candidateJSON(t, "memories/a.md", "a", "ok", nil)
	exactCandidate := append(base, bytes.Repeat([]byte(" "), 256*1024-len(base))...)
	if _, err := guard.Inspect(intake.NewCandidate(exactCandidate)); err != nil {
		t.Fatalf("Inspect() rejected exact candidate limit: %v", err)
	}
	if _, err := guard.Inspect(intake.NewCandidate(append(exactCandidate, ' '))); err == nil {
		t.Fatal("Inspect() accepted candidate over limit")
	}
}

func TestGuardRejectsEverySensitiveDetector(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	values := []string{
		"-----BEGIN TEST PRIVATE KEY-----",
		"Authorization: Bearer value",
		"api_key=value",
		"AKIA" + strings.Repeat("A", 16),
		"ghp_" + strings.Repeat("a", 36),
		"xoxb-" + strings.Repeat("a", 10),
		"sk-" + strings.Repeat("a", 20),
		"person@example.invalid",
		"+12025550123",
		"session_id=value",
		".claude/projects/value.jsonl",
		validCPF("123456789"),
	}
	for index, value := range values {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			_, err := guard.Inspect(intake.NewCandidate(candidateJSON(t, "a.md", "a", value, nil)))
			var rejection intake.Rejection
			if !errors.As(err, &rejection) || rejection.Reason != "sensitive_content" {
				t.Fatalf("Inspect() error = %v, want sensitive_content", err)
			}
		})
	}
}

func TestGuardScansEveryMetadataFieldAndRejectsControls(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	personal := "person@example.invalid"
	tests := [][]byte{
		candidateJSON(t, "docs/"+personal+".md", "a", "ok", nil),
		candidateJSON(t, "a.md", validCPF("123456789"), "ok", nil),
		candidateJSON(t, "a.md", "a", "ok", []string{"docs/" + personal}),
	}
	for _, raw := range tests {
		_, err := guard.Inspect(intake.NewCandidate(raw))
		var rejection intake.Rejection
		if !errors.As(err, &rejection) || rejection.Reason != "sensitive_content" {
			t.Fatalf("Inspect() error = %v, want sensitive_content", err)
		}
	}
	for _, raw := range [][]byte{
		candidateJSON(t, "bad\tpath.md", "a", "ok", nil),
		candidateJSON(t, "a.md", "a", "bad"+string(rune(0x85)), nil),
	} {
		_, err := guard.Inspect(intake.NewCandidate(raw))
		var rejection intake.Rejection
		if !errors.As(err, &rejection) || rejection.Reason != "control_character" {
			t.Fatalf("Inspect() error = %v, want control_character", err)
		}
	}
}

func TestGuardFindsValidCPFAfterInvalidDigitSequence(t *testing.T) {
	guard, err := intake.New()
	if err != nil {
		t.Fatal(err)
	}
	value := "12345678901 " + validCPF("123456789")
	_, err = guard.Inspect(intake.NewCandidate(candidateJSON(t, "a.md", "a", value, nil)))
	var rejection intake.Rejection
	if !errors.As(err, &rejection) || rejection.Reason != "sensitive_content" {
		t.Fatalf("Inspect() error = %v, want sensitive_content", err)
	}
}

func candidateJSON(t *testing.T, target, section, replacement string, sources []string) []byte {
	t.Helper()
	if sources == nil {
		sources = []string{}
	}
	raw, err := json.Marshal(map[string]any{
		"target_path": target, "section_id": section, "replacement": replacement, "source_paths": sources,
	})
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
