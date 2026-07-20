package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/constant"
)

func TestSiteAllowsTaskRejectsDisabledOrEndedProbe(t *testing.T) {
	site := Site{
		ID:               1,
		ConfigVersion:    2,
		ManagementStatus: constant.SiteManagementActive,
		AuthStatus:       constant.SiteAuthUnauthorized,
		OnlineStatus:     constant.SiteOnlineOffline,
	}
	if !siteAllowsTask(site, constant.TaskTypeSiteProbe) {
		t.Fatal("active site must remain probe eligible before authorization and while offline")
	}

	endedAt := int64(1_752_400_800)
	tests := []struct {
		name   string
		mutate func(*Site)
	}{
		{name: "missing config version", mutate: func(site *Site) { site.ConfigVersion = 0 }},
		{name: "management disabled", mutate: func(site *Site) { site.ManagementStatus = constant.SiteManagementDisabled }},
		{name: "statistics ended", mutate: func(site *Site) { site.StatisticsEndAt = &endedAt }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := site
			test.mutate(&current)
			if siteAllowsTask(current, constant.TaskTypeSiteProbe) {
				t.Fatal("site unexpectedly allowed to run probe")
			}
		})
	}
}

func TestProbeTaskLifecycleFenceRejectsEnqueueClaimAndCommitThenAllowsNewVersion(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	now := int64(1_752_900_000)
	repository := NewCollectionTaskRepository(database.GORM)

	t.Run("pending claim", func(t *testing.T) {
		site := createRunnableSite(t, database, fmt.Sprintf("run-probe-pending-%d", time.Now().UnixNano()), now)
		run, _, err := repository.EnqueueSiteTask(ctx, SiteTaskEnqueueRequest{
			SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion, TaskType: constant.TaskTypeSiteProbe,
			TriggerType: constant.CollectionTriggerSchedule, RequestID: "req_probe_pending", Now: now,
		})
		if err != nil {
			t.Fatalf("enqueue active probe: %v", err)
		}
		if err := database.GORM.Model(&Site{}).Where("id = ?", site.ID).Updates(map[string]any{
			"management_status": constant.SiteManagementDisabled,
			"disabled_at":       now,
		}).Error; err != nil {
			t.Fatalf("disable pending probe site: %v", err)
		}
		if _, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
			TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now + 1, RequestID: "claim_probe_pending",
		}); !errors.Is(err, ErrSiteRunConfigChanged) {
			t.Fatalf("claim disabled probe error = %v, want %v", err, ErrSiteRunConfigChanged)
		}
		var persisted CollectionRun
		if err := database.GORM.First(&persisted, run.ID).Error; err != nil || persisted.Status != CollectionTaskStatusPending {
			t.Fatalf("disabled probe claim changed run = %#v err=%v", persisted, err)
		}
		if _, _, err := repository.EnqueueSiteTask(ctx, SiteTaskEnqueueRequest{
			SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion, TaskType: constant.TaskTypeSiteProbe,
			TriggerType: constant.CollectionTriggerSchedule, RequestID: "req_probe_disabled", Now: now + 2,
		}); !errors.Is(err, ErrSiteRunConfigChanged) {
			t.Fatalf("enqueue disabled probe error = %v, want %v", err, ErrSiteRunConfigChanged)
		}

		newVersion := site.ConfigVersion + 1
		if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": CollectionTaskStatusFailed, "active_key": nil, "finished_at": now + 3,
		}).Error; err != nil {
			t.Fatalf("terminate old probe: %v", err)
		}
		if err := database.GORM.Model(&Site{}).Where("id = ?", site.ID).Updates(map[string]any{
			"management_status": constant.SiteManagementActive, "disabled_at": nil,
			"statistics_end_at": nil, "config_version": newVersion,
		}).Error; err != nil {
			t.Fatalf("enable probe site: %v", err)
		}
		newRun, deduplicated, err := repository.EnqueueSiteTask(ctx, SiteTaskEnqueueRequest{
			SiteID: site.ID, ExpectedConfigVersion: newVersion, TaskType: constant.TaskTypeSiteProbe,
			TriggerType: constant.CollectionTriggerSchedule, RequestID: "req_probe_enabled", Now: now + 4,
		})
		if err != nil || deduplicated || newRun.ID == run.ID || newRun.SiteConfigVersion != newVersion {
			t.Fatalf("enqueue enabled probe = %#v deduplicated=%t err=%v", newRun, deduplicated, err)
		}
		if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", newRun.ID).Updates(map[string]any{
			"status": CollectionTaskStatusFailed, "active_key": nil, "finished_at": now + 5,
		}).Error; err != nil {
			t.Fatalf("finish recovered probe fixture: %v", err)
		}
	})

	t.Run("running commit", func(t *testing.T) {
		site := createRunnableSite(t, database, fmt.Sprintf("run-probe-running-%d", time.Now().UnixNano()), now+10)
		run, _, err := repository.EnqueueSiteTask(ctx, SiteTaskEnqueueRequest{
			SiteID: site.ID, ExpectedConfigVersion: site.ConfigVersion, TaskType: constant.TaskTypeSiteProbe,
			TriggerType: constant.CollectionTriggerSchedule, RequestID: "req_probe_running", Now: now + 10,
		})
		if err != nil {
			t.Fatalf("enqueue running probe: %v", err)
		}
		claim, err := repository.ClaimNext(ctx, CollectionTaskClaimOptions{
			TaskTypes: []string{constant.TaskTypeSiteProbe}, Now: now + 11, RequestID: "claim_probe_running",
		})
		if err != nil || claim.Run.ID != run.ID {
			t.Fatalf("claim active probe = %#v err=%v", claim, err)
		}
		endedAt := now + 12
		if err := database.GORM.Model(&Site{}).Where("id = ?", site.ID).Updates(map[string]any{
			"management_status": constant.SiteManagementDisabled, "statistics_end_at": endedAt,
		}).Error; err != nil {
			t.Fatalf("disable running probe site: %v", err)
		}
		if _, err := repository.CommitClaim(ctx, CollectionTaskCommitRequest{
			RunID: run.ID, RequestID: claim.RequestID, Now: now + 13, RunStatus: CollectionTaskStatusSuccess,
			FetchedRows: 1, WrittenRows: 1,
		}); !errors.Is(err, ErrSiteRunConfigChanged) {
			t.Fatalf("commit disabled probe error = %v, want %v", err, ErrSiteRunConfigChanged)
		}
		var persisted CollectionRun
		if err := database.GORM.First(&persisted, run.ID).Error; err != nil || persisted.Status != CollectionTaskStatusRunning {
			t.Fatalf("disabled probe commit changed run = %#v err=%v", persisted, err)
		}
	})
}
