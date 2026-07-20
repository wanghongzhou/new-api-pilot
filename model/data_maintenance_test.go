package model

import (
	"testing"

	"new-api-pilot/constant"
)

func TestAuthorizationPricingScopeIsStableAndVersioned(t *testing.T) {
	if got := AuthorizationPricingScope(11, 7); got != "site:11:config:7" {
		t.Fatalf("authorization pricing scope = %q", got)
	}
	if AuthorizationPricingScope(11, 7) == AuthorizationPricingScope(11, 8) {
		t.Fatal("config versions must not share an authorization pricing scope")
	}
}

func TestAuthorizationPricingSiteEligibilityIgnoresOnlineStatus(t *testing.T) {
	for _, status := range []string{constant.SiteOnlineUnknown, constant.SiteOnlineOffline, constant.SiteOnlineOnline} {
		site := Site{ConfigVersion: 7, ManagementStatus: constant.SiteManagementActive,
			AuthStatus: constant.SiteAuthAuthorized, OnlineStatus: status}
		if !authorizationPricingSiteEligible(site) {
			t.Fatalf("online status %q made authorized site ineligible", status)
		}
	}
	ended := int64(1)
	if authorizationPricingSiteEligible(Site{ConfigVersion: 7, ManagementStatus: constant.SiteManagementActive,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsEndAt: &ended}) {
		t.Fatal("permanently ended site remained eligible")
	}
}

func TestResourceScopeRevisionFrozenVectors(t *testing.T) {
	open, err := resourceScopeRevisionFromRows([]any{[]any{int64(11), 7, int64(2101521600), nil}}, []any{}, []any{[]any{int64(1), int64(11), "node-a", int64(2101521600), nil, int64(2101608000)}})
	if err != nil {
		t.Fatal(err)
	}
	if open != "4eb87a819af1864dc2a3818156fd36c6b746c1c65b83b7a343fe8cfb884fa7e6" {
		t.Fatalf("open vector=%s", open)
	}
	closed, err := resourceScopeRevisionFromRows([]any{[]any{int64(11), 7, int64(2101521600), nil}}, []any{[]any{int64(3), int64(11), int64(2101525200), int64(2101528800), int64(2101608000)}}, []any{[]any{int64(1), int64(11), "node-a", int64(2101521600), int64(2101532400), int64(2101608000)}})
	if err != nil {
		t.Fatal(err)
	}
	if closed != "4eb0608c983c43a32ca70fe826f6015741c459c5f22517df15480de026cae7cb" {
		t.Fatalf("closed vector=%s", closed)
	}
}

func TestMaintenanceAuthorizationRequestIDIsAlwaysCollectionSafe(t *testing.T) {
	if got := maintenanceAuthorizationRequestID(11, 7, "web_valid-1"); got != "web_valid-1" {
		t.Fatalf("valid request id changed to %q", got)
	}
	got := maintenanceAuthorizationRequestID(11, 7, "contains space")
	if got != "maintenance-pricing-11-7" || !validCollectionRequestID(got) {
		t.Fatalf("derived request id = %q", got)
	}
}
