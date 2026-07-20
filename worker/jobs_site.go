package worker

import (
	"context"
	"fmt"

	"new-api-pilot/constant"
	"new-api-pilot/model"
)

type SitePeriodicJobRunner interface {
	ExecutePeriodicSiteTask(
		ctx context.Context,
		taskType string,
		siteID int64,
		expectedConfigVersion int,
		requestID string,
	) (fetchedRows int64, writtenRows int64, err error)
}

func SiteJobHandlers(runner SitePeriodicJobRunner) map[string]JobHandler {
	if runner == nil {
		return nil
	}
	handlers := make(map[string]JobHandler, 6)
	for _, taskType := range []string{
		constant.TaskTypeSiteProbe,
		constant.TaskTypeRealtimeStat,
		constant.TaskTypeResourceSnapshot,
		constant.TaskTypeUserSync,
		constant.TaskTypeChannelSync,
		constant.TaskTypePerformanceSync,
		constant.TaskTypeTopupSync,
		constant.TaskTypeRedemptionSync,
		constant.TaskTypeUpstreamTaskSync,
		constant.TaskTypeModelMetaSync,
		constant.TaskTypePlanSync,
		constant.TaskTypePricingSync,
		constant.TaskTypeSystemTaskSync,
	} {
		currentTaskType := taskType
		handlers[currentTaskType] = JobHandlerFunc(func(ctx context.Context, execution JobExecution) (JobOutcome, error) {
			run := execution.Claim.Run
			if run.TaskType != currentTaskType || run.SiteID == nil || *run.SiteID <= 0 || execution.Window != nil {
				return JobOutcome{}, model.ErrCollectionRunContract
			}
			fetched, written, err := runner.ExecutePeriodicSiteTask(
				ctx, currentTaskType, *run.SiteID, run.SiteConfigVersion, execution.RequestID,
			)
			if err != nil {
				return JobOutcome{}, err
			}
			if fetched < 0 || written < 0 {
				return JobOutcome{}, fmt.Errorf("periodic site task returned invalid row counts")
			}
			return JobOutcome{Result: JobResult{FetchedRows: fetched, WrittenRows: written}}, nil
		})
	}
	return handlers
}
