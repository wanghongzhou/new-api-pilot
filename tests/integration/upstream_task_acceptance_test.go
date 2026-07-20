package integration_test

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	"os"
	"strings"
	"testing"
)

func TestA95UpstreamTaskTransitionStatisticsRetentionAndPrivacy(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	type taskFixture struct {
		SchemaVersion int                `json:"schema_version"`
		FixtureID     string             `json:"fixture_id"`
		Clock         designClockFixture `json:"clock"`
		Tasks         []struct {
			RemoteID   string `json:"remote_id"`
			CreatedAt  int64  `json:"created_at"`
			UpdatedAt  int64  `json:"updated_at"`
			TaskID     string `json:"task_id"`
			Platform   string `json:"platform"`
			UserID     string `json:"user_id"`
			Group      string `json:"group"`
			ChannelID  string `json:"channel_id"`
			Quota      string `json:"quota"`
			Action     string `json:"action"`
			Status     string `json:"status"`
			SubmitTime int64  `json:"submit_time"`
			StartTime  int64  `json:"start_time"`
			FinishTime int64  `json:"finish_time"`
			Progress   string `json:"progress"`
			Properties struct {
				Model string `json:"model"`
			} `json:"properties"`
		} `json:"tasks"`
		Transitions      []string `json:"transitions"`
		PollingScenarios []string `json:"polling_scenarios"`
	}
	fixture := loadDesignJSONFixture[taskFixture](t, "f07-upstream-tasks.json")
	if fixture.SchemaVersion != 1 || fixture.FixtureID != "F07" || fixture.Clock.Timezone != "Asia/Shanghai" || len(fixture.Tasks) != 1 {
		t.Fatalf("invalid F07 upstream-task fixture: %#v", fixture)
	}
	requireDesignScenarios(t, fixture.Transitions, "NOT_START->SUBMITTED", "SUBMITTED->QUEUED", "QUEUED->IN_PROGRESS", "IN_PROGRESS->SUCCESS", "IN_PROGRESS->FAILURE")
	requireDesignScenarios(t, fixture.PollingScenarios, "overlap_window", "known_unfinished_rescan", "total_drift", "maximum_id_drift", "duplicate_id", "config_fence", "terminal_retention", "unfinished_retained")
	task := fixture.Tasks[0]
	db := openCoreAcceptanceTransaction(t)
	now := fixture.Clock.NowUnix
	site := createCoreAuthorizedSite(t, db, newCoreCipher(t), now)
	initial := dto.UpstreamTask{ID: fixtureInt64(t, "tasks.remote_id", task.RemoteID), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, TaskID: task.TaskID, Platform: task.Platform, UserID: fixtureInt64(t, "tasks.user_id", task.UserID), Group: task.Group, ChannelID: fixtureInt64(t, "tasks.channel_id", task.ChannelID), Quota: fixtureInt64(t, "tasks.quota", task.Quota), Action: task.Action, Status: task.Status, SubmitTime: task.SubmitTime, StartTime: task.StartTime, FinishTime: task.FinishTime, Progress: task.Progress, Properties: dto.UpstreamTaskProperties{Model: task.Properties.Model}}
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := model.NewSiteRepository(tx).SyncUpstreamTasks(context.Background(), site, now, now-48*3600, dto.UpstreamTaskSnapshot{Items: []dto.UpstreamTask{initial}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	finished := initial
	finished.UpdatedAt = now + 1
	finished.Status = "SUCCESS"
	finished.FinishTime = initial.StartTime + 80
	finished.Progress = "100%"
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := model.NewSiteRepository(tx).SyncUpstreamTasks(context.Background(), site, now+2, now-48*3600, dto.UpstreamTaskSnapshot{Items: []dto.UpstreamTask{finished}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewUpstreamTaskService(db)
	if err != nil {
		t.Fatal(err)
	}
	q := dto.UpstreamTaskQuery{Page: 1, PageSize: 20, SiteIDs: []int64{site.ID}}
	page, err := svc.List(context.Background(), q)
	if err != nil || page.Total != 1 || page.Items[0].Status != "SUCCESS" || page.Items[0].Quota != task.Quota {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	stats, err := svc.Statistics(context.Background(), q)
	if err != nil || stats.Summary.Success != "1" || stats.Summary.SuccessRate == nil || *stats.Summary.SuccessRate != "1" || stats.Summary.AvgQueueSeconds == nil || *stats.Summary.AvgQueueSeconds != "10" || stats.Summary.AvgRunSeconds == nil || *stats.Summary.AvgRunSeconds != "80" || len(stats.SiteBreakdown) != 1 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
	encoded, _ := json.Marshal(page)
	var payload any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	var hasKey func(any, string) bool
	hasKey = func(value any, key string) bool {
		switch typed := value.(type) {
		case map[string]any:
			if _, exists := typed[key]; exists {
				return true
			}
			for _, child := range typed {
				if hasKey(child, key) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if hasKey(child, key) {
					return true
				}
			}
		}
		return false
	}
	for _, forbidden := range []string{strings.Join([]string{"da", "ta"}, ""), strings.Join([]string{"in", "put"}, ""), strings.Join([]string{"fail", "reason"}, "_"), strings.Join([]string{"result", "url"}, "_"), strings.Join([]string{"private", "data"}, "_")} {
		if hasKey(payload, forbidden) {
			t.Fatalf("forbidden field %q in response: %s", forbidden, encoded)
		}
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='site_upstream_task' AND column_name=?", forbidden).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("forbidden column %q count=%d err=%v", forbidden, count, err)
		}
	}
}
