package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"new-api-pilot/common"
	"new-api-pilot/dto"
	"os"
	"strconv"
	"time"
)

type rankingExportClock struct{ at time.Time }

func (c rankingExportClock) Now() time.Time { return c.at }
func (c rankingExportClock) NewTimer(d time.Duration) common.Timer {
	return common.SystemClock{}.NewTimer(d)
}
func (c rankingExportClock) NewTicker(d time.Duration) common.Ticker {
	return common.SystemClock{}.NewTicker(d)
}

type LocalRankingExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.LocalRankingQuery
	Kind, Format, TemporaryPath                            string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
}

func GenerateLocalRankingExport(ctx context.Context, o LocalRankingExportOptions) (ExportGenerateResult, error) {
	if o.Database == nil || o.Query.Validate() != nil || (o.Kind != "model" && o.Kind != "vendor") {
		return ExportGenerateResult{}, ErrExportInvalid
	}
	if o.DiskFree == nil {
		o.DiskFree = availableExportDiskBytes
	}
	free, err := o.DiskFree(filepathDir(o.TemporaryPath))
	if err != nil {
		return ExportGenerateResult{}, err
	}
	if free < uint64(o.MinFreeBytes) {
		return ExportGenerateResult{}, &ExportDiskLowError{FreeBytes: free, ThresholdBytes: o.MinFreeBytes}
	}
	svc, err := NewLocalRankingService(o.Database, rankingExportClock{at: time.Unix(o.DataSnapshotAt, 0)})
	if err != nil {
		return ExportGenerateResult{}, err
	}
	response, err := svc.Query(ctx, o.Query, o.Kind)
	if err != nil {
		return ExportGenerateResult{}, err
	}
	f, err := os.OpenFile(o.TemporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	defer f.Close()
	var w exportRowWriter
	if o.Format == dto.ExportFormatCSV {
		w, err = newCSVExportWriter(f, o.MaxFileBytes)
	} else {
		w, err = newXLSXExportWriter(f, o.TemporaryPath, o.MaxFileBytes, exportXLSXDataRowsPerSheet)
	}
	if err != nil {
		return ExportGenerateResult{}, err
	}
	if err = w.WriteHeader([]string{"dimension_id", "dimension_name", "token_used", "request_count", "quota", "share", "growth", "rank", "period", "data_status", "data_snapshot_at", "exported_at"}); err != nil {
		return ExportGenerateResult{}, err
	}
	for _, item := range response.Items {
		growth := ""
		if item.Growth != nil {
			growth = *item.Growth
		}
		if err = w.WriteRow([]string{item.DimensionID, item.DimensionName, item.TokenUsed, item.RequestCount, item.Quota, item.Share, growth, fmtInt(item.Rank), response.Period, response.DataStatus, fmtInt64(o.DataSnapshotAt), fmtInt64(o.ExportedAt)}); err != nil {
			return ExportGenerateResult{}, err
		}
	}
	if err = w.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err = f.Sync(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err = f.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	info, err := os.Stat(o.TemporaryPath)
	if err != nil {
		return ExportGenerateResult{}, err
	}
	return ExportGenerateResult{FileSize: info.Size(), RowCount: int64(len(response.Items))}, nil
}
func fmtInt(v int) string     { return strconv.Itoa(v) }
func fmtInt64(v int64) string { return strconv.FormatInt(v, 10) }
