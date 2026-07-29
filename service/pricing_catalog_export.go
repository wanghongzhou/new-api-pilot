package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type PricingCatalogExportOptions struct {
	Database                                               *gorm.DB
	Query                                                  dto.PricingCatalogQuery
	Kind, Format, TemporaryPath                            string
	DataSnapshotAt, ExportedAt, MaxFileBytes, MinFreeBytes int64
	DiskFree                                               ExportDiskFreeFunc
}

func GeneratePricingCatalogExport(ctx context.Context, o PricingCatalogExportOptions) (ExportGenerateResult, error) {
	if o.Database == nil || o.Query.Validate() != nil || (o.Kind != "pricing" && o.Kind != "group") {
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
	if o.Kind == "pricing" {
		err = w.WriteHeader([]string{"site_id", "site_name", "model_name", "vendor_name", "vendor_id", "quota_type", "billing_mode", "billing_expr", "pricing_source", "ability_available", "model_ratio", "model_price", "completion_ratio", "cache_ratio", "create_cache_ratio", "image_ratio", "audio_ratio", "audio_completion_ratio", "owner_by", "enable_groups", "supported_endpoint_types", "pricing_version", "remote_state", "missing_count", "collected_at", "data_snapshot_at", "exported_at"})
	} else {
		err = w.WriteHeader([]string{"site_id", "site_name", "name", "ratio", "topup_ratio", "description", "user_selectable", "default_use_auto_group", "auto_priority", "outgoing_overrides", "incoming_overrides", "visible_to_groups", "hidden_from_groups", "remote_state", "missing_count", "active_pricing_count", "missing_pricing_count", "model_names", "missing_model_names", "collected_at", "data_snapshot_at", "exported_at"})
	}
	if err != nil {
		return ExportGenerateResult{}, err
	}
	repo := model.NewPricingCatalogRepository(o.Database)
	var count int64
	for page := 1; ; page++ {
		q := o.Query
		q.Page, q.PageSize = page, 100
		if o.Kind == "pricing" {
			rows, total, e := repo.List(ctx, q)
			if e != nil {
				return ExportGenerateResult{}, e
			}
			for _, r := range rows {
				ptr := func(v *string) string {
					if v == nil {
						return ""
					}
					return *v
				}
				decimalPtr := func(v *string) string {
					raw := ptr(v)
					if raw == "" {
						return ""
					}
					return pricingOutputDecimal(raw)
				}
				values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, r.ModelName, r.VendorKey, strconv.FormatInt(r.VendorID, 10), strconv.FormatInt(r.QuotaType, 10), r.BillingMode, r.BillingExpr, r.PricingSource, strconv.FormatBool(r.AbilityAvailable), pricingOutputDecimal(r.ModelRatio), pricingOutputDecimal(r.ModelPrice), pricingOutputDecimal(r.CompletionRatio), decimalPtr(r.CacheRatio), decimalPtr(r.CreateCacheRatio), decimalPtr(r.ImageRatio), decimalPtr(r.AudioRatio), decimalPtr(r.AudioCompletionRatio), r.OwnerBy, strings.Join(pricingStrings(r.EnableGroupsJSON), "|"), strings.Join(pricingStrings(r.SupportedEndpointTypesJSON), "|"), r.PricingVersion, r.RemoteState, strconv.Itoa(r.MissingCount), strconv.FormatInt(r.CollectedAt, 10), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
				if e = w.WriteRow(values); e != nil {
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
		} else {
			rows, total, e := repo.ListGroups(ctx, q)
			if e != nil {
				return ExportGenerateResult{}, e
			}
			associationRows, e := repo.GroupAssociations(ctx, rows)
			if e != nil {
				return ExportGenerateResult{}, e
			}
			associations := map[string]pricingGroupAssociation{}
			for _, associationRow := range associationRows {
				key := strconv.FormatInt(associationRow.SiteID, 10) + "\x00" + associationRow.GroupName
				association := associations[key]
				if association.models == nil {
					association.models = map[string]struct{}{}
					association.missingModels = map[string]struct{}{}
				}
				if associationRow.RemoteState == "missing" {
					association.missing++
					association.missingModels[associationRow.ModelName] = struct{}{}
				} else {
					association.active++
					association.models[associationRow.ModelName] = struct{}{}
				}
				associations[key] = association
			}
			for _, r := range rows {
				ratio := ""
				if r.RatioDecimal != nil {
					ratio = pricingOutputDecimal(*r.RatioDecimal)
				}
				topupRatio := ""
				if r.TopupRatioDecimal != nil {
					topupRatio = pricingOutputDecimal(*r.TopupRatioDecimal)
				}
				autoPriority := ""
				if r.AutoPriority != nil {
					autoPriority = strconv.Itoa(*r.AutoPriority)
				}
				jsonMap := func(raw string) string {
					value, _ := json.Marshal(pricingStringMap(raw))
					return string(value)
				}
				association := associations[strconv.FormatInt(r.SiteID, 10)+"\x00"+r.GroupName]
				values := []string{strconv.FormatInt(r.SiteID, 10), r.SiteName, r.GroupName, ratio, topupRatio, r.Description, strconv.FormatBool(r.UserSelectable), strconv.FormatBool(r.DefaultUseAutoGroup), autoPriority, jsonMap(r.OutgoingOverridesJSON), jsonMap(r.IncomingOverridesJSON), jsonMap(r.VisibleToGroupsJSON), strings.Join(pricingStrings(r.HiddenFromGroupsJSON), "|"), r.RemoteState, strconv.Itoa(r.MissingCount), strconv.FormatInt(association.active, 10), strconv.FormatInt(association.missing, 10), strings.Join(sortedPricingKeys(association.models), "|"), strings.Join(sortedPricingKeys(association.missingModels), "|"), strconv.FormatInt(r.CollectedAt, 10), strconv.FormatInt(o.DataSnapshotAt, 10), strconv.FormatInt(o.ExportedAt, 10)}
				if e = w.WriteRow(values); e != nil {
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
