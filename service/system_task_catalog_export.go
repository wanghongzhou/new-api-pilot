package service

import (
	"context"
	"errors"
	"os"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type SystemTaskExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.SystemTaskQuery
	Format, TemporaryPath                                  string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
	OnPage                                                 func(context.Context, int, int64) error
}

func systemTaskExportValue(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}
func GenerateSystemTaskExport(ctx context.Context, o SystemTaskExportOptions) (ExportGenerateResult, error) {
	if o.Database == nil || o.Query.Validate() != nil {
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
	header := []string{"site_id", "site_name", "remote_id", "task_id", "type", "status", "error_present", "error_code", "progress_total", "progress_processed", "progress_percent", "progress_remaining", "deleted_count", "tested", "succeeded", "failed", "disabled", "enabled", "checked_channels", "changed_channels", "detected_add_models", "detected_remove_models", "failed_channels", "auto_added_models", "unfinished_tasks", "channels_scanned", "platforms_scanned", "null_tasks_failed", "remote_created_at", "remote_updated_at", "collected_at", "data_status", "truncated", "truncation_reason", "source_limit", "observed_count", "data_snapshot_at", "exported_at"}
	if err = w.WriteHeader(header); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewSystemTaskRepository(o.Database)
	statuses, _, _, err := repo.CollectionStatuses(ctx, o.Query.SiteIDs)
	if err != nil {
		return ExportGenerateResult{}, err
	}
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		rows, total, e := repo.List(ctx, q)
		if e != nil {
			return ExportGenerateResult{}, e
		}
		for _, r := range rows {
			state := statuses[r.SiteID]
			status := state.DataStatus
			if status == "" {
				status = "pending"
			}
			reason := ""
			switch {
			case state.Truncated && state.IDGap:
				reason = "source_limit_and_id_gap"
			case state.Truncated:
				reason = "source_limit"
			case state.IDGap:
				reason = "id_gap"
			}
			values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteID, 10), r.RemoteTaskID, r.TaskType, r.RemoteStatus, strconv.FormatBool(r.ErrorPresent), r.ErrorCode, systemTaskExportValue(r.Total), systemTaskExportValue(r.Processed), systemTaskExportValue(r.Progress), systemTaskExportValue(r.Remaining), systemTaskExportValue(r.DeletedCount), systemTaskExportValue(r.Tested), systemTaskExportValue(r.Succeeded), systemTaskExportValue(r.Failed), systemTaskExportValue(r.Disabled), systemTaskExportValue(r.Enabled), systemTaskExportValue(r.CheckedChannels), systemTaskExportValue(r.ChangedChannels), systemTaskExportValue(r.DetectedAddModels), systemTaskExportValue(r.DetectedRemoveModels), systemTaskExportValue(r.FailedChannels), systemTaskExportValue(r.AutoAddedModels), systemTaskExportValue(r.UnfinishedTasks), systemTaskExportValue(r.ChannelsScanned), systemTaskExportValue(r.PlatformsScanned), systemTaskExportValue(r.NullTasksFailed), strconv.FormatInt(r.RemoteCreatedAt, 10), strconv.FormatInt(r.RemoteUpdatedAt, 10), strconv.FormatInt(r.CollectedAt, 10), status, strconv.FormatBool(state.Truncated || state.IDGap), reason, "100", strconv.FormatInt(state.ObservedCount, 10), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
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
	return ExportGenerateResult{FileSize: info.Size(), RowCount: count}, nil
}
