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

var upstreamLogExportColumns = []string{
	"site_id", "site_name", "created_at", "type", "remote_user_id", "username", "model_name", "token_id", "token_name",
	"channel_id", "group", "request_id", "upstream_request_id", "quota", "prompt_tokens", "completion_tokens",
	"use_time_seconds", "is_stream", "content_redacted", "data_snapshot_at", "exported_at",
}

type UpstreamLogExportOptions struct {
	Database       *gorm.DB
	Query          dto.LogQuery
	Format         string
	TemporaryPath  string
	DataSnapshotAt int64
	ExportedAt     int64
	MaxFileBytes   int64
	MinFreeBytes   int64
	DiskFree       ExportDiskFreeFunc
	OnPage         func(context.Context, int, int64) error
}

func GenerateUpstreamLogExport(ctx context.Context, options UpstreamLogExportOptions) (ExportGenerateResult, error) {
	if options.Database == nil || options.TemporaryPath == "" || options.DataSnapshotAt <= 0 || options.ExportedAt <= 0 ||
		options.MaxFileBytes <= 0 || options.MinFreeBytes <= 0 || (options.Format != dto.ExportFormatCSV && options.Format != dto.ExportFormatXLSX) || options.Query.Validate() != nil {
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
	if err = writer.WriteHeader(upstreamLogExportColumns); err != nil {
		return ExportGenerateResult{}, err
	}
	repository := model.NewUpstreamLogRepository(options.Database)
	var rowCount int64
	for page := 1; ; page++ {
		query := options.Query
		query.Page, query.PageSize = page, 100
		rows, total, readErr := repository.Query(ctx, query)
		if readErr != nil {
			return ExportGenerateResult{}, readErr
		}
		for _, row := range rows {
			values := []string{strconv.FormatInt(row.SiteID, 10), row.SiteName, strconv.FormatInt(row.CreatedAt, 10), strconv.Itoa(row.Type),
				strconv.FormatInt(row.RemoteUserID, 10), row.Username, row.ModelName, strconv.FormatInt(row.TokenID, 10), row.TokenName,
				strconv.FormatInt(row.ChannelID, 10), row.UseGroup, row.RequestID, row.UpstreamRequestID, strconv.FormatInt(row.Quota, 10),
				strconv.FormatInt(row.PromptTokens, 10), strconv.FormatInt(row.CompletionTokens, 10), strconv.FormatInt(row.UseTimeSeconds, 10),
				strconv.FormatBool(row.IsStream), row.ContentRedacted, strconv.FormatInt(options.DataSnapshotAt, 10), strconv.FormatInt(options.ExportedAt, 10)}
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
