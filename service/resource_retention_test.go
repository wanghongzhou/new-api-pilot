package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type scriptedResourceRetentionRepository struct {
	requests []model.ResourceRetentionBatchRequest
	steps    map[string][]scriptedResourceRetentionStep
}

type scriptedResourceRetentionStep struct {
	result model.ResourceRetentionBatchResult
	err    error
}

func (repository *scriptedResourceRetentionRepository) CleanResourceMinuteBatch(
	_ context.Context,
	request model.ResourceRetentionBatchRequest,
) (model.ResourceRetentionBatchResult, error) {
	repository.requests = append(repository.requests, request)
	steps := repository.steps[request.Table]
	if len(steps) == 0 {
		return model.ResourceRetentionBatchResult{}, errors.New("unexpected retention batch")
	}
	step := steps[0]
	repository.steps[request.Table] = steps[1:]
	return step.result, step.err
}

func TestResourceRetentionServiceUsesExactCutoffStableCursorAndBoundedBatches(t *testing.T) {
	now := time.Unix(1_768_665_599, 0)
	clock := testsupport.NewFakeClock(now)
	repository := &scriptedResourceRetentionRepository{steps: map[string][]scriptedResourceRetentionStep{
		model.ResourceMinuteTableInstance: {
			{result: model.ResourceRetentionBatchResult{
				Scanned: 2, Deleted: 2,
				Last: model.ResourceRetentionCursor{SiteID: 1, MinuteTS: 100020, ID: 2}, HasMore: true,
			}},
			{result: model.ResourceRetentionBatchResult{
				Scanned: 1, Deleted: 1,
				Last: model.ResourceRetentionCursor{SiteID: 2, MinuteTS: 100080, ID: 3},
			}},
		},
		model.ResourceMinuteTableSite: {
			{result: model.ResourceRetentionBatchResult{
				Scanned: 1, Deleted: 1,
				Last: model.ResourceRetentionCursor{SiteID: 2, MinuteTS: 100080, ID: 4},
			}},
		},
	}}
	cleaner, err := NewResourceRetentionService(ResourceRetentionServiceOptions{
		Repository: repository, Clock: clock, BatchSize: 2, MaximumBatches: 3,
	})
	if err != nil {
		t.Fatalf("create resource retention service: %v", err)
	}
	report, err := cleaner.Clean(context.Background(), 90)
	if err != nil {
		t.Fatalf("clean resource retention: %v", err)
	}
	wantCutoff := now.Unix() - now.Unix()%60 - 90*24*60*60
	if report.Cutoff != wantCutoff || report.RetentionDays != 90 || !report.Complete {
		t.Fatalf("retention report = %#v", report)
	}
	if report.Instance.Scanned != 3 || report.Instance.Deleted != 3 ||
		report.Instance.SkippedUnfinalized != 0 || report.Instance.Batches != 2 || !report.Instance.Complete {
		t.Fatalf("instance retention report = %#v", report.Instance)
	}
	if report.Site.Scanned != 1 || report.Site.Deleted != 1 || report.Site.Batches != 1 || !report.Site.Complete {
		t.Fatalf("site retention report = %#v", report.Site)
	}
	if len(repository.requests) != 3 {
		t.Fatalf("retention requests = %d, want 3", len(repository.requests))
	}
	for _, request := range repository.requests {
		if request.Cutoff != wantCutoff || request.MaximumRows != 2 {
			t.Fatalf("retention request = %#v", request)
		}
	}
	if repository.requests[0].After != (model.ResourceRetentionCursor{}) ||
		repository.requests[1].After != (model.ResourceRetentionCursor{SiteID: 1, MinuteTS: 100020, ID: 2}) ||
		repository.requests[2].After != (model.ResourceRetentionCursor{}) {
		t.Fatalf("retention cursors = %#v", repository.requests)
	}
}

func TestResourceRetentionServiceReportsBoundedBlockedDiagnosticsWithoutCursorProgress(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_768_665_599, 0))
	repository := &scriptedResourceRetentionRepository{steps: map[string][]scriptedResourceRetentionStep{
		model.ResourceMinuteTableInstance: {{result: model.ResourceRetentionBatchResult{
			Scanned: 2, SkippedUnfinalized: 2, SkippedMissingHourly: 1, SkippedDailyNotFinal: 1,
			PendingRows: true, BlockedDiagnosticsTruncated: true,
		}}},
		model.ResourceMinuteTableSite: {{result: model.ResourceRetentionBatchResult{
			Scanned: 1, SkippedUnfinalized: 1, SkippedMissingHourly: 1,
			PendingRows: true,
		}}},
	}}
	cleaner, err := NewResourceRetentionService(ResourceRetentionServiceOptions{
		Repository: repository, Clock: clock, BatchSize: 2, MaximumBatches: 3,
	})
	if err != nil {
		t.Fatalf("create resource retention service: %v", err)
	}
	report, err := cleaner.Clean(context.Background(), 90)
	if err != nil || report.Complete || report.Instance.Complete || report.Site.Complete ||
		!report.Instance.PendingRows || !report.Site.PendingRows ||
		!report.Instance.BlockedDiagnosticsTruncated || report.Site.BlockedDiagnosticsTruncated {
		t.Fatalf("blocked retention report=%#v err=%v", report, err)
	}
	if len(repository.requests) != 2 || repository.requests[0].After != (model.ResourceRetentionCursor{}) ||
		repository.requests[1].After != (model.ResourceRetentionCursor{}) {
		t.Fatalf("blocked diagnostics advanced cursor: %#v", repository.requests)
	}
}

func TestResourceRetentionServiceFailsSafeForInvalidRetentionAndRepositoryFailure(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_768_665_599, 0))
	repository := &scriptedResourceRetentionRepository{steps: map[string][]scriptedResourceRetentionStep{}}
	cleaner, err := NewResourceRetentionService(ResourceRetentionServiceOptions{Repository: repository, Clock: clock})
	if err != nil {
		t.Fatalf("create resource retention service: %v", err)
	}
	for _, days := range []int{0, -1, 3651} {
		if _, cleanErr := cleaner.Clean(context.Background(), days); !errors.Is(cleanErr, ErrResourceRetentionInvalid) {
			t.Fatalf("retention days %d error = %v", days, cleanErr)
		}
	}
	if len(repository.requests) != 0 {
		t.Fatalf("invalid retention reached repository: %#v", repository.requests)
	}

	failure := errors.New("injected batch failure")
	repository.steps[model.ResourceMinuteTableInstance] = []scriptedResourceRetentionStep{
		{result: model.ResourceRetentionBatchResult{
			Scanned: 1, Deleted: 1,
			Last: model.ResourceRetentionCursor{SiteID: 1, MinuteTS: 100020, ID: 1}, HasMore: true,
		}},
		{err: failure},
	}
	report, cleanErr := cleaner.Clean(context.Background(), 90)
	if !errors.Is(cleanErr, failure) || report.Instance.Deleted != 1 || report.Site.Batches != 0 {
		t.Fatalf("partial failure report=%#v err=%v", report, cleanErr)
	}
	requestCount := len(repository.requests)
	repository.steps[model.ResourceMinuteTableInstance] = []scriptedResourceRetentionStep{{
		result: model.ResourceRetentionBatchResult{},
	}}
	repository.steps[model.ResourceMinuteTableSite] = []scriptedResourceRetentionStep{{
		result: model.ResourceRetentionBatchResult{},
	}}
	resumed, resumeErr := cleaner.Clean(context.Background(), 90)
	if resumeErr != nil || !resumed.Complete {
		t.Fatalf("resumed cleanup report=%#v err=%v", resumed, resumeErr)
	}
	if repository.requests[requestCount].After != (model.ResourceRetentionCursor{}) {
		t.Fatalf("resumed cleanup did not restart from stable origin: %#v", repository.requests[requestCount])
	}
}

func TestResourceRetentionServiceRejectsNonAdvancingCursorAndHardCapsWork(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_768_665_599, 0))
	repository := &scriptedResourceRetentionRepository{steps: map[string][]scriptedResourceRetentionStep{
		model.ResourceMinuteTableInstance: {
			{result: model.ResourceRetentionBatchResult{
				Scanned: 1, Deleted: 1,
				Last: model.ResourceRetentionCursor{SiteID: 1, MinuteTS: 100020, ID: 1}, HasMore: true,
			}},
			{result: model.ResourceRetentionBatchResult{
				Scanned: 1, Deleted: 1,
				Last: model.ResourceRetentionCursor{SiteID: 1, MinuteTS: 100020, ID: 1}, HasMore: true,
			}},
		},
	}}
	cleaner, err := NewResourceRetentionService(ResourceRetentionServiceOptions{
		Repository: repository, Clock: clock, BatchSize: 1, MaximumBatches: 2,
	})
	if err != nil {
		t.Fatalf("create resource retention service: %v", err)
	}
	report, cleanErr := cleaner.Clean(context.Background(), 90)
	if !errors.Is(cleanErr, ErrResourceRetentionInvalid) || report.Instance.Batches != 2 {
		t.Fatalf("non-advancing cursor report=%#v err=%v", report, cleanErr)
	}
}
