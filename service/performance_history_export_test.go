package service

import (
	"context"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratePerformanceHistoryExportAverageAndCounters(t *testing.T) {
	db := openUpstreamLogExportDatabase(t)
	now := int64(2100900000)
	site := model.Site{Name: "Performance Export", BaseURL: "https://performance-export-" + time.Now().Format("150405.000000000") + ".example", ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	row := model.SitePerformanceMetricBucket{SiteID: site.ID, ModelName: "=gpt", RemoteGroup: "default", BucketTS: now - 60, SeriesSchema: "ts,avg", MetricSource: model.PerformanceMetricSourceOfficialAverage, AvgTTFTMS: "10.5", AvgLatencyMS: "20.25", SuccessRate: "0.9", AvgTPS: "30.125", ConfigVersion: 1, CollectedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "performance."+format)
		result, err := GeneratePerformanceHistoryExport(context.Background(), PerformanceHistoryExportOptions{Database: db.GORM, Query: dto.PerformanceHistoryQuery{Page: 1, PageSize: 100, StartTimestamp: now - 3600, EndTimestamp: now + 1, SiteIDs: []int64{site.ID}}, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 1 {
			t.Fatalf("%s export=%+v err=%v", format, result, err)
		}
	}
}

func TestPerformanceHistoryExportDoesNotApplyAnUndocumentedRowCap(t *testing.T) {
	done, err := performanceHistoryExportPageDone(100100, 100200, 100)
	if err != nil || done {
		t.Fatalf("large export page done=%t err=%v", done, err)
	}
	if done, err = performanceHistoryExportPageDone(100200, 100200, 100); err != nil || !done {
		t.Fatalf("complete large export done=%t err=%v", done, err)
	}
}
