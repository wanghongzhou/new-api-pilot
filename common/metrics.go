package common

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const metricOther = "other"

var (
	httpMethodLabels        = labelSet("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS")
	upstreamOperationLabels = labelSet(
		"status", "self", "get_user", "list_users", "search_users", "list_channels",
		"flow", "data", "instances", "log_stat", "login", "token",
	)
	upstreamResultLabels = labelSet(
		"success", "address_forbidden", "unavailable", "auth_expired", "permission_denied",
		"rate_limited", "upstream_error", "response_invalid", "envelope_invalid",
		"response_too_large", "version_mismatch", "export_disabled",
		"credential_origin_mismatch", "token_rotation_result_unknown", "data_mismatch",
	)
	queueLabels      = labelSet("probe", "realtime", "resource", "metadata", "usage", "backfill")
	taskStatusLabels = labelSet("pending", "running")
	taskTypeLabels   = labelSet(
		"site_probe", "realtime_stat", "resource_snapshot", "user_sync", "channel_sync",
		"usage_hour", "usage_backfill", "usage_validation", "account_rebuild", "customer_rebuild",
	)
	taskResultLabels       = labelSet("claimed", "success", "retry", "failed", "takeover", "released", "lost", "canceled")
	collectionEventLabels  = labelSet("claim", "retry", "takeover", "completion")
	collectionResultLabels = labelSet(
		"success", "pending", "failed", "exhausted", "lost",
	)
	collectionWindowStatusLabels = labelSet("pending", "complete", "missing", "unavailable")
	alertLevelLabels             = labelSet("warning", "critical")
	alertEventStatusLabels       = labelSet("pending", "firing")
	alertTransitionLabels        = labelSet("pending", "firing", "resolved", "unchanged", "duplicate", "unknown")
	alertDeliveryStatusLabels    = labelSet("pending", "success", "failed")
	alertDeliveryResultLabels    = labelSet("success", "retry", "failed", "takeover", "disabled", "exhausted")
	alertChannelLabels           = labelSet("dingtalk")
	exportStatusLabels           = labelSet("pending", "running", "success", "failed", "expired")
	exportEventLabels            = labelSet("claim", "takeover", "retry", "completion", "failure", "cleanup")
	exportResultLabels           = labelSet("success", "pending", "failed", "exhausted", "lost")
	exportFormatLabels           = labelSet("csv", "xlsx")
	runtimeComponentLabels       = labelSet("application", "collection", "alert", "export", "metrics", "scheduler")
	runtimeRecoveryResultLabels  = labelSet("success", "failure")
)

type Metrics struct {
	gatherer prometheus.Gatherer

	httpRequests       *prometheus.CounterVec
	httpDuration       *prometheus.HistogramVec
	upstreamRequests   *prometheus.CounterVec
	upstreamDuration   *prometheus.HistogramVec
	dbConnections      *prometheus.GaugeVec
	taskQueueDepth     *prometheus.GaugeVec
	taskOldestAge      *prometheus.GaugeVec
	taskAttempts       *prometheus.CounterVec
	collectionEvents   *prometheus.CounterVec
	collectionWindows  *prometheus.GaugeVec
	collectionLag      prometheus.Gauge
	collectionStale    prometheus.Gauge
	schedulerHeartbeat prometheus.Gauge
	clockOffset        prometheus.Gauge
	alertEvents        *prometheus.GaugeVec
	alertTransitions   *prometheus.CounterVec
	alertDeliveryQueue *prometheus.GaugeVec
	alertDeliveries    *prometheus.CounterVec
	exportJobs         *prometheus.GaugeVec
	exportEvents       *prometheus.CounterVec
	exportBytes        *prometheus.CounterVec
	exportDuration     *prometheus.HistogramVec
	exportFreeBytes    prometheus.Gauge
	exportTotalBytes   prometheus.Gauge
	runtimeReady       *prometheus.GaugeVec
	runtimeRecoveries  *prometheus.CounterVec
	ready              prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetricsWithRegisterer(registry, registry)
	if err == nil {
		return metrics
	}
	// A fresh private registry cannot normally fail. Keep observability
	// optional if a future collector contract is accidentally invalid.
	return &Metrics{gatherer: prometheus.NewRegistry()}
}

func NewMetricsWithRegisterer(
	registerer prometheus.Registerer,
	gatherer prometheus.Gatherer,
) (*Metrics, error) {
	if registerer == nil || gatherer == nil {
		return nil, errors.New("metrics registerer and gatherer are required")
	}
	metrics := &Metrics{gatherer: gatherer}
	var err error
	if metrics.httpRequests, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_http_requests_total",
		Help: "HTTP requests handled by the platform.",
	}, []string{"method", "route", "status_class"})); err != nil {
		return nil, err
	}
	if metrics.httpDuration, err = registerMetric(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "new_api_pilot_http_request_duration_seconds",
		Help:    "Duration of HTTP requests handled by the platform.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status_class"})); err != nil {
		return nil, err
	}
	if metrics.upstreamRequests, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_upstream_requests_total",
		Help: "Requests to new-api sites by bounded operation and result.",
	}, []string{"operation", "result"})); err != nil {
		return nil, err
	}
	if metrics.upstreamDuration, err = registerMetric(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "new_api_pilot_upstream_request_duration_seconds",
		Help:    "Duration of requests to new-api sites.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"operation", "result"})); err != nil {
		return nil, err
	}
	if metrics.dbConnections, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_db_connections",
		Help: "Current database connection pool state.",
	}, []string{"state"})); err != nil {
		return nil, err
	}
	if metrics.taskQueueDepth, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_task_queue_depth",
		Help: "Collection tasks by bounded queue and active status.",
	}, []string{"queue", "status"})); err != nil {
		return nil, err
	}
	if metrics.taskOldestAge, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_task_oldest_age_seconds",
		Help: "Age of the oldest pending task in a bounded queue.",
	}, []string{"queue"})); err != nil {
		return nil, err
	}
	if metrics.taskAttempts, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_task_attempts_total",
		Help: "Collection task attempts by bounded task type and result.",
	}, []string{"task_type", "result"})); err != nil {
		return nil, err
	}
	if metrics.collectionEvents, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_collection_events_total",
		Help: "Collection claim, retry, takeover, and completion events.",
	}, []string{"queue", "event", "result"})); err != nil {
		return nil, err
	}
	if metrics.collectionWindows, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_collection_windows",
		Help: "Persisted collection windows by completeness status.",
	}, []string{"status"})); err != nil {
		return nil, err
	}
	if metrics.collectionLag, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_collection_lag_seconds",
		Help: "Maximum lag from the newest complete window across eligible sites.",
	})); err != nil {
		return nil, err
	}
	if metrics.collectionStale, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_collection_stale_sites",
		Help: "Number of eligible sites whose newest complete window is stale.",
	})); err != nil {
		return nil, err
	}
	if metrics.schedulerHeartbeat, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_scheduler_heartbeat_timestamp_seconds",
		Help: "Unix timestamp of the latest successful scheduler pass.",
	})); err != nil {
		return nil, err
	}
	if metrics.clockOffset, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_clock_offset_seconds",
		Help: "Database clock offset from the application clock in seconds.",
	})); err != nil {
		return nil, err
	}
	if metrics.alertEvents, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_alert_events",
		Help: "Active alert events by bounded level and status.",
	}, []string{"level", "status"})); err != nil {
		return nil, err
	}
	if metrics.alertTransitions, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_alert_transitions_total",
		Help: "Alert evaluator transitions by bounded level and transition.",
	}, []string{"level", "transition"})); err != nil {
		return nil, err
	}
	if metrics.alertDeliveryQueue, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_alert_delivery_queue",
		Help: "Alert delivery records by bounded status.",
	}, []string{"status"})); err != nil {
		return nil, err
	}
	if metrics.alertDeliveries, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_alert_deliveries_total",
		Help: "Alert delivery attempts by bounded channel and result.",
	}, []string{"channel", "result"})); err != nil {
		return nil, err
	}
	if metrics.exportJobs, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_export_jobs",
		Help: "Export jobs by bounded status.",
	}, []string{"status"})); err != nil {
		return nil, err
	}
	if metrics.exportEvents, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_export_events_total",
		Help: "Export worker lifecycle events.",
	}, []string{"event", "result"})); err != nil {
		return nil, err
	}
	if metrics.exportBytes, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_export_bytes_total",
		Help: "Bytes successfully published by export format.",
	}, []string{"format"})); err != nil {
		return nil, err
	}
	if metrics.exportDuration, err = registerMetric(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "new_api_pilot_export_duration_seconds",
		Help:    "Export attempt duration by format and result.",
		Buckets: []float64{0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"format", "result"})); err != nil {
		return nil, err
	}
	if metrics.exportFreeBytes, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_export_free_bytes",
		Help: "Free bytes on the export filesystem.",
	})); err != nil {
		return nil, err
	}
	if metrics.exportTotalBytes, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_export_total_bytes",
		Help: "Total bytes on the export filesystem.",
	})); err != nil {
		return nil, err
	}
	if metrics.runtimeReady, err = registerMetric(registerer, prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "new_api_pilot_runtime_ready",
		Help: "Readiness of each bounded runtime component.",
	}, []string{"component"})); err != nil {
		return nil, err
	}
	if metrics.runtimeRecoveries, err = registerMetric(registerer, prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_api_pilot_runtime_recoveries_total",
		Help: "Runtime recovery passes by component and result.",
	}, []string{"component", "result"})); err != nil {
		return nil, err
	}
	if metrics.ready, err = registerMetric(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "new_api_pilot_ready",
		Help: "Whether the platform is ready to serve traffic.",
	})); err != nil {
		return nil, err
	}
	return metrics, nil
}

func registerMetric[T prometheus.Collector](registerer prometheus.Registerer, collector T) (T, error) {
	var zero T
	err := registerer.Register(collector)
	if err == nil {
		return collector, nil
	}
	var already prometheus.AlreadyRegisteredError
	if !errors.As(err, &already) {
		return zero, fmt.Errorf("register prometheus collector: %w", err)
	}
	existing, ok := any(already.ExistingCollector).(T)
	if !ok {
		return zero, fmt.Errorf("prometheus collector already registered with incompatible type %T", already.ExistingCollector)
	}
	return existing, nil
}

func (m *Metrics) Handler() http.Handler {
	if m == nil || m.gatherer == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.gatherer, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

func (m *Metrics) ObserveHTTP(method, route string, status int, duration time.Duration) {
	if m == nil || m.httpRequests == nil || m.httpDuration == nil {
		return
	}
	method = normalizeLabel(httpMethodLabels, strings.ToUpper(strings.TrimSpace(method)))
	route = normalizeHTTPRoute(route)
	statusClass := metricOther
	if status >= 100 && status <= 599 {
		statusClass = strconv.Itoa(status/100) + "xx"
	}
	seconds := nonNegativeDuration(duration).Seconds()
	m.httpRequests.WithLabelValues(method, route, statusClass).Inc()
	m.httpDuration.WithLabelValues(method, route, statusClass).Observe(seconds)
}

func (m *Metrics) ObserveUpstream(operation, result string, duration time.Duration) {
	if m == nil || m.upstreamRequests == nil || m.upstreamDuration == nil {
		return
	}
	operation = normalizeLabel(upstreamOperationLabels, operation)
	result = normalizeLabel(upstreamResultLabels, result)
	m.upstreamRequests.WithLabelValues(operation, result).Inc()
	m.upstreamDuration.WithLabelValues(operation, result).Observe(nonNegativeDuration(duration).Seconds())
}

func (m *Metrics) SetDBStats(stats sql.DBStats) {
	if m == nil || m.dbConnections == nil {
		return
	}
	m.dbConnections.WithLabelValues("idle").Set(float64(stats.Idle))
	m.dbConnections.WithLabelValues("in_use").Set(float64(stats.InUse))
	m.dbConnections.WithLabelValues("max").Set(float64(stats.MaxOpenConnections))
}

func (m *Metrics) SetReady(ready bool) {
	if m == nil || m.ready == nil {
		return
	}
	m.ready.Set(boolFloat(ready))
}

func (m *Metrics) SetRuntimeReady(component string, ready bool) {
	if m == nil || m.runtimeReady == nil {
		return
	}
	component = normalizeLabel(runtimeComponentLabels, component)
	m.runtimeReady.WithLabelValues(component).Set(boolFloat(ready))
}

func (m *Metrics) IncrementRuntimeRecovery(component, result string) {
	if m == nil || m.runtimeRecoveries == nil {
		return
	}
	component = normalizeLabel(runtimeComponentLabels, component)
	result = normalizeLabel(runtimeRecoveryResultLabels, result)
	m.runtimeRecoveries.WithLabelValues(component, result).Inc()
}

func (m *Metrics) SetSchedulerHeartbeat(timestamp time.Time) {
	if m == nil || m.schedulerHeartbeat == nil || timestamp.IsZero() {
		return
	}
	m.schedulerHeartbeat.Set(float64(timestamp.Unix()))
}

func (m *Metrics) SetClockOffset(seconds float64) {
	if m == nil || m.clockOffset == nil {
		return
	}
	m.clockOffset.Set(seconds)
}

func (m *Metrics) SetTaskQueueDepth(queue, status string, value float64) {
	if m == nil || m.taskQueueDepth == nil {
		return
	}
	queue = normalizeLabel(queueLabels, queue)
	status = normalizeLabel(taskStatusLabels, status)
	m.taskQueueDepth.WithLabelValues(queue, status).Set(nonNegativeFloat(value))
}

func (m *Metrics) SetTaskOldestAge(queue string, age time.Duration) {
	if m == nil || m.taskOldestAge == nil {
		return
	}
	queue = normalizeLabel(queueLabels, queue)
	m.taskOldestAge.WithLabelValues(queue).Set(nonNegativeDuration(age).Seconds())
}

func (m *Metrics) IncrementTaskAttempt(taskType, result string) {
	if m == nil || m.taskAttempts == nil {
		return
	}
	taskType = normalizeLabel(taskTypeLabels, taskType)
	result = normalizeLabel(taskResultLabels, result)
	m.taskAttempts.WithLabelValues(taskType, result).Inc()
}

func (m *Metrics) IncrementCollectionEvent(queue, event, result string) {
	m.AddCollectionEvents(queue, event, result, 1)
}

func (m *Metrics) AddCollectionEvents(queue, event, result string, value float64) {
	if m == nil || m.collectionEvents == nil {
		return
	}
	queue = normalizeLabel(queueLabels, queue)
	event = normalizeLabel(collectionEventLabels, event)
	result = normalizeLabel(collectionResultLabels, result)
	if value > 0 {
		m.collectionEvents.WithLabelValues(queue, event, result).Add(value)
	}
}

func (m *Metrics) SetCollectionWindowCount(status string, value float64) {
	if m == nil || m.collectionWindows == nil {
		return
	}
	status = normalizeLabel(collectionWindowStatusLabels, status)
	m.collectionWindows.WithLabelValues(status).Set(nonNegativeFloat(value))
}

func (m *Metrics) SetCollectionLag(lag time.Duration) {
	if m == nil || m.collectionLag == nil {
		return
	}
	m.collectionLag.Set(nonNegativeDuration(lag).Seconds())
}

func (m *Metrics) SetCollectionStaleSites(value float64) {
	if m == nil || m.collectionStale == nil {
		return
	}
	m.collectionStale.Set(nonNegativeFloat(value))
}

func (m *Metrics) SetAlertEventCount(level, status string, value float64) {
	if m == nil || m.alertEvents == nil {
		return
	}
	level = normalizeLabel(alertLevelLabels, strings.ToLower(level))
	status = normalizeLabel(alertEventStatusLabels, strings.ToLower(status))
	m.alertEvents.WithLabelValues(level, status).Set(nonNegativeFloat(value))
}

func (m *Metrics) IncrementAlertTransition(level, transition string) {
	if m == nil || m.alertTransitions == nil {
		return
	}
	level = normalizeLabel(alertLevelLabels, strings.ToLower(level))
	transition = normalizeLabel(alertTransitionLabels, strings.ToLower(transition))
	m.alertTransitions.WithLabelValues(level, transition).Inc()
}

func (m *Metrics) SetAlertDeliveryCount(status string, value float64) {
	if m == nil || m.alertDeliveryQueue == nil {
		return
	}
	status = normalizeLabel(alertDeliveryStatusLabels, strings.ToLower(status))
	m.alertDeliveryQueue.WithLabelValues(status).Set(nonNegativeFloat(value))
}

func (m *Metrics) IncrementAlertDelivery(channel, result string) {
	m.AddAlertDeliveries(channel, result, 1)
}

func (m *Metrics) AddAlertDeliveries(channel, result string, value float64) {
	if m == nil || m.alertDeliveries == nil {
		return
	}
	channel = normalizeLabel(alertChannelLabels, strings.ToLower(channel))
	result = normalizeLabel(alertDeliveryResultLabels, strings.ToLower(result))
	if value > 0 {
		m.alertDeliveries.WithLabelValues(channel, result).Add(value)
	}
}

func (m *Metrics) SetExportJobCount(status string, value float64) {
	if m == nil || m.exportJobs == nil {
		return
	}
	status = normalizeLabel(exportStatusLabels, strings.ToLower(status))
	m.exportJobs.WithLabelValues(status).Set(nonNegativeFloat(value))
}

func (m *Metrics) IncrementExportEvent(event, result string) {
	m.AddExportEvents(event, result, 1)
}

func (m *Metrics) AddExportEvents(event, result string, value float64) {
	if m == nil || m.exportEvents == nil {
		return
	}
	event = normalizeLabel(exportEventLabels, strings.ToLower(event))
	result = normalizeLabel(exportResultLabels, strings.ToLower(result))
	if value > 0 {
		m.exportEvents.WithLabelValues(event, result).Add(value)
	}
}

func (m *Metrics) ObserveExport(format, result string, size int64, duration time.Duration) {
	if m == nil || m.exportDuration == nil || m.exportBytes == nil {
		return
	}
	format = normalizeLabel(exportFormatLabels, strings.ToLower(format))
	result = normalizeLabel(exportResultLabels, strings.ToLower(result))
	m.exportDuration.WithLabelValues(format, result).Observe(nonNegativeDuration(duration).Seconds())
	if result == "success" && size > 0 {
		m.exportBytes.WithLabelValues(format).Add(float64(size))
	}
}

func (m *Metrics) SetExportFilesystem(freeBytes, totalBytes uint64) {
	if m == nil || m.exportFreeBytes == nil || m.exportTotalBytes == nil {
		return
	}
	m.exportFreeBytes.Set(float64(freeBytes))
	m.exportTotalBytes.Set(float64(totalBytes))
}

func labelSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func normalizeLabel(allowed map[string]struct{}, value string) string {
	if _, exists := allowed[value]; exists {
		return value
	}
	return metricOther
}

func normalizeHTTPRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" || route == "unmatched" {
		return "unmatched"
	}
	if len(route) > 160 || !strings.HasPrefix(route, "/") || strings.ContainsAny(route, "?#\\\x00\r\n") {
		return "unmatched"
	}
	for _, segment := range strings.Split(strings.TrimPrefix(route, "/"), "/") {
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, ":") {
			segment = strings.TrimPrefix(segment, ":")
			if segment == "" {
				return "unmatched"
			}
		}
		for _, character := range segment {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '_' && character != '-' {
				return "unmatched"
			}
		}
	}
	return route
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func nonNegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
