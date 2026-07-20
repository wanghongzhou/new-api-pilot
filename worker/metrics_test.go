package worker

import (
	"errors"
	"testing"
	"time"

	"new-api-pilot/model"
	"new-api-pilot/service"
)

type recordingExecutorMetrics struct {
	taskType         string
	taskResult       string
	queue            string
	collectionEvent  string
	collectionResult string
}

func (metrics *recordingExecutorMetrics) IncrementTaskAttempt(taskType, result string) {
	metrics.taskType, metrics.taskResult = taskType, result
}

func (metrics *recordingExecutorMetrics) IncrementCollectionEvent(queue, event, result string) {
	metrics.queue, metrics.collectionEvent, metrics.collectionResult = queue, event, result
}

func TestExecutorMetricsClassifyPersistedOutcomes(t *testing.T) {
	for _, test := range []struct {
		status          string
		result          string
		wantTaskResult  string
		wantEvent       string
		wantEventResult string
	}{
		{status: model.CollectionTaskStatusSuccess, wantTaskResult: "success", wantEvent: "completion", wantEventResult: "success"},
		{status: model.CollectionTaskStatusPending, wantTaskResult: "retry", wantEvent: "retry", wantEventResult: "pending"},
		{status: model.CollectionTaskStatusFailed, wantTaskResult: "failed", wantEvent: "completion", wantEventResult: "failed"},
		{status: model.CollectionTaskStatusPending, result: "retry", wantTaskResult: "retry", wantEvent: "retry", wantEventResult: "pending"},
		{status: model.CollectionTaskStatusPending, result: "released", wantTaskResult: "released", wantEvent: "retry", wantEventResult: "pending"},
	} {
		metrics := &recordingExecutorMetrics{}
		executor := &Executor{metrics: metrics}
		executor.recordOutcome("usage_hour", test.status, test.result)
		if metrics.taskResult != test.wantTaskResult || metrics.collectionEvent != test.wantEvent ||
			metrics.collectionResult != test.wantEventResult || metrics.queue != "usage" {
			t.Errorf("outcome %q/%q recorded %+v", test.status, test.result, metrics)
		}
	}
}

type recordingExportMetrics struct {
	event  string
	result string
	count  int
}

func (metrics *recordingExportMetrics) IncrementExportEvent(event, result string) {
	metrics.event, metrics.result = event, result
	metrics.count++
}

func (*recordingExportMetrics) AddExportEvents(string, string, float64) {}

func (*recordingExportMetrics) ObserveExport(string, string, int64, time.Duration) {}

func (*recordingExportMetrics) SetRuntimeReady(string, bool) {}

func (*recordingExportMetrics) IncrementRuntimeRecovery(string, string) {}

func TestExportClaimLostRecordsStableFailureEventOnce(t *testing.T) {
	metrics := &recordingExportMetrics{}
	runtime := &ExportRuntime{metrics: metrics}
	if err := runtime.lostClaim(model.ErrExportClaimLost); err != nil {
		t.Fatalf("lost claim changed business result: %v", err)
	}
	if metrics.count != 1 || metrics.event != "failure" || metrics.result != "lost" {
		t.Fatalf("lost claim metric = %+v", metrics)
	}
	ordinary := errors.New("repository unavailable")
	if err := runtime.lostClaim(ordinary); !errors.Is(err, ordinary) {
		t.Fatalf("ordinary error was suppressed: %v", err)
	}
	if metrics.count != 1 {
		t.Fatalf("ordinary error emitted a claim-lost event: %+v", metrics)
	}
}

func TestExportFailureMetricResult(t *testing.T) {
	claim := model.ExportClaim{Job: model.ExportJob{AttemptCount: 1}}
	if result := exportFailureMetricResult(claim, errors.New("temporary")); result != "pending" {
		t.Fatalf("temporary first failure = %q", result)
	}
	claim.Job.AttemptCount = 2
	if result := exportFailureMetricResult(claim, errors.New("temporary")); result != "exhausted" {
		t.Fatalf("temporary exhausted failure = %q", result)
	}
	if result := exportFailureMetricResult(claim, service.ErrExportContract); result != "failed" {
		t.Fatalf("contract failure = %q", result)
	}
	if result := exportFailureMetricResult(claim, model.ErrExportClaimLost); result != "lost" {
		t.Fatalf("lost claim = %q", result)
	}
}

func TestRecordWorkerMetricRecoversPanic(t *testing.T) {
	reached := false
	recordWorkerMetric(func() { panic("metrics unavailable") })
	reached = true
	if !reached {
		t.Fatal("recorder panic escaped worker metric boundary")
	}
}
