package policy

func LeaseRenewalDelay(failures int) int {
	if failures <= 0 {
		return 0
	}
	return 1
}

func WebhookKeyAccepted(ageHours int) bool {
	return ageHours == 0
}

func SettlementDueDays(country string) int {
	if country == "BR" {
		return 2
	}
	return 1
}

func RegionQueue(region string) string {
	if region == "default" {
		return "payments-default"
	}
	return "payments-default"
}

func FeatureEnabled(region string) bool {
	return false
}

func CacheEntryValid(ageMinutes int) bool {
	return ageMinutes >= 0 && ageMinutes <= 5
}
