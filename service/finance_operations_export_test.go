package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestGenerateFinanceOperationsExportsSafeExactFields(t *testing.T) {
	db := openUpstreamLogExportDatabase(t)
	now := int64(2100910000)
	site := model.Site{Name: "Finance Export", BaseURL: "https://finance-export-" + time.Now().Format("150405.000000000") + ".example", ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	seen := now
	if err := db.GORM.Create(&model.SiteTopupOrder{SiteID: site.ID, RemoteID: 1, RemoteUserID: 2, Amount: 9007199254740993, Money: "10.1234567890", PaymentMethod: "=stripe", PaymentProvider: "stripe", CreateTime: now, RemoteStatus: "success", RemoteState: "normal", ConfigVersion: 1, FirstSeenAt: now, LastSeenAt: &seen, CollectedAt: now, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Create(&model.SiteRedemption{SiteID: site.ID, RemoteID: 3, Name: "+batch", RemoteStatus: 1, Quota: 9007199254740993, CreatedTime: now, ExpiredTime: now - 1, RemoteState: "normal", ConfigVersion: 1, FirstSeenAt: now, LastSeenAt: &seen, CollectedAt: now, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"topup", "redemption"} {
		for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
			path := filepath.Join(t.TempDir(), kind+"."+format)
			result, err := GenerateFinanceOperationsExport(context.Background(), FinanceOperationsExportOptions{Database: db.GORM, Query: dto.FinanceInventoryQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}}, Kind: kind, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, Now: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
			if err != nil || result.RowCount != 1 {
				t.Fatalf("%s/%s result=%+v err=%v", kind, format, result, err)
			}
			if format == dto.ExportFormatCSV {
				raw, _ := os.ReadFile(path)
				text := string(raw)
				for _, forbidden := range []string{strings.Join([]string{"trade", "no"}, "_"), strings.Join([]string{"k", "ey"}, "")} {
					if strings.Contains(text, forbidden) {
						t.Fatalf("forbidden export field %q in %s", forbidden, text)
					}
				}
				if !strings.Contains(text, "'=") && !strings.Contains(text, "'+") {
					t.Fatalf("formula injection was not escaped: %s", text)
				}
				if kind == "redemption" && !strings.Contains(text, ",expired,") {
					t.Fatalf("redemption expiration was not derived from the export clock: %s", text)
				}
			}
		}
	}
}

func TestFinanceExportFiltersFreezeEverySupportedDimension(t *testing.T) {
	remoteID, remoteUserID := "9007199254740993", "9007199254740995"
	query, fields := (dto.ExportFilters{
		StartTimestamp: 100, EndTimestamp: 200,
		SiteIDs: []string{"7"}, FinanceStatuses: []string{"success"},
		FinanceProviders: []string{"stripe"}, FinanceMethods: []string{"card"},
		FinanceStates: []string{"missing"}, RemoteID: &remoteID, RemoteUserID: &remoteUserID,
		Keyword: "batch",
	}).FinanceInventoryQuery()
	if fields != nil || len(query.SiteIDs) != 1 || query.SiteIDs[0] != 7 || query.RemoteID == nil || *query.RemoteID != 9007199254740993 || query.RemoteUserID == nil || *query.RemoteUserID != 9007199254740995 || len(query.Statuses) != 1 || query.Statuses[0] != "success" || len(query.Providers) != 1 || query.Providers[0] != "stripe" || len(query.Methods) != 1 || query.Methods[0] != "card" || len(query.States) != 1 || query.States[0] != "missing" || query.Keyword != "batch" || query.StartTimestamp != 100 || query.EndTimestamp != 200 {
		t.Fatalf("frozen finance query=%#v fields=%#v", query, fields)
	}
}
