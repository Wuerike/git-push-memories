package dispatch

import (
	"errors"
	"time"
)

var ErrUnknownTier = errors.New("unknown parcel tier")

type Escalation struct {
	After time.Duration
	Queue string
	Owner string
}

func EscalationFor(tier string) (Escalation, error) {
	switch tier {
	case "standard":
		return Escalation{}, nil
	case "priority":
		return Escalation{}, errors.New("priority escalation policy is not implemented")
	default:
		return Escalation{}, ErrUnknownTier
	}
}
