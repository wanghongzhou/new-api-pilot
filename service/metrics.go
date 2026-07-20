package service

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

type UpstreamMetricsRecorder interface {
	ObserveUpstream(operation, result string, duration time.Duration)
}

type AlertTransitionMetricsRecorder interface {
	IncrementAlertTransition(level, transition string)
}

type AlertDeliveryMetricsRecorder interface {
	IncrementAlertDelivery(channel, result string)
	AddAlertDeliveries(channel, result string, value float64)
}

func recordServiceMetric(record func()) {
	if record == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	record()
}

func upstreamMetricOperation(method, endpoint string) string {
	switch {
	case method == http.MethodGet && endpoint == "/api/status":
		return "status"
	case method == http.MethodGet && endpoint == "/api/user/self":
		return "self"
	case method == http.MethodGet && endpoint == "/api/user/":
		return "list_users"
	case method == http.MethodGet && endpoint == "/api/user/search":
		return "search_users"
	case method == http.MethodGet && strings.HasPrefix(endpoint, "/api/user/") && endpoint != "/api/user/token":
		return "get_user"
	case method == http.MethodGet && endpoint == "/api/channel/":
		return "list_channels"
	case method == http.MethodGet && endpoint == "/api/data/flow":
		return "flow"
	case method == http.MethodGet && endpoint == "/api/data":
		return "data"
	case method == http.MethodGet && endpoint == "/api/system-info/instances":
		return "instances"
	case method == http.MethodGet && endpoint == "/api/log/stat":
		return "log_stat"
	case method == http.MethodPost && endpoint == "/api/user/login":
		return "login"
	case method == http.MethodGet && endpoint == "/api/user/token":
		return "token"
	default:
		return "other"
	}
}

func upstreamMetricResult(err error) string {
	if err == nil {
		return "success"
	}
	var requestError *UpstreamRequestError
	if errors.As(err, &requestError) && requestError.Kind != "" {
		return string(requestError.Kind)
	}
	return "response_invalid"
}
