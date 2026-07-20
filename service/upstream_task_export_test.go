package service

import (
	"context"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateUpstreamTaskExportSafeFields(t *testing.T) {
	db := openUpstreamLogExportDatabase(t)
	now := int64(2101210000)
	site := model.Site{Name: "Task Export", BaseURL: "https://task-export-" + time.Now().Format("150405.000000000") + ".example", ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.SiteUpstreamTask{
		{SiteID: site.ID, RemoteID: 1, RemoteCreatedAt: now - 20, RemoteUpdatedAt: now - 10, TaskID: "=task", Platform: "video", RemoteUserID: 7, RemoteGroup: "default", RemoteChannelID: 3, Quota: 9007199254740993, Action: "generate", RemoteStatus: "SUCCESS", SubmitTime: now - 20, StartTime: now - 15, FinishTime: now - 5, Progress: "100%", ModelName: "safe-model", SourceHash: strings.Repeat("a", 64), ConfigVersion: 1, FirstSeenAt: now, LastSeenAt: now, CollectedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: site.ID, RemoteID: 2, RemoteCreatedAt: now - 20, RemoteUpdatedAt: now - 10, TaskID: "excluded", Platform: "video", RemoteUserID: 8, RemoteGroup: "default", RemoteChannelID: 4, Quota: 1, Action: "generate", RemoteStatus: "SUCCESS", SubmitTime: now - 20, StartTime: now - 15, FinishTime: now - 5, Progress: "100%", ModelName: "safe-model", SourceHash: strings.Repeat("b", 64), ConfigVersion: 1, FirstSeenAt: now, LastSeenAt: now, CollectedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.GORM.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	remoteChannelID := int64(3)
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "tasks."+format)
		result, err := GenerateUpstreamTaskExport(context.Background(), UpstreamTaskExportOptions{Database: db.GORM, Query: dto.UpstreamTaskQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}, RemoteChannelID: &remoteChannelID}, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 1 {
			t.Fatalf("%s result=%+v err=%v", format, result, err)
		}
		if format == dto.ExportFormatCSV {
			raw, _ := os.ReadFile(path)
			text := string(raw)
			header := strings.Split(strings.TrimPrefix(strings.SplitN(text, "\n", 2)[0], "\ufeff"), ",")
			for _, forbidden := range []string{strings.Join([]string{"da", "ta"}, ""), strings.Join([]string{"in", "put"}, ""), strings.Join([]string{"fail", "reason"}, "_"), strings.Join([]string{"result", "url"}, "_"), strings.Join([]string{"private", "data"}, "_")} {
				for _, column := range header {
					if column == forbidden {
						t.Fatalf("forbidden field %q in export: %s", forbidden, text)
					}
				}
			}
			if !strings.Contains(text, "'=") {
				t.Fatalf("formula not escaped: %s", text)
			}
			if strings.Contains(text, "excluded") {
				t.Fatalf("remote_channel_id filter was not applied: %s", text)
			}
		}
	}
}
