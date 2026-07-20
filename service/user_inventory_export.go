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

var userInventoryExportColumns = []string{
	"site_id", "site_name", "remote_user_id", "remote_created_at", "username", "display_name", "role", "status", "group",
	"quota", "used_quota", "balance", "request_count", "last_login_at", "remote_state", "missing_count", "first_seen_at",
	"last_seen_at", "account_id", "data_snapshot_at", "exported_at",
}

type UserInventoryExportOptions struct {
	Database       *gorm.DB
	Query          dto.UserInventoryQuery
	Format         string
	TemporaryPath  string
	DataSnapshotAt int64
	ExportedAt     int64
	MaxFileBytes   int64
	MinFreeBytes   int64
	DiskFree       ExportDiskFreeFunc
	OnPage         func(context.Context, int, int64) error
}

func GenerateUserInventoryExport(ctx context.Context, options UserInventoryExportOptions) (ExportGenerateResult, error) {
	if options.Database == nil || options.TemporaryPath == "" || options.DataSnapshotAt <= 0 || options.ExportedAt <= 0 || options.MaxFileBytes <= 0 ||
		options.MinFreeBytes <= 0 || (options.Format != dto.ExportFormatCSV && options.Format != dto.ExportFormatXLSX) || options.Query.Validate() != nil {
		return ExportGenerateResult{}, ErrExportInvalid
	}
	if options.DiskFree == nil {
		options.DiskFree = availableExportDiskBytes
	}
	free, err := options.DiskFree(filepathDir(options.TemporaryPath))
	if err != nil {
		return ExportGenerateResult{}, err
	}
	if free < uint64(options.MinFreeBytes) {
		return ExportGenerateResult{}, &ExportDiskLowError{FreeBytes: free, ThresholdBytes: options.MinFreeBytes}
	}
	file, err := os.OpenFile(options.TemporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	var writer exportRowWriter
	if options.Format == dto.ExportFormatCSV {
		writer, err = newCSVExportWriter(file, options.MaxFileBytes)
	} else {
		writer, err = newXLSXExportWriter(file, options.TemporaryPath, options.MaxFileBytes, exportXLSXDataRowsPerSheet)
	}
	if err != nil {
		return ExportGenerateResult{}, err
	}
	if err := writer.WriteHeader(userInventoryExportColumns); err != nil {
		return ExportGenerateResult{}, err
	}
	repository := model.NewSiteUserInventoryRepository(options.Database)
	var rowCount int64
	for page := 1; ; page++ {
		query := options.Query
		query.Page, query.PageSize = page, 100
		rows, total, readErr := repository.List(ctx, query)
		if readErr != nil {
			return ExportGenerateResult{}, readErr
		}
		for _, row := range rows {
			lastSeen, accountID := "", ""
			if row.LastSeenAt != nil {
				lastSeen = strconv.FormatInt(*row.LastSeenAt, 10)
			}
			if row.AccountID != nil {
				accountID = strconv.FormatInt(*row.AccountID, 10)
			}
			values := []string{strconv.FormatInt(row.SiteID, 10), row.SiteName, strconv.FormatInt(row.RemoteUserID, 10), strconv.FormatInt(row.RemoteCreatedAt, 10),
				row.Username, row.DisplayName, strconv.Itoa(row.RemoteRole), strconv.Itoa(row.RemoteStatus), row.RemoteGroup, strconv.FormatInt(row.Quota, 10),
				strconv.FormatInt(row.UsedQuota, 10), strconv.FormatInt(row.Balance, 10), strconv.FormatInt(row.RequestCount, 10), strconv.FormatInt(row.LastLoginAt, 10),
				row.RemoteState, strconv.Itoa(row.MissingCount), strconv.FormatInt(row.FirstSeenAt, 10), lastSeen, accountID,
				strconv.FormatInt(options.DataSnapshotAt, 10), strconv.FormatInt(options.ExportedAt, 10)}
			if err := writer.WriteRow(values); err != nil {
				return ExportGenerateResult{}, err
			}
			rowCount++
		}
		if options.OnPage != nil {
			if err := options.OnPage(ctx, page, rowCount); err != nil {
				return ExportGenerateResult{}, err
			}
		}
		if rowCount >= total {
			break
		}
		if len(rows) == 0 || rowCount > 100000 {
			return ExportGenerateResult{}, ErrExportContract
		}
	}
	if err := writer.Close(); err != nil {
		return ExportGenerateResult{}, err
	}
	if err := file.Sync(); err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	if err := file.Close(); err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	closed = true
	info, err := os.Stat(options.TemporaryPath)
	if err != nil {
		return ExportGenerateResult{}, errors.Join(ErrExportWrite, err)
	}
	if info.Size() > options.MaxFileBytes {
		return ExportGenerateResult{}, &ExportFileTooLargeError{ObservedBytes: info.Size(), LimitBytes: options.MaxFileBytes}
	}
	return ExportGenerateResult{FileSize: info.Size(), RowCount: rowCount}, nil
}
