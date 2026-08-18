package worker

import (
	"context"
	"errors"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

func classifyCollectionTaskError(cause error) error {
	if cause == nil {
		return nil
	}
	var classified *TaskExecutionError
	if errors.As(cause, &classified) {
		return cause
	}

	kind := "internal_error"
	retryable := true
	switch {
	case errors.Is(cause, service.ErrUpstreamAuthExpired):
		kind, retryable = "authorization_expired", false
	case errors.Is(cause, service.ErrUpstreamPermissionDenied):
		kind, retryable = "permission_denied", false
	case errors.Is(cause, service.ErrUpstreamRateLimited):
		kind = "rate_limited"
	case errors.Is(cause, service.ErrUpstreamUnavailable), errors.Is(cause, context.DeadlineExceeded):
		kind = "upstream_unavailable"
	case errors.Is(cause, service.ErrUpstreamRemote):
		kind = "upstream_server_error"
	case errors.Is(cause, service.ErrUpstreamResponseInvalid), errors.Is(cause, service.ErrUpstreamEnvelopeInvalid):
		kind, retryable = "response_invalid", false
	case errors.Is(cause, service.ErrUpstreamResponseTooLarge):
		kind, retryable = "response_too_large", false
	case errors.Is(cause, service.ErrUpstreamExportDisabled):
		kind, retryable = "export_disabled", false
	case errors.Is(cause, service.ErrUpstreamAddressForbidden):
		kind, retryable = "address_forbidden", false
	case errors.Is(cause, service.ErrSiteConfigChanged), errors.Is(cause, model.ErrSiteRunConfigChanged), errors.Is(cause, model.ErrUpstreamLogFence):
		kind, retryable = "configuration_changed", false
	case errors.Is(cause, model.ErrCollectionRunContract):
		kind, retryable = "task_contract_invalid", false
	}

	params, err := common.Marshal(map[string]any{"failure_kind": kind})
	if err != nil {
		params = nil
	}
	result := &TaskExecutionError{
		Code:      string(constant.MessageCollectionExecutionFailed),
		Params:    params,
		Retryable: retryable,
	}
	var upstream *service.UpstreamRequestError
	if errors.As(cause, &upstream) && upstream != nil && (upstream.HasRetryAfter || upstream.RetryAfter > 0) {
		result.RetryAfter = upstream.RetryAfter
		result.HasRetryAfter = upstream.HasRetryAfter
	}
	return result
}
