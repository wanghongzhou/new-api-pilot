package service

import (
	"new-api-pilot/model"
	"testing"
)

func TestPerformanceCollectionStatus(t *testing.T) {
	tests := []struct {
		name     string
		coverage model.PerformanceCollectionCoverage
		want     string
	}{
		{name: "no eligible sites", want: "complete"},
		{name: "all successful", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, SuccessfulSites: 2}, want: "complete"},
		{name: "some successful", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, SuccessfulSites: 1}, want: "partial"},
		{name: "all unavailable", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, UnavailableSites: 2}, want: "unavailable"},
		{name: "some unavailable", coverage: model.PerformanceCollectionCoverage{SiteCount: 2, UnavailableSites: 1}, want: "partial"},
		{name: "never collected", coverage: model.PerformanceCollectionCoverage{SiteCount: 2}, want: "pending"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := performanceCollectionStatus(test.coverage); got != test.want {
				t.Fatalf("performanceCollectionStatus(%#v)=%q want %q", test.coverage, got, test.want)
			}
		})
	}
}
