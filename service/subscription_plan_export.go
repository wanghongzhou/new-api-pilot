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

type SubscriptionPlanExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.SubscriptionPlanQuery
	Format, TemporaryPath                                  string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
}

func GenerateSubscriptionPlanExport(ctx context.Context, o SubscriptionPlanExportOptions) (ExportGenerateResult, error) {
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
	cols := []string{"site_id", "site_name", "remote_id", "title", "subtitle", "price_amount", "currency", "duration_unit", "duration_value", "custom_seconds", "enabled", "sort_order", "total_amount", "quota_reset_period", "quota_reset_custom_seconds", "created_at", "updated_at", "remote_state", "missing_count", "data_snapshot_at", "exported_at"}
	if err = w.WriteHeader(cols); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewSubscriptionPlanRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		rows, total, e := repo.List(ctx, q)
		if e != nil {
			return ExportGenerateResult{}, e
		}
		for _, r := range rows {
			price, _ := canonicalNonNegativeMoneyDecimal(r.PriceAmount)
			v := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteID, 10), r.Title, r.Subtitle, price, r.Currency, r.DurationUnit, strconv.Itoa(r.DurationValue), strconv.FormatInt(r.CustomSeconds, 10), strconv.FormatBool(r.Enabled), strconv.Itoa(r.SortOrder), strconv.FormatInt(r.TotalAmount, 10), r.QuotaResetPeriod, strconv.FormatInt(r.QuotaResetCustomSeconds, 10), strconv.FormatInt(r.RemoteCreatedAt, 10), strconv.FormatInt(r.RemoteUpdatedAt, 10), r.RemoteState, strconv.Itoa(r.MissingCount), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
			if e = w.WriteRow(v); e != nil {
				return ExportGenerateResult{}, e
			}
			count++
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
