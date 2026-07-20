package model

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSiteUserInventorySnapshotAtomicStatesAndHourly(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_100_200_000)
	hour := now - now%3600
	site := createRunnableSite(t, database, fmt.Sprintf("inventory-%d", time.Now().UnixNano()), now)
	repository := NewCollectionTaskRepository(database.GORM)
	initial := []SiteUserObservation{
		{RemoteUserID: 1, RemoteCreatedAt: hour - 7200, Username: "alice", DisplayName: "Alice", RemoteRole: 1, RemoteStatus: 1, RemoteGroup: "vip", Quota: 100, UsedQuota: 40, RequestCount: 7, LastLoginAt: hour + 60},
		{RemoteUserID: 2, RemoteCreatedAt: hour + 120, Username: "bob", DisplayName: "Bob", RemoteRole: 2, RemoteStatus: 1, RemoteGroup: "default", Quota: 200, UsedQuota: 50, RequestCount: 9},
	}
	written, err := repository.ApplySiteUserSnapshot(context.Background(), site, now, hour, initial)
	if err != nil || written != 4 {
		t.Fatalf("initial inventory snapshot written=%d err=%v", written, err)
	}
	var inventory []SiteUserInventory
	if err := database.GORM.Where("site_id = ?", site.ID).Order("remote_user_id").Find(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 2 || inventory[0].RemoteState != SiteUserInventoryNormal || inventory[1].RemoteState != SiteUserInventoryNormal {
		t.Fatalf("initial inventory = %+v", inventory)
	}
	var hourly []SiteUserInventoryHourly
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", site.ID, hour).Order("remote_role").Find(&hourly).Error; err != nil {
		t.Fatal(err)
	}
	if len(hourly) != 2 || hourly[0].ActiveUserCount+hourly[1].ActiveUserCount != 1 || hourly[0].NewUserCount+hourly[1].NewUserCount != 1 {
		t.Fatalf("hourly inventory = %+v", hourly)
	}

	conflict := initial[0]
	conflict.RemoteCreatedAt++
	if _, err := repository.ApplySiteUserSnapshot(context.Background(), site, now+1, hour, []SiteUserObservation{conflict}); err != nil {
		t.Fatalf("conflict inventory snapshot: %v", err)
	}
	inventory = nil
	if err := database.GORM.Where("site_id = ?", site.ID).Order("remote_user_id").Find(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory[0].RemoteState != SiteUserInventoryIdentityMismatch || inventory[1].RemoteState != SiteUserInventoryMissing {
		t.Fatalf("conflict/missing inventory = %+v", inventory)
	}

	duplicate := []SiteUserObservation{initial[0], initial[0]}
	if _, err := repository.ApplySiteUserSnapshot(context.Background(), site, now+2, hour, duplicate); err == nil {
		t.Fatal("duplicate remote user IDs were accepted")
	}
	var count int64
	if err := database.GORM.Model(&SiteUserInventory{}).Where("site_id = ?", site.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("failed snapshot partially committed count=%d err=%v", count, err)
	}
}
