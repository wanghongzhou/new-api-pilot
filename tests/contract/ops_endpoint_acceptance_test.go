package contract

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/router"
)

func TestA48OpsEndpointAcceptance(t *testing.T) {
	t.Run("health_readiness_and_live_ready_metric", testA48LiveReadiness)
	t.Run("redis_failure_is_readyz_only", testA48RedisReadinessFailure)
	t.Run("metrics_readiness_concurrency_and_timeout", testA48MetricsReadinessConcurrencyAndTimeout)
	t.Run("readiness_times_out_noncooperative_check", testA48ReadinessTimesOutNoncooperativeCheck)
	t.Run("metrics_CIDR_and_forwarded_for_boundary", testA48MetricsCIDR)
	t.Run("metric_family_type_label_and_secret_contract", testA48MetricContract)
}

func testA48RedisReadinessFailure(t *testing.T) {
	readiness := readyA48Readiness(func(context.Context) error { return nil })
	readiness.AddCheck("redis", func(context.Context) error { return errors.New("redis unavailable") })
	engine := newA48Engine(t, readiness, common.NewMetrics(), nil, "127.0.0.1/32")

	assertA48Status(t, engine, "/healthz", "127.0.0.1:1000", http.StatusOK, nil)
	body := assertA48Status(t, engine, "/readyz", "127.0.0.1:1000", http.StatusServiceUnavailable, nil)
	if !strings.Contains(body, `"failed_checks":["redis"]`) {
		t.Fatalf("redis readiness body = %s", body)
	}
}

func testA48MetricsReadinessConcurrencyAndTimeout(t *testing.T) {
	ready := readyA48Readiness(func(context.Context) error { return nil })
	engine := newA48Engine(t, ready, common.NewMetrics(), nil, "127.0.0.1/32")
	var wait sync.WaitGroup
	failures := make(chan string, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			request.RemoteAddr = "127.0.0.1:1000"
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "new_api_pilot_ready 1") {
				failures <- response.Body.String()
			}
		}()
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Errorf("concurrent metrics scrape failed: %s", failure)
	}

	timedOut := readyA48Readiness(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	timeoutEngine := newA48Engine(t, timedOut, common.NewMetrics(), nil, "127.0.0.1/32")
	started := time.Now()
	body := assertA48Status(t, timeoutEngine, "/metrics", "127.0.0.1:1000", http.StatusOK, nil)
	duration := time.Since(started)
	if duration < 1800*time.Millisecond || duration > 3*time.Second {
		t.Fatalf("metrics readiness duration=%s want bounded near 2s", duration)
	}
	assertA48ReadyMetric(t, body, "0")
}

func testA48ReadinessTimesOutNoncooperativeCheck(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	readiness := readyA48Readiness(func(context.Context) error {
		close(started)
		<-release
		close(finished)
		return nil
	})
	engine := newA48Engine(t, readiness, common.NewMetrics(), nil, "127.0.0.1/32")

	startedAt := time.Now()
	body := assertA48Status(t, engine, "/readyz", "127.0.0.1:1000", http.StatusServiceUnavailable, nil)
	if elapsed := time.Since(startedAt); elapsed < 1800*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("noncooperative readiness duration=%s want bounded near 2s", elapsed)
	}
	if !strings.Contains(body, `"database"`) {
		t.Fatalf("timed out readiness response omitted database failure: %s", body)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("noncooperative readiness check did not start")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("noncooperative readiness check did not finish after release")
	}
}

func testA48LiveReadiness(t *testing.T) {
	var databaseUp atomic.Bool
	databaseUp.Store(true)
	readiness := readyA48Readiness(func(context.Context) error {
		if !databaseUp.Load() {
			return errors.New("database unavailable")
		}
		return nil
	})
	metrics := common.NewMetrics()
	engine := newA48Engine(t, readiness, metrics, nil, "127.0.0.1/32")

	assertA48Status(t, engine, "/healthz", "127.0.0.1:1000", http.StatusOK, nil)
	assertA48Status(t, engine, "/readyz", "127.0.0.1:1000", http.StatusOK, nil)
	body := assertA48Status(t, engine, "/metrics", "127.0.0.1:1000", http.StatusOK, nil)
	assertA48ReadyMetric(t, body, "1")

	databaseUp.Store(false)
	assertA48Status(t, engine, "/healthz", "127.0.0.1:1000", http.StatusOK, nil)
	body = assertA48Status(t, engine, "/metrics", "127.0.0.1:1000", http.StatusOK, nil)
	assertA48ReadyMetric(t, body, "0")
	notReady := assertA48Status(t, engine, "/readyz", "127.0.0.1:1000", http.StatusServiceUnavailable, nil)
	if !strings.Contains(notReady, `"database"`) {
		t.Fatalf("database failure is absent from readiness response: %s", notReady)
	}

	databaseUp.Store(true)
	body = assertA48Status(t, engine, "/metrics", "127.0.0.1:1000", http.StatusOK, nil)
	assertA48ReadyMetric(t, body, "1")
}

func testA48MetricsCIDR(t *testing.T) {
	readiness := readyA48Readiness(func(context.Context) error { return nil })
	engine := newA48Engine(t, readiness, common.NewMetrics(), []string{"10.48.0.0/24"}, "127.0.0.1/32")

	assertA48Status(t, engine, "/metrics", "127.0.0.1:1000", http.StatusOK, nil)
	assertA48Status(t, engine, "/metrics", "192.0.2.48:1000", http.StatusForbidden, map[string]string{
		"X-Forwarded-For": "127.0.0.1",
		"X-Real-IP":       "127.0.0.1",
	})
	assertA48Status(t, engine, "/healthz", "192.0.2.48:1000", http.StatusOK, nil)
	assertA48Status(t, engine, "/readyz", "192.0.2.48:1000", http.StatusOK, nil)
}

func testA48MetricContract(t *testing.T) {
	metrics := common.NewMetrics()
	metrics.ObserveHTTP("GET", "/healthz", http.StatusOK, time.Millisecond)
	metrics.ObserveUpstream("status", "success", time.Millisecond)
	metrics.SetDBStats(sql.DBStats{MaxOpenConnections: 4, Idle: 2, InUse: 1})
	metrics.SetTaskQueueDepth("usage", "pending", 1)
	metrics.SetTaskOldestAge("usage", time.Second)
	metrics.IncrementTaskAttempt("usage_hour", "success")
	metrics.IncrementCollectionEvent("usage", "claim", "success")
	metrics.SetCollectionWindowCount("complete", 1)
	metrics.SetCollectionLag(time.Second)
	metrics.SetCollectionStaleSites(1)
	metrics.SetSchedulerHeartbeat(time.Unix(1_768_665_599, 0))
	metrics.SetClockOffset(1)
	metrics.SetAlertEventCount("critical", "firing", 1)
	metrics.IncrementAlertTransition("critical", "firing")
	metrics.SetAlertDeliveryCount("failed", 1)
	metrics.IncrementAlertDelivery("dingtalk", "failed")
	metrics.SetExportJobCount("running", 1)
	metrics.IncrementExportEvent("completion", "success")
	metrics.ObserveExport("csv", "success", 10, time.Second)
	metrics.SetExportFilesystem(10, 100)
	metrics.SetRuntimeReady("application", true)
	metrics.IncrementRuntimeRecovery("collection", "success")
	metrics.SetReady(true)

	metrics.ObserveHTTP("TOKEN-a48-secret", "/api/sites/48?token=a48-secret", 799, time.Second)
	metrics.ObserveUpstream("https://secret.example/api", "request-id-a48-secret", time.Second)
	metrics.SetRuntimeReady("site-48", true)

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status=%d", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{
		"TOKEN-a48-secret", "token=a48-secret", "secret.example", "request-id-a48-secret", "site-48",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics leaked %q", forbidden)
		}
	}

	expected := a48MetricContract()
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse Prometheus text: %v", err)
	}
	for name, contract := range expected {
		family, exists := families[name]
		if !exists {
			t.Errorf("missing metric family %s", name)
			continue
		}
		if got := family.GetType().String(); got != contract.metricType {
			t.Errorf("metric %s type=%s want %s", name, got, contract.metricType)
		}
		for _, metric := range family.Metric {
			labels := make([]string, 0, len(metric.Label))
			for _, pair := range metric.Label {
				labels = append(labels, pair.GetName())
			}
			sort.Strings(labels)
			if !reflect.DeepEqual(labels, contract.labels) {
				t.Errorf("metric %s labels=%v want %v", name, labels, contract.labels)
			}
		}
	}
	for name := range families {
		if strings.HasPrefix(name, "new_api_pilot_") {
			if _, expectedFamily := expected[name]; !expectedFamily {
				t.Errorf("unexpected new-api-pilot metric family %s", name)
			}
		}
	}
}

type a48MetricSpec struct {
	metricType string
	labels     []string
}

func a48MetricContract() map[string]a48MetricSpec {
	return map[string]a48MetricSpec{
		"new_api_pilot_http_requests_total":                   {"COUNTER", []string{"method", "route", "status_class"}},
		"new_api_pilot_http_request_duration_seconds":         {"HISTOGRAM", []string{"method", "route", "status_class"}},
		"new_api_pilot_upstream_requests_total":               {"COUNTER", []string{"operation", "result"}},
		"new_api_pilot_upstream_request_duration_seconds":     {"HISTOGRAM", []string{"operation", "result"}},
		"new_api_pilot_db_connections":                        {"GAUGE", []string{"state"}},
		"new_api_pilot_task_queue_depth":                      {"GAUGE", []string{"queue", "status"}},
		"new_api_pilot_task_oldest_age_seconds":               {"GAUGE", []string{"queue"}},
		"new_api_pilot_task_attempts_total":                   {"COUNTER", []string{"result", "task_type"}},
		"new_api_pilot_collection_events_total":               {"COUNTER", []string{"event", "queue", "result"}},
		"new_api_pilot_collection_windows":                    {"GAUGE", []string{"status"}},
		"new_api_pilot_collection_lag_seconds":                {"GAUGE", []string{}},
		"new_api_pilot_collection_stale_sites":                {"GAUGE", []string{}},
		"new_api_pilot_scheduler_heartbeat_timestamp_seconds": {"GAUGE", []string{}},
		"new_api_pilot_clock_offset_seconds":                  {"GAUGE", []string{}},
		"new_api_pilot_alert_events":                          {"GAUGE", []string{"level", "status"}},
		"new_api_pilot_alert_transitions_total":               {"COUNTER", []string{"level", "transition"}},
		"new_api_pilot_alert_delivery_queue":                  {"GAUGE", []string{"status"}},
		"new_api_pilot_alert_deliveries_total":                {"COUNTER", []string{"channel", "result"}},
		"new_api_pilot_export_jobs":                           {"GAUGE", []string{"status"}},
		"new_api_pilot_export_events_total":                   {"COUNTER", []string{"event", "result"}},
		"new_api_pilot_export_bytes_total":                    {"COUNTER", []string{"format"}},
		"new_api_pilot_export_duration_seconds":               {"HISTOGRAM", []string{"format", "result"}},
		"new_api_pilot_export_free_bytes":                     {"GAUGE", []string{}},
		"new_api_pilot_export_total_bytes":                    {"GAUGE", []string{}},
		"new_api_pilot_runtime_ready":                         {"GAUGE", []string{"component"}},
		"new_api_pilot_runtime_recoveries_total":              {"COUNTER", []string{"component", "result"}},
		"new_api_pilot_ready":                                 {"GAUGE", []string{}},
	}
}

func readyA48Readiness(databaseCheck common.ReadinessCheck) *common.Readiness {
	readiness := common.NewReadiness()
	readiness.AddCheck("database", databaseCheck)
	readiness.SetInitialized(true)
	readiness.SetSchedulerReady(true)
	return readiness
}

func newA48Engine(
	t *testing.T,
	readiness *common.Readiness,
	metrics *common.Metrics,
	trustedProxies []string,
	metricsCIDR string,
) http.Handler {
	t.Helper()
	engine, err := router.New(router.Options{
		Config: config.Config{
			AppEnv:              config.EnvironmentTest,
			TrustedProxies:      trustedProxies,
			MetricsAllowedCIDRs: []netip.Prefix{netip.MustParsePrefix(metricsCIDR)},
		},
		Readiness: readiness,
		Metrics:   metrics,
	})
	if err != nil {
		t.Fatalf("create A48 router: %v", err)
	}
	return engine
}

func assertA48Status(
	t *testing.T,
	handler http.Handler,
	target, remote string,
	want int,
	headers map[string]string,
) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.RemoteAddr = remote
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("GET %s from %s status=%d want=%d body=%s", target, remote, response.Code, want, response.Body.String())
	}
	return response.Body.String()
}

func assertA48ReadyMetric(t *testing.T, body, want string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "new_api_pilot_ready ") {
			if got := strings.TrimSpace(strings.TrimPrefix(line, "new_api_pilot_ready ")); got != want {
				t.Fatalf("new_api_pilot_ready=%s want %s", got, want)
			}
			return
		}
	}
	t.Fatal("new_api_pilot_ready sample is missing")
}
