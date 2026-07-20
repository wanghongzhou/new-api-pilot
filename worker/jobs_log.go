package worker

import (
	"context"

	"new-api-pilot/constant"
	"new-api-pilot/model"
)

type UpstreamLogCollector interface {
	ExecuteScheduledLogTask(context.Context, int64, int, string) (int64, int64, error)
}

func LogJobHandlers(collector UpstreamLogCollector) map[string]JobHandler {
	if collector == nil {
		return nil
	}
	return map[string]JobHandler{
		constant.TaskTypeLogSync: JobHandlerFunc(func(ctx context.Context, execution JobExecution) (JobOutcome, error) {
			run := execution.Claim.Run
			if run.TaskType != constant.TaskTypeLogSync || run.SiteID == nil || *run.SiteID <= 0 || execution.Window != nil {
				return JobOutcome{}, model.ErrCollectionRunContract
			}
			fetched, written, err := collector.ExecuteScheduledLogTask(ctx, *run.SiteID, run.SiteConfigVersion, execution.RequestID)
			if err != nil {
				return JobOutcome{}, err
			}
			if fetched < 0 || written < 0 {
				return JobOutcome{}, model.ErrCollectionRunContract
			}
			return JobOutcome{Result: JobResult{FetchedRows: fetched, WrittenRows: written}}, nil
		}),
	}
}
