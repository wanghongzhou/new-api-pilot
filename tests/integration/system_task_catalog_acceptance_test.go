package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
)

func TestA100SystemTaskReadOnlyMonitoring(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	type fixtureTask struct {
		RemoteID     string  `json:"remote_id"`
		TaskID       string  `json:"task_id"`
		Type         string  `json:"type"`
		Status       string  `json:"status"`
		CreatedAt    int64   `json:"created_at"`
		UpdatedAt    int64   `json:"updated_at"`
		ErrorPresent bool    `json:"error_present"`
		ErrorCode    *string `json:"error_code"`
		Progress     *struct {
			Total     string `json:"total"`
			Processed string `json:"processed"`
			Progress  int    `json:"progress"`
			Remaining string `json:"remaining"`
		} `json:"progress"`
	}
	type fixtureType struct {
		SchemaVersion int                `json:"schema_version"`
		FixtureID     string             `json:"fixture_id"`
		Clock         designClockFixture `json:"clock"`
		Tasks         []fixtureTask      `json:"tasks"`
		Scenarios     []string           `json:"scenarios"`
	}
	fixture := loadDesignJSONFixture[fixtureType](t, "f11-system-tasks.json")
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F11" || len(fixture.Tasks) != 5 {
		t.Fatalf("invalid F11: %#v", fixture)
	}
	requireDesignScenarios(t, fixture.Scenarios, "list_limit_100_truncated_partial", "current_supplements_active_outside_list", "typed_progress_result_only", "raw_fields_discarded_at_decode_boundary", "terminal_retention_by_updated_at", "active_never_deleted_by_age")
	db := openCoreAcceptanceTransaction(t)
	site := createCoreAuthorizedSite(t, db, newCoreCipher(t), fixture.Clock.NowUnix)
	items := make([]dto.UpstreamSystemTask, 0, len(fixture.Tasks))
	for _, task := range fixture.Tasks {
		item := dto.UpstreamSystemTask{ID: fixtureInt64(t, "remote_id", task.RemoteID), TaskID: task.TaskID, Type: task.Type, Status: task.Status, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, ErrorPresent: task.ErrorPresent}
		if task.ErrorCode != nil {
			item.ErrorCode = *task.ErrorCode
		}
		if task.Progress != nil {
			total := fixtureInt64(t, "progress.total", task.Progress.Total)
			processed := fixtureInt64(t, "progress.processed", task.Progress.Processed)
			progress := int64(task.Progress.Progress)
			remaining := fixtureInt64(t, "progress.remaining", task.Progress.Remaining)
			item.Total = &total
			item.Processed = &processed
			item.Progress = &progress
			item.Remaining = &remaining
		}
		items = append(items, item)
	}
	repo := model.NewSiteRepository(db)
	written, err := repo.SyncSystemTasks(context.Background(), site, fixture.Clock.NowUnix, dto.UpstreamSystemTaskSnapshot{Items: items, Partial: true, Truncated: true, IDGap: true})
	if err != nil || written != 5 {
		t.Fatalf("sync written=%d err=%v", written, err)
	}
	svc, err := service.NewSystemTaskCatalogService(db)
	if err != nil {
		t.Fatal(err)
	}
	q := dto.SystemTaskQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}}
	page, err := svc.List(context.Background(), q)
	if err != nil || page.Total != "5" || page.DataStatus != "partial" || !page.Truncated || page.TruncationReason == nil || *page.TruncationReason != "source_limit_and_id_gap" || page.SourceLimit != "100" || page.ObservedCount != "5" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	if page.Items[0].Progress == nil || page.Items[0].Progress.Total == nil || *page.Items[0].Progress.Total != "9007199254740993" || page.Items[0].Progress.Progress == nil || *page.Items[0].Progress.Progress != 44 {
		t.Fatalf("typed progress=%#v", page.Items[0])
	}
	stats, err := svc.Statistics(context.Background(), q)
	if err != nil || stats.Summary.Total != "5" || stats.Summary.Active != "1" || stats.DataStatus != "partial" {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "system_tasks."+format)
		exported, exportErr := service.GenerateSystemTaskExport(context.Background(), service.SystemTaskExportOptions{Database: db, Query: q, Format: format, TemporaryPath: path, DataSnapshotAt: fixture.Clock.NowUnix, ExportedAt: fixture.Clock.NowUnix, MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if exportErr != nil || exported.RowCount != 5 {
			t.Fatalf("format=%s export=%+v err=%v", format, exported, exportErr)
		}
		if format == dto.ExportFormatCSV {
			payload, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, forbidden := range []string{"active_key", "locked_by", "payload", "state", "raw error"} {
				if strings.Contains(string(payload), forbidden) {
					t.Fatalf("export leaked %s", forbidden)
				}
			}
		}
	}
	if err = repo.DeleteTerminalSystemTasksBefore(context.Background(), fixture.Clock.NowUnix-1); err != nil {
		t.Fatal(err)
	}
	var remaining []model.SiteSystemTask
	if err = db.Where("site_id=?", site.ID).Find(&remaining).Error; err != nil || len(remaining) != 1 || remaining[0].RemoteStatus != "running" {
		t.Fatalf("remaining=%+v err=%v", remaining, err)
	}
	for _, forbidden := range []string{"active_key", "locked_by", "payload", "state", "result", "error", "raw_json"} {
		var count int64
		err = db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='site_system_task' AND column_name=?", forbidden).Scan(&count).Error
		if err != nil || count != 0 {
			t.Fatalf("forbidden=%s count=%d err=%v", forbidden, count, err)
		}
	}
}
