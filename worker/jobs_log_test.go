package worker

import (
	"context"
	"errors"
	"testing"

	"new-api-pilot/constant"
	"new-api-pilot/model"
)

type recordingLogCollector struct {
	siteID    int64
	version   int
	requestID string
	err       error
}

func (collector *recordingLogCollector) ExecuteScheduledLogTask(_ context.Context, siteID int64, version int, requestID string) (int64, int64, error) {
	collector.siteID, collector.version, collector.requestID = siteID, version, requestID
	return 5, 4, collector.err
}

func TestLogJobHandlerUsesClaimFenceAndPropagatesRetryableFailure(t *testing.T) {
	collector := &recordingLogCollector{}
	handler := LogJobHandlers(collector)[constant.TaskTypeLogSync]
	siteID := int64(9)
	outcome, err := handler.Execute(context.Background(), JobExecution{Claim: model.CollectionTaskClaim{Run: model.CollectionRun{
		TaskType: constant.TaskTypeLogSync, SiteID: &siteID, SiteConfigVersion: 3,
	}}, RequestID: "wrk-log"})
	if err != nil || outcome.Result.FetchedRows != 5 || outcome.Result.WrittenRows != 4 || collector.siteID != 9 || collector.version != 3 || collector.requestID != "wrk-log" {
		t.Fatalf("log job outcome=%+v collector=%+v err=%v", outcome, collector, err)
	}
	collector.err = errors.New("temporary upstream failure")
	if _, err := handler.Execute(context.Background(), JobExecution{Claim: model.CollectionTaskClaim{Run: model.CollectionRun{
		TaskType: constant.TaskTypeLogSync, SiteID: &siteID, SiteConfigVersion: 3,
	}}, RequestID: "wrk-log-retry"}); err == nil {
		t.Fatal("log handler swallowed retryable failure")
	}
}
