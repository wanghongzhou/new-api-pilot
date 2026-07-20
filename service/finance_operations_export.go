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

var topupExportColumns = []string{"site_id", "site_name", "remote_id", "remote_user_id", "amount", "money", "payment_method", "payment_provider", "create_time", "complete_time", "status", "remote_state", "missing_count", "first_seen_at", "last_seen_at", "data_snapshot_at", "exported_at"}
var redemptionExportColumns = []string{"site_id", "site_name", "remote_id", "remote_user_id", "name", "status", "derived_status", "quota", "created_time", "redeemed_time", "used_user_id", "expired_time", "remote_state", "missing_count", "first_seen_at", "last_seen_at", "data_snapshot_at", "exported_at"}

type FinanceOperationsExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.FinanceInventoryQuery
	Kind, Format, TemporaryPath                            string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
	OnPage                                                 func(context.Context, int, int64) error
	Now                                                    int64
}

func GenerateFinanceOperationsExport(ctx context.Context, o FinanceOperationsExportOptions) (ExportGenerateResult, error) {
	if o.Database == nil || o.TemporaryPath == "" || o.DataSnapshotAt <= 0 || o.ExportedAt <= 0 || o.MaxFileBytes <= 0 || o.MinFreeBytes <= 0 || (o.Kind != "topup" && o.Kind != "redemption") || (o.Format != dto.ExportFormatCSV && o.Format != dto.ExportFormatXLSX) || o.Query.Validate() != nil {
		return ExportGenerateResult{}, ErrExportInvalid
	}
	if o.Now <= 0 {
		o.Now = o.DataSnapshotAt
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
	var writer exportRowWriter
	if o.Format == dto.ExportFormatCSV {
		writer, err = newCSVExportWriter(file, o.MaxFileBytes)
	} else {
		writer, err = newXLSXExportWriter(file, o.TemporaryPath, o.MaxFileBytes, exportXLSXDataRowsPerSheet)
	}
	if err != nil {
		return ExportGenerateResult{}, err
	}
	columns := topupExportColumns
	if o.Kind == "redemption" {
		columns = redemptionExportColumns
	}
	if err = writer.WriteHeader(columns); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewFinanceRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		if o.Kind == "topup" {
			rows, total, e := repo.ListTopups(ctx, q)
			if e != nil {
				return ExportGenerateResult{}, e
			}
			for _, r := range rows {
				last := ""
				if r.LastSeenAt != nil {
					last = strconv.FormatInt(*r.LastSeenAt, 10)
				}
				values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteID, 10), strconv.FormatInt(r.RemoteUserID, 10), strconv.FormatInt(r.Amount, 10), r.Money, r.PaymentMethod, r.PaymentProvider, strconv.FormatInt(r.CreateTime, 10), strconv.FormatInt(r.CompleteTime, 10), r.RemoteStatus, r.RemoteState, strconv.Itoa(r.MissingCount), strconv.FormatInt(r.FirstSeenAt, 10), last, strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
				if e = writer.WriteRow(values); e != nil {
					return ExportGenerateResult{}, e
				}
				count++
			}
			if count >= total {
				break
			}
			if len(rows) == 0 || count > 100000 {
				return ExportGenerateResult{}, ErrExportContract
			}
		} else {
			rows, total, e := repo.ListRedemptions(ctx, q)
			if e != nil {
				return ExportGenerateResult{}, e
			}
			for _, r := range rows {
				last := ""
				if r.LastSeenAt != nil {
					last = strconv.FormatInt(*r.LastSeenAt, 10)
				}
				derived := strconv.Itoa(r.RemoteStatus)
				if r.RemoteStatus == 1 && r.ExpiredTime != 0 && r.ExpiredTime < o.Now {
					derived = "expired"
				}
				values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteID, 10), strconv.FormatInt(r.RemoteUserID, 10), r.Name, strconv.Itoa(r.RemoteStatus), derived, strconv.FormatInt(r.Quota, 10), strconv.FormatInt(r.CreatedTime, 10), strconv.FormatInt(r.RedeemedTime, 10), strconv.FormatInt(r.UsedUserID, 10), strconv.FormatInt(r.ExpiredTime, 10), r.RemoteState, strconv.Itoa(r.MissingCount), strconv.FormatInt(r.FirstSeenAt, 10), last, strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
				if e = writer.WriteRow(values); e != nil {
					return ExportGenerateResult{}, e
				}
				count++
			}
			if count >= total {
				break
			}
			if len(rows) == 0 || count > 100000 {
				return ExportGenerateResult{}, ErrExportContract
			}
		}
		if o.OnPage != nil {
			if err = o.OnPage(ctx, page, count); err != nil {
				return ExportGenerateResult{}, err
			}
		}
	}
	if err = writer.Close(); err != nil {
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
