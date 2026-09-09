package intake

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCorpusMarkersCanBeInjectedWithoutEnteringProductionPolicy(t *testing.T) {
	fixture, err := os.ReadFile("../../docs/phase-1/corpus/code-repo/private-markers.txt")
	if err != nil {
		t.Fatal(err)
	}
	var markers []string
	for _, line := range strings.Split(string(fixture), "\n") {
		_, value, found := strings.Cut(line, "=")
		if found && value != "" {
			markers = append(markers, value)
		}
	}
	patterns := make([]string, len(markers))
	for i, marker := range markers {
		patterns[i] = regexp.QuoteMeta(marker)
	}
	guard, err := newGuard(patterns)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range markers {
		raw, err := json.Marshal(map[string]any{
			"target_path": "memories/a.md", "section_id": "a", "replacement": marker, "source_paths": []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = guard.Inspect(NewCandidate(raw))
		var rejection Rejection
		if !errors.As(err, &rejection) || rejection.Reason != "sensitive_content" {
			t.Fatalf("Inspect() did not classify fixture marker as sensitive")
		}
	}
}

func TestDetectorCompilationFailurePreventsInitialization(t *testing.T) {
	_, err := newGuard([]string{"["})
	var rejection Rejection
	if !errors.As(err, &rejection) || rejection.Reason != "detector_initialization_failed" {
		t.Fatalf("newGuard() error = %v", err)
	}
}
