package worker

import (
	"context"
	"errors"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

type LocalRebuildWindowBuilder interface {
	PrepareWindow(context.Context, service.LocalRebuildRequest) (model.LocalRebuildMutation, error)
}

func LocalRebuildJobHandlers(builder LocalRebuildWindowBuilder) map[string]JobHandler {
	if builder == nil {
		return nil
	}
	handlers := make(map[string]JobHandler, 2)
	for _, taskType := range []string{constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild} {
		currentTaskType := taskType
		handlers[currentTaskType] = JobHandlerFunc(func(
			ctx context.Context,
			execution JobExecution,
		) (JobOutcome, error) {
			run := execution.Claim.Run
			if run.TaskType != currentTaskType || execution.Window == nil || run.SiteID != nil ||
				execution.RequestID == "" || execution.RequestID != execution.Claim.RequestID ||
				run.LastRequestID != execution.RequestID {
				return JobOutcome{}, model.ErrCollectionRunContract
			}
			mutation, err := builder.PrepareWindow(ctx, service.LocalRebuildRequest{
				Run: run, Window: *execution.Window, RequestID: execution.RequestID,
			})
			if errors.Is(err, model.ErrLocalRebuildDependencyPending) {
				return JobOutcome{}, &TaskExecutionError{
					Code: model.LocalRebuildDependencyPendingCode, Retryable: true,
				}
			}
			if err != nil {
				return JobOutcome{}, err
			}
			return JobOutcome{TransactionMutation: mutation}, nil
		})
	}
	return handlers
}
