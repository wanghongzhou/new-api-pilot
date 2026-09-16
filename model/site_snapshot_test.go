package model

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSyncInstancesSkipsUnchangedCurrentRowButKeepsMinuteFacts(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	now := int64(2_100_600_000)
	site := createRunnableSite(t, database, fmt.Sprintf("instance-delta-%d", time.Now().UnixNano()), now)
	repository := NewSiteRepository(database.GORM)
	startedAt := now - 3600
	lastSeenAt := now - 1
	staleAfter := int64(90)
	firstMinute := now - now%60

	write := func(minute, syncedAt int64) error {
		cpu := 12.5
		return repository.SyncInstances(context.Background(), []SiteInstanceWrite{{
			Instance: SiteInstance{
				SiteID: site.ID, NodeName: "node-1", Hostname: "host-1", IsMaster: true,
				RuntimeVersion: "v1", GOOS: "linux", GOARCH: "amd64", UpstreamStatus: "online",
				UpstreamStaleAfterSeconds: &staleAfter, CurrentStatus: "online", FirstSeenAt: now,
				StartedAt: &startedAt, LastSeenAt: &lastSeenAt, LastSyncedAt: syncedAt,
				CreatedAt: now, UpdatedAt: syncedAt,
			},
			Sample: SiteInstanceStatusMinutely{
				SiteID: site.ID, NodeName: "node-1", MinuteTS: minute, Status: "online",
				CPUPercent: &cpu, StartedAt: &startedAt, LastSeenAt: &lastSeenAt, CreatedAt: syncedAt,
			},
		}})
	}

	if err := write(firstMinute, now); err != nil {
		t.Fatalf("first instance sync: %v", err)
	}
	if err := write(firstMinute+60, now+60); err != nil {
		t.Fatalf("unchanged instance sync: %v", err)
	}

	var current SiteInstance
	if err := database.GORM.Where("site_id=? AND node_name=?", site.ID, "node-1").Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.LastSyncedAt != now || current.UpdatedAt != now {
		t.Fatalf("unchanged current row was refreshed: last_synced_at=%d updated_at=%d", current.LastSyncedAt, current.UpdatedAt)
	}
	var minuteCount int64
	if err := database.GORM.Model(&SiteInstanceStatusMinutely{}).
		Where("site_id=? AND node_name=? AND minute_ts IN ?", site.ID, "node-1", []int64{firstMinute, firstMinute + 60}).
		Count(&minuteCount).Error; err != nil {
		t.Fatal(err)
	}
	if minuteCount != 2 {
		t.Fatalf("minute fact count=%d, want 2", minuteCount)
	}
}
