package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogStatBoundsEveryRequest(t *testing.T) {
	var now atomic.Int64
	now.Store(1768622400)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/log/stat" || len(query) != 2 ||
			query.Get("start_timestamp") != strconv.FormatInt(now.Load()-60, 10) ||
			query.Get("end_timestamp") != strconv.FormatInt(now.Load(), 10) {
			t.Errorf("unexpected realtime request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("X-Request-ID") != "bounded-realtime" || r.Header.Get("Authorization") == "" {
			t.Error("missing request identity or authentication")
		}
		_, _ = w.Write([]byte(`{"success":true,"message":"","data":{"quota":0,"rpm":120,"tpm":24000}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	client.now = func() time.Time { return time.Unix(now.Load(), 0) }
	for range 2 {
		stat, err := client.LogStat(context.Background(), "bounded-realtime")
		if err != nil || stat.RPM != 120 || stat.TPM != 24000 {
			t.Fatalf("stat=%+v err=%v", stat, err)
		}
		now.Add(60)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests=%d", calls.Load())
	}
}

func TestLogStatRejectsUnboundedClockAndDoesNotFallback(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("start_timestamp") != "1768622340" || r.URL.Query().Get("end_timestamp") != "1768622400" {
			t.Errorf("unbounded request: %s", r.URL)
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	for _, timestamp := range []int64{-1, 0, 60} {
		client.now = func() time.Time { return time.Unix(timestamp, 0) }
		if _, err := client.LogStat(context.Background(), "invalid-clock"); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("timestamp=%d err=%v", timestamp, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid clock sent a request")
	}
	client.now = func() time.Time { return time.Unix(1768622400, 0) }
	if _, err := client.LogStat(context.Background(), "no-fallback"); err == nil {
		t.Fatal("upstream failure was ignored")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected fallback requests: %d", calls.Load())
	}
}
