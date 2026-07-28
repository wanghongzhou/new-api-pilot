package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/dto"
)

func systemTaskFixture(id, updated int64, status string) dto.UpstreamSystemTask {
	value := int64(2)
	return dto.UpstreamSystemTask{ID: id, TaskID: fmt.Sprintf("systask_%d", id), Type: "async_task_poll", Status: status, CreatedAt: 1, UpdatedAt: updated, UnfinishedTasks: &value}
}
func TestSystemTaskSyncPartialIdempotencyAndTerminalRetention(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2101300000)
	site := createRunnableSite(t, db, fmt.Sprintf("system-task-%d", time.Now().UnixNano()), now)
	repo := NewSiteRepository(db.GORM)
	snapshot := dto.UpstreamSystemTaskSnapshot{Items: []dto.UpstreamSystemTask{systemTaskFixture(1, 2, "running"), systemTaskFixture(2, 2, "succeeded")}, Partial: true, Truncated: true}
	written, err := repo.SyncSystemTasks(context.Background(), site, now, snapshot)
	if err != nil || written != 2 {
		t.Fatalf("written=%d err=%v", written, err)
	}
	written, err = repo.SyncSystemTasks(context.Background(), site, now+1, snapshot)
	if err != nil || written != 0 {
		t.Fatalf("idempotent written=%d err=%v", written, err)
	}
	var state SiteSystemTaskCollectionState
	if err = db.GORM.Where("site_id=? AND resource_kind='list'", site.ID).Take(&state).Error; err != nil || state.DataStatus != "partial" || !state.Truncated {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if err = repo.DeleteTerminalSystemTasksBefore(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	var rows []SiteSystemTask
	if err = db.GORM.Where("site_id=?", site.ID).Find(&rows).Error; err != nil || len(rows) != 1 || rows[0].RemoteStatus != "running" {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestSystemTaskSyncPersistsLogDetailCleanupAndUnsupportedTypeDiagnostic(t *testing.T) {
	db := openLockedSiteRunDatabase(t)
	now := int64(2101300000)
	site := createRunnableSite(t, db, fmt.Sprintf("system-task-log-detail-%d", time.Now().UnixNano()), now)
	repo := NewSiteRepository(db.GORM)
	archivedDays := int64(3)
	snapshot := dto.UpstreamSystemTaskSnapshot{
		Items:   []dto.UpstreamSystemTask{{ID: 1, TaskID: "systask_log_detail", Type: "log_detail_cleanup", Status: "succeeded", CreatedAt: 1, UpdatedAt: 2, ArchivedDays: &archivedDays}},
		Partial: true, UnsupportedTypes: 1, ListObservedCount: 2,
	}
	written, err := repo.SyncSystemTasks(context.Background(), site, now, snapshot)
	if err != nil || written != 1 {
		t.Fatalf("written=%d err=%v", written, err)
	}
	var row SiteSystemTask
	if err = db.GORM.Where("site_id=?", site.ID).Take(&row).Error; err != nil || row.DeletedCount == nil || *row.DeletedCount != 3 {
		t.Fatalf("row=%+v err=%v", row, err)
	}
	var state SiteSystemTaskCollectionState
	if err = db.GORM.Where("site_id=? AND resource_kind='list'", site.ID).Take(&state).Error; err != nil || state.DataStatus != "partial" || state.LastErrorCode != "SYSTEM_TASK_UNSUPPORTED_TYPE" || state.ObservedCount != 2 {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}
