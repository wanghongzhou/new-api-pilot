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

func TestGenerateChannelInventoryExportSafeDecimalFields(t *testing.T) {
	db := openUpstreamLogExportDatabase(t)
	now := int64(2100600000)
	site := model.Site{Name: "Channel Export", BaseURL: "https://channel-export-" + time.Now().Format("150405.000000000") + ".example", ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	seen := now
	row := model.SiteChannelInventory{SiteID: site.ID, RemoteChannelID: 7, Name: "=primary", RemoteType: 1, RemoteStatus: 1, ResponseTimeMS: 123, Balance: "9007199254740993.123456789", Models: "gpt", RemoteGroup: "default", UsedQuota: 9007199254740993, Priority: 8, Weight: 9, AutoBan: 1, Tag: "prod", RemoteState: model.SiteChannelInventoryNormal, ConfigVersion: 1, FirstSeenAt: now, LastSeenAt: &seen, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "channel."+format)
		result, err := GenerateChannelInventoryExport(context.Background(), ChannelInventoryExportOptions{Database: db.GORM, Query: dto.ChannelInventoryQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}}, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 1 {
			t.Fatalf("%s result=%+v err=%v", format, result, err)
		}
		payload, _ := os.ReadFile(path)
		lower := strings.ToLower(string(payload))
		for _, forbidden := range []string{"secret", "access_token", "multi_key", "header_override", "param_override"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s leaked %s", format, forbidden)
			}
		}
	}
}
