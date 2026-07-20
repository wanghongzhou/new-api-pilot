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

var upstreamTaskExportColumns = []string{"site_id", "site_name", "remote_id", "created_at", "updated_at", "task_id", "platform", "user_id", "group", "channel_id", "quota", "action", "status", "submit_time", "start_time", "finish_time", "progress", "model", "first_seen_at", "last_seen_at", "data_snapshot_at", "exported_at"}

type UpstreamTaskExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.UpstreamTaskQuery
	Format, TemporaryPath                                  string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
	OnPage                                                 func(context.Context, int, int64) error
}

func GenerateUpstreamTaskExport(ctx context.Context, o UpstreamTaskExportOptions) (ExportGenerateResult, error) {
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
	if err = w.WriteHeader(upstreamTaskExportColumns); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewUpstreamTaskRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		rows, total, e := repo.List(ctx, q)
		if e != nil {
			return ExportGenerateResult{}, e
		}
		for _, r := range rows {
			values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteID, 10), strconv.FormatInt(r.RemoteCreatedAt, 10), strconv.FormatInt(r.RemoteUpdatedAt, 10), r.TaskID, r.Platform, strconv.FormatInt(r.RemoteUserID, 10), r.RemoteGroup, strconv.FormatInt(r.RemoteChannelID, 10), strconv.FormatInt(r.Quota, 10), r.Action, r.RemoteStatus, strconv.FormatInt(r.SubmitTime, 10), strconv.FormatInt(r.StartTime, 10), strconv.FormatInt(r.FinishTime, 10), r.Progress, r.ModelName, strconv.FormatInt(r.FirstSeenAt, 10), strconv.FormatInt(r.LastSeenAt, 10), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
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
		if count >= total {
			break
		}
		if len(rows) == 0 {
			return ExportGenerateResult{}, ErrExportContract
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
