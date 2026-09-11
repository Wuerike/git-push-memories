package policy

import "testing"

func TestHiddenLeaseRenewal(t *testing.T) {
	for failures, want := range map[int]int{1: 2, 3: 6, 8: 10} {
		if got := LeaseRenewalDelay(failures); got != want {
			t.Fatalf("LeaseRenewalDelay(%d) = %d, want %d", failures, got, want)
		}
	}
}

func TestHiddenWebhookGrace(t *testing.T) {
	for age, want := range map[int]bool{0: true, 1: true, 24: true, 25: false} {
		if got := WebhookKeyAccepted(age); got != want {
			t.Fatalf("WebhookKeyAccepted(%d) = %v, want %v", age, got, want)
		}
	}
}

func TestHiddenSettlementWindow(t *testing.T) {
	if got := SettlementDueDays("BR"); got != 3 {
		t.Fatalf("SettlementDueDays(BR) = %d, want 3", got)
	}
}

func TestHiddenRegionRouting(t *testing.T) {
	if got := RegionQueue("eu-west"); got != "payments-eu-v2" {
		t.Fatalf("RegionQueue(eu-west) = %q", got)
	}
}

func TestHiddenFeatureGate(t *testing.T) {
	if !FeatureEnabled("us-east") || FeatureEnabled("eu-west") {
		t.Fatal("feature gate does not match the current regional rollout")
	}
}

func TestHiddenCacheTTL(t *testing.T) {
	for age, want := range map[int]bool{1: true, 30: true, 31: false} {
		if got := CacheEntryValid(age); got != want {
			t.Fatalf("CacheEntryValid(%d) = %v, want %v", age, got, want)
		}
	}
}
