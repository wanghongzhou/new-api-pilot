package common

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistrationIsPrivateAndIdempotent(t *testing.T) {
	registry := prometheus.NewRegistry()
	first, err := NewMetricsWithRegisterer(registry, registry)
	if err != nil {
		t.Fatalf("register first metrics set: %v", err)
	}
	second, err := NewMetricsWithRegisterer(registry, registry)
	if err != nil {
		t.Fatalf("register second metrics set: %v", err)
	}
	first.ObserveHTTP("GET", "/api/sites/:id", 200, time.Second)
	second.ObserveHTTP("GET", "/api/sites/:id", 200, 2*time.Second)
	response := httptest.NewRecorder()
	first.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(response.Body.String(), `new_api_pilot_http_requests_total{method="GET",route="/api/sites/:id",status_class="2xx"} 2`) {
		t.Fatalf("shared request counter was not reused\n%s", response.Body.String())
	}

	private := NewMetrics()
	private.SetReady(true)
	globalFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather default registry: %v", err)
	}
	for _, family := range globalFamilies {
		if strings.HasPrefix(family.GetName(), "new_api_pilot_") {
			t.Fatalf("private metrics leaked into default registry: %s", family.GetName())
		}
	}
}

func TestMetricsRejectIncompatibleDuplicateRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_http_requests_total",
		Help: "incompatible collector",
	}))
	if _, err := NewMetricsWithRegisterer(registry, registry); err == nil {
		t.Fatal("incompatible duplicate registration unexpectedly succeeded")
	}
}

func TestMetricLabelsAreBoundedAndDoNotLeakSensitiveValues(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveHTTP("TOKEN-secret", "/api/sites/99?token=secret", 799, -time.Second)
	metrics.ObserveUpstream("https://secret.example/api", "request-id-secret", time.Millisecond)
	metrics.SetTaskQueueDepth("site-99", "token-secret", -1)
	metrics.IncrementTaskAttempt("user-99", "request-id-secret")
	metrics.IncrementCollectionEvent("site-99", "token-secret", "request-id-secret")
	metrics.SetCollectionWindowCount("site-99", 1)
	metrics.SetAlertEventCount("site-99", "request-id-secret", 1)
	metrics.IncrementAlertTransition("site-99", "request-id-secret")
	metrics.IncrementAlertDelivery("https://secret.example/hook", "request-id-secret")
	metrics.SetExportJobCount("user-99", 1)
	metrics.IncrementExportEvent("token-secret", "request-id-secret")
	metrics.ObserveExport("model-secret", "request-id-secret", 100, time.Second)
	metrics.SetRuntimeReady("site-99", true)
	metrics.IncrementRuntimeRecovery("site-99", "request-id-secret")

	response := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/metrics", nil)
	metrics.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("metrics scrape status = %d", response.Code)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"TOKEN-secret", "token=secret", "secret.example", "site-99", "user-99", "request-id-secret", "model-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics leaked forbidden label value %q", forbidden)
		}
	}
	for _, expected := range []string{
		`method="other",route="unmatched",status_class="other"`,
		`operation="other",result="other"`,
		`queue="other",status="other"`,
		`component="other",result="other"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics scrape missing normalized labels %q", expected)
		}
	}
}

func TestMetricsExposeRequiredCollectorFamilies(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveHTTP("GET", "/healthz", 200, time.Millisecond)
	metrics.ObserveUpstream("status", "success", time.Millisecond)
	metrics.SetDBStats(sql.DBStats{MaxOpenConnections: 10, Idle: 2, InUse: 1})
	metrics.SetTaskQueueDepth("usage", "pending", 1)
	metrics.SetTaskOldestAge("usage", time.Minute)
	metrics.IncrementTaskAttempt("usage_hour", "success")
	metrics.IncrementCollectionEvent("usage", "claim", "success")
	metrics.SetCollectionWindowCount("complete", 1)
	metrics.SetCollectionLag(time.Minute)
	metrics.SetCollectionStaleSites(1)
	metrics.SetSchedulerHeartbeat(time.Unix(1_752_400_800, 0))
	metrics.SetClockOffset(-2)
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

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	for _, name := range []string{
		"new_api_pilot_http_requests_total",
		"new_api_pilot_http_request_duration_seconds",
		"new_api_pilot_upstream_requests_total",
		"new_api_pilot_upstream_request_duration_seconds",
		"new_api_pilot_db_connections",
		"new_api_pilot_task_queue_depth",
		"new_api_pilot_task_oldest_age_seconds",
		"new_api_pilot_task_attempts_total",
		"new_api_pilot_collection_events_total",
		"new_api_pilot_collection_windows",
		"new_api_pilot_collection_lag_seconds",
		"new_api_pilot_collection_stale_sites",
		"new_api_pilot_scheduler_heartbeat_timestamp_seconds",
		"new_api_pilot_clock_offset_seconds",
		"new_api_pilot_alert_events",
		"new_api_pilot_alert_transitions_total",
		"new_api_pilot_alert_delivery_queue",
		"new_api_pilot_alert_deliveries_total",
		"new_api_pilot_export_jobs",
		"new_api_pilot_export_events_total",
		"new_api_pilot_export_bytes_total",
		"new_api_pilot_export_duration_seconds",
		"new_api_pilot_export_free_bytes",
		"new_api_pilot_export_total_bytes",
		"new_api_pilot_runtime_ready",
		"new_api_pilot_runtime_recoveries_total",
		"new_api_pilot_ready",
	} {
		if !strings.Contains(response.Body.String(), name) {
			t.Errorf("metrics scrape missing family %s", name)
		}
	}
}
