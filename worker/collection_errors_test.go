package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

func TestClassifyCollectionTaskErrorReturnsSafeActionableKinds(t *testing.T) {
	tests := []struct {
		name      string
		cause     error
		wantKind  string
		retryable bool
	}{
		{name: "unavailable", cause: service.ErrUpstreamUnavailable, wantKind: "upstream_unavailable", retryable: true},
		{name: "timeout", cause: context.DeadlineExceeded, wantKind: "upstream_unavailable", retryable: true},
		{name: "authorization", cause: service.ErrUpstreamAuthExpired, wantKind: "authorization_expired"},
		{name: "permission", cause: service.ErrUpstreamPermissionDenied, wantKind: "permission_denied"},
		{name: "rate limited", cause: service.ErrUpstreamRateLimited, wantKind: "rate_limited", retryable: true},
		{name: "remote", cause: service.ErrUpstreamRemote, wantKind: "upstream_server_error", retryable: true},
		{name: "invalid response", cause: service.ErrUpstreamResponseInvalid, wantKind: "response_invalid"},
		{name: "large response", cause: service.ErrUpstreamResponseTooLarge, wantKind: "response_too_large"},
		{name: "export disabled", cause: service.ErrUpstreamExportDisabled, wantKind: "export_disabled"},
		{name: "address forbidden", cause: service.ErrUpstreamAddressForbidden, wantKind: "address_forbidden"},
		{name: "configuration", cause: model.ErrSiteRunConfigChanged, wantKind: "configuration_changed"},
		{name: "contract", cause: model.ErrCollectionRunContract, wantKind: "task_contract_invalid"},
		{name: "unknown", cause: errors.New("database password=secret"), wantKind: "internal_error", retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classified := classifyCollectionTaskError(test.cause)
			var executionError *TaskExecutionError
			if !errors.As(classified, &executionError) {
				t.Fatalf("classified error = %T, want TaskExecutionError", classified)
			}
			if executionError.Code != string(constant.MessageCollectionExecutionFailed) || executionError.Retryable != test.retryable {
				t.Fatalf("classified error = %#v", executionError)
			}
			var params map[string]any
			if err := common.Unmarshal(executionError.Params, &params); err != nil || len(params) != 1 || params["failure_kind"] != test.wantKind {
				t.Fatalf("classified params = %#v, %v", params, err)
			}
			if string(executionError.Params) == test.cause.Error() || string(executionError.Params) == "database password=secret" {
				t.Fatalf("classified params leaked cause: %s", executionError.Params)
			}
		})
	}
}

func TestClassifyCollectionTaskErrorKeepsSafeRetryAfter(t *testing.T) {
	cause := &service.UpstreamRequestError{
		Kind: service.UpstreamErrorRateLimited, RetryAfter: 2 * time.Minute, HasRetryAfter: true,
	}
	var executionError *TaskExecutionError
	if !errors.As(classifyCollectionTaskError(cause), &executionError) ||
		executionError.RetryAfter != 2*time.Minute || !executionError.HasRetryAfter {
		t.Fatalf("classified retry = %#v", executionError)
	}
}
