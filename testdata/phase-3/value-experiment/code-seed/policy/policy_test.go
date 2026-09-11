package policy

import "testing"

func TestPublicDefaultsRemainStable(t *testing.T) {
	if LeaseRenewalDelay(0) != 0 {
		t.Fatal("zero failures must not delay renewal")
	}
	if WebhookKeyAccepted(-1) {
		t.Fatal("negative key age must be rejected")
	}
	if SettlementDueDays("unknown") != 1 {
		t.Fatal("unknown countries use the default settlement window")
	}
	if RegionQueue("default") != "payments-default" {
		t.Fatal("default routing changed")
	}
	if FeatureEnabled("unknown") {
		t.Fatal("unknown regions must not enable the feature")
	}
	if !CacheEntryValid(0) || CacheEntryValid(-1) {
		t.Fatal("cache age boundary changed")
	}
}
