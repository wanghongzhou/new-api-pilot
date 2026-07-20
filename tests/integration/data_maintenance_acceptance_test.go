package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"new-api-pilot/constant"
	"new-api-pilot/model"
)

var a102Location = time.FixedZone("Asia/Shanghai", 8*60*60)

func TestA102ResourceExpectedCoverageAndFinalize(t *testing.T) {
	db := openCoreAcceptanceTransaction(t)
	start := time.Date(2036, 8, 5, 0, 0, 0, 0, a102Location).Unix()
	end := start + 24*60*60
	now := end + 12*60*60
	monitoringStart := start + 30*60
	site := createCorePendingSite(t, db, now)
	if err := db.Model(&site).Updates(map[string]any{"monitoring_start_at": monitoringStart, "config_version": 7}).Error; err != nil {
		t.Fatalf("configure A102 site: %v", err)
	}
	site.ConfigVersion, site.MonitoringStartAt = 7, &monitoringStart
	firstSeen := start + 30*60 + 30
	retired := start + 3*60*60
	instance := model.SiteInstance{
		SiteID: site.ID, NodeName: "node-a", Hostname: "node-a", CurrentStatus: "retired",
		UpstreamStatus: "online", FirstSeenAt: firstSeen, LastSyncedAt: now,
		CreatedAt: now, UpdatedAt: now, RetiredAt: &retired,
	}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatalf("create A102 instance: %v", err)
	}
	if err := db.Create(&model.SiteInstanceLifecycle{SiteID: site.ID, NodeName: instance.NodeName,
		StartMinuteTS: firstSeen - firstSeen%60, EndMinuteTS: &retired, EvidenceStatus: "known", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create A102 instance lifecycle: %v", err)
	}
	pauseEnd := start + 2*60*60
	if err := db.Create(&model.SiteMonitoringPause{SiteID: site.ID, StartMinuteTS: start + 60*60, EndMinuteTS: &pauseEnd, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create A102 pause: %v", err)
	}
	cpu, memory, disk := 10.0, 20.0, 30.0
	minutes := []int64{start + 30*60, start + 2*60*60}
	for offset := int64(0); offset < 60; offset++ {
		minutes = append(minutes, start+4*60*60+offset*60)
	}
	for _, minute := range minutes {
		if err := db.Create(&model.SiteInstanceStatusMinutely{SiteID: site.ID, NodeName: instance.NodeName, MinuteTS: minute,
			Status: "online", CPUPercent: &cpu, MemoryPercent: &memory, DiskUsedPercent: &disk, CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.SiteStatusMinutely{SiteID: site.ID, MinuteTS: minute, InstanceCount: 1, OnlineInstanceCount: 1,
			CPUMaxPercent: &cpu, CPUAvgPercent: &cpu, MemoryMaxPercent: &memory, MemoryAvgPercent: &memory,
			DiskMaxUsedPercent: &disk, HealthStatus: "ok", CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
	}
	stableCalculated := now - 99
	if err := db.Exec(`INSERT INTO site_instance_status_hourly (site_id,node_name,hour_ts,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,disk_last_used_percent,online_samples,abnormal_samples,sample_count,expected_sample_count,data_status,last_calculated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, site.ID, instance.NodeName, start+4*3600, 99, 98, 97, 96, 95, 94, 60, 0, 60, 60, "complete", stableCalculated).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO site_status_hourly (site_id,hour_ts,instance_count_max,online_instance_count_min,cpu_max_percent,cpu_avg_percent,memory_max_percent,memory_avg_percent,disk_max_used_percent,abnormal_samples,sample_count,expected_sample_count,data_status,health_status,last_calculated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, site.ID, start+4*3600, 1, 1, 99, 98, 97, 96, 95, 0, 60, 60, "complete", "ok", stableCalculated).Error; err != nil {
		t.Fatal(err)
	}
	repository := model.NewDataMaintenanceRepository(db)
	var gap model.ResourceMaintenanceBatchResult
	for attempt := 0; attempt < 100 && !gap.Complete; attempt++ {
		var err error
		gap, err = repository.RepairResourceRollupGaps(context.Background(), 20360805, start, end, 2, now)
		if err != nil {
			t.Fatalf("repair resource gaps attempt %d: %v", attempt, err)
		}
		if gap.Items > 2 {
			t.Fatalf("gap batch items=%d, want <=2", gap.Items)
		}
	}
	if !gap.Complete {
		t.Fatalf("gap did not complete: %#v", gap)
	}
	var stable struct {
		CPUMax         float64 `gorm:"column:cpu_max_percent"`
		LastCalculated int64   `gorm:"column:last_calculated_at"`
	}
	if err := db.Table("site_instance_status_hourly").Select("cpu_max_percent,last_calculated_at").Where("site_id=? AND node_name=? AND hour_ts=?", site.ID, instance.NodeName, start+4*3600).Take(&stable).Error; err != nil {
		t.Fatal(err)
	}
	if stable.CPUMax != 99 || stable.LastCalculated != stableCalculated {
		t.Fatalf("complete hourly was rewritten: %#v", stable)
	}
	legacyNode := "legacy-node"
	legacyStart := start
	legacyEnd := end
	if err := db.Create(&model.SiteInstanceLifecycle{SiteID: site.ID, NodeName: legacyNode, StartMinuteTS: legacyStart, EndMinuteTS: &legacyEnd, EvidenceStatus: "legacy_unknown", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	legacyCalculated := now - 777
	if err := db.Exec(`INSERT INTO site_instance_status_daily (site_id,node_name,date_key,online_samples,abnormal_samples,sample_count,expected_sample_count,data_status,is_final,last_calculated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, site.ID, legacyNode, 20360805, 1, 2, 3, 4, "partial", 1, legacyCalculated).Error; err != nil {
		t.Fatal(err)
	}
	type hourRow struct {
		HourTS              int64
		SampleCount         int
		ExpectedSampleCount int
		DataStatus          string
		HealthStatus        string
	}
	var siteHours []hourRow
	if err := db.Table("site_status_hourly").Where("site_id=?", site.ID).Order("hour_ts").Find(&siteHours).Error; err != nil {
		t.Fatal(err)
	}
	if len(siteHours) != 24 || siteHours[0].ExpectedSampleCount != 30 || siteHours[0].SampleCount != 1 || siteHours[0].DataStatus != "partial" ||
		siteHours[1].ExpectedSampleCount != 0 || siteHours[1].DataStatus != "paused" ||
		siteHours[3].SampleCount != 0 || siteHours[3].DataStatus != "missing" || siteHours[3].HealthStatus != "unavailable" {
		t.Fatalf("site hours do not preserve expected coverage: %#v", siteHours)
	}
	var instanceHours []hourRow
	if err := db.Table("site_instance_status_hourly").Where("site_id=? AND node_name=?", site.ID, instance.NodeName).Order("hour_ts").Find(&instanceHours).Error; err != nil {
		t.Fatal(err)
	}
	if len(instanceHours) != 4 || instanceHours[0].ExpectedSampleCount != 30 || instanceHours[1].DataStatus != "paused" || instanceHours[2].ExpectedSampleCount != 60 || instanceHours[3].DataStatus != "complete" {
		t.Fatalf("instance hours do not honor floored lifecycle: %#v", instanceHours)
	}
	broken := createCorePendingSite(t, db, now)
	if err := db.Model(&broken).Updates(map[string]any{"monitoring_start_at": start, "config_version": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FinalizeResourceDaily(context.Background(), 20360805, start, end, 2, now+1); err == nil {
		t.Fatal("partial finalize unexpectedly succeeded")
	}
	var knownUnfinal int64
	if err := db.Table("site_instance_status_daily").Where("site_id=? AND node_name=? AND date_key=? AND is_final=0", site.ID, instance.NodeName, 20360805).Count(&knownUnfinal).Error; err != nil {
		t.Fatal(err)
	}
	if knownUnfinal != 1 {
		t.Fatalf("known daily finalized before all sites succeeded: %d", knownUnfinal)
	}
	var legacy struct {
		Sample     int   `gorm:"column:sample_count"`
		Final      bool  `gorm:"column:is_final"`
		Calculated int64 `gorm:"column:last_calculated_at"`
	}
	if err := db.Table("site_instance_status_daily").Select("sample_count,is_final,last_calculated_at").Where("site_id=? AND node_name=? AND date_key=?", site.ID, legacyNode, 20360805).Take(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.Sample != 3 || !legacy.Final || legacy.Calculated != legacyCalculated {
		t.Fatalf("legacy daily changed: %#v", legacy)
	}
	if err := db.Model(&broken).Update("monitoring_start_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	finalized, err := repository.FinalizeResourceDaily(context.Background(), 20360805, start, end, 2, now+2)
	if err != nil {
		t.Fatalf("finalize resource daily: %v", err)
	}
	if !finalized.Complete {
		t.Fatalf("finalize result = %#v", finalized)
	}
	var notFinal int64
	if err := db.Table("site_status_daily").Where("site_id=? AND date_key=? AND is_final=0", site.ID, 20360805).Count(&notFinal).Error; err != nil {
		t.Fatal(err)
	}
	if notFinal != 0 {
		t.Fatalf("site daily not final rows = %d", notFinal)
	}
}

func TestA102RetentionExactBoundariesAndExclusions(t *testing.T) {
	db := openCoreAcceptanceTransaction(t)
	now := int64(2101608000)
	site := createCorePendingSite(t, db, now)
	site.ConfigVersion = 1
	createRun := func(task, trigger, status string, finished int64, message *string) model.CollectionRun {
		t.Helper()
		run, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{TaskType: task, TriggerType: trigger, Priority: 0, RequestID: "a102-retention", Now: now})
		if err != nil {
			t.Fatal(err)
		}
		created, _, err := model.NewSiteRepository(db).CreateOrGetRun(context.Background(), &run)
		if err != nil {
			t.Fatal(err)
		}
		updates := map[string]any{"status": status, "active_key": nil, "finished_at": finished, "updated_at": now}
		if message != nil {
			updates["error_message"] = *message
		}
		if err := db.Model(&model.CollectionRun{}).Where("id=?", created.ID).Updates(updates).Error; err != nil {
			t.Fatal(err)
		}
		created.ID = created.ID
		return created
	}
	message := "private detail"
	redactionCutoff := now - 90*24*60*60
	redactOld := createRun(constant.TaskTypePricingSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusFailed, redactionCutoff-1, &message)
	redactEqual := createRun(constant.TaskTypePlanSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusFailed, redactionCutoff, &message)
	redactNew := createRun(constant.TaskTypeSystemTaskSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusFailed, redactionCutoff+1, &message)
	cleanupCutoff := now - 30*24*60*60
	deleteUser := createRun(constant.TaskTypeUserSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusSuccess, cleanupCutoff-1, nil)
	deleteChannel := createRun(constant.TaskTypeChannelSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusSuccess, cleanupCutoff-1, nil)
	keepEqual := createRun(constant.TaskTypeUserSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusSuccess, cleanupCutoff, nil)
	keepFailed := createRun(constant.TaskTypeChannelSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusFailed, cleanupCutoff-1, nil)
	keepManual := createRun(constant.TaskTypeUserSync, constant.CollectionTriggerManual, model.CollectionTaskStatusSuccess, cleanupCutoff-1, nil)
	keepOther := createRun(constant.TaskTypePricingSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusSuccess, cleanupCutoff-1, nil)
	keepChild := createRun(constant.TaskTypeChannelSync, constant.CollectionTriggerSchedule, model.CollectionTaskStatusSuccess, cleanupCutoff-1, nil)
	if err := db.Create(&model.CollectionRunWindow{RunID: keepChild.ID, SiteID: site.ID, HourTS: now - now%3600, Status: "success", UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	repo := model.NewDataMaintenanceRepository(db)
	for i := 0; i < 10; i++ {
		result, err := repo.RedactCollectionRunErrors(context.Background(), 20360806, redactionCutoff, 2, now)
		if err != nil {
			t.Fatal(err)
		}
		if result.Complete {
			break
		}
	}
	for _, item := range []struct {
		id       int64
		wantNull bool
	}{{redactOld.ID, true}, {redactEqual.ID, false}, {redactNew.ID, false}} {
		var run model.CollectionRun
		if err := db.First(&run, item.id).Error; err != nil {
			t.Fatal(err)
		}
		if !run.ErrorMessage.Valid != item.wantNull {
			t.Fatalf("redaction id=%d null=%t want=%t", item.id, !run.ErrorMessage.Valid, item.wantNull)
		}
	}
	for i := 0; i < 10; i++ {
		result, err := repo.CleanupMetadataDiagnosticRuns(context.Background(), 20360806, cleanupCutoff, 2, now)
		if err != nil {
			t.Fatal(err)
		}
		if result.Complete {
			break
		}
	}
	for _, id := range []int64{deleteUser.ID, deleteChannel.ID} {
		var count int64
		if err := db.Model(&model.CollectionRun{}).Where("id=?", id).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("cleanup target %d count=%d err=%v", id, count, err)
		}
	}
	for _, id := range []int64{keepEqual.ID, keepFailed.ID, keepManual.ID, keepOther.ID, keepChild.ID} {
		var count int64
		if err := db.Model(&model.CollectionRun{}).Where("id=?", id).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("cleanup exclusion %d count=%d err=%v", id, count, err)
		}
	}
}

func TestA102SiteDeleteMaintenanceStatePolicy(t *testing.T) {
	db := openCoreAcceptanceTransaction(t)
	const now = int64(2101608000)
	site := createCorePendingSite(t, db, now)
	if err := db.Model(&site).Update("config_version", 7).Error; err != nil {
		t.Fatal(err)
	}
	repo := model.NewDataMaintenanceRepository(db)
	if err := repo.EnsureAuthorizationPricingIntent(context.Background(), site.ID, 7, "a102-delete", now); err != nil {
		t.Fatal(err)
	}
	sites := model.NewSiteRepository(db)
	assertBlocked := func(want bool) {
		blockers, err := sites.SiteDeleteBlockers(context.Background(), site.ID)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, blocker := range blockers {
			if blocker == model.SiteDeleteDependencyDataMaintenance {
				found = true
			}
		}
		if found != want {
			t.Fatalf("maintenance blocker=%t want=%t", found, want)
		}
	}
	assertBlocked(true)
	if err := db.Model(&model.DataMaintenanceState{}).Where("site_id=?", site.ID).Updates(map[string]any{"status": model.MaintenanceStatusComplete, "next_attempt_at": 0, "updated_at": now + 1}).Error; err != nil {
		t.Fatal(err)
	}
	assertBlocked(false)
	if err := sites.DeleteOwnedMetadata(context.Background(), site.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.DataMaintenanceState{}).Where("site_id=?", site.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("complete cleanup count=%d err=%v", count, err)
	}
	if err := repo.EnsureAuthorizationPricingIntent(context.Background(), site.ID, 7, "a102-delete-failed", now+2); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.DataMaintenanceState{}).Where("site_id=?", site.ID).Updates(map[string]any{"status": model.MaintenanceStatusFailed, "next_attempt_at": now + 3600, "updated_at": now + 2}).Error; err != nil {
		t.Fatal(err)
	}
	assertBlocked(false)
	if err := sites.DeleteOwnedMetadata(context.Background(), site.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.DataMaintenanceState{}).Where("site_id=?", site.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed cleanup count=%d err=%v", count, err)
	}
}

func TestA102AuthorizationPricingIntentIdempotencyFailureAndFence(t *testing.T) {
	db := openCoreAcceptanceTransaction(t)
	const now = int64(2101608000)
	readySite := func(configVersion int) model.Site {
		site := createCorePendingSite(t, db, now)
		if err := db.Model(&site).Updates(map[string]any{"config_version": configVersion, "auth_status": constant.SiteAuthAuthorized}).Error; err != nil {
			t.Fatal(err)
		}
		site.ConfigVersion = configVersion
		site.AuthStatus = constant.SiteAuthAuthorized
		caps := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
		for _, key := range constant.SiteCapabilityKeys() {
			status := constant.CapabilityStatusPassed
			if key == constant.CapabilityFlowDataConsistency {
				status = constant.CapabilityStatusSkipped
			}
			caps = append(caps, model.SiteCapability{SiteID: site.ID, CapabilityKey: key, Status: status, MessageCode: "CAPABILITY_OK", MessageParams: []byte("{}"), CheckedAt: now})
		}
		if err := db.Create(&caps).Error; err != nil {
			t.Fatal(err)
		}
		return site
	}
	repo := model.NewDataMaintenanceRepository(db)
	site := readySite(7)
	for i := 0; i < 3; i++ {
		if err := repo.EnsureAuthorizationPricingIntent(context.Background(), site.ID, 7, "a102-auth", now); err != nil {
			t.Fatal(err)
		}
	}
	result, err := model.NewDataMaintenanceRepository(db).ProcessAuthorizationPricingIntent(context.Background(), now)
	if err != nil || !result.Completed || result.RunID <= 0 {
		t.Fatalf("process pricing result=%#v err=%v", result, err)
	}
	var active int64
	if err := db.Model(&model.CollectionRun{}).Where("site_id=? AND task_type=? AND status IN ?", site.ID, constant.TaskTypePricingSync, []string{"pending", "running"}).Count(&active).Error; err != nil || active != 1 {
		t.Fatalf("active pricing runs=%d err=%v", active, err)
	}
	failureSite := readySite(8)
	if err := repo.EnsureAuthorizationPricingIntent(context.Background(), failureSite.ID, 8, "a102-failure", now); err != nil {
		t.Fatal(err)
	}
	callbackName := "a102:fail-collection-run-create"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "collection_run" {
			tx.AddError(errors.New("injected enqueue failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failed, err := repo.ProcessAuthorizationPricingIntent(context.Background(), now)
	if err != nil || !failed.Attempted || failed.Completed {
		t.Fatalf("failure result=%#v err=%v", failed, err)
	}
	if err := db.Callback().Create().Remove(callbackName); err != nil {
		t.Fatal(err)
	}
	var failureState model.DataMaintenanceState
	if err := db.Where("site_id=? AND site_config_version=?", failureSite.ID, 8).Take(&failureState).Error; err != nil {
		t.Fatal(err)
	}
	if failureState.Status != model.MaintenanceStatusFailed || failureState.ErrorCode != model.MaintenanceErrorEnqueue || failureState.RequestID != "a102-failure" {
		t.Fatalf("safe failure state=%#v", failureState)
	}
	payload, _ := json.Marshal(failureState)
	for _, forbidden := range []string{"injected enqueue failure", "access_token", "password", "upstream_body", "raw_error"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("failure diagnostic leaked %q: %s", forbidden, payload)
		}
	}
	var failedRuns int64
	if err := db.Model(&model.CollectionRun{}).Where("site_id=? AND task_type=?", failureSite.ID, constant.TaskTypePricingSync).Count(&failedRuns).Error; err != nil || failedRuns != 0 {
		t.Fatalf("failed enqueue runs=%d err=%v", failedRuns, err)
	}
	var persisted model.Site
	if err := db.First(&persisted, failureSite.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.AuthStatus != constant.SiteAuthAuthorized {
		t.Fatalf("enqueue failure rolled back authorization: %s", persisted.AuthStatus)
	}
	fencedSite := readySite(9)
	if err := repo.EnsureAuthorizationPricingIntent(context.Background(), fencedSite.ID, 9, "a102-fence", now); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&fencedSite).Update("config_version", 10).Error; err != nil {
		t.Fatal(err)
	}
	fenced, err := repo.ProcessAuthorizationPricingIntent(context.Background(), now)
	if err != nil || !fenced.Completed || fenced.RunID != 0 {
		t.Fatalf("fence result=%#v err=%v", fenced, err)
	}
	var fencedState model.DataMaintenanceState
	if err := db.Where("site_id=? AND site_config_version=?", fencedSite.ID, 9).Take(&fencedState).Error; err != nil {
		t.Fatal(err)
	}
	if fencedState.ErrorCode != model.MaintenanceErrorSiteConfigChanged {
		t.Fatalf("fence state=%#v", fencedState)
	}
}
