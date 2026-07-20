package worker

import (
	"context"
	"errors"
	"math"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

type UsageHourCollector interface {
	CollectHour(context.Context, service.UsageCollectionRequest) (service.UsageCollectionResult, error)
}

func UsageJobHandlers(collector UsageHourCollector) map[string]JobHandler {
	if collector == nil {
		return nil
	}
	handlers := make(map[string]JobHandler, 3)
	for _, taskType := range []string{
		constant.TaskTypeUsageHour,
		constant.TaskTypeUsageBackfill,
		constant.TaskTypeUsageValidation,
	} {
		currentTaskType := taskType
		handlers[currentTaskType] = JobHandlerFunc(func(
			ctx context.Context,
			execution JobExecution,
		) (JobOutcome, error) {
			run := execution.Claim.Run
			if run.TaskType != currentTaskType || run.SiteID == nil || execution.Window == nil ||
				execution.Claim.RequestID == "" || execution.RequestID != execution.Claim.RequestID ||
				run.LastRequestID != execution.Claim.RequestID {
				return JobOutcome{}, model.ErrCollectionRunContract
			}
			window := *execution.Window
			result, err := collector.CollectHour(ctx, service.UsageCollectionRequest{
				Run: run, Window: window, RequestID: execution.Claim.RequestID,
			})
			if err != nil {
				return JobOutcome{}, classifyUsageExecutionError(err, nil)
			}
			if result.FlowRows < 0 || result.DataRows < 0 || result.FlowRows > math.MaxInt64-result.DataRows ||
				result.SourceRequestID != execution.Claim.RequestID || !result.Commit.Valid() {
				return JobOutcome{}, model.ErrCollectionRunContract
			}
			outcome := JobOutcome{
				Result: JobResult{
					FetchedRows: result.FlowRows + result.DataRows,
					WrittenRows: result.Planned.WrittenRows,
				},
				TransactionMutation: result.Commit,
			}
			if result.Failure != nil {
				return outcome, classifyUsageExecutionError(result.Failure.Cause, result.Failure)
			}
			return outcome, nil
		})
	}
	return handlers
}

func classifyUsageExecutionError(cause error, failure *service.UsageCollectionFailure) error {
	if cause == nil {
		return &TaskExecutionError{Code: model.CollectionTaskExecutionFailedCode}
	}
	params := []byte(nil)
	if failure != nil {
		params = append(params, failure.Params...)
	}
	code := model.CollectionTaskExecutionFailedCode
	retryable := false
	retryAfter := time.Duration(0)
	switch {
	case errors.Is(cause, model.ErrSiteRunConfigChanged):
		code = constant.CodeSiteConfigChanged
	case errors.Is(cause, service.ErrUpstreamDataMismatch):
		code = string(constant.MessageDataValidationMismatch)
		retryable = true
	case errors.Is(cause, service.ErrUpstreamResponseInvalid), errors.Is(cause, service.ErrUpstreamEnvelopeInvalid):
		code = string(constant.MessageUpstreamResponseInvalid)
	case errors.Is(cause, service.ErrUpstreamResponseTooLarge):
		code = string(constant.MessageUpstreamResponseTooLarge)
	case errors.Is(cause, service.ErrUpstreamAddressForbidden):
		code = constant.CodeUpstreamAddressForbidden
	case errors.Is(cause, service.ErrUpstreamExportDisabled),
		errors.Is(cause, service.ErrUpstreamCredentialOriginMismatch),
		errors.Is(cause, service.ErrUpstreamAuthExpired),
		errors.Is(cause, service.ErrUpstreamPermissionDenied):
		if failure != nil && failure.Code != "" {
			code = failure.Code
		} else {
			code = constant.CodeUpstreamUnavailable
		}
	case errors.Is(cause, context.Canceled):
		return context.Canceled
	case errors.Is(cause, context.DeadlineExceeded),
		errors.Is(cause, service.ErrUpstreamUnavailable),
		errors.Is(cause, service.ErrUpstreamRateLimited),
		errors.Is(cause, service.ErrUpstreamRemote):
		code = string(constant.MessageDataUpstreamUnavailable)
		retryable = true
	default:
		if failure != nil && failure.Code != "" {
			code = failure.Code
		}
	}
	var requestError *service.UpstreamRequestError
	if errors.As(cause, &requestError) {
		retryAfter = requestError.RetryAfter
	}
	return &TaskExecutionError{
		Code: code, Params: params, Retryable: retryable, RetryAfter: retryAfter,
		HasRetryAfter: requestError != nil && (requestError.HasRetryAfter || requestError.RetryAfter > 0),
	}
}
