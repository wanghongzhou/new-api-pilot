package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelMetaSnapshotFencePrivacyAndZeroContract(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":9,"model_name":"gpt-4o","description":"safe","icon":"https://icons.invalid/x.svg","tags":"chat","vendor_id":7,"status":1,"sync_official":1,"name_rule":0,"created_time":1,"updated_time":2,"pricing":{"secret":1},"billing_expr":"secret","endpoints":"secret","bound_channels":[{"secret":1}],"enable_groups":["vip"],"quota_types":[1],"matched_models":["gpt-mini"],"matched_count":1}]}}`)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snap, err := client.SnapshotModelMeta(context.Background(), "models")
	if err != nil || snap.Total != 1 || snap.Items[0].Icon != "https://icons.invalid/x.svg" || calls != 2 {
		t.Fatalf("snap=%#v calls=%d err=%v", snap, calls, err)
	}
}
func TestValidateModelMetaPageDefaultsOptionalFields(t *testing.T) {
	page, size, total := 1, upstreamPageSize, int64(1)
	id, created, updated := int64(1), int64(0), int64(0)
	name := "x"
	status, sync, rule := 1, 1, 0
	items := []upstreamModelMetaWire{{ID: &id, ModelName: &name, Status: &status, SyncOfficial: &sync, NameRule: &rule, CreatedTime: &created, UpdatedTime: &updated}}
	out, err := validateModelMetaPage(upstreamModelMetaPageWire{Page: &page, PageSize: &size, Total: &total, Items: &items}, 1)
	if err != nil || out.Items[0].VendorID != 0 || out.Items[0].Description != "" {
		t.Fatalf("out=%#v err=%v", out, err)
	}
}
func TestValidateModelMetaPageRejectsZeroTotalWithItems(t *testing.T) {
	page, size, total := 1, upstreamPageSize, int64(0)
	id, vendor, created, updated := int64(1), int64(0), int64(0), int64(0)
	name, empty := "x", ""
	status, sync, rule := 1, 1, 0
	items := []upstreamModelMetaWire{{ID: &id, ModelName: &name, Description: &empty, Icon: &empty, Tags: &empty, VendorID: &vendor, Status: &status, SyncOfficial: &sync, NameRule: &rule, CreatedTime: &created, UpdatedTime: &updated}}
	if _, err := validateModelMetaPage(upstreamModelMetaPageWire{Page: &page, PageSize: &size, Total: &total, Items: &items}, 1); err == nil {
		t.Fatal("zero total with items accepted")
	}
}
func TestModelMetaSnapshotRejectsPaginationTotalDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := 1
		fmt.Sscan(r.URL.Query().Get("p"), &page)
		total := int64(101)
		count := 100
		if page == 2 {
			total = 102
			count = 1
		}
		items := make([]map[string]any, 0, count)
		start := 200 - (page-1)*100
		for i := 0; i < count; i++ {
			items = append(items, map[string]any{"id": start - i, "model_name": fmt.Sprintf("m%d", start-i), "status": 1, "sync_official": 1, "name_rule": 0, "created_time": 1, "updated_time": 2})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "", "data": map[string]any{"page": page, "page_size": 100, "total": total, "items": items}})
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	if _, err := client.SnapshotModelMeta(context.Background(), "drift"); err == nil {
		t.Fatal("pagination total drift accepted")
	}
}
