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

var channelInventoryExportColumns = []string{"site_id", "site_name", "remote_channel_id", "name", "type", "status", "test_time", "response_time_ms", "balance", "balance_updated_at", "models", "group", "used_quota", "priority", "weight", "auto_ban", "tag", "remote_state", "missing_count", "first_seen_at", "last_seen_at", "data_snapshot_at", "exported_at"}

type ChannelInventoryExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.ChannelInventoryQuery
	Format, TemporaryPath                                  string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
	OnPage                                                 func(context.Context, int, int64) error
}

func GenerateChannelInventoryExport(ctx context.Context, o ChannelInventoryExportOptions) (ExportGenerateResult, error) {
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
	if err = w.WriteHeader(channelInventoryExportColumns); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewSiteChannelInventoryRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		rows, total, e := repo.List(ctx, q)
		if e != nil {
			return ExportGenerateResult{}, e
		}
		for _, r := range rows {
			last := ""
			if r.LastSeenAt != nil {
				last = strconv.FormatInt(*r.LastSeenAt, 10)
			}
			values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteChannelID, 10), r.Name, strconv.Itoa(r.RemoteType), strconv.FormatInt(int64(r.RemoteStatus), 10), strconv.FormatInt(r.TestTime, 10), strconv.FormatInt(r.ResponseTimeMS, 10), r.Balance, strconv.FormatInt(r.BalanceUpdatedAt, 10), r.Models, r.RemoteGroup, strconv.FormatInt(r.UsedQuota, 10), strconv.FormatInt(r.Priority, 10), strconv.FormatInt(r.Weight, 10), strconv.Itoa(r.AutoBan), r.Tag, r.RemoteState, strconv.Itoa(r.MissingCount), strconv.FormatInt(r.FirstSeenAt, 10), last, strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
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
		if len(rows) == 0 || count > 100000 {
			return ExportGenerateResult{}, ErrExportContract
		}
	}
	if err = w.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err = file.Sync(); err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	if err = file.Close(); err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	closed = true
	info, err := os.Stat(o.TemporaryPath)
	if err != nil {
		return ExportGenerateResult{}, err
	}
	return ExportGenerateResult{FileSize: info.Size(), RowCount: count}, nil
}
