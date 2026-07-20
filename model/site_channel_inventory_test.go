package model

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestSiteChannelInventorySnapshotDecimalMissingAndAtomicity(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2100500000)
	site := createRunnableSite(t, database, fmt.Sprintf("channel-inventory-%d", time.Now().UnixNano()), now)
	channels := []SiteChannel{{RemoteChannelID: 1, Name: "Primary", RemoteType: 1, RemoteStatus: 1, TestTime: now - 1, ResponseTimeMS: 123, Balance: "9007199254740993.123456789", BalanceUpdatedAt: now - 2, Models: "gpt-4o", RemoteGroup: "default", UsedQuota: 9007199254740993, Priority: 10, Weight: 20, AutoBan: 1, Tag: "primary"}, {RemoteChannelID: 2, Name: "Backup", RemoteType: 2, RemoteStatus: 2, ResponseTimeMS: 800, Balance: "0.1", Models: "claude", RemoteGroup: "vip"}}
	if err := database.GORM.Transaction(func(tx *gorm.DB) error {
		return NewSiteRepository(tx).SyncChannels(context.Background(), site.ID, now, channels)
	}); err != nil {
		t.Fatalf("initial channel snapshot: %v", err)
	}
	var rows []SiteChannelInventory
	if err := database.GORM.Where("site_id=?", site.ID).Order("remote_channel_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Balance != "9007199254740993.1234567890" || rows[0].RemoteState != SiteChannelInventoryNormal {
		t.Fatalf("channel inventory=%+v", rows)
	}
	var hourly SiteChannelInventoryHourly
	if err := database.GORM.Where("site_id=?", site.ID).Take(&hourly).Error; err != nil {
		t.Fatal(err)
	}
	if hourly.ChannelCount != 2 || hourly.AvailableCount != 1 || hourly.BalanceTotal != "9007199254740993.2234567890" {
		t.Fatalf("channel hourly=%+v", hourly)
	}
	if err := database.GORM.Transaction(func(tx *gorm.DB) error {
		return NewSiteRepository(tx).SyncChannels(context.Background(), site.ID, now+3600, channels[:1])
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Where("site_id=? AND remote_channel_id=2", site.ID).Take(&rows[1]).Error; err != nil || rows[1].RemoteState != SiteChannelInventoryMissing {
		t.Fatalf("missing channel=%+v err=%v", rows[1], err)
	}
	before := rows[1].UpdatedAt
	duplicate := []SiteChannel{channels[0], channels[0]}
	if err := database.GORM.Transaction(func(tx *gorm.DB) error {
		return NewSiteRepository(tx).SyncChannels(context.Background(), site.ID, now+7200, duplicate)
	}); err == nil {
		t.Fatal("duplicate channel snapshot accepted")
	}
	var preserved SiteChannelInventory
	_ = database.GORM.Where("site_id=? AND remote_channel_id=2", site.ID).Take(&preserved).Error
	if preserved.UpdatedAt != before {
		t.Fatalf("failed snapshot partially committed: %+v", preserved)
	}
}
