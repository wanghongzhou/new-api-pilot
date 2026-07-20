package service

import (
	"context"
	"errors"
	"fmt"

	"new-api-pilot/common"
	"new-api-pilot/model"
)

const (
	defaultResourceRetentionBatchSize      = 500
	defaultResourceRetentionMaximumBatches = 100
	maximumResourceRetentionDays           = 3650
)

var ErrResourceRetentionInvalid = errors.New("resource retention request is invalid")

type ResourceRetentionBatchCleaner interface {
	CleanResourceMinuteBatch(context.Context, model.ResourceRetentionBatchRequest) (model.ResourceRetentionBatchResult, error)
}

type ResourceRetentionServiceOptions struct {
	Repository     ResourceRetentionBatchCleaner
	Clock          common.Clock
	BatchSize      int
	MaximumBatches int
}

type ResourceRetentionTableResult struct {
	Scanned                     int   `json:"scanned"`
	Deleted                     int64 `json:"deleted"`
	SkippedUnfinalized          int   `json:"skipped_unfinalized"`
	SkippedMissingHourly        int   `json:"skipped_missing_hourly"`
	SkippedDailyNotFinal        int   `json:"skipped_daily_not_final"`
	PendingRows                 bool  `json:"pending_rows"`
	BlockedDiagnosticsTruncated bool  `json:"blocked_diagnostics_truncated"`
	Batches                     int   `json:"batches"`
	Complete                    bool  `json:"complete"`
}

type ResourceRetentionResult struct {
	RetentionDays int                          `json:"retention_days"`
	Cutoff        int64                        `json:"cutoff"`
	Instance      ResourceRetentionTableResult `json:"instance"`
	Site          ResourceRetentionTableResult `json:"site"`
	Complete      bool                         `json:"complete"`
}

type ResourceRetentionService struct {
	repository     ResourceRetentionBatchCleaner
	clock          common.Clock
	batchSize      int
	maximumBatches int
}

func NewResourceRetentionService(options ResourceRetentionServiceOptions) (*ResourceRetentionService, error) {
	if options.Repository == nil || options.Clock == nil {
		return nil, fmt.Errorf("resource retention service dependencies are required")
	}
	if options.BatchSize <= 0 {
		options.BatchSize = defaultResourceRetentionBatchSize
	}
	if options.BatchSize > 5000 {
		return nil, ErrResourceRetentionInvalid
	}
	if options.MaximumBatches <= 0 {
		options.MaximumBatches = defaultResourceRetentionMaximumBatches
	}
	if options.MaximumBatches > 10000 {
		return nil, ErrResourceRetentionInvalid
	}
	return &ResourceRetentionService{
		repository: options.Repository, clock: options.Clock,
		batchSize: options.BatchSize, maximumBatches: options.MaximumBatches,
	}, nil
}

func (service *ResourceRetentionService) Clean(
	ctx context.Context,
	retentionDays int,
) (ResourceRetentionResult, error) {
	if service == nil || service.repository == nil || service.clock == nil ||
		retentionDays < 1 || retentionDays > maximumResourceRetentionDays {
		return ResourceRetentionResult{}, ErrResourceRetentionInvalid
	}
	now := service.clock.Now().Unix()
	if now <= 0 {
		return ResourceRetentionResult{}, ErrResourceRetentionInvalid
	}
	cutoff := now - now%60 - int64(retentionDays)*24*60*60
	if cutoff <= 0 || cutoff%60 != 0 {
		return ResourceRetentionResult{}, ErrResourceRetentionInvalid
	}
	result := ResourceRetentionResult{RetentionDays: retentionDays, Cutoff: cutoff}
	instance, err := service.cleanTable(ctx, model.ResourceMinuteTableInstance, cutoff)
	result.Instance = instance
	if err != nil {
		return result, err
	}
	site, err := service.cleanTable(ctx, model.ResourceMinuteTableSite, cutoff)
	result.Site = site
	if err != nil {
		return result, err
	}
	result.Complete = result.Instance.Complete && result.Site.Complete
	return result, nil
}

func (service *ResourceRetentionService) cleanTable(
	ctx context.Context,
	table string,
	cutoff int64,
) (ResourceRetentionTableResult, error) {
	var result ResourceRetentionTableResult
	var cursor model.ResourceRetentionCursor
	for batch := 0; batch < service.maximumBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		page, err := service.repository.CleanResourceMinuteBatch(ctx, model.ResourceRetentionBatchRequest{
			Table: table, Cutoff: cutoff, After: cursor, MaximumRows: service.batchSize,
		})
		if err != nil {
			return result, fmt.Errorf("clean %s batch: %w", table, err)
		}
		if page.Scanned < 0 || page.Scanned > service.batchSize || page.Deleted < 0 ||
			page.Deleted > int64(page.Scanned) || page.SkippedUnfinalized < 0 ||
			page.SkippedUnfinalized > page.Scanned || page.SkippedMissingHourly < 0 ||
			page.SkippedMissingHourly > page.Scanned || page.SkippedDailyNotFinal < 0 ||
			page.SkippedDailyNotFinal > page.Scanned ||
			int(page.Deleted)+page.SkippedUnfinalized > page.Scanned ||
			(page.SkippedUnfinalized > 0 && !page.PendingRows) ||
			(page.BlockedDiagnosticsTruncated && !page.PendingRows) {
			return result, ErrResourceRetentionInvalid
		}
		result.Batches++
		result.Scanned += page.Scanned
		result.Deleted += page.Deleted
		result.SkippedUnfinalized += page.SkippedUnfinalized
		result.SkippedMissingHourly += page.SkippedMissingHourly
		result.SkippedDailyNotFinal += page.SkippedDailyNotFinal
		result.PendingRows = page.PendingRows
		result.BlockedDiagnosticsTruncated = result.BlockedDiagnosticsTruncated || page.BlockedDiagnosticsTruncated
		if page.PendingRows {
			return result, nil
		}
		if page.Scanned == 0 {
			result.Complete = true
			return result, nil
		}
		if !resourceRetentionCursorAdvances(cursor, page.Last) {
			return result, ErrResourceRetentionInvalid
		}
		cursor = page.Last
		if !page.HasMore {
			result.Complete = true
			return result, nil
		}
	}
	result.PendingRows = true
	return result, nil
}

func resourceRetentionCursorAdvances(previous, next model.ResourceRetentionCursor) bool {
	if next.SiteID > previous.SiteID {
		return next.MinuteTS > 0 && next.ID > 0
	}
	if next.SiteID < previous.SiteID || next.SiteID == 0 {
		return false
	}
	if next.MinuteTS > previous.MinuteTS {
		return next.ID > 0
	}
	return next.MinuteTS == previous.MinuteTS && next.ID > previous.ID
}
