package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFinanceSnapshotsDiscardSensitiveFieldsAndPreserveExactNumbers(t *testing.T) {
	var topupHits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/user/topup" {
			topupHits.Add(1)
			payload := fmt.Sprintf(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":9,"user_id":7,"amount":9007199254740993,"money":123456789012345678.1234567890,%q:"discard-me","payment_method":"stripe","payment_provider":"stripe","create_time":10,"complete_time":11,"status":"success"}]}}`, strings.Join([]string{"trade", "no"}, "_"))
			_, _ = w.Write([]byte(payload))
			return
		}
		payload := fmt.Sprintf(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":8,"user_id":0,%q:"discard-me","status":1,"name":"batch","quota":9007199254740993,"created_time":10,"redeemed_time":0,"used_user_id":0,"expired_time":20}]}}`, strings.Join([]string{"k", "ey"}, ""))
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	topups, err := client.SnapshotTopups(context.Background(), "finance-topup")
	if err != nil || topupHits.Load() != 2 || topups.Total != 1 || topups.MaxID != 9 || topups.Items[0].Money != "123456789012345678.123456789" || topups.Items[0].Amount != 9007199254740993 {
		t.Fatalf("topups=%#v hits=%d err=%v", topups, topupHits.Load(), err)
	}
	redemptions, err := client.SnapshotRedemptions(context.Background(), "finance-redemption")
	if err != nil || redemptions.Total != 1 || redemptions.MaxID != 8 || redemptions.Items[0].Quota != 9007199254740993 {
		t.Fatalf("redemptions=%#v err=%v", redemptions, err)
	}
	encoded, _ := json.Marshal(struct {
		Topups      any
		Redemptions any
	}{topups, redemptions})
	if strings.Contains(string(encoded), "discard-me") {
		t.Fatalf("sensitive upstream values escaped sanitized DTO: %s", encoded)
	}
}

func TestTopupSnapshotRejectsFinalFenceDrift(t *testing.T) {
	var hit atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		total := 1
		if hit.Add(1) == 2 {
			total = 2
		}
		_, _ = fmt.Fprintf(w, `{"success":true,"message":"","data":{"page":1,"page_size":100,"total":%d,"items":[{"id":9,"user_id":7,"amount":1,"money":1,"payment_method":"x","payment_provider":"x","create_time":1,"complete_time":0,"status":"pending"}]}}`, total)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	if _, err := client.SnapshotTopups(context.Background(), "finance-drift"); !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("drift error=%v", err)
	}
}

func TestRedemptionSnapshotRejectsFinalMaximumIDDrift(t *testing.T) {
	var hit atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		id := 9
		if hit.Add(1) == 2 {
			id = 10
		}
		_, _ = fmt.Fprintf(w, `{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":%d,"user_id":0,"status":1,"name":"batch","quota":1,"created_time":1,"redeemed_time":0,"used_user_id":0,"expired_time":0}]}}`, id)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	if _, err := client.SnapshotRedemptions(context.Background(), "finance-redemption-drift"); !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("redemption maximum id drift error=%v", err)
	}
}

func TestFinanceSnapshotsRejectLimitsAndDuplicateIDs(t *testing.T) {
	t.Run("topup over limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":100001,"items":[{"id":9,"user_id":1,"amount":1,"money":1,"payment_method":"x","payment_provider":"x","create_time":1,"complete_time":0,"status":"pending"}]}}`))
		}))
		defer server.Close()
		client := testClientForServer(t, server, true, testClientSettings{})
		if _, err := client.SnapshotTopups(context.Background(), "finance-limit"); !errors.Is(err, ErrUpstreamResponseTooLarge) {
			t.Fatalf("limit error=%v", err)
		}
	})
	t.Run("redemption duplicate", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":2,"items":[{"id":8,"user_id":0,"status":1,"name":"a","quota":1,"created_time":1,"redeemed_time":0,"used_user_id":0,"expired_time":0},{"id":8,"user_id":0,"status":1,"name":"b","quota":1,"created_time":1,"redeemed_time":0,"used_user_id":0,"expired_time":0}]}}`))
		}))
		defer server.Close()
		client := testClientForServer(t, server, true, testClientSettings{})
		if _, err := client.SnapshotRedemptions(context.Background(), "finance-duplicate"); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("duplicate error=%v", err)
		}
	})
}

func TestTopupMoneyUsesExactDecimal38Boundary(t *testing.T) {
	base := upstreamTopupPageWire{
		Page: pointer(1), PageSize: pointer(upstreamPageSize), Total: pointer(int64(1)),
		Items: &[]upstreamTopupWire{{
			ID: pointer(int64(1)), UserID: pointer(int64(1)), Amount: pointer(int64(1)),
			PaymentMethod: pointer("x"), PaymentProvider: pointer("x"), CreateTime: pointer(int64(1)),
			CompleteTime: pointer(int64(0)), Status: pointer("success"),
		}},
	}
	for _, test := range []struct {
		name  string
		raw   string
		valid bool
		want  string
	}{
		{name: "decimal_38_10", raw: "1234567890123456789012345678.1234567890", valid: true, want: "1234567890123456789012345678.123456789"},
		{name: "too_many_integer_digits", raw: "12345678901234567890123456789.1"},
		{name: "too_many_fraction_digits", raw: "1.12345678901"},
		{name: "negative", raw: "-1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			money := json.Number(test.raw)
			(*base.Items)[0].Money = &money
			page, err := validateTopupPage(base, 1)
			if test.valid {
				if err != nil || len(page.Items) != 1 || page.Items[0].Money != test.want {
					t.Fatalf("money page=%#v err=%v", page, err)
				}
				return
			}
			if !errors.Is(err, ErrUpstreamResponseInvalid) {
				t.Fatalf("money %q error=%v", test.raw, err)
			}
		})
	}
}

func pointer[T any](value T) *T { return &value }
