package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingUpstreamMetrics struct {
	operation string
	result    string
	duration  time.Duration
	panic     bool
}

func (metrics *recordingUpstreamMetrics) ObserveUpstream(operation, result string, duration time.Duration) {
	if metrics.panic {
		panic("metrics recorder unavailable")
	}
	metrics.operation, metrics.result, metrics.duration = operation, result, duration
}

func TestUpstreamMetricsUseBoundedOperationAndResult(t *testing.T) {
	for _, test := range []struct {
		method, endpoint, want string
	}{
		{method: http.MethodGet, endpoint: "/api/status", want: "status"},
		{method: http.MethodGet, endpoint: "/api/user/9007199254740993", want: "get_user"},
		{method: http.MethodGet, endpoint: "https://secret.example/api?token=secret", want: "other"},
	} {
		if got := upstreamMetricOperation(test.method, test.endpoint); got != test.want {
			t.Errorf("operation for %s %s = %q, want %q", test.method, test.endpoint, got, test.want)
		}
	}
	for _, forbidden := range []string{"secret.example", "9007199254740993", "token=secret"} {
		if strings.Contains(upstreamMetricOperation(http.MethodGet, "https://secret.example/"+forbidden), forbidden) {
			t.Fatalf("operation leaked input %q", forbidden)
		}
	}
}

func TestUpstreamRecorderPanicDoesNotChangeRequestResult(t *testing.T) {
	manifest := loadF02Manifest(t)
	_, client := newF02Client(t, manifest.Scenarios["supported"], time.Unix(manifest.FixedNowUnix, 0))
	client.metrics = &recordingUpstreamMetrics{panic: true}
	status, err := client.Status(context.Background(), "metrics-request-id-secret")
	if err != nil {
		t.Fatalf("metrics panic changed upstream result: %v", err)
	}
	if status.Version != statusVersion {
		t.Fatalf("unexpected status after metrics panic: %+v", status)
	}
}

func TestUpstreamRecorderReceivesNoURLOrRequestID(t *testing.T) {
	manifest := loadF02Manifest(t)
	_, client := newF02Client(t, manifest.Scenarios["supported"], time.Unix(manifest.FixedNowUnix, 0))
	metrics := &recordingUpstreamMetrics{}
	client.metrics = metrics
	if _, err := client.Status(context.Background(), "metrics-request-id-secret"); err != nil {
		t.Fatalf("read upstream status: %v", err)
	}
	if metrics.operation != "status" || metrics.result != "success" || metrics.duration < 0 {
		t.Fatalf("unexpected upstream metric: %+v", metrics)
	}
	if strings.Contains(metrics.operation+metrics.result, "request-id-secret") ||
		strings.Contains(metrics.operation+metrics.result, "fixture.example") {
		t.Fatalf("upstream metric leaked request identity or URL: %+v", metrics)
	}
}
