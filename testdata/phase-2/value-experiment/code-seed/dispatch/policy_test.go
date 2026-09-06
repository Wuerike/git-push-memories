package dispatch

import (
	"errors"
	"testing"
)

func TestStandardParcelsDoNotEscalate(t *testing.T) {
	policy, err := EscalationFor("standard")
	if err != nil {
		t.Fatal(err)
	}
	if policy != (Escalation{}) {
		t.Fatalf("standard policy = %+v", policy)
	}
}

func TestUnknownTierIsRejected(t *testing.T) {
	if _, err := EscalationFor("overnight"); !errors.Is(err, ErrUnknownTier) {
		t.Fatalf("error = %v, want ErrUnknownTier", err)
	}
}
