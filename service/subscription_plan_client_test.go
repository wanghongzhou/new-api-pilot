package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubscriptionPlanSnapshotExactAndPrivateFieldsDiscarded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":true,"message":"","data":[{"plan":{"id":9,"title":"Pro","subtitle":"safe","price_amount":19.990000,"currency":"USD","duration_unit":"month","duration_value":1,"custom_seconds":0,"enabled":true,"sort_order":10,"total_amount":9007199254740993,"quota_reset_period":"monthly","quota_reset_custom_seconds":0,"created_at":1,"updated_at":2,"stripe_price_id":"secret","creem_product_id":"secret","waffo_pancake_product_id":"secret","allow_balance_pay":true,"allow_wallet_overflow":true,"max_purchase_per_user":1,"upgrade_group":"vip","downgrade_group":"default"}}]}`)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snap, err := client.SnapshotSubscriptionPlans(context.Background(), "plans")
	if err != nil || len(snap.Items) != 1 || snap.Items[0].PriceAmount != "19.99" || snap.Items[0].TotalAmount != 9007199254740993 {
		t.Fatalf("snap=%#v err=%v", snap, err)
	}
}
func TestSubscriptionPlanSnapshotRejectsDuplicate(t *testing.T) {
	id := int64(1)
	title, empty, price, currency, unit, reset := "x", "", "1", "USD", "month", "never"
	duration := 1
	zero := int64(0)
	enabled := true
	sort := 0
	created, updated := int64(1), int64(2)
	n := json.Number(price)
	p := upstreamSubscriptionPlanWire{ID: &id, Title: &title, Subtitle: &empty, PriceAmount: &n, Currency: &currency, DurationUnit: &unit, DurationValue: &duration, CustomSeconds: &zero, Enabled: &enabled, SortOrder: &sort, TotalAmount: &zero, QuotaResetPeriod: &reset, QuotaResetCustomSeconds: &zero, CreatedAt: &created, UpdatedAt: &updated}
	if _, err := validateSubscriptionPlans([]upstreamSubscriptionPlanDTO{{Plan: &p}, {Plan: &p}}); err == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestSubscriptionPlanSnapshotRejectsMissingOfficialSafeField(t *testing.T) {
	id, title, price, currency, unit, reset := int64(1), "x", json.Number("1"), "USD", "month", "never"
	duration, zero, enabled, sort := 1, int64(0), true, 0
	p := upstreamSubscriptionPlanWire{ID: &id, Title: &title, PriceAmount: &price, Currency: &currency, DurationUnit: &unit, DurationValue: &duration, CustomSeconds: &zero, Enabled: &enabled, SortOrder: &sort, TotalAmount: &zero, QuotaResetPeriod: &reset, QuotaResetCustomSeconds: &zero, CreatedAt: &zero, UpdatedAt: &zero}
	if _, err := validateSubscriptionPlans([]upstreamSubscriptionPlanDTO{{Plan: &p}}); err == nil {
		t.Fatal("missing official subtitle was accepted")
	}
}
