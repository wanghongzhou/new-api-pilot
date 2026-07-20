package integration_test

import (
	"context"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestA97LocalModelAndVendorRankings(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	db := openCoreAcceptanceTransaction(t)
	loc := time.FixedZone("Asia/Shanghai", 8*3600)
	nowTime := time.Date(2026, 7, 15, 12, 0, 0, 0, loc)
	now := nowTime.Unix()
	start := time.Date(2026, 7, 15, 0, 0, 0, 0, loc).Unix()
	site1 := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	site2 := createCoreAuthorizedSite(t, db, newCoreCipher(t), now+1)
	metas := []model.SiteModelMeta{{SiteID: site1.ID, RemoteID: 1, ModelName: "gpt", VendorID: 7, RemoteStatus: 1, SyncOfficial: 1, NameRule: 0, SourceHash: strings.Repeat("a", 64), ConfigVersion: site1.ConfigVersion, CollectedAt: now, CreatedAt: now, UpdatedAt: now}, {SiteID: site2.ID, RemoteID: 1, ModelName: "gpt", VendorID: 8, RemoteStatus: 1, SyncOfficial: 1, NameRule: 0, SourceHash: strings.Repeat("b", 64), ConfigVersion: site2.ConfigVersion, CollectedAt: now, CreatedAt: now, UpdatedAt: now}, {SiteID: site1.ID, RemoteID: 2, ModelName: "unknown-model", VendorID: 9, RemoteStatus: 1, SyncOfficial: 1, NameRule: 1, SourceHash: strings.Repeat("c", 64), ConfigVersion: site1.ConfigVersion, CollectedAt: now, CreatedAt: now, UpdatedAt: now}}
	if err := db.Create(&metas).Error; err != nil {
		t.Fatal(err)
	}
	facts := []model.UsageFactHourly{{SiteID: site1.ID, RemoteUserID: 1, ModelName: "gpt", HourTS: start + 3600, RequestCount: 10, Quota: 1000, TokenUsed: 100, CollectedAt: now}, {SiteID: site1.ID, RemoteUserID: 1, ModelName: "unknown-model", HourTS: start + 3600, RequestCount: 5, Quota: 500, TokenUsed: 50, CollectedAt: now}, {SiteID: site2.ID, RemoteUserID: 1, ModelName: "gpt", HourTS: start + 3600, RequestCount: 5, Quota: 500, TokenUsed: 50, CollectedAt: now}, {SiteID: site1.ID, RemoteUserID: 1, ModelName: "gpt", HourTS: start - 3600, RequestCount: 5, Quota: 500, TokenUsed: 50, CollectedAt: now}, {SiteID: site2.ID, RemoteUserID: 1, ModelName: "gpt", HourTS: start - 3600, RequestCount: 5, Quota: 500, TokenUsed: 50, CollectedAt: now}}
	if err := db.Create(&facts).Error; err != nil {
		t.Fatal(err)
	}
	windows := []model.CollectionWindow{}
	for _, site := range []model.Site{site1, site2} {
		for h := start; h < now; h += 3600 {
			status := model.CollectionWindowStatusComplete
			if site.ID == site2.ID && h == now-3600 {
				status = model.CollectionWindowStatusUnavailable
			}
			windows = append(windows, model.CollectionWindow{SiteID: site.ID, HourTS: h, Status: status, UpdatedAt: now})
		}
	}
	if err := db.Create(&windows).Error; err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewLocalRankingService(db, testsupport.NewFakeClock(nowTime))
	if err != nil {
		t.Fatal(err)
	}
	q := dto.LocalRankingQuery{Period: "today", SiteIDs: []int64{site1.ID, site2.ID}}
	models, err := svc.Query(context.Background(), q, "model")
	if err != nil || len(models.Items) != 2 || models.Items[0].DimensionID != "gpt" || models.Items[0].TokenUsed != "150" || models.Items[0].RequestCount != "15" || models.Items[0].Quota != "1500" || models.Items[0].Share != "0.75" || models.Items[0].Growth == nil || *models.Items[0].Growth != "0.5" || models.Items[1].TokenUsed != "50" || models.Items[1].Share != "0.25" || models.Items[1].Growth != nil || models.DataStatus != "partial" {
		t.Fatalf("models=%#v err=%v", models, err)
	}
	if len(models.Movers) != 1 || models.Movers[0].DimensionID != "gpt" || len(models.Droppers) != 0 {
		t.Fatalf("directional movers/droppers=%#v/%#v", models.Movers, models.Droppers)
	}
	vendors, err := svc.Query(context.Background(), q, "vendor")
	foundUnknown := false
	for _, item := range vendors.Items {
		if item.DimensionName == "unknown" {
			foundUnknown = true
		}
	}
	if err != nil || len(vendors.Items) != 3 || vendors.Items[0].DimensionID != "7" || !foundUnknown {
		t.Fatalf("vendors=%#v err=%v", vendors, err)
	}
	if len(models.History) == 0 || len(models.SiteBreakdown) != 3 {
		t.Fatalf("history/breakdown=%#v", models)
	}
	breakdownTotal := int64(0)
	for _, item := range models.SiteBreakdown {
		value, parseErr := strconv.ParseInt(item.TokenUsed, 10, 64)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		breakdownTotal += value
	}
	if breakdownTotal != 200 {
		t.Fatalf("site breakdown total=%d want=200", breakdownTotal)
	}
	empty, err := svc.Query(context.Background(), dto.LocalRankingQuery{Period: "today", SiteIDs: []int64{site2.ID + 1000000}}, "model")
	if err != nil || empty.DataStatus != "pending" || len(empty.Items) != 0 {
		t.Fatalf("empty scope=%#v err=%v", empty, err)
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "ranking."+format)
		result, err := service.GenerateLocalRankingExport(context.Background(), service.LocalRankingExportOptions{Database: db, Query: q, Kind: "model", Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 2 {
			t.Fatalf("export %s result=%+v err=%v", format, result, err)
		}
	}
}
