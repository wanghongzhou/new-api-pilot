package worker

import (
	"context"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSchedulerSiteEligibleRejectsDisabledOrEndedProbe(t *testing.T) {
	site := model.Site{
		ID:               1,
		ConfigVersion:    2,
		ManagementStatus: constant.SiteManagementActive,
		AuthStatus:       constant.SiteAuthUnauthorized,
		OnlineStatus:     constant.SiteOnlineOffline,
	}
	if !schedulerSiteEligible(site, constant.TaskTypeSiteProbe) {
		t.Fatal("active site must remain probe eligible before authorization and while offline")
	}

	endedAt := int64(1_752_400_800)
	tests := []struct {
		name   string
		mutate func(*model.Site)
	}{
		{name: "missing id", mutate: func(site *model.Site) { site.ID = 0 }},
		{name: "missing config version", mutate: func(site *model.Site) { site.ConfigVersion = 0 }},
		{name: "management disabled", mutate: func(site *model.Site) { site.ManagementStatus = constant.SiteManagementDisabled }},
		{name: "statistics ended", mutate: func(site *model.Site) { site.StatisticsEndAt = &endedAt }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := site
			test.mutate(&current)
			if schedulerSiteEligible(current, constant.TaskTypeSiteProbe) {
				t.Fatal("site unexpectedly eligible for probe")
			}
		})
	}
}

func TestSchedulerRecoversOnlyExactPendingValidationHours(t *testing.T) {
	database := openWorkerTestDatabase(t)
	now := time.Date(2026, time.September, 16, 3, 0, 0, 0, beijingLocation)
	site := createWorkerTestSite(t, database, "historical-validation-recovery", now.Unix())
	capabilities := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, model.SiteCapability{
			SiteID: site.ID, CapabilityKey: key, Status: status, CheckedAt: now.Unix(),
		})
	}
	if err := model.NewSiteRepository(database.GORM).ReplaceCapabilities(context.Background(), site.ID, capabilities); err != nil {
		t.Fatalf("create recovery capabilities: %v", err)
	}
	dayStart := time.Date(2026, time.September, 10, 0, 0, 0, 0, beijingLocation).Unix()
	dateEnd := dayStart + 24*3600
	verifiedAt := dateEnd
	rows := []model.CollectionWindow{
		{SiteID: site.ID, HourTS: dayStart, Status: model.CollectionWindowStatusComplete, VerifiedAt: &verifiedAt, UpdatedAt: now.Unix()},
		{SiteID: site.ID, HourTS: dayStart + 3600, Status: model.CollectionWindowStatusComplete, UpdatedAt: now.Unix()},
		{SiteID: site.ID, HourTS: dayStart + 2*3600, Status: model.CollectionWindowStatusComplete, UpdatedAt: now.Unix()},
		{SiteID: site.ID, HourTS: dayStart + 3*3600, Status: model.CollectionWindowStatusComplete, VerifiedAt: &verifiedAt, UpdatedAt: now.Unix()},
		{SiteID: site.ID, HourTS: dayStart + 4*3600, Status: model.CollectionWindowStatusComplete, UpdatedAt: now.Unix()},
		{SiteID: site.ID, HourTS: dayStart + 5*3600, Status: model.CollectionWindowStatusMissing, UpdatedAt: now.Unix()},
	}
	if err := database.GORM.Create(&rows).Error; err != nil {
		t.Fatalf("seed validation windows: %v", err)
	}
	scheduler, err := NewScheduler(SchedulerOptions{
		Repository: model.NewCollectionTaskRepository(database.GORM),
		Settings:   model.NewCollectorSettingRepository(database.GORM),
		Clock:      testsupport.NewFakeClock(now),
	})
	if err != nil {
		t.Fatalf("create recovery scheduler: %v", err)
	}
	if err := scheduler.Startup(context.Background()); err != nil {
		t.Fatalf("run recovery scheduler: %v", err)
	}
	var runs []model.CollectionRun
	if err := database.GORM.Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypeUsageValidation).
		Order("start_timestamp ASC").Find(&runs).Error; err != nil {
		t.Fatalf("load recovery runs: %v", err)
	}
	recovered := make([][2]int64, 0, len(runs))
	for _, run := range runs {
		if run.StartTimestamp == nil || run.EndTimestamp == nil {
			continue
		}
		if *run.StartTimestamp >= dayStart && *run.EndTimestamp <= dateEnd {
			recovered = append(recovered, [2]int64{*run.StartTimestamp, *run.EndTimestamp})
		}
	}
	want := [][2]int64{{dayStart + 3600, dayStart + 3*3600}, {dayStart + 4*3600, dayStart + 5*3600}}
	if len(recovered) != len(want) {
		t.Fatalf("recovery ranges = %#v, want %#v", recovered, want)
	}
	for index := range want {
		if recovered[index] != want[index] {
			t.Errorf("recovery range %d = %#v, want %#v", index, recovered[index], want[index])
		}
	}
	before := len(runs)
	if err := scheduler.RunOnce(context.Background()); err != nil {
		t.Fatalf("repeat recovery scheduler: %v", err)
	}
	var after int64
	if err := database.GORM.Model(&model.CollectionRun{}).
		Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypeUsageValidation).Count(&after).Error; err != nil {
		t.Fatalf("count repeated recovery runs: %v", err)
	}
	if after != int64(before) {
		t.Fatalf("same-day recovery created duplicate runs: before=%d after=%d", before, after)
	}
}
