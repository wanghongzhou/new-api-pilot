package service

import (
	"testing"
	"time"

	testsupport "new-api-pilot/tests/support"
)

func TestRuntimeSettingsBuildsCanonicalHotReloadSnapshot(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	store := &RuntimeSettingsStore{clock: clock}
	values := runtimeSettingsTestValues()
	values["upstream.allowed_host_suffixes"] = "api.example.com,example.com"
	values["upstream.allowed_cidrs"] = "10.0.0.0/8,192.168.1.2/32"

	snapshot, err := store.Build(values)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	store.Store(snapshot)

	loaded := store.Snapshot()
	if loaded.FastTaskRetention != 24*time.Hour || loaded.FastTaskCount != 100 {
		t.Fatalf("fast task settings = %s/%d", loaded.FastTaskRetention, loaded.FastTaskCount)
	}
	if len(loaded.AllowedHosts) != 2 || loaded.AllowedHosts[0] != "api.example.com" {
		t.Fatalf("allowed hosts = %v", loaded.AllowedHosts)
	}
	if len(loaded.AllowedCIDRs) != 2 || loaded.AllowedCIDRs[0].String() != "10.0.0.0/8" {
		t.Fatalf("allowed CIDRs = %v", loaded.AllowedCIDRs)
	}

	loaded.AllowedHosts[0] = "mutated.invalid"
	if store.Snapshot().AllowedHosts[0] != "api.example.com" {
		t.Fatal("Snapshot() exposed mutable runtime state")
	}
}

func TestRuntimeSettingsAllowEmptyNetworkListsAndRejectInvalidRelationships(t *testing.T) {
	values := runtimeSettingsTestValues()
	values["upstream.allowed_host_suffixes"] = ""
	values["upstream.allowed_cidrs"] = ""
	if _, err := runtimeSettingsFromValues(values, testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))); err != nil {
		t.Fatalf("empty upstream lists must use safe-public policy: %v", err)
	}

	values["upstream.connect_timeout_seconds"] = "31"
	if _, err := runtimeSettingsFromValues(values, testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))); err == nil {
		t.Fatal("connect timeout above request timeout was accepted")
	}
}

func TestRuntimeSettingCanonicalizersNormalizeOperatorInput(t *testing.T) {
	hosts, err := canonicalUpstreamHostSuffixes("*.EXAMPLE.com\napi.example.com,example.com.")
	if err != nil || hosts != "api.example.com,example.com" {
		t.Fatalf("canonical hosts = %q, %v", hosts, err)
	}
	cidrs, err := canonicalUpstreamCIDRs("192.168.1.7/24\n10.0.0.1")
	if err != nil || cidrs != "10.0.0.1/32,192.168.1.0/24" {
		t.Fatalf("canonical CIDRs = %q, %v", cidrs, err)
	}
}

func runtimeSettingsTestValues() map[string]string {
	return map[string]string{
		"fast_task.history_retention_seconds":      "86400",
		"fast_task.history_count":                  "100",
		"upstream.allowed_host_suffixes":           "",
		"upstream.allowed_cidrs":                   "",
		"upstream.connect_timeout_seconds":         "5",
		"upstream.response_header_timeout_seconds": "15",
		"upstream.request_timeout_seconds":         "30",
		"upstream.export_timeout_seconds":          "120",
		"upstream.rate_limit_requests":             "300",
		"upstream.rate_limit_window_seconds":       "180",
		"upstream.max_inflight_per_origin":         "4",
	}
}
