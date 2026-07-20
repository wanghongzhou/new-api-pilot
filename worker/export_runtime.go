package worker

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

const (
	defaultExportPollInterval      = time.Second
	defaultExportRecoveryInterval  = time.Minute
	defaultExportCleanupInterval   = time.Hour
	defaultExportLeaseDuration     = 5 * time.Minute
	defaultExportHeartbeatInterval = time.Minute
)

type ExportRuntimeOptions struct {
	Database          *gorm.DB
	Clock             common.Clock
	ExportDir         string
	DiskFree          service.ExportDiskFreeFunc
	PollInterval      time.Duration
	RecoveryInterval  time.Duration
	CleanupInterval   time.Duration
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	Metrics           ExportMetricsRecorder
}

type ExportRuntime struct {
	database          *gorm.DB
	repository        *model.ExportRepository
	clock             common.Clock
	exportDir         string
	diskFree          service.ExportDiskFreeFunc
	pollInterval      time.Duration
	recoveryInterval  time.Duration
	cleanupInterval   time.Duration
	leaseDuration     time.Duration
	heartbeatInterval time.Duration
	metrics           ExportMetricsRecorder

	mu              sync.Mutex
	running         bool
	ready           bool
	cancelAdmission context.CancelFunc
	cancelExecution context.CancelFunc
	done            chan struct{}
	runErr          error
}

func NewExportRuntime(options ExportRuntimeOptions) (*ExportRuntime, error) {
	if options.Database == nil || options.Clock == nil {
		return nil, errors.New("export runtime dependencies are required")
	}
	directory, err := service.SecureExportDirectory(options.ExportDir)
	if err != nil {
		return nil, err
	}
	if options.PollInterval <= 0 {
		options.PollInterval = defaultExportPollInterval
	}
	if options.RecoveryInterval <= 0 {
		options.RecoveryInterval = defaultExportRecoveryInterval
	}
	if options.CleanupInterval <= 0 {
		options.CleanupInterval = defaultExportCleanupInterval
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = defaultExportLeaseDuration
	}
	if options.HeartbeatInterval <= 0 {
		options.HeartbeatInterval = defaultExportHeartbeatInterval
	}
	if options.HeartbeatInterval >= options.LeaseDuration {
		return nil, errors.New("export heartbeat interval must be shorter than its lease")
	}
	return &ExportRuntime{
		database: options.Database, repository: model.NewExportRepository(options.Database),
		clock: options.Clock, exportDir: directory, diskFree: options.DiskFree,
		pollInterval: options.PollInterval, recoveryInterval: options.RecoveryInterval,
		cleanupInterval: options.CleanupInterval, leaseDuration: options.LeaseDuration,
		heartbeatInterval: options.HeartbeatInterval, metrics: options.Metrics,
	}, nil
}

func (runtime *ExportRuntime) Takeover(ctx context.Context) (int, error) {
	return runtime.recover(ctx, true)
}

func (runtime *ExportRuntime) RecoverOnce(ctx context.Context) (int, error) {
	return runtime.recover(ctx, false)
}

func (runtime *ExportRuntime) recover(ctx context.Context, takeover bool) (int, error) {
	now := runtime.clock.Now().Unix()
	recovered, err := runtime.repository.RecoverRunning(ctx, now, takeover)
	if err != nil {
		recordWorkerMetric(func() { runtime.metrics.IncrementRuntimeRecovery("export", "failure") })
		return 0, err
	}
	for _, item := range recovered {
		if item.FilePath == "" {
			continue
		}
		if err := service.RemoveExportArtifact(runtime.exportDir, item.FilePath); err != nil {
			recordWorkerMetric(func() { runtime.metrics.IncrementRuntimeRecovery("export", "failure") })
			return 0, err
		}
		if err := runtime.repository.ClearArtifactPath(ctx, item.JobID, item.FilePath, now); err != nil {
			recordWorkerMetric(func() { runtime.metrics.IncrementRuntimeRecovery("export", "failure") })
			return 0, err
		}
	}
	recordWorkerMetric(func() { runtime.metrics.IncrementRuntimeRecovery("export", "success") })
	if len(recovered) > 0 {
		var retried, failed int
		for _, item := range recovered {
			if item.Failed {
				failed++
			} else {
				retried++
			}
		}
		if takeover {
			if retried > 0 {
				recordWorkerMetric(func() {
					runtime.metrics.AddExportEvents("takeover", "pending", float64(retried))
				})
			}
			if failed > 0 {
				recordWorkerMetric(func() {
					runtime.metrics.AddExportEvents("takeover", "failed", float64(failed))
				})
			}
		} else {
			if retried > 0 {
				recordWorkerMetric(func() {
					runtime.metrics.AddExportEvents("retry", "pending", float64(retried))
				})
			}
			if failed > 0 {
				recordWorkerMetric(func() {
					runtime.metrics.AddExportEvents("failure", "failed", float64(failed))
				})
			}
		}
	}
	return len(recovered), nil
}

func (runtime *ExportRuntime) CleanupOnce(ctx context.Context) (int, error) {
	now := runtime.clock.Now().Unix()
	expired, err := runtime.repository.Expire(ctx, now)
	if err != nil {
		return 0, err
	}
	for _, item := range expired {
		if item.FilePath == "" {
			continue
		}
		if err := service.RemoveExportArtifact(runtime.exportDir, item.FilePath); err != nil {
			return 0, err
		}
		if err := runtime.repository.ClearArtifactPath(ctx, item.JobID, item.FilePath, now); err != nil {
			return 0, err
		}
	}
	if len(expired) > 0 {
		recordWorkerMetric(func() {
			runtime.metrics.AddExportEvents("cleanup", "success", float64(len(expired)))
		})
	}
	return len(expired), nil
}

func (runtime *ExportRuntime) RunOnce(ctx context.Context) (bool, error) {
	if runtime == nil || runtime.repository == nil {
		return false, errors.New("export runtime is not initialized")
	}
	token, err := newExportClaimToken()
	if err != nil {
		return false, err
	}
	now := runtime.clock.Now().Unix()
	claim, err := runtime.repository.Claim(ctx, now, token, now+int64(runtime.leaseDuration/time.Second))
	if err != nil || claim == nil {
		return claim != nil, err
	}
	recordWorkerMetric(func() { runtime.metrics.IncrementExportEvent("claim", "success") })
	if err := runtime.execute(ctx, *claim, token); err != nil && !errors.Is(err, context.Canceled) {
		return true, err
	}
	return true, ctx.Err()
}

func (runtime *ExportRuntime) execute(parent context.Context, claim model.ExportClaim, token string) (executionErr error) {
	startedAt := runtime.clock.Now()
	metricResult := "lost"
	var metricSize int64
	defer func() {
		recordWorkerMetric(func() {
			runtime.metrics.ObserveExport(
				claim.Job.Format, metricResult, metricSize, runtime.clock.Now().Sub(startedAt),
			)
		})
	}()
	finishFailure := func(cause error, temporaryName string) error {
		plannedResult := exportFailureMetricResult(claim, cause)
		err := runtime.finishFailure(parent, claim, token, cause, temporaryName)
		if errors.Is(err, model.ErrExportClaimLost) {
			metricResult = "lost"
			runtime.recordExportClaimLost()
			return nil
		}
		if err != nil {
			metricResult = "lost"
			return err
		}
		metricResult = plannedResult
		event := "failure"
		if plannedResult == "pending" {
			event = "retry"
		}
		recordWorkerMetric(func() { runtime.metrics.IncrementExportEvent(event, plannedResult) })
		return nil
	}
	var filters dto.ExportFilters
	if json.Unmarshal(claim.Job.Filters, &filters) != nil {
		return finishFailure(service.ErrExportContract, "")
	}
	filters.Normalize()
	var query dto.StatisticsQuery
	var logQuery dto.LogQuery
	var inventoryQuery dto.UserInventoryQuery
	var channelInventoryQuery dto.ChannelInventoryQuery
	var performanceHistoryQuery dto.PerformanceHistoryQuery
	var financeInventoryQuery dto.FinanceInventoryQuery
	var upstreamTaskQuery dto.UpstreamTaskQuery
	var modelCatalogQuery dto.ModelCatalogQuery
	var localRankingQuery dto.LocalRankingQuery
	var subscriptionPlanQuery dto.SubscriptionPlanQuery
	var pricingCatalogQuery dto.PricingCatalogQuery
	var systemTaskQuery dto.SystemTaskQuery
	if claim.Job.StatisticsType == "logs" {
		var fields map[string]string
		logQuery, fields = filters.LogQuery()
		if fields != nil || logQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "user_inventory" {
		var fields map[string]string
		inventoryQuery, fields = filters.UserInventoryQuery()
		if fields != nil || inventoryQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "channel_inventory" {
		var fields map[string]string
		channelInventoryQuery, fields = filters.ChannelInventoryQuery()
		if fields != nil || channelInventoryQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "performance_history" {
		var fields map[string]string
		performanceHistoryQuery, fields = filters.PerformanceHistoryQuery()
		if fields != nil || performanceHistoryQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "topup_inventory" || claim.Job.StatisticsType == "redemption_inventory" {
		var fields map[string]string
		financeInventoryQuery, fields = filters.FinanceInventoryQuery()
		if fields != nil || financeInventoryQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "upstream_tasks" {
		var fields map[string]string
		upstreamTaskQuery, fields = filters.UpstreamTaskQuery()
		if fields != nil || upstreamTaskQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "model_catalog" {
		var fields map[string]string
		modelCatalogQuery, fields = filters.ModelCatalogQuery()
		if fields != nil || modelCatalogQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "model_rankings" || claim.Job.StatisticsType == "vendor_rankings" {
		var fields map[string]string
		localRankingQuery, fields = filters.LocalRankingQuery()
		if fields != nil || localRankingQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "subscription_plans" {
		var fields map[string]string
		subscriptionPlanQuery, fields = filters.SubscriptionPlanQuery()
		if fields != nil || subscriptionPlanQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "pricing_catalog" || claim.Job.StatisticsType == "group_catalog" {
		var fields map[string]string
		pricingCatalogQuery, fields = filters.PricingCatalogQuery()
		if fields != nil || pricingCatalogQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else if claim.Job.StatisticsType == "system_tasks" {
		var fields map[string]string
		systemTaskQuery, fields = filters.SystemTaskQuery()
		if fields != nil || systemTaskQuery.Validate() != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	} else {
		var fields map[string]string
		query, fields = filters.StatisticsQuery(claim.Job.StatisticsType)
		if fields != nil || query.ValidateForExport(claim.Job.StatisticsType) != nil {
			return finishFailure(service.ErrExportContract, "")
		}
	}
	executionCtx, cancelExecution := context.WithCancel(parent)
	defer cancelExecution()
	var progress atomic.Int64
	heartbeatDone := make(chan error, 1)
	go runtime.heartbeat(executionCtx, cancelExecution, claim.Job.ID, token, &progress, heartbeatDone)
	stopHeartbeat := func() error {
		cancelExecution()
		return <-heartbeatDone
	}

	tx := runtime.database.WithContext(executionCtx).Begin(&sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if tx.Error != nil {
		_ = stopHeartbeat()
		return finishFailure(errors.Join(service.ErrStatisticsRead, tx.Error), "")
	}
	transactionOpen := true
	rollback := func() {
		if transactionOpen {
			_ = tx.Rollback().Error
			transactionOpen = false
		}
	}
	defer rollback()
	var iterator *service.StatisticsExportIterator
	var rates service.ExportRateSnapshot
	var err error
	if claim.Job.StatisticsType == "logs" || claim.Job.StatisticsType == "user_inventory" || claim.Job.StatisticsType == "channel_inventory" || claim.Job.StatisticsType == "performance_history" || claim.Job.StatisticsType == "topup_inventory" || claim.Job.StatisticsType == "redemption_inventory" || claim.Job.StatisticsType == "upstream_tasks" || claim.Job.StatisticsType == "model_catalog" || claim.Job.StatisticsType == "model_rankings" || claim.Job.StatisticsType == "vendor_rankings" || claim.Job.StatisticsType == "subscription_plans" || claim.Job.StatisticsType == "pricing_catalog" || claim.Job.StatisticsType == "group_catalog" || claim.Job.StatisticsType == "system_tasks" {
		rates = service.ExportRateSnapshot{Sites: []service.ExportRateSnapshotSite{}}
	} else {
		iterator, err = service.NewStatisticsExportIterator(service.StatisticsExportIteratorOptions{
			Database: tx, Clock: runtime.clock, Scope: claim.Job.StatisticsType, Query: query,
			PageSize: service.StatisticsExportPageSize,
		})
		if err != nil {
			_ = stopHeartbeat()
			return finishFailure(errors.Join(service.ErrStatisticsRead, err), "")
		}
		currentRates, rateErr := iterator.LoadRateSnapshot(executionCtx)
		if rateErr != nil {
			_ = stopHeartbeat()
			return finishFailure(errors.Join(service.ErrStatisticsRead, rateErr), "")
		}
		rates = service.NewExportRateSnapshot(currentRates)
	}
	if len(claim.Job.RateSnapshot) == 0 {
		raw, marshalErr := json.Marshal(rates)
		if marshalErr != nil {
			_ = stopHeartbeat()
			return finishFailure(service.ErrExportContract, "")
		}
		now := runtime.clock.Now().Unix()
		if err := runtime.repository.SetRateSnapshot(parent, claim.Job.ID, token, raw, now); err != nil {
			_ = stopHeartbeat()
			return runtime.lostClaim(err)
		}
	} else if claim.Job.StatisticsType != "logs" && claim.Job.StatisticsType != "user_inventory" && claim.Job.StatisticsType != "channel_inventory" && claim.Job.StatisticsType != "performance_history" && claim.Job.StatisticsType != "topup_inventory" && claim.Job.StatisticsType != "redemption_inventory" && claim.Job.StatisticsType != "upstream_tasks" && claim.Job.StatisticsType != "model_catalog" && claim.Job.StatisticsType != "model_rankings" && claim.Job.StatisticsType != "vendor_rankings" && claim.Job.StatisticsType != "subscription_plans" && claim.Job.StatisticsType != "pricing_catalog" && claim.Job.StatisticsType != "group_catalog" && claim.Job.StatisticsType != "system_tasks" {
		rates, err = service.ParseExportRateSnapshot(claim.Job.RateSnapshot)
		if err != nil {
			_ = stopHeartbeat()
			return finishFailure(err, "")
		}
	}
	snapshotAt := runtime.clock.Now().Unix()
	if err := runtime.repository.SetDataSnapshot(parent, claim.Job.ID, token, snapshotAt, snapshotAt); err != nil {
		_ = stopHeartbeat()
		return runtime.lostClaim(err)
	}
	finalName := exportFileName(claim.Job, filters)
	temporaryName := "." + finalName + "." + token + ".tmp"
	temporaryPath, err := service.ExportArtifactPath(runtime.exportDir, temporaryName)
	if err != nil {
		_ = stopHeartbeat()
		return finishFailure(err, temporaryName)
	}
	if err := runtime.repository.SetTemporaryPath(parent, claim.Job.ID, token, temporaryName, snapshotAt); err != nil {
		_ = stopHeartbeat()
		return runtime.lostClaim(err)
	}
	progressPage := func(ctx context.Context, page int, _ int64) error {
		value := int64(page)
		if value > 95 {
			value = 95
		}
		progress.Store(value)
		now := runtime.clock.Now().Unix()
		return runtime.repository.Heartbeat(ctx, claim.Job.ID, token, now,
			now+int64(runtime.leaseDuration/time.Second), int(value))
	}
	var generated service.ExportGenerateResult
	if claim.Job.StatisticsType == "logs" {
		generated, err = service.GenerateUpstreamLogExport(executionCtx, service.UpstreamLogExportOptions{
			Database: tx, Query: logQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath,
			DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes,
			MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage,
		})
	} else if claim.Job.StatisticsType == "user_inventory" {
		generated, err = service.GenerateUserInventoryExport(executionCtx, service.UserInventoryExportOptions{
			Database: tx, Query: inventoryQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath,
			DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes,
			MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage,
		})
	} else if claim.Job.StatisticsType == "channel_inventory" {
		generated, err = service.GenerateChannelInventoryExport(executionCtx, service.ChannelInventoryExportOptions{Database: tx, Query: channelInventoryQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage})
	} else if claim.Job.StatisticsType == "performance_history" {
		generated, err = service.GeneratePerformanceHistoryExport(executionCtx, service.PerformanceHistoryExportOptions{Database: tx, Query: performanceHistoryQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage})
	} else if claim.Job.StatisticsType == "topup_inventory" || claim.Job.StatisticsType == "redemption_inventory" {
		kind := "topup"
		if claim.Job.StatisticsType == "redemption_inventory" {
			kind = "redemption"
		}
		generated, err = service.GenerateFinanceOperationsExport(executionCtx, service.FinanceOperationsExportOptions{Database: tx, Query: financeInventoryQuery, Kind: kind, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage, Now: snapshotAt})
	} else if claim.Job.StatisticsType == "upstream_tasks" {
		generated, err = service.GenerateUpstreamTaskExport(executionCtx, service.UpstreamTaskExportOptions{Database: tx, Query: upstreamTaskQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage})
	} else if claim.Job.StatisticsType == "model_catalog" {
		generated, err = service.GenerateModelCatalogExport(executionCtx, service.ModelCatalogExportOptions{Database: tx, Query: modelCatalogQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage})
	} else if claim.Job.StatisticsType == "model_rankings" || claim.Job.StatisticsType == "vendor_rankings" {
		kind := "model"
		if claim.Job.StatisticsType == "vendor_rankings" {
			kind = "vendor"
		}
		generated, err = service.GenerateLocalRankingExport(executionCtx, service.LocalRankingExportOptions{Database: tx, Query: localRankingQuery, Kind: kind, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree})
	} else if claim.Job.StatisticsType == "subscription_plans" {
		generated, err = service.GenerateSubscriptionPlanExport(executionCtx, service.SubscriptionPlanExportOptions{Database: tx, Query: subscriptionPlanQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree})
	} else if claim.Job.StatisticsType == "pricing_catalog" || claim.Job.StatisticsType == "group_catalog" {
		kind := "pricing"
		if claim.Job.StatisticsType == "group_catalog" {
			kind = "group"
		}
		generated, err = service.GeneratePricingCatalogExport(executionCtx, service.PricingCatalogExportOptions{Database: tx, Query: pricingCatalogQuery, Kind: kind, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree})
	} else if claim.Job.StatisticsType == "system_tasks" {
		generated, err = service.GenerateSystemTaskExport(executionCtx, service.SystemTaskExportOptions{Database: tx, Query: systemTaskQuery, Format: claim.Job.Format, TemporaryPath: temporaryPath, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt, MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes, DiskFree: runtime.diskFree, OnPage: progressPage})
	} else {
		generated, err = service.GenerateExportFile(executionCtx, service.ExportGenerateOptions{
			Iterator: iterator, Format: claim.Job.Format, TemporaryPath: temporaryPath,
			Rates: rates, DataSnapshotAt: snapshotAt, ExportedAt: snapshotAt,
			MaxFileBytes: claim.Settings.MaxFileBytes, MinFreeBytes: claim.Settings.MinFreeDiskBytes,
			DiskFree: runtime.diskFree, OnPage: progressPage,
		})
	}
	rollback()
	if err != nil {
		heartbeatErr := stopHeartbeat()
		if heartbeatErr != nil && !errors.Is(heartbeatErr, context.Canceled) {
			err = heartbeatErr
		}
		if errors.Is(parent.Err(), context.Canceled) {
			_ = service.RemoveExportArtifact(runtime.exportDir, temporaryName)
			return parent.Err()
		}
		return finishFailure(err, temporaryName)
	}
	if err := service.PublishExportArtifact(runtime.exportDir, temporaryName, finalName); err != nil {
		_ = stopHeartbeat()
		return finishFailure(err, temporaryName)
	}
	if err := stopHeartbeat(); err != nil && !errors.Is(err, context.Canceled) {
		_ = service.RemoveExportArtifact(runtime.exportDir, finalName)
		return runtime.lostClaim(err)
	}
	finishedAt := runtime.clock.Now().Unix()
	expiresAt := finishedAt + int64(claim.Settings.FileTTLHours)*3600
	if err := runtime.repository.Complete(
		parent, claim.Job.ID, token, finalName, finalName,
		generated.FileSize, generated.RowCount, finishedAt, expiresAt,
	); err != nil {
		_ = service.RemoveExportArtifact(runtime.exportDir, finalName)
		return runtime.lostClaim(err)
	}
	metricResult, metricSize = "success", generated.FileSize
	recordWorkerMetric(func() { runtime.metrics.IncrementExportEvent("completion", "success") })
	return nil
}

func (runtime *ExportRuntime) heartbeat(
	ctx context.Context,
	cancel context.CancelFunc,
	jobID int64,
	token string,
	progress *atomic.Int64,
	done chan<- error,
) {
	ticker := runtime.clock.NewTicker(runtime.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-ticker.C():
			now := runtime.clock.Now().Unix()
			err := runtime.repository.Heartbeat(ctx, jobID, token, now,
				now+int64(runtime.leaseDuration/time.Second), int(progress.Load()))
			if err != nil {
				cancel()
				done <- err
				return
			}
		}
	}
}

func (runtime *ExportRuntime) finishFailure(
	ctx context.Context,
	claim model.ExportClaim,
	token string,
	cause error,
	temporaryName string,
) error {
	if temporaryName != "" {
		_ = service.RemoveExportArtifact(runtime.exportDir, temporaryName)
	}
	if errors.Is(cause, model.ErrExportClaimLost) {
		return nil
	}
	code := constant.MessageExportWriteFailed
	params := map[string]any{"export_id": strconv.FormatInt(claim.Job.ID, 10)}
	retryable := true
	technical := "export write attempt failed"
	var disk *service.ExportDiskLowError
	var tooLarge *service.ExportFileTooLargeError
	switch {
	case errors.As(cause, &disk):
		code = constant.MessageExportDiskLow
		params["free_bytes"] = strconv.FormatUint(disk.FreeBytes, 10)
		params["threshold_bytes"] = strconv.FormatInt(disk.ThresholdBytes, 10)
		retryable = false
		technical = "export disk space is below the configured threshold"
	case errors.As(cause, &tooLarge):
		code = constant.MessageExportFileTooLarge
		params["file_bytes"] = strconv.FormatInt(tooLarge.ObservedBytes, 10)
		params["limit_bytes"] = strconv.FormatInt(tooLarge.LimitBytes, 10)
		retryable = false
		technical = "export file exceeded the configured limit"
	case errors.Is(cause, service.ErrExportFileTooLarge):
		code = constant.MessageExportFileTooLarge
		params["file_bytes"] = strconv.FormatInt(claim.Settings.MaxFileBytes+1, 10)
		params["limit_bytes"] = strconv.FormatInt(claim.Settings.MaxFileBytes, 10)
		retryable = false
		technical = "export file exceeded the configured limit"
	case errors.Is(cause, service.ErrStatisticsRead):
		code = constant.MessageExportSnapshotFailed
		technical = "export snapshot query failed"
	case errors.Is(cause, service.ErrExportContract), errors.Is(cause, service.ErrExportInvalid):
		code = constant.MessageExportSnapshotFailed
		retryable = false
		technical = "export contract validation failed"
	}
	rawParams, _ := json.Marshal(params)
	now := runtime.clock.Now().Unix()
	var retryAt *int64
	if retryable && claim.Job.AttemptCount < 2 {
		value := now + 60
		retryAt = &value
	}
	if err := runtime.repository.FinishAttempt(ctx, claim.Job.ID, token, string(code), rawParams, technical, now, retryAt); err != nil {
		return err
	}
	return nil
}

func exportFailureMetricResult(claim model.ExportClaim, cause error) string {
	if errors.Is(cause, model.ErrExportClaimLost) {
		return "lost"
	}
	retryable := true
	var disk *service.ExportDiskLowError
	var tooLarge *service.ExportFileTooLargeError
	switch {
	case errors.As(cause, &disk), errors.As(cause, &tooLarge),
		errors.Is(cause, service.ErrExportFileTooLarge),
		errors.Is(cause, service.ErrExportContract),
		errors.Is(cause, service.ErrExportInvalid):
		retryable = false
	}
	if retryable && claim.Job.AttemptCount < 2 {
		return "pending"
	}
	if retryable {
		return "exhausted"
	}
	return "failed"
}

func (runtime *ExportRuntime) lostClaim(err error) error {
	if errors.Is(err, model.ErrExportClaimLost) {
		runtime.recordExportClaimLost()
		return nil
	}
	return err
}

func (runtime *ExportRuntime) recordExportClaimLost() {
	recordWorkerMetric(func() {
		runtime.metrics.IncrementExportEvent("failure", "lost")
	})
}

func (runtime *ExportRuntime) Start(parent context.Context) error {
	if runtime == nil || parent == nil {
		return errors.New("export runtime is not initialized")
	}
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("export", false) })
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return errors.New("export runtime is already running")
	}
	admissionCtx, cancelAdmission := context.WithCancel(parent)
	executionCtx, cancelExecution := context.WithCancel(parent)
	runtime.running = true
	runtime.ready = false
	runtime.cancelAdmission = cancelAdmission
	runtime.cancelExecution = cancelExecution
	runtime.done = make(chan struct{})
	runtime.runErr = nil
	done := runtime.done
	runtime.mu.Unlock()

	if _, err := runtime.Takeover(admissionCtx); err != nil {
		runtime.failStart(cancelAdmission, cancelExecution, done, err)
		return err
	}
	if _, err := runtime.CleanupOnce(admissionCtx); err != nil {
		runtime.failStart(cancelAdmission, cancelExecution, done, err)
		return err
	}
	if err := admissionCtx.Err(); err != nil {
		runtime.failStart(cancelAdmission, cancelExecution, done, err)
		return err
	}

	started := make(chan struct{})
	go runtime.runLoop(admissionCtx, executionCtx, cancelAdmission, cancelExecution, done, started)
	<-started
	runtime.mu.Lock()
	if !runtime.running {
		err := runtime.runErr
		runtime.mu.Unlock()
		if err == nil {
			err = admissionCtx.Err()
		}
		return err
	}
	if err := admissionCtx.Err(); err != nil {
		runtime.mu.Unlock()
		cancelAdmission()
		cancelExecution()
		<-done
		return err
	}
	runtime.ready = true
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("export", true) })
	return nil
}

func (runtime *ExportRuntime) failStart(
	cancelAdmission context.CancelFunc,
	cancelExecution context.CancelFunc,
	done chan struct{},
	err error,
) {
	cancelAdmission()
	cancelExecution()
	runtime.mu.Lock()
	runtime.running = false
	runtime.ready = false
	runtime.runErr = err
	close(done)
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("export", false) })
}

func (runtime *ExportRuntime) runLoop(
	admissionCtx context.Context,
	executionCtx context.Context,
	cancelAdmission context.CancelFunc,
	cancelExecution context.CancelFunc,
	done chan struct{},
	started chan<- struct{},
) {
	err := runtime.runAdmissions(admissionCtx, executionCtx, started)
	cancelAdmission()
	cancelExecution()
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	runtime.mu.Lock()
	runtime.ready = false
	runtime.running = false
	runtime.runErr = err
	close(done)
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("export", false) })
}

func (runtime *ExportRuntime) runAdmissions(
	admissionCtx context.Context,
	executionCtx context.Context,
	started chan<- struct{},
) error {
	poll := runtime.clock.NewTicker(runtime.pollInterval)
	recovery := runtime.clock.NewTicker(runtime.recoveryInterval)
	cleanup := runtime.clock.NewTicker(runtime.cleanupInterval)
	defer poll.Stop()
	defer recovery.Stop()
	defer cleanup.Stop()
	close(started)
	for {
		select {
		case <-admissionCtx.Done():
			return nil
		case <-poll.C():
			if _, err := runtime.RunOnce(executionCtx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		case <-recovery.C():
			if _, err := runtime.RecoverOnce(admissionCtx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		case <-cleanup.C():
			if _, err := runtime.CleanupOnce(admissionCtx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (runtime *ExportRuntime) Quiesce() error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if !runtime.running {
		err := runtime.runErr
		runtime.mu.Unlock()
		return err
	}
	runtime.ready = false
	cancel := runtime.cancelAdmission
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("export", false) })
	cancel()
	return nil
}

func (runtime *ExportRuntime) Stop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if !runtime.running {
		err := runtime.runErr
		runtime.mu.Unlock()
		return err
	}
	runtime.ready = false
	cancelAdmission := runtime.cancelAdmission
	cancelExecution := runtime.cancelExecution
	done := runtime.done
	runtime.mu.Unlock()
	recordWorkerMetric(func() { runtime.metrics.SetRuntimeReady("export", false) })
	cancelAdmission()
	return awaitRuntimeStop(ctx, done, cancelExecution, true, func() error {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.runErr
	})
}

func (runtime *ExportRuntime) Ready() bool {
	if runtime == nil {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.ready
}

func (runtime *ExportRuntime) Run(ctx context.Context) error {
	if err := runtime.Start(ctx); err != nil {
		return err
	}
	runtime.mu.Lock()
	done := runtime.done
	runtime.mu.Unlock()
	<-done
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.runErr
}

func exportFileName(job model.ExportJob, filters dto.ExportFilters) string {
	return fmt.Sprintf("statistics-%s-%d-%d-%d.%s",
		job.StatisticsType, filters.StartTimestamp, filters.EndTimestamp, job.ID, job.Format)
}

func newExportClaimToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate export claim token: %w", err)
	}
	return hex.EncodeToString(value), nil
}
