package model

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"testing"
	"time"
)

func TestUpstreamTaskFailureStateRejectsStaleConfigVersion(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2101199000)
	site := createRunnableSite(t, db, fmt.Sprintf("task-fence-%d", time.Now().UnixNano()), now)
	stale := site
	if err := db.GORM.Model(&Site{}).Where("id = ?", site.ID).Update("config_version", site.ConfigVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewSiteRepository(db.GORM).MarkUpstreamTaskCollectionFailure(context.Background(), stale, now+1, "TEST_FAILURE"); !errors.Is(err, ErrSiteRunConfigChanged) {
		t.Fatalf("stale failure fence error = %v", err)
	}
	var count int64
	if err := db.GORM.Model(&SiteUpstreamTaskCollectionState{}).Where("site_id = ?", site.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("stale failure state count=%d err=%v", count, err)
	}
}

func taskFixture(id, updated int64, status string) dto.UpstreamTask {
	return dto.UpstreamTask{ID: id, CreatedAt: 10, UpdatedAt: updated, TaskID: fmt.Sprintf("task_%d", id), Platform: "video", UserID: 7, Group: "default", ChannelID: 3, Quota: 9007199254740993, Action: "generate", Status: status, SubmitTime: 10, StartTime: 12, FinishTime: func() int64 {
		if UpstreamTaskTerminal(status) {
			return 20
		}
		return 0
	}(), Progress: "50%", Properties: dto.UpstreamTaskProperties{Model: "safe-model"}}
}
func TestUpstreamTaskTransitionIdempotencyAndRetention(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2101200000)
	site := createRunnableSite(t, db, fmt.Sprintf("task-%d", time.Now().UnixNano()), now)
	repo := NewSiteRepository(db.GORM)
	initial := dto.UpstreamTaskSnapshot{Items: []dto.UpstreamTask{taskFixture(1, 11, "IN_PROGRESS"), taskFixture(2, 11, "IN_PROGRESS")}}
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncUpstreamTasks(context.Background(), site, now, now-48*3600, initial)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var row SiteUpstreamTask
	if err := db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	updatedAt := row.UpdatedAt
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		written, err := NewSiteRepository(tx).SyncUpstreamTasks(context.Background(), site, now+1, now-48*3600, initial)
		if err == nil && written != 0 {
			t.Fatalf("idempotent written=%d", written)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&row).Error
	if row.UpdatedAt != updatedAt || row.LastSeenAt != now+1 {
		t.Fatalf("idempotent row=%+v", row)
	}
	finished := taskFixture(1, 12, "SUCCESS")
	finished.FinishTime = 20
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		written, err := NewSiteRepository(tx).SyncUpstreamTasks(context.Background(), site, now+2, now-48*3600, dto.UpstreamTaskSnapshot{Items: []dto.UpstreamTask{finished}})
		if err == nil && written != 1 {
			t.Fatalf("transition written=%d", written)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.GORM.Where("site_id=? AND remote_id=1", site.ID).Take(&row).Error
	if row.RemoteStatus != "SUCCESS" || row.RemoteUpdatedAt != 12 {
		t.Fatalf("transition row=%+v", row)
	}
	if err := repo.DeleteTerminalUpstreamTasksBefore(context.Background(), 21); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	_ = db.GORM.Model(&SiteUpstreamTask{}).Where("site_id=?", site.ID).Order("remote_id").Pluck("remote_id", &ids).Error
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("retention ids=%v", ids)
	}
	unfinished, err := repo.ListUnfinishedUpstreamTaskIDs(context.Background(), site.ID)
	if err != nil || len(unfinished) != 1 || unfinished[0] != "task_2" {
		t.Fatalf("unfinished=%v err=%v", unfinished, err)
	}
}

func TestUpstreamTaskStatisticsUseExactDenominators(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2101201000)
	site := createRunnableSite(t, db, fmt.Sprintf("task-stats-%d", time.Now().UnixNano()), now)
	items := []dto.UpstreamTask{taskFixture(1, 11, "SUCCESS"), taskFixture(2, 11, "FAILURE"), taskFixture(3, 11, "IN_PROGRESS")}
	items[0].FinishTime = 20
	items[1].FinishTime = 30
	items[2].StartTime = 15
	if err := db.GORM.Transaction(func(tx *gorm.DB) error {
		_, err := NewSiteRepository(tx).SyncUpstreamTasks(context.Background(), site, now, now-48*3600, dto.UpstreamTaskSnapshot{Items: items})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := NewUpstreamTaskRepository(db.GORM).Metrics(context.Background(), dto.UpstreamTaskQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}}, "summary")
	if err != nil || len(rows) != 1 || rows[0].Total != 3 || rows[0].Success != 1 || rows[0].Failure != 1 || rows[0].Running != 1 || rows[0].QueueCount != 3 || rows[0].RunCount != 2 || rows[0].TotalCount != 2 {
		t.Fatalf("metrics=%+v err=%v", rows, err)
	}
}

func TestUpstreamTaskCollectionStatusesExposePartialAndUnavailable(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2101202000)
	completeSite := createRunnableSite(t, db, fmt.Sprintf("task-complete-%d", time.Now().UnixNano()), now)
	unavailableSite := createRunnableSite(t, db, fmt.Sprintf("task-unavailable-%d", time.Now().UnixNano()), now)
	pendingSite := createRunnableSite(t, db, fmt.Sprintf("task-pending-%d", time.Now().UnixNano()), now)
	successAt, failureAt := now-10, now-5
	states := []SiteUpstreamTaskCollectionState{
		{SiteID: completeSite.ID, OverlapStart: now - 48*3600, LastSuccessAt: &successAt, ConfigVersion: completeSite.ConfigVersion, UpdatedAt: now},
		{SiteID: unavailableSite.ID, OverlapStart: now - 48*3600, LastFailureAt: &failureAt, LastErrorCode: "upstream_response_invalid", ConfigVersion: unavailableSite.ConfigVersion, UpdatedAt: now},
	}
	if err := db.GORM.Create(&states).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewUpstreamTaskRepository(db.GORM)
	statuses, overall, err := repo.CollectionStatuses(context.Background(), []int64{completeSite.ID, unavailableSite.ID, pendingSite.ID})
	if err != nil || overall != "partial" || statuses[completeSite.ID] != "complete" || statuses[unavailableSite.ID] != "unavailable" {
		t.Fatalf("statuses=%v overall=%q err=%v", statuses, overall, err)
	}
	statuses, overall, err = repo.CollectionStatuses(context.Background(), []int64{unavailableSite.ID})
	if err != nil || overall != "unavailable" || statuses[unavailableSite.ID] != "unavailable" {
		t.Fatalf("unavailable statuses=%v overall=%q err=%v", statuses, overall, err)
	}
}
