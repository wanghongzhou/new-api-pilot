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

type ModelCatalogExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.ModelCatalogQuery
	Format, TemporaryPath                                  string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
	OnPage                                                 func(context.Context, int, int64) error
}

func GenerateModelCatalogExport(ctx context.Context, o ModelCatalogExportOptions) (ExportGenerateResult, error) {
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
	cols := []string{"site_id", "site_name", "remote_id", "model_name", "description", "icon", "tags", "vendor_id", "status", "sync_official", "name_rule", "created_time", "updated_time", "covered_channels", "covered_groups", "data_snapshot_at", "exported_at"}
	if err = w.WriteHeader(cols); err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewModelCatalogRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		rows, total, e := repo.List(ctx, q)
		if e != nil {
			return ExportGenerateResult{}, e
		}
		for _, r := range rows {
			v := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, strconv.FormatInt(r.RemoteID, 10), r.ModelName, r.Description, r.Icon, r.Tags, strconv.FormatInt(r.VendorID, 10), strconv.Itoa(r.RemoteStatus), strconv.Itoa(r.SyncOfficial), strconv.Itoa(r.NameRule), strconv.FormatInt(r.RemoteCreatedTime, 10), strconv.FormatInt(r.RemoteUpdatedTime, 10), strconv.FormatInt(r.CoveredChannels, 10), strconv.FormatInt(r.CoveredGroups, 10), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
			if e = w.WriteRow(v); e != nil {
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
