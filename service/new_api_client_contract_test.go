package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func TestStrictUpstreamJSON(t *testing.T) {
	validData := `{"version":"test-version","system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true}`
	tests := []struct {
		name    string
		payload []byte
		want    error
	}{
		{name: "invalid UTF-8", payload: append([]byte(`{"success":true,"message":"","data":"`), 0xff, '"', '}'), want: ErrUpstreamResponseInvalid},
		{name: "uppercase envelope success", payload: []byte(`{"Success":true,"message":"","data":` + validData + `}`), want: ErrUpstreamResponseInvalid},
		{name: "envelope EqualFold conflict", payload: []byte(`{"success":true,"Success":true,"message":"","data":` + validData + `}`), want: ErrUpstreamResponseInvalid},
		{name: "uppercase envelope data", payload: []byte(`{"success":true,"message":"","Data":` + validData + `}`), want: ErrUpstreamResponseInvalid},
		{name: "duplicate data key", payload: []byte(`{"success":true,"message":"","data":{"version":"test-version","version":"other","system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true}}`), want: ErrUpstreamResponseInvalid},
		{name: "duplicate ignored nested key", payload: []byte(`{"success":true,"message":"","data":{"version":"test-version","system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true,"ignored":{"x":1,"x":2}}}`), want: ErrUpstreamResponseInvalid},
		{name: "trailing JSON", payload: []byte(`{"success":true,"message":"","data":` + validData + `}{}`), want: ErrUpstreamResponseInvalid},
		{name: "wrong success type", payload: []byte(`{"success":"true","message":"","data":` + validData + `}`), want: ErrUpstreamEnvelopeInvalid},
		{name: "wrong message type", payload: []byte(`{"success":true,"message":7,"data":` + validData + `}`), want: ErrUpstreamEnvelopeInvalid},
		{name: "failed envelope", payload: []byte(`{"success":false,"message":"denied","data":{}}`), want: ErrUpstreamEnvelopeInvalid},
		{name: "missing data", payload: []byte(`{"success":true,"message":""}`), want: ErrUpstreamEnvelopeInvalid},
		{name: "wrong field primitive", payload: []byte(`{"success":true,"message":"","data":{"version":7,"system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true}}`), want: ErrUpstreamResponseInvalid},
		{name: "uppercase known data field", payload: []byte(`{"success":true,"message":"","data":{"Version":"test-version","system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true}}`), want: ErrUpstreamResponseInvalid},
		{name: "decimal scale overflow", payload: []byte(`{"success":true,"message":"","data":{"version":"test-version","system_name":"Test","quota_per_unit":0.00000000001,"usd_exchange_rate":1,"enable_data_export":true}}`), want: ErrUpstreamResponseInvalid},
		{name: "decimal integer overflow", payload: []byte(`{"success":true,"message":"","data":{"version":"test-version","system_name":"Test","quota_per_unit":100000000000000000000,"usd_exchange_rate":1,"enable_data_export":true}}`), want: ErrUpstreamResponseInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write(test.payload)
			}))
			defer server.Close()
			client := testClientForServer(t, server, false, testClientSettings{})
			if _, err := client.Status(context.Background(), "strict-json"); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestSnapshotUsersRejectsInventoryOverLimitBeforeSecondPage(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hits.Add(1)
		if request.URL.Query().Get("p") != "1" {
			t.Fatalf("unexpected inventory page request: %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":100001,"items":[{"id":1,"username":"root","display_name":"Root","role":100,"status":1,"group":"default","quota":1,"used_quota":0,"request_count":0,"created_at":1,"last_login_at":0,"DeletedAt":null}]}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	if _, err := client.SnapshotUsers(context.Background(), "inventory-limit"); !errors.Is(err, ErrUpstreamResponseTooLarge) {
		t.Fatalf("oversized user inventory error = %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("oversized inventory requested %d pages, want 1", hits.Load())
	}
}

func TestSnapshotChannelsKeepsDecimalOperationsAndNeverExposesKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"page":1,"page_size":100,"total":1,"items":[{"id":9,"name":"primary","type":1,"status":1,"key":"secret-must-not-be-decoded","test_time":10,"response_time":123,"balance":9007199254740993.123456789,"balance_updated_time":11,"models":"gpt","group":"default","used_quota":9007199254740993,"priority":7,"weight":8,"auto_ban":1,"tag":"prod"}]}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	snapshot, err := client.SnapshotChannels(context.Background(), "channel-contract")
	if err != nil || len(snapshot.Items) != 1 || snapshot.Items[0].Balance != "9007199254740993.123456789" || snapshot.Items[0].UsedQuota != 9007199254740993 || snapshot.Items[0].ResponseTimeMS != 123 {
		t.Fatalf("channel snapshot=%#v err=%v", snapshot, err)
	}
}

func TestStatusAllowsEmptyVersionForInternalFork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"version":"","system_name":"Internal Fork","quota_per_unit":500000,"usd_exchange_rate":6.82,"enable_data_export":true}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, false, testClientSettings{})
	status, err := client.Status(context.Background(), "empty-version")
	if err != nil {
		t.Fatalf("empty upstream version rejected: %v", err)
	}
	if status.Version != "" || status.SystemName != "Internal Fork" || !status.DataExportEnabled {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestPerformanceSummaryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/perf-metrics/summary" || request.URL.Query().Get("hours") != "168" {
			http.Error(writer, "unexpected request", http.StatusBadRequest)
			return
		}
		if request.Header.Get("Authorization") == "" || request.Header.Get("New-Api-User") != "1" || request.Header.Get("X-Request-ID") != "performance-contract" {
			http.Error(writer, "missing management credentials", http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"models":[{"model_name":"model-a","request_count":10,"success_rate":90,"avg_latency_ms":120,"avg_tps":25},{"model_name":"model-b","request_count":30,"success_rate":100,"avg_latency_ms":80,"avg_tps":45}]}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	summary, err := client.PerformanceSummary(context.Background(), "performance-contract", 168)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if len(summary.Models) != 2 || summary.Models[1].RequestCount != 30 || summary.Models[0].SuccessRate != 90 {
		t.Fatalf("unexpected performance summary: %#v", summary)
	}
}

func TestPerformanceSummaryRejectsInvalidMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"models":[{"model_name":"model-a","request_count":-1,"success_rate":100,"avg_latency_ms":1,"avg_tps":1}]}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	if _, err := client.PerformanceSummary(context.Background(), "performance-invalid", 24); !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("expected invalid upstream response, got %v", err)
	}
}

func TestPerformanceHistoryPreservesOfficialAverageSeriesWithoutInventingCounters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/perf-metrics/summary":
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"models":[{"model_name":"gpt-4o","avg_latency_ms":100,"success_rate":0.9,"avg_tps":20,"request_count":10}]}}`))
		case "/api/perf-metrics":
			if request.URL.Query().Get("model") != "gpt-4o" || request.URL.Query().Get("hours") != "24" {
				t.Fatalf("performance detail query=%s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"model_name":"gpt-4o","series_schema":"ts,avg_ttft_ms,avg_latency_ms,success_rate,avg_tps","groups":[{"group":"default","avg_ttft_ms":50,"avg_latency_ms":100,"success_rate":0.9,"avg_tps":20,"series":[{"ts":1,"avg_ttft_ms":50.5,"avg_latency_ms":100.25,"success_rate":0.9,"avg_tps":20.125}]}]}}`))
		default:
			t.Fatalf("unexpected performance path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	history, err := client.PerformanceHistory(context.Background(), "performance-history", 24)
	if err != nil || history.CounterReady || len(history.Models) != 1 || len(history.Models[0].Groups) != 1 || history.Models[0].Groups[0].Series[0].AvgLatencyMS != "100.25" || history.Models[0].Groups[0].Series[0].Counters.RequestCount != nil {
		t.Fatalf("performance history=%#v err=%v", history, err)
	}
}

func TestStrictJSONUnknownExtensionsRemainCompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"","future_envelope":{"Data":1},"data":{` +
			`"version":"test-version","system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true,` +
			`"future_field":{"Version":1,"Items":[{"Status":"new"}]}}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, false, testClientSettings{})
	if _, err := client.Status(context.Background(), "unknown-extension"); err != nil {
		t.Fatalf("truly unknown extension fields were rejected: %v", err)
	}
}

func TestStrictJSONKnownFieldCaseInNestedStructsAndSlices(t *testing.T) {
	instanceBase := `{"node_name":"node-1","status":"online","stale_after_seconds":90,"started_at":0,"last_seen_at":0,` +
		`"info":{"node":{"name":"node-1"},"role":{"is_master":true},"runtime":{"version":"go1.25","goos":"linux","goarch":"amd64"},"host":{"hostname":"host"}}}`
	tests := []struct {
		name     string
		payload  string
		endpoint string
	}{
		{name: "slice item field", endpoint: "instances", payload: strings.Replace(instanceBase, `"node_name"`, `"Node_Name"`, 1)},
		{name: "nested struct field", endpoint: "instances", payload: strings.Replace(instanceBase, `"runtime"`, `"Runtime"`, 1)},
		{name: "nested raw role field", endpoint: "instances", payload: strings.Replace(instanceBase, `"is_master"`, `"Is_Master"`, 1)},
		{name: "nested deleted marker", endpoint: "user", payload: `{"id":1,"username":"root","display_name":"Root","role":100,"status":1,"group":"default","quota":0,"used_quota":0,"request_count":0,"created_at":1,"last_login_at":0,"DeletedAt":{"valid":true}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(`{"success":true,"message":"","data":` + func() string {
					if test.endpoint == "instances" {
						return "[" + test.payload + "]"
					}
					return test.payload
				}() + `}`))
			}))
			defer server.Close()
			client := testClientForServer(t, server, true, testClientSettings{})
			var err error
			if test.endpoint == "instances" {
				_, err = client.Instances(context.Background(), "nested-case")
			} else {
				_, err = client.GetUser(context.Background(), "nested-case", 1)
			}
			if !errors.Is(err, ErrUpstreamResponseInvalid) {
				t.Fatalf("known field case variant was accepted: %v", err)
			}
		})
	}
}

func TestUpstreamIntegerOverflowAndSafeAggregation(t *testing.T) {
	t.Run("wire int64 overflow", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"quota":0,"rpm":9223372036854775808,"tpm":0}}`))
		}))
		defer server.Close()
		client := testClientForServer(t, server, true, testClientSettings{})
		if _, err := client.LogStat(context.Background(), "overflow-wire"); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("int64 overflow was not rejected: %v", err)
		}
	})

	t.Run("duplicate flow sum overflow", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":[` +
				`{"user_id":1,"username":"root","model_name":"m","channel_id":1,"count":9223372036854775807,"quota":0,"token_used":0},` +
				`{"user_id":1,"username":"root","model_name":"m","channel_id":1,"count":1,"quota":0,"token_used":0}]}`))
		}))
		defer server.Close()
		client := testClientForServer(t, server, true, testClientSettings{})
		if _, err := client.FlowHour(context.Background(), "overflow-sum", f02HourStart); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("aggregate overflow was not rejected: %v", err)
		}
	})

	t.Run("consistency sum overflow", func(t *testing.T) {
		flow := []dto.UpstreamFlowRow{
			{UserID: 1, Username: "root", ModelName: "m", RequestCount: math.MaxInt64},
			{UserID: 1, Username: "root", ModelName: "m", RequestCount: 1},
		}
		if err := ValidateFlowDataConsistency(flow, nil); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("consistency overflow was not rejected: %v", err)
		}
	})
}

func TestFlowUsernameCanonicalizationIsOrderIndependent(t *testing.T) {
	wire := []upstreamFlowRowWire{
		flowWireForTest(2, "zeta", "model-b", 2, 1, 10, 100),
		flowWireForTest(1, "", "model-a", 1, 1, 20, 200),
		flowWireForTest(1, "zeta", "model-a", 1, 2, 30, 300),
		flowWireForTest(1, "alpha", "model-a", 1, 3, 40, 400),
		flowWireForTest(2, "", "model-b", 2, 4, 50, 500),
	}
	reversed := make([]upstreamFlowRowWire, len(wire))
	for index := range wire {
		reversed[len(wire)-1-index] = wire[index]
	}

	forward, err := validateAndAggregateFlowRows(wire)
	if err != nil {
		t.Fatalf("aggregate forward flow rows: %v", err)
	}
	backward, err := validateAndAggregateFlowRows(reversed)
	if err != nil {
		t.Fatalf("aggregate reversed flow rows: %v", err)
	}
	if len(forward) != 2 || forward[0].Username != "alpha" || forward[0].RequestCount != 6 ||
		forward[0].Quota != 90 || forward[0].TokenUsed != 900 || forward[1].Username != "zeta" ||
		forward[1].RequestCount != 5 || forward[1].Quota != 60 || forward[1].TokenUsed != 600 {
		t.Fatalf("unexpected canonical flow rows: %+v", forward)
	}
	forwardJSON, err := json.Marshal(forward)
	if err != nil {
		t.Fatalf("encode forward flow rows: %v", err)
	}
	backwardJSON, err := json.Marshal(backward)
	if err != nil {
		t.Fatalf("encode backward flow rows: %v", err)
	}
	if sha256.Sum256(forwardJSON) != sha256.Sum256(backwardJSON) {
		t.Fatalf("input order changed canonical flow hash: forward=%s backward=%s", forwardJSON, backwardJSON)
	}

	allEmpty, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{
		flowWireForTest(1, "", "model-a", 1, 1, 2, 3),
		flowWireForTest(1, "", "model-a", 1, 4, 5, 6),
	})
	if err != nil || len(allEmpty) != 1 || allEmpty[0].Username != "" || allEmpty[0].RequestCount != 5 {
		t.Fatalf("all-empty usernames were not aggregated: %+v, %v", allEmpty, err)
	}
}

func TestFlowParityDimensionsRemainDistinctAndTokenNameIsCanonical(t *testing.T) {
	base := flowWireForTest(1, "user", "model", 7, 1, 2, 3)
	groupA, groupB, nodeA, nodeB := "group-a", "group-b", "node-a", "node-b"
	tokenID := int64(9)
	alpha, zeta := "alpha", "zeta"
	base.UseGroup, base.TokenID, base.TokenName, base.NodeName = &groupA, &tokenID, &zeta, &nodeA
	duplicate := base
	duplicate.TokenName = &alpha
	otherGroup := base
	otherGroup.UseGroup = &groupB
	otherNode := base
	otherNode.NodeName = &nodeB

	rows, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{otherNode, duplicate, otherGroup, base})
	if err != nil {
		t.Fatalf("aggregate parity dimensions: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("parity dimensions were merged: %+v", rows)
	}
	for _, row := range rows {
		if row.UseGroup == groupA && row.NodeName == nodeA {
			if row.RequestCount != 2 || row.TokenName != alpha {
				t.Fatalf("duplicate canonical token snapshot = %+v", row)
			}
			return
		}
	}
	t.Fatal("canonical duplicate row not found")
}

func TestFlowParityDimensionValidation(t *testing.T) {
	valid := flowWireForTest(1, "user", "model", 0, 1, 2, 3)
	negativeTokenID := int64(-1)
	valid.TokenID = &negativeTokenID
	if _, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{valid}); !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("negative token id error = %v", err)
	}

	tooLong := strings.Repeat("g", 129)
	valid = flowWireForTest(1, "user", "model", 0, 1, 2, 3)
	valid.UseGroup = &tooLong
	if _, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{valid}); !errors.Is(err, ErrUpstreamResponseInvalid) {
		t.Fatalf("oversized group error = %v", err)
	}

	tests := []struct {
		name  string
		apply func(*upstreamFlowRowWire)
	}{
		{
			name: "token name invalid UTF-8",
			apply: func(row *upstreamFlowRowWire) {
				value := string([]byte{0xff})
				row.TokenName = &value
			},
		},
		{
			name: "token name overlong",
			apply: func(row *upstreamFlowRowWire) {
				value := strings.Repeat("t", 256)
				row.TokenName = &value
			},
		},
		{
			name: "node name invalid UTF-8",
			apply: func(row *upstreamFlowRowWire) {
				value := string([]byte{0xff})
				row.NodeName = &value
			},
		},
		{
			name: "node name overlong",
			apply: func(row *upstreamFlowRowWire) {
				value := strings.Repeat("n", 129)
				row.NodeName = &value
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := flowWireForTest(1, "user", "model", 0, 1, 2, 3)
			test.apply(&row)
			if _, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{row}); !errors.Is(err, ErrUpstreamResponseInvalid) {
				t.Fatalf("invalid parity dimension error = %v", err)
			}
		})
	}
}

func TestFlowOptionalParityDimensionsNormalizeToZeroValues(t *testing.T) {
	row := flowWireForTest(1, "user", "model", 0, 1, 2, 3)
	rows, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{row})
	if err != nil {
		t.Fatalf("aggregate optional parity dimensions: %v", err)
	}
	if len(rows) != 1 || rows[0].UseGroup != "" || rows[0].TokenID != 0 ||
		rows[0].TokenName != "" || rows[0].NodeName != "" {
		t.Fatalf("optional parity dimensions were not normalized: %+v", rows)
	}
}

func TestFlowDataConsistencyAcceptsCanonicalEmptyUsername(t *testing.T) {
	flow := []dto.UpstreamFlowRow{{
		UserID: 1, Username: "", ModelName: "model", ChannelID: 0,
		RequestCount: 1, Quota: 2, TokenUsed: 3,
	}}
	data := []dto.UpstreamDataRow{{
		ModelName: "model", CreatedAt: f02HourStart,
		RequestCount: 1, Quota: 2, TokenUsed: 3,
	}}
	if err := ValidateFlowDataConsistency(flow, data); err != nil {
		t.Fatalf("canonical empty username was rejected: %v", err)
	}
}

func TestFlowUsernameUnicodeBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		username string
		valid    bool
	}{
		{name: "empty", username: "", valid: true},
		{name: "255 Unicode characters", username: strings.Repeat("界", 255), valid: true},
		{name: "256 Unicode characters", username: strings.Repeat("界", 256), valid: false},
		{name: "invalid UTF-8", username: string([]byte{0xff}), valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := validateAndAggregateFlowRows([]upstreamFlowRowWire{
				flowWireForTest(1, test.username, "model", 0, 1, 2, 3),
			})
			if !test.valid {
				if !errors.Is(err, ErrUpstreamResponseInvalid) {
					t.Fatalf("invalid username was accepted: rows=%+v err=%v", rows, err)
				}
				return
			}
			if err != nil || len(rows) != 1 || rows[0].Username != test.username {
				t.Fatalf("valid username was rejected: rows=%+v err=%v", rows, err)
			}
		})
	}
}

func flowWireForTest(userID int64, username, modelName string, channelID, requestCount, quota, tokenUsed int64) upstreamFlowRowWire {
	return upstreamFlowRowWire{
		UserID: &userID, Username: &username, ModelName: &modelName,
		ChannelID: json.RawMessage(strconv.FormatInt(channelID, 10)), RequestCount: &requestCount,
		Quota: &quota, TokenUsed: &tokenUsed,
	}
}

func TestFlowChannelIDOmitEmptyContract(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		wantID    int64
		wantError bool
	}{
		{name: "missing maps to zero", field: "", wantID: 0},
		{name: "explicit zero", field: `,"channel_id":0`, wantID: 0},
		{name: "positive", field: `,"channel_id":9`, wantID: 9},
		{name: "negative", field: `,"channel_id":-1`, wantError: true},
		{name: "null", field: `,"channel_id":null`, wantError: true},
		{name: "overflow", field: `,"channel_id":9223372036854775808`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := `{"success":true,"message":"","data":[{"user_id":1,"username":"root","model_name":"m"` +
				test.field + `,"count":1,"quota":2,"token_used":3}]}`
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(payload))
			}))
			defer server.Close()
			client := testClientForServer(t, server, true, testClientSettings{})
			rows, err := client.FlowHour(context.Background(), "flow-channel", f02HourStart)
			if test.wantError {
				if !errors.Is(err, ErrUpstreamResponseInvalid) {
					t.Fatalf("expected invalid channel_id, got rows=%+v err=%v", rows, err)
				}
				return
			}
			if err != nil || len(rows) != 1 || rows[0].ChannelID != test.wantID {
				t.Fatalf("channel_id mapping failed: rows=%+v err=%v", rows, err)
			}
		})
	}
}

func TestCredentialOriginAndHeaderIsolation(t *testing.T) {
	t.Run("origin mismatch blocks credentials before network", func(t *testing.T) {
		var managementHits atomic.Int32
		var publicCredentials atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/status" {
				if request.Header.Get("Authorization") != "" || request.Header.Get("New-Api-User") != "" {
					publicCredentials.Store(true)
				}
				_, _ = writer.Write([]byte(testStatusEnvelope))
				return
			}
			managementHits.Add(1)
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		}))
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		baseURL := logicalTestBaseURL(t, server, "fixture.example", "http")
		client := newTestUpstreamClient(t, baseURL, parsed.Host,
			staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
			redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{
				withCredentials: true, credentialOrigin: "http://other.example:" + parsed.Port(),
			})
		if _, err := client.Status(context.Background(), "origin-public"); err != nil {
			t.Fatalf("public status should remain available: %v", err)
		}
		if _, err := client.Self(context.Background(), "origin-management"); !errors.Is(err, ErrUpstreamCredentialOriginMismatch) {
			t.Fatalf("mismatched credentials were not blocked: %v", err)
		}
		if managementHits.Load() != 0 || publicCredentials.Load() {
			t.Fatal("credentials crossed their origin/public boundary")
		}
	})

	t.Run("path-only change retains credentials", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/new/api/user/self" || request.Header.Get("Authorization") != "test-root-token" ||
				request.Header.Get("New-Api-User") != "1" {
				http.Error(writer, "unexpected request", http.StatusBadRequest)
				return
			}
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		}))
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		origin := logicalTestBaseURL(t, server, "fixture.example", "http")
		client := newTestUpstreamClient(t, origin+"/new", parsed.Host,
			staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
			redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{
				withCredentials: true, credentialOrigin: origin + "/old",
			})
		if _, err := client.Self(context.Background(), "origin-path"); err != nil {
			t.Fatalf("path-only credential reuse failed: %v", err)
		}
	})

	t.Run("temporary login session never sends Authorization", func(t *testing.T) {
		var violations atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "" || request.Header.Get("X-Request-ID") == "" {
				violations.Add(1)
			}
			switch request.URL.Path {
			case "/api/user/login":
				if request.Header.Get("New-Api-User") != "" || request.Header.Get("Cookie") != "" {
					violations.Add(1)
				}
				http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
				_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
			case "/api/user/token":
				if request.Header.Get("New-Api-User") != "1" || request.Header.Get("Cookie") == "" {
					violations.Add(1)
				}
				_, _ = writer.Write([]byte(`{"success":true,"message":"","data":"rotated-token"}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()
		client := testClientForServer(t, server, false, testClientSettings{})
		identity, token, err := client.LoginAndGenerateAccessToken(context.Background(), "session-headers", "root", "password")
		if err != nil || identity.ID != 1 || token != "rotated-token" || violations.Load() != 0 {
			t.Fatalf("temporary session isolation failed: id=%d token=%q violations=%d err=%v", identity.ID, token, violations.Load(), err)
		}
	})
}

func TestUpstreamHTTPStatusPrecedesEnvelope(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{status: http.StatusUnauthorized, want: ErrUpstreamAuthExpired},
		{status: http.StatusForbidden, want: ErrUpstreamPermissionDenied},
		{status: http.StatusTooManyRequests, want: ErrUpstreamRateLimited},
		{status: http.StatusInternalServerError, want: ErrUpstreamRemote},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(`not-json secret-response-body`))
			}))
			defer server.Close()
			client := testClientForServer(t, server, true, testClientSettings{})
			_, err := client.Self(context.Background(), "http-status")
			if !errors.Is(err, test.want) {
				t.Fatalf("status %d classified as %v", test.status, err)
			}
			if strings.Contains(err.Error(), "secret-response-body") {
				t.Fatal("HTTP error exposed the response body")
			}
		})
	}
}

func TestUpstreamErrorsAreRedacted(t *testing.T) {
	const token = "test-root-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"body-secret","data":{"id":"wrong"}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	_, err := client.Self(context.Background(), "redaction")
	if err == nil {
		t.Fatal("expected invalid response")
	}
	for _, secret := range []string{token, "body-secret", "fixture.example", server.URL} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %v", secret, err)
		}
	}
}

func TestUpstreamHeaderValueValidation(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = writer.Write([]byte(testStatusEnvelope))
	}))
	defer server.Close()
	client := testClientForServer(t, server, false, testClientSettings{})
	for _, requestID := range []string{"", "contains space", "contains\nnewline", strings.Repeat("a", 65)} {
		if _, err := client.Status(context.Background(), requestID); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Errorf("invalid request ID %q was accepted: %v", requestID, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("invalid request IDs reached the network")
	}

	for _, token := range []string{"contains space", "contains\nnewline", "contains\x00nul"} {
		_, err := NewNewAPIClient(NewAPIClientOptions{
			BaseURL: "https://fixture.example", CredentialOrigin: "https://fixture.example",
			AccessToken: token, RootUserID: 1,
			AllowedHostSuffixes: []string{"fixture.example"},
		})
		if err == nil || strings.Contains(err.Error(), token) {
			t.Errorf("unsafe token was accepted or leaked: %v", err)
		}
	}
}

func TestCanonicalPositiveDecimal(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "1", want: "1", ok: true},
		{raw: "7.2000000000", want: "7.2", ok: true},
		{raw: "1e2", want: "100", ok: true},
		{raw: "1e-10", want: "0.0000000001", ok: true},
		{raw: "99999999999999999999.9999999999", want: "99999999999999999999.9999999999", ok: true},
		{raw: "0", ok: false},
		{raw: "-1", ok: false},
		{raw: "1e20", ok: false},
		{raw: "1e-11", ok: false},
	}
	for _, test := range tests {
		actual, ok := canonicalPositiveDecimal(test.raw)
		if ok != test.ok || actual != test.want {
			t.Errorf("canonicalPositiveDecimal(%q) = %q, %t; want %q, %t", test.raw, actual, ok, test.want, test.ok)
		}
	}
}

func TestOfficialStructuredInstanceRoleAndOptionalResources(t *testing.T) {
	fixedNow := time.Unix(1768622400, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":[{` +
			`"node_name":"node-1","status":"online","stale_after_seconds":90,"started_at":1768406400,"last_seen_at":1768622390,` +
			`"info":{"node":{"name":"node-1"},"role":{"is_master":true},"runtime":{"version":"go1.25","goos":"linux","goarch":"amd64"},"host":{"hostname":"host-1"}}}]}`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	baseURL := logicalTestBaseURL(t, server, "fixture.example", "http")
	client, err := newNewAPIClient(NewAPIClientOptions{
		BaseURL: baseURL, CredentialOrigin: baseURL, AccessToken: "test-root-token", RootUserID: 1,
		AllowedHostSuffixes: []string{"fixture.example"},
	}, newAPIClientDependencies{
		transport: upstreamTransportDependencies{
			resolver: staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
			dialer:   redirectingUpstreamDialer{target: parsed.Host},
		},
		now: func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := client.Instances(context.Background(), "structured-role")
	if err != nil || len(instances) != 1 || instances[0].IsMaster == nil || !*instances[0].IsMaster ||
		instances[0].CPUPercent != nil || instances[0].StorageTotalBytes != nil {
		t.Fatalf("structured instance contract failed: %+v, %v", instances, err)
	}
}

func testClientForServer(t *testing.T, server *httptest.Server, credentials bool, settings testClientSettings) *NewAPIClient {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	settings.withCredentials = credentials
	return newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
		staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
		redirectingUpstreamDialer{target: parsed.Host}, settings)
}

func TestParseRetryAfterPresenceAndBounds(t *testing.T) {
	now := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name    string
		raw     string
		want    time.Duration
		present bool
	}{
		{name: "ten_minutes", raw: "600", want: 10 * time.Minute, present: true},
		{name: "two_hours_capped", raw: "7200", want: time.Hour, present: true},
		{name: "zero_is_valid", raw: "0", present: true},
		{name: "missing", raw: ""},
		{name: "malformed", raw: "later"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, present := parseRetryAfter(test.raw, now)
			if got != test.want || present != test.present {
				t.Fatalf("parse Retry-After %q = %s/%t, want %s/%t", test.raw, got, present, test.want, test.present)
			}
		})
	}
}

func TestNewAPIClientClassifiesUsageAuthorizationHTTPStatuses(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		kind   UpstreamErrorKind
		cause  error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, kind: UpstreamErrorAuthExpired, cause: ErrUpstreamAuthExpired},
		{name: "forbidden", status: http.StatusForbidden, kind: UpstreamErrorPermissionDenied, cause: ErrUpstreamPermissionDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
			}))
			defer server.Close()
			client := testClientForServer(t, server, true, testClientSettings{})
			_, err := client.FlowHour(context.Background(), "usage-auth-status", f02HourStart)
			var requestError *UpstreamRequestError
			if !errors.As(err, &requestError) || !errors.Is(err, test.cause) ||
				requestError.Kind != test.kind || requestError.StatusCode != test.status {
				t.Fatalf("usage HTTP %d error = %#v, %v", test.status, requestError, err)
			}
		})
	}
}

func upstreamSelfEnvelope() string {
	return `{"success":true,"message":"","data":{"id":1,"username":"root","display_name":"Root","role":100,"status":1,"group":"default"}}`
}
