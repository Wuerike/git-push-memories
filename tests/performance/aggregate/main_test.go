package main

import "testing"

func TestSummarizeUsesNearestRankForTenRuns(t *testing.T) {
	got := summarize([]int64{10, 1, 9, 2, 8, 3, 7, 4, 6, 5})
	if got.P50 != 5 || got.P95 != 10 {
		t.Fatalf("summarize() = %+v, want p50=5 p95=10", got)
	}
}
