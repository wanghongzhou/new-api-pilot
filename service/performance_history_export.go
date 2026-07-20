package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"os"
	"strconv"
)

var performanceHistoryExportColumns = []string{"site_id", "site_name", "model_name", "group", "bucket_start", "series_schema", "metric_source", "avg_ttft_ms", "avg_latency_ms", "success_rate", "avg_tps", "request_count", "success_count", "total_latency_ms", "ttft_sum_ms", "ttft_count", "output_tokens", "generation_ms", "collected_at", "data_snapshot_at", "exported_at"}

type PerformanceHistoryExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.PerformanceHistoryQuery
	Format, TemporaryPath                                  string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
	OnPage                                                 func(context.Context, int, int64) error
}

func GeneratePerformanceHistoryExport(ctx context.Context, o PerformanceHistoryExportOptions) (ExportGenerateResult, error) {
	if o.Database == nil || o.TemporaryPath == "" || o.DataSnapshotAt <= 0 || o.ExportedAt <= 0 || o.MaxFileBytes <= 0 || o.MinFreeBytes <= 0 || (o.Format != dto.ExportFormatCSV && o.Format != dto.ExportFormatXLSX) || o.Query.Validate() != nil {
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
	file, err := os.OpenFile(o.TemporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	var w exportRowWriter
	if o.Format == dto.ExportFormatCSV {
		w, err = newCSVExportWriter(file, o.MaxFileBytes)
	} else {
		w, err = newXLSXExportWriter(file, o.TemporaryPath, o.MaxFileBytes, exportXLSXDataRowsPerSheet)
	}
	if err != nil {
		return ExportGenerateResult{}, err
	}
	if err = w.WriteHeader(performanceHistoryExportColumns); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewPerformanceHistoryRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		rows, total, e := repo.List(ctx, q)
		if e != nil {
			return ExportGenerateResult{}, e
		}
		for _, r := range rows {
			v := func(p *int64) string {
				if p == nil {
					return ""
				}
				return strconv.FormatInt(*p, 10)
			}
			values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, r.ModelName, r.RemoteGroup, strconv.FormatInt(r.BucketTS, 10), r.SeriesSchema, r.MetricSource, r.AvgTTFTMS, r.AvgLatencyMS, r.SuccessRate, r.AvgTPS, v(r.RequestCount), v(r.SuccessCount), v(r.TotalLatencyMS), v(r.TTFTSumMS), v(r.TTFTCount), v(r.OutputTokens), v(r.GenerationMS), strconv.FormatInt(r.CollectedAt, 10), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
			if e = w.WriteRow(values); e != nil {
				return ExportGenerateResult{}, e
			}
			count++
		}
		if o.OnPage != nil {
			if e = o.OnPage(ctx, page, count); e != nil {
				return ExportGenerateResult{}, e
			}
		}
		done, pageErr := performanceHistoryExportPageDone(count, total, len(rows))
		if pageErr != nil {
			return ExportGenerateResult{}, pageErr
		}
		if done {
			break
		}
	}
	if err = w.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err = file.Sync(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err = file.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	closed = true
	info, err := os.Stat(o.TemporaryPath)
	if err != nil {
		return ExportGenerateResult{}, err
	}
	return ExportGenerateResult{FileSize: info.Size(), RowCount: count}, nil
}

func performanceHistoryExportPageDone(count, total int64, rows int) (bool, error) {
	if count >= total {
		return true, nil
	}
	if rows == 0 {
		return false, ErrExportContract
	}
	return false, nil
}
