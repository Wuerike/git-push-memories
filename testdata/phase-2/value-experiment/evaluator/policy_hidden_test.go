package dispatch

import (
	"errors"
	"testing"
	"time"
)

func TestPriorityEscalationUsesCurrentOperationalPolicy(t *testing.T) {
	policy, err := EscalationFor("priority")
	if err != nil {
		t.Fatal(err)
	}
	want := Escalation{After: 90 * time.Second, Queue: "route-amber", Owner: "routing-ops"}
	if policy != want {
		t.Fatalf("priority policy = %+v, want %+v", policy, want)
	}
}

func TestPriorityImplementationPreservesOtherBehavior(t *testing.T) {
	standard, err := EscalationFor("standard")
	if err != nil || standard != (Escalation{}) {
		t.Fatalf("standard policy = %+v, err = %v", standard, err)
	}
	if _, err := EscalationFor("overnight"); !errors.Is(err, ErrUnknownTier) {
		t.Fatalf("unknown tier error = %v", err)
	}
}
