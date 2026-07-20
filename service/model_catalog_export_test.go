package service

import (
	"context"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateModelCatalogExportCSVAndXLSX(t *testing.T) {
	db := openUpstreamLogExportDatabase(t)
	now := int64(2101310000)
	site := model.Site{Name: "Model Export", BaseURL: "https://model-export-" + time.Now().Format("150405.000000000") + ".example", ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	row := model.SiteModelMeta{SiteID: site.ID, RemoteID: 1, ModelName: "=safe", Description: "description", Icon: "https://icons.invalid/raw.svg", Tags: "chat", VendorID: 7, RemoteStatus: 1, SyncOfficial: 1, NameRule: 0, RemoteCreatedTime: 1, RemoteUpdatedTime: 2, SourceHash: strings.Repeat("a", 64), ConfigVersion: 1, CollectedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "models."+format)
		result, err := GenerateModelCatalogExport(context.Background(), ModelCatalogExportOptions{Database: db.GORM, Query: dto.ModelCatalogQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}}, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 1 {
			t.Fatalf("format=%s result=%+v err=%v", format, result, err)
		}
	}
}
