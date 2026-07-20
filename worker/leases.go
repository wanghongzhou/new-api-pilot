package worker

import (
	"errors"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
)

type TaskExecutionError struct {
	Code          string
	Params        []byte
	Retryable     bool
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (err *TaskExecutionError) Error() string {
	if err == nil || err.Code == "" {
		return "collection task execution failed"
	}
	return err.Code
}

func defaultAttemptPolicy() model.CollectionTaskAttemptPolicy {
	return model.CollectionTaskAttemptPolicy{
		DefaultMaxAttempts: 1,
		MaxAttempts: map[string]int{
			constant.TaskTypeSiteProbe:        1,
			constant.TaskTypeRealtimeStat:     1,
			constant.TaskTypeResourceSnapshot: 1,
			constant.TaskTypeUserSync:         1,
			constant.TaskTypeChannelSync:      1,
			constant.TaskTypePerformanceSync:  3,
			constant.TaskTypeTopupSync:        3,
			constant.TaskTypeRedemptionSync:   3,
			constant.TaskTypeUpstreamTaskSync: 3,
			constant.TaskTypeModelMetaSync:    3,
			constant.TaskTypePlanSync:         3,
			constant.TaskTypePricingSync:      3,
			constant.TaskTypeSystemTaskSync:   3,
			constant.TaskTypeLogSync:          3,
			constant.TaskTypeUsageHour:        4,
			constant.TaskTypeUsageBackfill:    5,
			constant.TaskTypeUsageValidation:  5,
			constant.TaskTypeAccountRebuild:   5,
			constant.TaskTypeCustomerRebuild:  5,
		},
	}
}

func maxAttempts(policy model.CollectionTaskAttemptPolicy, taskType string) int {
	if value := policy.MaxAttempts[taskType]; value > 0 {
		return value
	}
	if policy.DefaultMaxAttempts > 0 {
		return policy.DefaultMaxAttempts
	}
	return 3
}

func retryDecision(taskType string, attempt int, cause error, policy model.CollectionTaskAttemptPolicy) (bool, time.Duration, string) {
	code := model.CollectionTaskExecutionFailedCode
	retryable := true
	delay := taskBackoff(taskType, attempt)
	var executionError *TaskExecutionError
	if errors.As(cause, &executionError) {
		if executionError.Code != "" {
			code = executionError.Code
		}
		retryable = executionError.Retryable
		if executionError.HasRetryAfter || executionError.RetryAfter > 0 {
			retryAfter := executionError.RetryAfter
			if retryAfter < 0 {
				retryAfter = 0
			}
			if retryAfter > time.Hour {
				retryAfter = time.Hour
			}
			delay = retryAfter
		}
	}
	if attempt >= maxAttempts(policy, taskType) {
		retryable = false
	}
	return retryable, delay, code
}

func taskBackoff(taskType string, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	switch taskType {
	case constant.TaskTypeUsageHour:
		switch attempt {
		case 1:
			return time.Minute
		case 2:
			return 5 * time.Minute
		default:
			return 15 * time.Minute
		}
	case constant.TaskTypeUsageValidation:
		switch attempt {
		case 1:
			return 5 * time.Minute
		case 2:
			return 15 * time.Minute
		case 3:
			return time.Hour
		default:
			return 6 * time.Hour
		}
	case constant.TaskTypeUsageBackfill, constant.TaskTypeAccountRebuild, constant.TaskTypeCustomerRebuild:
		switch attempt {
		case 1:
			return time.Minute
		case 2:
			return 5 * time.Minute
		case 3:
			return 15 * time.Minute
		default:
			return time.Hour
		}
	}
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}
