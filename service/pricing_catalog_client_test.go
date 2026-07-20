package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestPricingGroupAndPricingSnapshotsUseIndependentManagementRequests(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"|"+r.Header.Get("X-Request-ID"))
		if r.Header.Get("Authorization") != "test-root-token" || r.Header.Get("New-Api-User") != "1" {
			t.Errorf("management headers missing: %#v", r.Header)
		}
		switch r.URL.Path {
		case "/api/group/":
			fmt.Fprint(w, `{"success":true,"message":"","data":["vip","default"]}`)
		case "/api/pricing":
			fmt.Fprint(w, `{"success":true,"data":[{"model_name":"gpt-x","description":"safe","icon":"icon","tags":"chat","vendor_id":7,"quota_type":0,"model_ratio":1e-3,"model_price":2.500000000000000001,"owner_by":"openai","completion_ratio":3,"cache_ratio":0.25,"create_cache_ratio":null,"image_ratio":4,"audio_ratio":5,"audio_completion_ratio":6,"enable_groups":["vip","default"],"supported_endpoint_types":["openai-response","openai"],"billing_mode":"expr","billing_expr":"secret","pricing_version":"item-secret","unknown_private":{"token":"secret"}}],"vendors":[{"id":7,"name":"  OpenAI  ","description":"discarded","icon":"discarded"}],"group_ratio":{"vip":1.25,"default":1e-2},"usable_group":{"vip":"VIP users","default":"Default users"},"supported_endpoint":{"openai":{"path":"/v1/chat/completions","headers":{"secret":"x"}}},"auto_groups":["secret"],"pricing_version":"version-1","unknown_top_level":{"secret":"discarded"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})

	groups, err := client.SnapshotPricingGroups(context.Background(), "groups-request")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{groups.Groups[0].Name, groups.Groups[1].Name}; !reflect.DeepEqual(got, []string{"default", "vip"}) {
		t.Fatalf("groups=%#v", groups.Groups)
	}
	pricing, err := client.SnapshotPricing(context.Background(), "pricing-request")
	if err != nil {
		t.Fatal(err)
	}
	if len(pricing.Items) != 1 || pricing.PricingVersion != "version-1" {
		t.Fatalf("pricing=%#v", pricing)
	}
	item := pricing.Items[0]
	if item.VendorKey != "OpenAI" || item.ModelRatio != "0.001" || item.ModelPrice != "2.500000000000000001" || item.CompletionRatio != "3" || !item.RootVisible {
		t.Fatalf("item=%#v", item)
	}
	if item.CacheRatio == nil || *item.CacheRatio != "0.25" || item.CreateCacheRatio != nil || item.AudioCompletionRatio == nil || *item.AudioCompletionRatio != "6" {
		t.Fatalf("optional ratios=%#v", item)
	}
	if !reflect.DeepEqual(item.EnableGroups, []string{"default", "vip"}) || !reflect.DeepEqual(item.SupportedEndpointTypes, []string{"openai", "openai-response"}) {
		t.Fatalf("canonical lists=%#v", item)
	}
	if len(pricing.Groups) != 2 || pricing.Groups[0].Name != "default" || pricing.Groups[0].Ratio == nil || *pricing.Groups[0].Ratio != "0.01" || !pricing.Groups[0].RootVisible || pricing.Groups[1].Description != "VIP users" {
		t.Fatalf("pricing groups=%#v", pricing.Groups)
	}
	if !reflect.DeepEqual(requests, []string{"/api/group/|groups-request", "/api/pricing|pricing-request"}) {
		t.Fatalf("requests=%#v", requests)
	}
}

func TestPricingSnapshotCanonicalVendorFallbacksAndInvisibleGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"success":true,"data":[{"model_name":"a","vendor_id":0,"quota_type":1,"model_ratio":0,"model_price":0,"owner_by":"","completion_ratio":0,"enable_groups":["all"],"supported_endpoint_types":["embeddings"]},{"model_name":"b","vendor_id":42,"quota_type":1,"model_ratio":1,"model_price":1,"owner_by":"x","completion_ratio":1,"enable_groups":[],"supported_endpoint_types":[]}],"vendors":[],"group_ratio":{},"usable_group":{},"pricing_version":"v"}`)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snapshot, err := client.SnapshotPricing(context.Background(), "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Items[0].VendorKey != "unknown" || snapshot.Items[1].VendorKey != "id:42" || snapshot.Items[0].ModelRatio != "0" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestPricingSnapshotsRejectInvalidIdentitiesAndEnums(t *testing.T) {
	validItem := `{"model_name":"x","vendor_id":0,"quota_type":0,"model_ratio":1,"model_price":1,"owner_by":"o","completion_ratio":1,"enable_groups":[],"supported_endpoint_types":["openai"]}`
	tests := []struct {
		name string
		body string
	}{
		{"unknown endpoint enum", `{"success":true,"data":[{"model_name":"x","vendor_id":0,"quota_type":0,"model_ratio":1,"model_price":1,"owner_by":"o","completion_ratio":1,"enable_groups":[],"supported_endpoint_types":["custom"]}],"vendors":[],"group_ratio":{},"usable_group":{},"pricing_version":"v"}`},
		{"duplicate pricing identity", `{"success":true,"data":[` + validItem + `,` + validItem + `],"vendors":[],"group_ratio":{},"usable_group":{},"pricing_version":"v"}`},
		{"missing required field", `{"success":true,"data":[{"model_name":"x"}],"vendors":[],"group_ratio":{},"usable_group":{},"pricing_version":"v"}`},
		{"negative decimal", `{"success":true,"data":[{"model_name":"x","vendor_id":0,"quota_type":0,"model_ratio":-1,"model_price":1,"owner_by":"o","completion_ratio":1,"enable_groups":[],"supported_endpoint_types":[]}],"vendors":[],"group_ratio":{},"usable_group":{},"pricing_version":"v"}`},
		{"duplicate json identity key", `{"success":true,"data":[],"vendors":[],"group_ratio":{"vip":1,"vip":2},"usable_group":{},"pricing_version":"v"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, test.body) }))
			defer server.Close()
			client := testClientForServer(t, server, true, testClientSettings{})
			if _, err := client.SnapshotPricing(context.Background(), "invalid"); err == nil {
				t.Fatal("invalid pricing response accepted")
			}
		})
	}
}

func TestPricingGroupSnapshotRejectsDuplicateAndEmptyNames(t *testing.T) {
	for _, data := range []string{`["vip","vip"]`, `[""]`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"success":true,"message":"","data":%s}`, data)
		}))
		client := testClientForServer(t, server, true, testClientSettings{})
		_, err := client.SnapshotPricingGroups(context.Background(), "invalid-groups")
		server.Close()
		if err == nil {
			t.Fatalf("invalid groups accepted: %s", data)
		}
	}
}
