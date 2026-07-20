package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestGenerateUserInventoryExportCSVAndXLSXUsesSafeFieldsOnly(t *testing.T) {
	database := openUpstreamLogExportDatabase(t)
	now := int64(2_100_300_000)
	site := model.Site{Name: "Inventory Export", BaseURL: "https://inventory-export-" + time.Now().Format("150405.000000000") + ".example",
		ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK,
		DataExportEnabled: true, CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create inventory export site: %v", err)
	}
	row := model.SiteUserInventory{SiteID: site.ID, RemoteUserID: 7, RemoteCreatedAt: now - 100, Username: "=alice", DisplayName: "Alice",
		RemoteRole: 10, RemoteStatus: 1, RemoteGroup: "vip", Quota: 100, UsedQuota: 40, RequestCount: 9, LastLoginAt: now - 10,
		RemoteState: model.SiteUserInventoryNormal, ConfigVersion: 1, FirstSeenAt: now, LastSeenAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&row).Error; err != nil {
		t.Fatalf("create inventory export row: %v", err)
	}
	query := dto.UserInventoryQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "inventory."+format)
		result, err := GenerateUserInventoryExport(context.Background(), UserInventoryExportOptions{Database: database.GORM, Query: query, Format: format,
			TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1,
			DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 1 || result.FileSize <= 0 {
			t.Fatalf("%s inventory export = %+v, %v", format, result, err)
		}
		var contents string
		if format == dto.ExportFormatCSV {
			payload, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			contents = string(payload)
			if !strings.Contains(contents, "'=alice") {
				t.Fatalf("CSV formula value was not escaped: %s", contents)
			}
		} else {
			book, openErr := excelize.OpenFile(path)
			if openErr != nil {
				t.Fatal(openErr)
			}
			rows, rowsErr := book.GetRows(book.GetSheetName(0))
			formula, formulaErr := book.GetCellFormula(book.GetSheetName(0), "E2")
			_ = book.Close()
			if rowsErr != nil || formulaErr != nil || len(rows) != 2 || formula != "" {
				t.Fatalf("XLSX inventory rows=%#v formula=%q errors=%v/%v", rows, formula, rowsErr, formulaErr)
			}
			contents = strings.Join(rows[0], "|") + "|" + strings.Join(rows[1], "|")
		}
		lower := strings.ToLower(contents)
		for _, forbidden := range []string{"email", "password", "access_token", "oauth", "wechat", "github", "setting", "stripe"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s inventory export leaked forbidden field %q: %s", format, forbidden, contents)
			}
		}
	}
}
