package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

const (
	defaultMetricsSampleInterval = time.Minute
	defaultMetricsSampleTimeout  = 2 * time.Second
	defaultCollectionStaleAfter  = 15 * time.Minute
)

type OperationalMetricsSnapshotter interface {
	Snapshot(context.Context) (model.OperationalMetricsSnapshot, error)
}

type ExportFilesystemUsageFunc func(path string) (freeBytes, totalBytes uint64, err error)

type MetricsSamplerOptions struct {
	Snapshotter     OperationalMetricsSnapshotter
	Database        *sql.DB
	Recorder        OperationalMetricsRecorder
	Clock           common.Clock
	ExportDir       string
	FilesystemUsage ExportFilesystemUsageFunc
	Interval        time.Duration
	Timeout         time.Duration
	StaleAfter      time.Duration
}

type MetricsSampler struct {
	snapshotter     OperationalMetricsSnapshotter
	database        *sql.DB
	recorder        OperationalMetricsRecorder
	clock           common.Clock
	exportDir       string
	filesystemUsage ExportFilesystemUsageFunc
	interval        time.Duration
	timeout         time.Duration
	staleAfter      time.Duration
}

func NewMetricsSampler(options MetricsSamplerOptions) (*MetricsSampler, error) {
	if options.Snapshotter == nil || options.Database == nil || options.Recorder == nil || options.Clock == nil {
		return nil, errors.New("metrics sampler dependencies are required")
	}
	if options.Interval <= 0 {
		options.Interval = defaultMetricsSampleInterval
	}
	if options.Timeout <= 0 {
		options.Timeout = defaultMetricsSampleTimeout
	}
	if options.StaleAfter <= 0 {
		options.StaleAfter = defaultCollectionStaleAfter
	}
	if options.FilesystemUsage == nil {
		options.FilesystemUsage = exportFilesystemUsage
	}
	return &MetricsSampler{
		snapshotter: options.Snapshotter, database: options.Database, recorder: options.Recorder,
		clock: options.Clock, exportDir: options.ExportDir, filesystemUsage: options.FilesystemUsage,
		interval: options.Interval, timeout: options.Timeout, staleAfter: options.StaleAfter,
	}, nil
}

func (sampler *MetricsSampler) RunOnce(parent context.Context) error {
	if sampler == nil || parent == nil {
		return errors.New("metrics sampler is not initialized")
	}
	recordWorkerMetric(func() { sampler.recorder.SetDBStats(sampler.database.Stats()) })
	ctx, cancel := context.WithTimeout(parent, sampler.timeout)
	defer cancel()
	snapshot, snapshotErr := sampler.snapshotter.Snapshot(ctx)
	if snapshotErr == nil {
		sampler.applySnapshot(snapshot, sampler.clock.Now())
	}
	freeBytes, totalBytes, filesystemErr := sampler.filesystemUsage(sampler.exportDir)
	if filesystemErr != nil {
		freeBytes, totalBytes = 0, 0
	}
	recordWorkerMetric(func() { sampler.recorder.SetExportFilesystem(freeBytes, totalBytes) })
	return errors.Join(snapshotErr, filesystemErr)
}

func (sampler *MetricsSampler) Run(ctx context.Context) error {
	if sampler == nil || ctx == nil {
		return errors.New("metrics sampler is not initialized")
	}
	sampler.sampleWithoutStopping(ctx)
	ticker := sampler.clock.NewTicker(sampler.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			sampler.sampleWithoutStopping(ctx)
		}
	}
}

func (sampler *MetricsSampler) sampleWithoutStopping(ctx context.Context) {
	if err := sampler.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("operational metrics sample failed error_type=%T", err)
	}
}

func (sampler *MetricsSampler) applySnapshot(snapshot model.OperationalMetricsSnapshot, now time.Time) {
	queues := []string{"probe", "realtime", "resource", "metadata", "usage", "backfill"}
	statuses := []string{"pending", "running"}
	depth := make(map[string]map[string]int64, len(queues))
	oldest := make(map[string]int64, len(queues))
	for _, queue := range queues {
		depth[queue] = map[string]int64{"pending": 0, "running": 0}
	}
	depth["other"] = map[string]int64{"pending": 0, "running": 0}
	for _, state := range snapshot.Tasks {
		queueKind, valid := QueueForTask(state.TaskType)
		queue := string(queueKind)
		if !valid {
			queue = "other"
		}
		if depth[queue] == nil {
			depth[queue] = map[string]int64{"pending": 0, "running": 0}
		}
		if state.Status != "pending" && state.Status != "running" {
			continue
		}
		depth[queue][state.Status] += state.Count
		if state.Status == "pending" && state.OldestCreatedAt != nil &&
			(oldest[queue] == 0 || *state.OldestCreatedAt < oldest[queue]) {
			oldest[queue] = *state.OldestCreatedAt
		}
	}
	for _, queue := range append(queues, "other") {
		for _, status := range statuses {
			value := depth[queue][status]
			recordWorkerMetric(func() { sampler.recorder.SetTaskQueueDepth(queue, status, float64(value)) })
		}
		age := time.Duration(0)
		if oldest[queue] > 0 && now.Unix() > oldest[queue] {
			age = time.Duration(now.Unix()-oldest[queue]) * time.Second
		}
		recordWorkerMetric(func() { sampler.recorder.SetTaskOldestAge(queue, age) })
	}

	windowCounts := statusCounts(snapshot.Windows)
	for _, status := range []string{"pending", "complete", "missing", "unavailable"} {
		value := windowCounts[status]
		recordWorkerMetric(func() { sampler.recorder.SetCollectionWindowCount(status, float64(value)) })
	}

	alertCounts := make(map[string]int64, len(snapshot.Alerts))
	for _, count := range snapshot.Alerts {
		alertCounts[count.Level+"\x00"+count.Status] += count.Count
	}
	for _, level := range []string{"warning", "critical"} {
		for _, status := range []string{"pending", "firing"} {
			value := alertCounts[level+"\x00"+status]
			recordWorkerMetric(func() { sampler.recorder.SetAlertEventCount(level, status, float64(value)) })
		}
	}

	deliveryCounts := statusCounts(snapshot.AlertDeliveries)
	for _, status := range []string{"pending", "success", "failed"} {
		value := deliveryCounts[status]
		recordWorkerMetric(func() { sampler.recorder.SetAlertDeliveryCount(status, float64(value)) })
	}
	exportCounts := statusCounts(snapshot.Exports)
	for _, status := range []string{"pending", "running", "success", "failed", "expired"} {
		value := exportCounts[status]
		recordWorkerMetric(func() { sampler.recorder.SetExportJobCount(status, float64(value)) })
	}

	maximumLag, staleSites := sampler.collectionLag(snapshot.EligibleSites, now)
	recordWorkerMetric(func() { sampler.recorder.SetCollectionLag(maximumLag) })
	recordWorkerMetric(func() { sampler.recorder.SetCollectionStaleSites(float64(staleSites)) })
	if snapshot.DatabaseUnix > 0 {
		offset := float64(snapshot.DatabaseUnix - now.Unix())
		recordWorkerMetric(func() { sampler.recorder.SetClockOffset(offset) })
	}
}

func (sampler *MetricsSampler) collectionLag(
	sites []model.OperationalEligibleSite,
	now time.Time,
) (time.Duration, int64) {
	var maximumSeconds int64
	var staleSites int64
	for _, site := range sites {
		var reference int64
		if site.NewestCompleteHour != nil {
			reference = *site.NewestCompleteHour + int64(time.Hour/time.Second)
		} else {
			for _, candidate := range []*int64{site.MonitoringStartAt, site.StatisticsStartAt} {
				if candidate != nil && *candidate > reference {
					reference = *candidate
				}
			}
		}
		lagSeconds := now.Unix() - reference
		if lagSeconds < 0 {
			lagSeconds = 0
		}
		if lagSeconds > maximumSeconds {
			maximumSeconds = lagSeconds
		}
		if time.Duration(lagSeconds)*time.Second > sampler.staleAfter {
			staleSites++
		}
	}
	return time.Duration(maximumSeconds) * time.Second, staleSites
}

func statusCounts(values []model.OperationalStatusCount) map[string]int64 {
	result := make(map[string]int64, len(values))
	for _, value := range values {
		result[value.Status] += value.Count
	}
	return result
}

func validateFilesystemUsage(freeBytes, totalBytes uint64) error {
	if totalBytes == 0 || freeBytes > totalBytes {
		return fmt.Errorf("invalid export filesystem capacity")
	}
	return nil
}
