package worker

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type staticOperationalSnapshotter struct {
	snapshot    model.OperationalMetricsSnapshot
	err         error
	hadDeadline bool
}

func (snapshotter *staticOperationalSnapshotter) Snapshot(
	ctx context.Context,
) (model.OperationalMetricsSnapshot, error) {
	_, snapshotter.hadDeadline = ctx.Deadline()
	return snapshotter.snapshot, snapshotter.err
}

func TestMetricsSamplerPublishesAggregateSnapshotAndFixedZeros(t *testing.T) {
	now := time.Unix(20_000, 0)
	oldestUsage := now.Add(-901 * time.Second).Unix()
	completeHour := int64(14_400)
	recentStart := now.Add(-5 * time.Minute).Unix()
	snapshotter := &staticOperationalSnapshotter{snapshot: model.OperationalMetricsSnapshot{
		Tasks: []model.OperationalTaskState{
			{TaskType: constant.TaskTypeUsageHour, Status: "pending", Count: 2, OldestCreatedAt: &oldestUsage},
			{TaskType: constant.TaskTypeUserSync, Status: "running", Count: 3},
		},
		Windows: []model.OperationalStatusCount{{Status: "complete", Count: 4}},
		EligibleSites: []model.OperationalEligibleSite{
			{NewestCompleteHour: &completeHour},
			{MonitoringStartAt: &recentStart},
		},
		Alerts:          []model.OperationalAlertCount{{Level: "critical", Status: "firing", Count: 1}},
		AlertDeliveries: []model.OperationalStatusCount{{Status: "failed", Count: 2}},
		Exports:         []model.OperationalStatusCount{{Status: "running", Count: 3}},
		DatabaseUnix:    now.Add(3 * time.Second).Unix(),
	}}
	database, err := sql.Open("mysql", "metrics:metrics@tcp(127.0.0.1:1)/unused")
	if err != nil {
		t.Fatalf("open inert metrics DB: %v", err)
	}
	defer database.Close()
	metrics := common.NewMetrics()
	sampler, err := NewMetricsSampler(MetricsSamplerOptions{
		Snapshotter: snapshotter, Database: database, Recorder: metrics,
		Clock: testsupport.NewFakeClock(now), ExportDir: t.TempDir(),
		FilesystemUsage: func(string) (uint64, uint64, error) { return 5, 10, nil },
	})
	if err != nil {
		t.Fatalf("create metrics sampler: %v", err)
	}
	if err := sampler.RunOnce(context.Background()); err != nil {
		t.Fatalf("sample operational metrics: %v", err)
	}
	if !snapshotter.hadDeadline {
		t.Fatal("snapshot query did not receive a bounded context")
	}

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{
		`new_api_pilot_task_queue_depth{queue="usage",status="pending"} 2`,
		`new_api_pilot_task_queue_depth{queue="probe",status="pending"} 0`,
		`new_api_pilot_task_oldest_age_seconds{queue="usage"} 901`,
		`new_api_pilot_collection_windows{status="complete"} 4`,
		`new_api_pilot_collection_windows{status="missing"} 0`,
		`new_api_pilot_collection_lag_seconds 2000`,
		`new_api_pilot_collection_stale_sites 1`,
		`new_api_pilot_alert_events{level="critical",status="firing"} 1`,
		`new_api_pilot_alert_delivery_queue{status="failed"} 2`,
		`new_api_pilot_export_jobs{status="running"} 3`,
		`new_api_pilot_export_free_bytes 5`,
		`new_api_pilot_export_total_bytes 10`,
		`new_api_pilot_clock_offset_seconds 3`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("metrics scrape missing %q\n%s", expected, body)
		}
	}
}

type panicOperationalRecorder struct {
	*common.Metrics
}

func (*panicOperationalRecorder) SetTaskQueueDepth(string, string, float64) {
	panic("metrics recorder unavailable")
}

func TestMetricsSamplerIsolatesRecorderPanics(t *testing.T) {
	now := time.Unix(20_000, 0)
	completeHour := int64(14_400)
	database, err := sql.Open("mysql", "metrics:metrics@tcp(127.0.0.1:1)/unused")
	if err != nil {
		t.Fatalf("open inert metrics DB: %v", err)
	}
	defer database.Close()
	metrics := common.NewMetrics()
	sampler, err := NewMetricsSampler(MetricsSamplerOptions{
		Snapshotter: &staticOperationalSnapshotter{snapshot: model.OperationalMetricsSnapshot{
			EligibleSites: []model.OperationalEligibleSite{{NewestCompleteHour: &completeHour}},
		}},
		Database: database, Recorder: &panicOperationalRecorder{Metrics: metrics},
		Clock: testsupport.NewFakeClock(now), ExportDir: t.TempDir(),
		FilesystemUsage: func(string) (uint64, uint64, error) { return 5, 10, nil },
	})
	if err != nil {
		t.Fatalf("create metrics sampler: %v", err)
	}
	if err := sampler.RunOnce(context.Background()); err != nil {
		t.Fatalf("recorder panic changed sampler result: %v", err)
	}
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(response.Body.String(), `new_api_pilot_collection_lag_seconds 2000`) {
		t.Fatalf("later metrics were skipped after recorder panic\n%s", response.Body.String())
	}
}
