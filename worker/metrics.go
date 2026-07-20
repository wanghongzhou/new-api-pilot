package worker

import (
	"database/sql"
	"time"
)

type ExecutorMetricsRecorder interface {
	IncrementTaskAttempt(taskType, result string)
	IncrementCollectionEvent(queue, event, result string)
}

type ReaperMetricsRecorder interface {
	AddCollectionEvents(queue, event, result string, value float64)
	IncrementRuntimeRecovery(component, result string)
}

type SchedulerMetricsRecorder interface {
	SetSchedulerHeartbeat(timestamp time.Time)
}

type RuntimeMetricsRecorder interface {
	SetRuntimeReady(component string, ready bool)
	IncrementRuntimeRecovery(component, result string)
}

type OperationalMetricsRecorder interface {
	SetDBStats(stats sql.DBStats)
	SetTaskQueueDepth(queue, status string, value float64)
	SetTaskOldestAge(queue string, age time.Duration)
	SetCollectionWindowCount(status string, value float64)
	SetCollectionLag(lag time.Duration)
	SetCollectionStaleSites(value float64)
	SetClockOffset(seconds float64)
	SetAlertEventCount(level, status string, value float64)
	SetAlertDeliveryCount(status string, value float64)
	SetExportJobCount(status string, value float64)
	SetExportFilesystem(freeBytes, totalBytes uint64)
}

type ExportMetricsRecorder interface {
	IncrementExportEvent(event, result string)
	AddExportEvents(event, result string, value float64)
	ObserveExport(format, result string, size int64, duration time.Duration)
	SetRuntimeReady(component string, ready bool)
	IncrementRuntimeRecovery(component, result string)
}

type WorkerMetricsRecorder interface {
	ExecutorMetricsRecorder
	ReaperMetricsRecorder
	SchedulerMetricsRecorder
	RuntimeMetricsRecorder
	OperationalMetricsRecorder
}

func recordWorkerMetric(record func()) {
	if record == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	record()
}
