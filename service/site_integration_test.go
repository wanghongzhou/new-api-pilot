package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

const siteIntegrationLock = "new-api-pilot-site-service-integration"

func TestCapabilityCheckProbesLatestCompleteHourOnce(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := &SiteService{clock: clock}

	flow, data, consistency := sites.checkFlowDataCapabilities(context.Background(), client, "req_capability_probe")
	if flow.status != constant.CapabilityStatusPassed || data.status != constant.CapabilityStatusPassed ||
		consistency.status != constant.CapabilityStatusSkipped {
		t.Fatalf("capability checks flow=%#v data=%#v consistency=%#v", flow, data, consistency)
	}
	wantHour := floorHour(clock.Now().Unix()) - 3600
	if len(client.flowHours) != 1 || client.flowHours[0] != wantHour ||
		len(client.dataHours) != 1 || client.dataHours[0] != wantHour {
		t.Fatalf("capability probe hours flow=%v data=%v want=[%d]", client.flowHours, client.dataHours, wantHour)
	}
}

func TestSiteAuthorizationPersistenceBoundaries(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	factory := &testSiteClientFactory{authenticated: client, public: client}
	sites := newIntegrationSiteService(t, tx, clock, factory)
	repository := model.NewSiteRepository(tx)

	first := newTestSite(clock.Now().Unix(), "https://site-one.example")
	if err := repository.Create(context.Background(), &first); err != nil {
		t.Fatalf("create site: %v", err)
	}
	client.selfErr = ErrUpstreamResponseInvalid
	request := existingTokenRequest("1", "verified-secret")
	if _, err := sites.Authorize(context.Background(), first.ID, request, "req_identity_failure"); !errors.Is(err, ErrSiteIncompatible) {
		t.Fatalf("identity failure error = %v", err)
	}
	persisted, err := repository.FindByID(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("reload failed authorization site: %v", err)
	}
	if persisted.AccessTokenEncrypted != nil || persisted.RootUserID != nil || persisted.StatisticsStartAt != nil || persisted.ConfigVersion != 1 {
		t.Fatalf("identity failure persisted credentials or bumped fence: %#v", persisted)
	}

	client.selfErr = nil
	result, err := sites.Authorize(context.Background(), first.ID, request, "req_authorize")
	if err != nil {
		t.Fatalf("authorize site: %v", err)
	}
	if result.BackfillRunID == nil || len(result.Capabilities) != len(constant.SiteCapabilityKeys()) {
		t.Fatalf("authorization result = %#v", result)
	}
	persisted, err = repository.FindByID(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("reload authorized site: %v", err)
	}
	if persisted.ConfigVersion != 2 || persisted.AuthStatus != constant.SiteAuthAuthorized || persisted.AccessTokenEncrypted == nil ||
		persisted.LastProbeAt == nil || persisted.LastProbeSuccessAt == nil || persisted.OnlineStatus != constant.SiteOnlineOnline {
		t.Fatalf("authorized site state = %#v", persisted)
	}
	if *persisted.AccessTokenEncrypted == "verified-secret" {
		t.Fatal("site token was stored as plaintext")
	}
	plaintext, err := sites.cipher.Decrypt(*persisted.AccessTokenEncrypted, siteTokenAAD(first.ID))
	if err != nil || string(plaintext) != "verified-secret" {
		t.Fatalf("decrypt stored token = %q, %v", plaintext, err)
	}
	if _, err := sites.cipher.Decrypt(*persisted.AccessTokenEncrypted, siteTokenAAD(first.ID+1)); !errors.Is(err, common.ErrInvalidCiphertext) {
		t.Fatalf("wrong-site AAD error = %v", err)
	}
	capabilities, err := repository.ListCapabilities(context.Background(), first.ID)
	if err != nil || len(capabilities) != len(constant.SiteCapabilityKeys()) {
		t.Fatalf("stored capabilities = %d, %v", len(capabilities), err)
	}

	second := newTestSite(clock.Now().Unix(), "https://site-two.example")
	if err := repository.Create(context.Background(), &second); err != nil {
		t.Fatalf("create export-disabled site: %v", err)
	}
	client.status.DataExportEnabled = false
	client.statusErr = ErrUpstreamExportDisabled
	result, err = sites.Authorize(context.Background(), second.ID, request, "req_export_disabled")
	if err != nil {
		t.Fatalf("authorize export-disabled site: %v", err)
	}
	if result.BackfillRunID != nil {
		t.Fatalf("export-disabled authorization queued backfill: %#v", result)
	}
	persisted, err = repository.FindByID(context.Background(), second.ID)
	if err != nil {
		t.Fatalf("reload export-disabled site: %v", err)
	}
	if persisted.AccessTokenEncrypted == nil || persisted.AuthStatus != constant.SiteAuthAuthorized || persisted.StatisticsStatus != constant.SiteStatisticsPendingConfig {
		t.Fatalf("export-disabled credentials/state = %#v", persisted)
	}
	if persisted.LastProbeAt == nil {
		t.Fatal("authorization did not persist the post-commit probe attempt")
	}

	third := newTestSite(clock.Now().Unix(), "https://site-three.example")
	if err := repository.Create(context.Background(), &third); err != nil {
		t.Fatalf("create unavailable site: %v", err)
	}
	client.status.DataExportEnabled = true
	client.statusErr = ErrUpstreamUnavailable
	result, err = sites.Authorize(context.Background(), third.ID, request, "req_probe_failure")
	if err != nil {
		t.Fatalf("authorize unavailable site: %v", err)
	}
	persisted, err = repository.FindByID(context.Background(), third.ID)
	if err != nil {
		t.Fatalf("reload unavailable site: %v", err)
	}
	if persisted.LastProbeAt == nil || persisted.LastProbeSuccessAt != nil || persisted.OnlineStatus != constant.SiteOnlineUnknown {
		t.Fatalf("post-commit probe failure state = %#v", persisted)
	}
}

func TestSiteAuthorizationAppliesCompleteUserSnapshotBeforeBackfill(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	site := newTestSite(clock.Now().Unix(), "https://authorization-snapshot.example")
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create site: %v", err)
	}
	customer := model.Customer{
		Name: "Authorization Snapshot", Status: dto.CustomerStatusUsing,
		StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := tx.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: client.root.ID,
		RemoteCreatedAt: client.root.CreatedAt - 1, Username: "stale-identity",
		RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	if _, err := sites.Authorize(
		context.Background(), site.ID, existingTokenRequest("1", "snapshot-token"), "req_authorization_snapshot",
	); err != nil {
		t.Fatalf("authorize site: %v", err)
	}
	var persisted model.Account
	if err := tx.First(&persisted, account.ID).Error; err != nil {
		t.Fatalf("read snapshot account: %v", err)
	}
	if persisted.RemoteState != model.AccountRemoteStateIdentityMismatch || persisted.StatisticsPausedAt == nil ||
		*persisted.StatisticsPausedAt != floorHour(clock.Now().Unix()) {
		t.Fatalf("authorization snapshot did not isolate identity mismatch: %#v", persisted)
	}
}

func TestDisabledSiteAuthorizationAppliesIdentityPauseCleanupWithoutStartingCollection(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	pauseAt := floorHour(clock.Now().Unix())
	site := newTestSite(clock.Now().Unix(), "https://disabled-authorization-snapshot.example")
	site.ManagementStatus = constant.SiteManagementDisabled
	site.StatisticsStatus = constant.SiteStatisticsPaused
	site.DisabledAt = &pauseAt
	site.StatisticsEndAt = &pauseAt
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create disabled authorization site: %v", err)
	}
	customer := model.Customer{
		Name: "Disabled Authorization Snapshot", Status: dto.CustomerStatusUsing,
		StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := tx.Create(&customer).Error; err != nil {
		t.Fatalf("create disabled authorization customer: %v", err)
	}
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: client.root.ID,
		RemoteCreatedAt: client.root.CreatedAt - 1, Username: "stale-root", RemoteStatus: 1,
		RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("create disabled authorization account: %v", err)
	}
	dateKey := beijingDateKey(clock.Now())
	for _, row := range []any{
		&model.AccountStatHourly{AccountID: account.ID, HourTS: pauseAt, RequestCount: 5, Quota: 5, TokenUsed: 5,
			DataStatus: model.UsageAggregationStatusComplete, LastCalculatedAt: clock.Now().Unix(), CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix()},
		&model.AccountStatDaily{AccountID: account.ID, DateKey: dateKey, RequestCount: 5, Quota: 5, TokenUsed: 5,
			DataStatus: model.UsageAggregationStatusComplete, LastCalculatedAt: clock.Now().Unix(), CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix()},
		&model.CustomerStatHourly{CustomerID: customer.ID, SiteID: site.ID, HourTS: pauseAt, RequestCount: 5, Quota: 5, TokenUsed: 5,
			ActiveUsers: 1, DataStatus: model.UsageAggregationStatusComplete, LastCalculatedAt: clock.Now().Unix(), CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix()},
		&model.CustomerStatDaily{CustomerID: customer.ID, SiteID: site.ID, DateKey: dateKey, RequestCount: 5, Quota: 5, TokenUsed: 5,
			ActiveUsers: 1, DataStatus: model.UsageAggregationStatusComplete, LastCalculatedAt: clock.Now().Unix(), CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix()},
	} {
		if err := tx.Create(row).Error; err != nil {
			t.Fatalf("create disabled authorization statistic: %v", err)
		}
	}
	result, err := sites.Authorize(context.Background(), site.ID,
		existingTokenRequest(strconv.FormatInt(client.root.ID, 10), "disabled-authorization-token"), "req_disabled_authorization")
	if err != nil {
		t.Fatalf("authorize disabled site: %v", err)
	}
	if result.BackfillRunID != nil {
		t.Fatalf("disabled authorization started collection: %#v", result)
	}
	persistedSite, err := repository.FindByID(context.Background(), site.ID)
	if err != nil || persistedSite.ManagementStatus != constant.SiteManagementDisabled ||
		persistedSite.AuthStatus != constant.SiteAuthAuthorized || persistedSite.StatisticsStatus != constant.SiteStatisticsPaused {
		t.Fatalf("disabled authorization site = %#v err=%v", persistedSite, err)
	}
	persistedAccount, err := model.NewAccountRepository(tx).FindByID(context.Background(), account.ID)
	if err != nil || persistedAccount.RemoteState != model.AccountRemoteStateIdentityMismatch ||
		persistedAccount.StatisticsPausedAt == nil || *persistedAccount.StatisticsPausedAt != pauseAt {
		t.Fatalf("disabled authorization account = %#v err=%v", persistedAccount, err)
	}
	for table, where := range map[string]string{
		"account_stat_hourly":  "account_id = ?",
		"account_stat_daily":   "account_id = ?",
		"customer_stat_hourly": "customer_id = ?",
		"customer_stat_daily":  "customer_id = ?",
	} {
		var count int64
		targetID := customer.ID
		if strings.HasPrefix(table, "account_") {
			targetID = account.ID
		}
		if err := tx.Table(table).Where(where, targetID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s rows after disabled authorization = %d err=%v", table, count, err)
		}
	}
}

func TestProbeDataExportTransitionNotifiesProbeAndSiteLifecycle(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	client.status.DataExportEnabled = false
	client.statusErr = ErrUpstreamExportDisabled
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	recorder := &sitePostCommitRecorder{}
	sites.postCommit = recorder
	site := newTestSite(clock.Now().Unix(), "https://probe-export-transition.example")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	site.DataExportEnabled = true
	site.Version = "v-test"
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create probe transition site: %v", err)
	}
	if err := storeReadyCapabilities(context.Background(), model.NewSiteRepository(tx), site.ID, clock.Now().Unix()); err != nil {
		t.Fatalf("store probe capabilities: %v", err)
	}
	if _, err := sites.Probe(context.Background(), site.ID, "req_probe_export_transition"); err != nil {
		t.Fatalf("probe export transition: %v", err)
	}
	if len(recorder.triggers) != 2 || recorder.triggers[0].Source != AlertSampleSourceProbe ||
		recorder.triggers[1].Source != AlertSampleSourceLifecycle || recorder.triggers[1].ScopeType != "site" ||
		recorder.triggers[1].ScopeID != site.ID || recorder.triggers[1].ObservedAt != recorder.triggers[0].ObservedAt {
		t.Fatalf("probe export triggers = %#v", recorder.triggers)
	}
}

func TestSiteDisableEnableFenceAndPauseAreIdempotent(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_896, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	now := clock.Now().Unix()
	statisticsStart := floorHour(now) - 2*3600
	rootID := int64(1)
	rootCreated := statisticsStart + 60
	source := "root_created_at"
	site := newTestSite(now, "https://lifecycle.example")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	site.DataExportEnabled = true
	site.Version = "v-test"
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreated
	site.StatisticsStartAt = &statisticsStart
	site.StatisticsStartSource = &source
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create lifecycle site: %v", err)
	}
	if err := storeReadyCapabilities(context.Background(), repository, site.ID, now); err != nil {
		t.Fatalf("store capabilities: %v", err)
	}
	oldEnd := statisticsStart + 3600
	oldRunModel, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{
		TaskType: constant.TaskTypeUsageHour, TriggerType: constant.CollectionTriggerSchedule,
		StartTimestamp: &statisticsStart, EndTimestamp: &oldEnd, Priority: constant.CollectionPriorityUsageRealtime,
		RequestID: "req_old", Now: now,
	})
	if err != nil {
		t.Fatalf("build old run: %v", err)
	}
	oldRun, _, err := repository.CreateOrGetRun(context.Background(), &oldRunModel)
	if err != nil {
		t.Fatalf("create old run: %v", err)
	}

	detail, err := sites.Disable(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("disable site: %v", err)
	}
	if detail.ConfigVersion != 2 || detail.ManagementStatus != constant.SiteManagementDisabled || detail.StatisticsStatus != constant.SiteStatisticsPaused {
		t.Fatalf("disabled detail = %#v", detail)
	}
	var terminated model.CollectionRun
	if err := tx.First(&terminated, oldRun.ID).Error; err != nil {
		t.Fatalf("reload terminated run: %v", err)
	}
	if terminated.Status != "failed" || terminated.ActiveKey != nil || terminated.ErrorCode != constant.CodeSiteConfigChanged {
		t.Fatalf("terminated run = %#v", terminated)
	}
	if _, err := sites.Disable(context.Background(), site.ID); err != nil {
		t.Fatalf("repeat disable: %v", err)
	}
	disabled, err := repository.FindByID(context.Background(), site.ID)
	if err != nil || disabled.ConfigVersion != 2 {
		t.Fatalf("repeat disable fence = %d, %v", disabled.ConfigVersion, err)
	}
	var pauseCount int64
	if err := tx.Model(&model.SiteMonitoringPause{}).Where("site_id = ?", site.ID).Count(&pauseCount).Error; err != nil || pauseCount != 1 {
		t.Fatalf("pause rows = %d, %v", pauseCount, err)
	}

	run, err := sites.Enable(context.Background(), site.ID, "req_enable")
	if err != nil {
		t.Fatalf("enable site: %v", err)
	}
	if run.TaskType != constant.TaskTypeUsageBackfill || run.Deduplicated {
		t.Fatalf("enable run = %#v", run)
	}
	enabled, err := repository.FindByID(context.Background(), site.ID)
	if err != nil || enabled.ConfigVersion != 3 || enabled.ManagementStatus != constant.SiteManagementActive || enabled.StatisticsStatus != constant.SiteStatisticsBackfilling {
		t.Fatalf("enabled site = %#v, %v", enabled, err)
	}
	var pause model.SiteMonitoringPause
	if err := tx.Where("site_id = ?", site.ID).First(&pause).Error; err != nil || pause.EndMinuteTS == nil {
		t.Fatalf("closed pause = %#v, %v", pause, err)
	}
	repeated, err := sites.Enable(context.Background(), site.ID, "req_enable_again")
	if err != nil {
		t.Fatalf("repeat enable: %v", err)
	}
	if repeated.ID != run.ID || !repeated.Deduplicated {
		t.Fatalf("repeat enable run = %#v, first=%#v", repeated, run)
	}
	enabled, _ = repository.FindByID(context.Background(), site.ID)
	if enabled.ConfigVersion != 3 {
		t.Fatalf("repeat enable bumped config version to %d", enabled.ConfigVersion)
	}
}

func TestSitePreflightTokenExpiryAndIntegrity(t *testing.T) {
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	sites := &SiteService{clock: clock, preflightSecret: []byte("01234567890123456789012345678901")}
	claims := sitePreflightClaims{SiteID: 7, ConfigVersion: 3, BaseURL: "https://site.example", ExpiresAt: clock.Now().Add(10 * time.Minute).Unix()}
	token, err := sites.signPreflightToken(claims)
	if err != nil {
		t.Fatalf("sign preflight: %v", err)
	}
	verified, err := sites.verifyPreflightToken(token)
	if err != nil || verified != claims {
		t.Fatalf("verify preflight = %#v, %v", verified, err)
	}
	if _, err := sites.verifyPreflightToken(token + "x"); !errors.Is(err, ErrBaseURLPreflightRequired) {
		t.Fatalf("tampered preflight error = %v", err)
	}
	clock.Advance(10 * time.Minute)
	if _, err := sites.verifyPreflightToken(token); !errors.Is(err, ErrBaseURLPreflightRequired) {
		t.Fatalf("expired preflight error = %v", err)
	}
}

func TestSiteUpdateIgnoresPreflightTokenWhenBaseURLIsUnchanged(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	site := newTestSite(clock.Now().Unix(), "https://metadata-only.example")
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create metadata-only site: %v", err)
	}

	detail, err := sites.Update(context.Background(), site.ID, dto.SiteUpdateRequest{
		Name: "Renamed Site", BaseURL: site.BaseURL, Remark: "metadata changed",
		BaseURLPreflightToken: "expired-or-invalid-token",
	})
	if err != nil {
		t.Fatalf("metadata-only update: %v", err)
	}
	if detail.Name != "Renamed Site" || detail.Remark != "metadata changed" || detail.ConfigVersion != 1 {
		t.Fatalf("metadata-only update detail = %#v", detail)
	}
}

func TestCollectionRunItemProgressIncludesUnavailableWindows(t *testing.T) {
	item := collectionRunItemFromModel(model.CollectionRun{
		ID: 1, SiteConfigVersion: 1, TaskType: constant.TaskTypeUsageHour,
		TargetType: "site", TargetID: 1, TriggerType: constant.CollectionTriggerSchedule,
		Status: "running", TotalWindows: 8, CompletedWindows: 1, FailedWindows: 1,
		UnavailableWindows: 2, CreatedRequestID: "req_progress", LastRequestID: "req_progress",
		CreatedAt: 1,
	}, false)
	if item.Progress != 0.5 {
		t.Fatalf("progress = %v, want 0.5", item.Progress)
	}
}

func TestManualBackfillLimitReadsCurrentSetting(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	end := floorHour(clock.Now().Unix())
	statisticsStart := end - 72*3600
	rootID := int64(1)
	rootCreated := statisticsStart + 60
	source := "root_created_at"
	site := newTestSite(clock.Now().Unix(), "https://dynamic-backfill.example")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	site.DataExportEnabled = true
	site.Version = "v-test"
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreated
	site.StatisticsStartAt = &statisticsStart
	site.StatisticsStartSource = &source
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create backfill site: %v", err)
	}
	if err := storeReadyCapabilities(context.Background(), repository, site.ID, clock.Now().Unix()); err != nil {
		t.Fatalf("store backfill capabilities: %v", err)
	}
	futureStart := end - 3600
	futureEnd := end + 3600
	futureRequest := dto.SiteBackfillRequest{StartTimestamp: &futureStart, EndTimestamp: &futureEnd}
	if _, err := sites.Backfill(context.Background(), site.ID, futureRequest, "req_future_end"); !errors.Is(err, ErrSiteInvalidBackfillRange) {
		t.Fatalf("future backfill end error = %v", err)
	}
	var futureRunCount int64
	if err := tx.Model(&model.CollectionRun{}).
		Where("site_id = ? AND task_type = ?", site.ID, constant.TaskTypeUsageBackfill).
		Count(&futureRunCount).Error; err != nil {
		t.Fatalf("count future backfill runs: %v", err)
	}
	if futureRunCount != 0 {
		t.Fatalf("future backfill run count = %d", futureRunCount)
	}
	setManualBackfillDays(t, tx, 1, clock.Now().Unix())
	start := end - 48*3600
	request := dto.SiteBackfillRequest{StartTimestamp: &start, EndTimestamp: &end}
	if _, err := sites.Backfill(context.Background(), site.ID, request, "req_limit_low"); !errors.Is(err, ErrSiteInvalidBackfillRange) {
		t.Fatalf("low dynamic limit error = %v", err)
	}
	setManualBackfillDays(t, tx, 3, clock.Now().Unix()+1)
	created, err := sites.Backfill(context.Background(), site.ID, request, "req_limit_high")
	if err != nil || created.Deduplicated {
		t.Fatalf("raised dynamic limit result = %#v, %v", created, err)
	}
	setManualBackfillDays(t, tx, 1, clock.Now().Unix()+2)
	if _, err := sites.Backfill(context.Background(), site.ID, request, "req_limit_lowered"); !errors.Is(err, ErrSiteInvalidBackfillRange) {
		t.Fatalf("lowered dynamic limit error = %v", err)
	}
}

func TestListInstancesUsesEffectiveRuleAndRejectsOldSamplesAsCurrent(t *testing.T) {
	tx := openSiteTestTransaction(t)
	now := int64(1_752_400_830)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	client := authorizedTestSiteClient(now)
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	site := newTestSite(now, "https://instances.example")
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create instance site: %v", err)
	}
	upsertInstanceStaleRule(t, tx, "global", 0, 90, now)
	currentMinute := floorMinute(now)
	oldMinute := currentMinute - 60
	createInstanceFixture(t, tx, site.ID, "exact", now-90, currentMinute, 10)
	createInstanceFixture(t, tx, site.ID, "fresh", now-89, currentMinute, 20)
	createInstanceFixture(t, tx, site.ID, "expired", now-91, currentMinute, 25)
	createInstanceFixture(t, tx, site.ID, "old", now-70, oldMinute, 30)
	if err := tx.Create(&model.SiteInstance{
		SiteID: site.ID, NodeName: "none", Hostname: "none.local", UpstreamStatus: "unknown",
		CurrentStatus: "online", FirstSeenAt: now - 300, LastSyncedAt: now - 120,
		CreatedAt: now - 300, UpdatedAt: now - 120,
	}).Error; err != nil {
		t.Fatalf("create no-sample instance: %v", err)
	}

	items, err := sites.ListInstances(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("list instances with global rule: %v", err)
	}
	byNode := instanceItemsByNode(items)
	if byNode["exact"].CurrentStatus != "stale" || byNode["exact"].DataStatus != "complete" {
		t.Fatalf("exact-threshold instance = %#v", byNode["exact"])
	}
	if byNode["fresh"].CurrentStatus != "online" || byNode["fresh"].EffectiveStaleAfterSeconds != 90 {
		t.Fatalf("fresh instance = %#v", byNode["fresh"])
	}
	if byNode["expired"].CurrentStatus != "stale" {
		t.Fatalf("expired instance = %#v", byNode["expired"])
	}
	old := byNode["old"]
	if old.CurrentStatus != "unknown" || old.DataStatus != "missing" || old.SampledAt == nil ||
		old.CPUPercent != nil || old.MemoryPercent != nil || old.DiskUsedPercent != nil ||
		old.DiskTotalBytes != nil || old.DiskUsedBytes != nil {
		t.Fatalf("old instance exposed current data = %#v", old)
	}
	if none := byNode["none"]; none.CurrentStatus != "unknown" || none.DataStatus != "missing" || none.SampledAt != nil {
		t.Fatalf("no-sample instance = %#v", none)
	}

	upsertInstanceStaleRule(t, tx, "site", site.ID, 120, now+1)
	items, err = sites.ListInstances(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("list instances with site override: %v", err)
	}
	byNode = instanceItemsByNode(items)
	if byNode["exact"].CurrentStatus != "online" || byNode["exact"].EffectiveStaleAfterSeconds != 120 ||
		byNode["expired"].CurrentStatus != "online" {
		t.Fatalf("site override was not applied in real time: exact=%#v expired=%#v", byNode["exact"], byNode["expired"])
	}
}

func setManualBackfillDays(t *testing.T, tx *gorm.DB, days int, now int64) {
	t.Helper()
	if err := tx.Exec(`INSERT INTO platform_setting
  (setting_key, setting_value, value_type, is_secret, updated_at)
VALUES ('collector.manual_backfill_max_days', ?, 'int', 0, ?)
ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value), value_type = 'int', is_secret = 0, updated_at = VALUES(updated_at)`,
		strconv.Itoa(days), now).Error; err != nil {
		t.Fatalf("set manual backfill days: %v", err)
	}
}

func upsertInstanceStaleRule(t *testing.T, tx *gorm.DB, scopeType string, scopeID, threshold, now int64) {
	t.Helper()
	if err := tx.Exec(`INSERT INTO alert_rule
  (rule_key, name, enabled, level, metric, compare_operator, threshold_value, for_times,
   scope_type, scope_id, created_at, updated_at)
VALUES ('instance_stale', 'Instance Stale', 1, 'warning', 'instance.stale_seconds', '>=', ?, 1, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE threshold_value = VALUES(threshold_value), updated_at = VALUES(updated_at)`,
		threshold, scopeType, scopeID, now, now).Error; err != nil {
		t.Fatalf("upsert instance stale rule: %v", err)
	}
}

func createInstanceFixture(t *testing.T, tx *gorm.DB, siteID int64, node string, lastSeen, minute int64, cpu float64) {
	t.Helper()
	if err := tx.Create(&model.SiteInstance{
		SiteID: siteID, NodeName: node, Hostname: node + ".local", UpstreamStatus: "online",
		CurrentStatus: "online", FirstSeenAt: minute - 300, LastSeenAt: &lastSeen,
		LastSyncedAt: minute, CreatedAt: minute - 300, UpdatedAt: minute,
	}).Error; err != nil {
		t.Fatalf("create instance %s: %v", node, err)
	}
	memory := cpu + 1
	disk := cpu + 2
	diskTotal := int64(1000)
	diskUsed := int64(500)
	if err := tx.Create(&model.SiteInstanceStatusMinutely{
		SiteID: siteID, NodeName: node, MinuteTS: minute, Status: "online",
		CPUPercent: &cpu, MemoryPercent: &memory, DiskUsedPercent: &disk,
		DiskTotalBytes: &diskTotal, DiskUsedBytes: &diskUsed, LastSeenAt: &lastSeen, CreatedAt: minute,
	}).Error; err != nil {
		t.Fatalf("create instance sample %s: %v", node, err)
	}
}

func instanceItemsByNode(items []dto.SiteInstanceItem) map[string]dto.SiteInstanceItem {
	result := make(map[string]dto.SiteInstanceItem, len(items))
	for _, item := range items {
		result[item.NodeName] = item
	}
	return result
}

func TestSiteProbeCapabilityFenceIsTransitionBased(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	factory := &testSiteClientFactory{authenticated: client, public: client}
	sites := newIntegrationSiteService(t, tx, clock, factory)
	repository := model.NewSiteRepository(tx)
	now := clock.Now().Unix()
	statisticsStart := floorHour(now) - 3600
	rootID := int64(1)
	rootCreated := statisticsStart + 60
	source := "root_created_at"
	site := newTestSite(now, "https://probe.example")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	site.DataExportEnabled = true
	site.Version = "v-test"
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreated
	site.StatisticsStartAt = &statisticsStart
	site.StatisticsStartSource = &source
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create probe site: %v", err)
	}
	if err := storeReadyCapabilities(context.Background(), repository, site.ID, now); err != nil {
		t.Fatalf("store probe capabilities: %v", err)
	}
	oldEnd := statisticsStart + 3600
	oldRunModel, err := model.NewSiteCollectionRun(site, model.SiteRunSpec{
		TaskType: constant.TaskTypeUsageHour, TriggerType: constant.CollectionTriggerSchedule,
		StartTimestamp: &statisticsStart, EndTimestamp: &oldEnd, Priority: constant.CollectionPriorityUsageRealtime,
		RequestID: "req_probe_old", Now: now,
	})
	if err != nil {
		t.Fatalf("build probe old run: %v", err)
	}
	oldRun, _, err := repository.CreateOrGetRun(context.Background(), &oldRunModel)
	if err != nil {
		t.Fatalf("create probe old run: %v", err)
	}

	client.statusErr = ErrUpstreamExportDisabled
	probe, err := sites.Probe(context.Background(), site.ID, "req_probe")
	if err != nil {
		t.Fatalf("probe status error: %v", err)
	}
	if !probe.ProbeSuccess || probe.OnlineStatus != constant.SiteOnlineOnline {
		t.Fatalf("probe result = %#v", probe)
	}
	persisted, err := repository.FindByID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("probe site = %#v, %v", persisted, err)
	}
	var terminated model.CollectionRun
	if err := tx.First(&terminated, oldRun.ID).Error; err != nil || terminated.Status != "failed" {
		t.Fatalf("probe did not terminate old run: %#v, %v", terminated, err)
	}
	if persisted.ConfigVersion != site.ConfigVersion+1 || persisted.StatisticsStatus != constant.SiteStatisticsPendingConfig {
		t.Fatalf("probe capability transition state = %#v", persisted)
	}
	if _, err := sites.Probe(context.Background(), site.ID, "req_probe_repeat"); err != nil {
		t.Fatalf("repeat failed capability probe: %v", err)
	}
	repeated, err := repository.FindByID(context.Background(), site.ID)
	if err != nil || repeated.ConfigVersion != persisted.ConfigVersion {
		t.Fatalf("repeated failed capability probe bumped fence again: %#v, %v", repeated, err)
	}

	offline := newTestSite(now, "https://offline.example")
	if err := repository.Create(context.Background(), &offline); err != nil {
		t.Fatalf("create offline site: %v", err)
	}
	factory.err = ErrUpstreamUnavailable
	for attempt := 1; attempt <= 3; attempt++ {
		result, err := sites.Probe(context.Background(), offline.ID, "req_offline_"+strconv.Itoa(attempt))
		if err != nil {
			t.Fatalf("offline probe %d: %v", attempt, err)
		}
		if result.ProbeSuccess || result.ContractStatus != "unavailable" {
			t.Fatalf("offline probe %d result = %#v", attempt, result)
		}
		if attempt < 3 && result.OnlineStatus == constant.SiteOnlineOffline {
			t.Fatalf("site became offline after only %d failures", attempt)
		}
	}
	offline, _ = repository.FindByID(context.Background(), offline.ID)
	if offline.OnlineStatus != constant.SiteOnlineOffline || offline.ProbeFailCount != 3 {
		t.Fatalf("offline site state = %#v", offline)
	}
}

func TestSiteDeleteDependencyChecksUseValidSiteColumns(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)

	empty := newTestSite(clock.Now().Unix(), "https://delete-empty.example")
	if err := repository.Create(context.Background(), &empty); err != nil {
		t.Fatalf("create empty site: %v", err)
	}
	if err := sites.Delete(context.Background(), empty.ID); err != nil {
		t.Fatalf("delete empty site: %v", err)
	}
	if _, err := repository.FindByID(context.Background(), empty.ID); !model.IsNotFound(err) {
		t.Fatalf("deleted site lookup error = %v", err)
	}

	restricted := newTestSite(clock.Now().Unix(), "https://delete-restricted.example")
	if err := repository.Create(context.Background(), &restricted); err != nil {
		t.Fatalf("create restricted site: %v", err)
	}
	dependentRun, err := model.NewSiteCollectionRun(restricted, model.SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_delete", Now: clock.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("build dependent run: %v", err)
	}
	if _, _, err := repository.CreateOrGetRun(context.Background(), &dependentRun); err != nil {
		t.Fatalf("create dependent run: %v", err)
	}
	deleteErr := sites.Delete(context.Background(), restricted.ID)
	var restrictedDetails *SiteDeleteRestrictedError
	if !errors.Is(deleteErr, ErrSiteDeleteRestricted) || !errors.As(deleteErr, &restrictedDetails) {
		t.Fatalf("delete restricted site error = %v", deleteErr)
	}
	if len(restrictedDetails.DependencyTypes) != 1 || restrictedDetails.DependencyTypes[0] != model.SiteDeleteDependencyActiveCollection {
		t.Fatalf("delete restricted dependency types = %#v", restrictedDetails.DependencyTypes)
	}
}

func TestSiteDeleteCleansValuelessMetadataButPreservesHistory(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	now := clock.Now().Unix()

	deletable := newTestSite(now, "https://delete-metadata.example")
	if err := repository.Create(context.Background(), &deletable); err != nil {
		t.Fatalf("create deletable site: %v", err)
	}
	if err := tx.Create(&model.SiteChannel{SiteID: deletable.ID, RemoteChannelID: 1, Name: "channel", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create deletable channel: %v", err)
	}
	if err := tx.Create(&model.SiteMonitoringPause{
		SiteID: deletable.ID, StartMinuteTS: floorMinute(now), Reason: "management_disabled", CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create deletable pause: %v", err)
	}
	if err := tx.Create(&model.SiteInstance{
		SiteID: deletable.ID, NodeName: "node", Hostname: "node.local", UpstreamStatus: "unknown",
		CurrentStatus: "unknown", FirstSeenAt: now, LastSyncedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create deletable current instance: %v", err)
	}
	terminalRun, err := model.NewSiteCollectionRun(deletable, model.SiteRunSpec{
		TaskType: constant.TaskTypeSiteProbe, TriggerType: constant.CollectionTriggerManual,
		Priority: 0, RequestID: "req_delete_terminal", Now: now,
	})
	if err != nil {
		t.Fatalf("build deletable terminal run: %v", err)
	}
	createdRun, _, err := repository.CreateOrGetRun(context.Background(), &terminalRun)
	if err != nil {
		t.Fatalf("create deletable terminal run: %v", err)
	}
	if err := tx.Model(&model.CollectionRun{}).Where("id = ?", createdRun.ID).Updates(map[string]any{
		"status": "success", "active_key": nil, "finished_at": now, "updated_at": now,
	}).Error; err != nil {
		t.Fatalf("finish deletable terminal run: %v", err)
	}
	if err := tx.Exec(`INSERT INTO collection_cursor (site_id, cursor_key, last_complete_hour, updated_at)
VALUES (?, 'usage', NULL, ?)`, deletable.ID, now).Error; err != nil {
		t.Fatalf("create deletable cursor: %v", err)
	}
	if err := tx.Exec(`INSERT INTO collection_window (site_id, hour_ts, status, updated_at)
VALUES (?, ?, 'missing', ?)`, deletable.ID, floorHour(now)-3600, now).Error; err != nil {
		t.Fatalf("create deletable collection window: %v", err)
	}
	if err := tx.Exec(`INSERT INTO aggregation_bucket_lock (lock_key, updated_at) VALUES (?, ?)`,
		"stats:site:"+strconv.FormatInt(deletable.ID, 10)+":hour:"+strconv.FormatInt(floorHour(now), 10), now).Error; err != nil {
		t.Fatalf("create deletable aggregation lock: %v", err)
	}
	overrideID := createSiteAlertOverride(t, tx, deletable.ID, now)
	if err := tx.Exec(`INSERT INTO alert_event
  (rule_id, rule_key, site_id, target_type, target_key, level, status, consecutive_count,
   message_code, message, first_observed_at, resolved_at, created_at, updated_at)
VALUES (?, 'instance_stale', ?, 'site', ?, 'warning', 'resolved', 1,
        'CAPABILITY_OK', 'never fired', ?, ?, ?, ?)`,
		overrideID, deletable.ID, strconv.FormatInt(deletable.ID, 10), now, now, now, now).Error; err != nil {
		t.Fatalf("create valueless resolved alert: %v", err)
	}

	if err := sites.Delete(context.Background(), deletable.ID); err != nil {
		t.Fatalf("delete site with valueless metadata: %v", err)
	}
	if _, err := repository.FindByID(context.Background(), deletable.ID); !model.IsNotFound(err) {
		t.Fatalf("deleted metadata site lookup error = %v", err)
	}
	for _, check := range []struct {
		name  string
		query string
		args  []any
	}{
		{name: "run", query: "SELECT COUNT(*) FROM collection_run WHERE site_id = ?", args: []any{deletable.ID}},
		{name: "window", query: "SELECT COUNT(*) FROM collection_window WHERE site_id = ?", args: []any{deletable.ID}},
		{name: "rule", query: "SELECT COUNT(*) FROM alert_rule WHERE scope_type = 'site' AND scope_id = ?", args: []any{deletable.ID}},
		{name: "event", query: "SELECT COUNT(*) FROM alert_event WHERE site_id = ?", args: []any{deletable.ID}},
		{name: "instance", query: "SELECT COUNT(*) FROM site_instance WHERE site_id = ?", args: []any{deletable.ID}},
	} {
		var count int
		if err := tx.Raw(check.query, check.args...).Scan(&count).Error; err != nil || count != 0 {
			t.Fatalf("deleted %s metadata count = %d, %v", check.name, count, err)
		}
	}

	factSite := newTestSite(now, "https://delete-fact.example")
	if err := repository.Create(context.Background(), &factSite); err != nil {
		t.Fatalf("create fact blocker site: %v", err)
	}
	if err := tx.Exec(`INSERT INTO usage_fact_hourly
  (site_id, remote_user_id, username_snapshot, model_name, channel_id, hour_ts,
   request_count, quota, token_used, collected_at)
VALUES (?, 1, 'root', 'model', 1, ?, 1, 1, 1, ?)`, factSite.ID, floorHour(now), now).Error; err != nil {
		t.Fatalf("create fact blocker: %v", err)
	}
	if err := sites.Delete(context.Background(), factSite.ID); !errors.Is(err, ErrSiteDeleteRestricted) {
		t.Fatalf("fact blocker delete error = %v", err)
	}

	resourceSite := newTestSite(now, "https://delete-resource.example")
	if err := repository.Create(context.Background(), &resourceSite); err != nil {
		t.Fatalf("create resource blocker site: %v", err)
	}
	if err := tx.Create(&model.SiteInstanceStatusMinutely{
		SiteID: resourceSite.ID, NodeName: "historical", MinuteTS: floorMinute(now),
		Status: "online", CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create resource history blocker: %v", err)
	}
	if err := sites.Delete(context.Background(), resourceSite.ID); !errors.Is(err, ErrSiteDeleteRestricted) {
		t.Fatalf("resource history blocker delete error = %v", err)
	}

	alertSite := newTestSite(now, "https://delete-alert.example")
	if err := repository.Create(context.Background(), &alertSite); err != nil {
		t.Fatalf("create alert blocker site: %v", err)
	}
	alertRuleID := createSiteAlertOverride(t, tx, alertSite.ID, now)
	if err := tx.Exec(`INSERT INTO alert_event
  (rule_id, rule_key, site_id, target_type, target_key, level, status, consecutive_count,
   message_code, message, first_observed_at, first_fired_at, resolved_at, created_at, updated_at)
VALUES (?, 'instance_stale', ?, 'site', ?, 'warning', 'resolved', 1,
        'CAPABILITY_OK', 'historical', ?, ?, ?, ?, ?)`,
		alertRuleID, alertSite.ID, strconv.FormatInt(alertSite.ID, 10), now, now, now, now, now).Error; err != nil {
		t.Fatalf("create historical alert blocker: %v", err)
	}
	if err := sites.Delete(context.Background(), alertSite.ID); !errors.Is(err, ErrSiteDeleteRestricted) {
		t.Fatalf("historical alert blocker delete error = %v", err)
	}
}

func TestSiteDeleteRollsBackMetadataCleanupWhenFinalDeleteFails(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	repository := model.NewSiteRepository(tx)
	now := clock.Now().Unix()
	site := newTestSite(now, "https://delete-rollback.example")
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create rollback site: %v", err)
	}
	if err := tx.Create(&model.SiteChannel{SiteID: site.ID, RemoteChannelID: 7, Name: "rollback", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("create rollback channel: %v", err)
	}
	const callbackName = "test:fail-final-site-delete"
	if err := tx.Callback().Delete().Before("gorm:delete").Register(callbackName, func(callbackDB *gorm.DB) {
		if callbackDB.Statement != nil && callbackDB.Statement.Table == "site" {
			callbackDB.AddError(errors.New("injected final site delete failure"))
		}
	}); err != nil {
		t.Fatalf("register delete failure callback: %v", err)
	}
	defer func() { _ = tx.Callback().Delete().Remove(callbackName) }()
	if err := sites.Delete(context.Background(), site.ID); err == nil || !strings.Contains(err.Error(), "injected final site delete failure") {
		t.Fatalf("injected delete error = %v", err)
	}
	if _, err := repository.FindByID(context.Background(), site.ID); err != nil {
		t.Fatalf("site was not restored after rollback: %v", err)
	}
	var channels int64
	if err := tx.Model(&model.SiteChannel{}).Where("site_id = ?", site.ID).Count(&channels).Error; err != nil || channels != 1 {
		t.Fatalf("channel rollback count = %d, %v", channels, err)
	}
}

func createSiteAlertOverride(t *testing.T, tx *gorm.DB, siteID, now int64) int64 {
	t.Helper()
	if err := tx.Exec(`INSERT INTO alert_rule
  (rule_key, name, enabled, level, metric, compare_operator, threshold_value, for_times,
   scope_type, scope_id, created_at, updated_at)
VALUES ('instance_stale', 'Instance Stale', 1, 'warning', 'instance.stale_seconds', '>=', 90, 1,
        'site', ?, ?, ?)`, siteID, now, now).Error; err != nil {
		t.Fatalf("create site alert override: %v", err)
	}
	var id int64
	if err := tx.Raw(`SELECT id FROM alert_rule
WHERE rule_key = 'instance_stale' AND level = 'warning' AND scope_type = 'site' AND scope_id = ?`, siteID).
		Scan(&id).Error; err != nil || id <= 0 {
		t.Fatalf("read site alert override id = %d, %v", id, err)
	}
	return id
}

func openSiteTestTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve test lock connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", siteIntegrationLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire site test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", siteIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", siteIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("seed service test settings: %v", err)
	}
	tx := database.GORM.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin site test transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", siteIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
	})
	return tx
}

func newIntegrationSiteService(t *testing.T, tx *gorm.DB, clock common.Clock, factory SiteClientFactory) *SiteService {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	sites, err := NewSiteService(SiteServiceOptions{
		Repository: model.NewSiteRepository(tx), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create site service: %v", err)
	}
	return sites
}

func newTestSite(now int64, baseURL string) model.Site {
	return model.Site{
		Name: "Test Site", BaseURL: baseURL, ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineUnknown,
		AuthStatus: constant.SiteAuthUnauthorized, StatisticsStatus: constant.SiteStatisticsPendingConfig,
		HealthStatus: constant.SiteHealthUnavailable, CreatedAt: now, UpdatedAt: now,
	}
}

func existingTokenRequest(rootID, token string) dto.SiteAuthorizeRequest {
	return dto.SiteAuthorizeRequest{Mode: "existing_token", RootUserID: &rootID, AccessToken: &token}
}

func storeReadyCapabilities(ctx context.Context, repository *model.SiteRepository, siteID, now int64) error {
	capabilities := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, model.SiteCapability{
			SiteID: siteID, CapabilityKey: key, Status: status, MessageCode: string(constant.MessageCapabilityOK),
			MessageParams: []byte(`{"site_id":"` + strconv.FormatInt(siteID, 10) + `","capability_key":"` + key + `"}`), CheckedAt: now,
		})
	}
	return repository.ReplaceCapabilities(ctx, siteID, capabilities)
}

type testSiteClientFactory struct {
	public        SiteUpstreamClient
	authenticated SiteUpstreamClient
	err           error
}

func (factory *testSiteClientFactory) NewPublic(string) (SiteUpstreamClient, error) {
	if factory.err != nil {
		return nil, factory.err
	}
	return factory.public, nil
}

func (factory *testSiteClientFactory) NewAuthenticated(string, string, string, int64) (SiteUpstreamClient, error) {
	if factory.err != nil {
		return nil, factory.err
	}
	return factory.authenticated, nil
}

type testSiteClient struct {
	status           dto.UpstreamStatus
	statusErr        error
	self             dto.UpstreamIdentity
	selfErr          error
	root             dto.UpstreamUser
	snapshot         dto.UpstreamUserSnapshot
	channels         dto.UpstreamChannelSnapshot
	channelsErr      error
	instances        []dto.UpstreamInstance
	instancesErr     error
	realtime         dto.UpstreamLogStat
	realtimeErr      error
	performance      dto.UpstreamPerformanceHistory
	performanceErr   error
	topups           dto.UpstreamTopupSnapshot
	topupsErr        error
	redemptions      dto.UpstreamRedemptionSnapshot
	redemptionsErr   error
	upstreamTasks    dto.UpstreamTaskSnapshot
	upstreamTasksErr error
	snapshotErr      error
	flowErr          error
	dataErr          error
	flowHours        []int64
	dataHours        []int64
	loginToken       string
}

type sitePostCommitRecorder struct {
	triggers []AlertPostCommitTrigger
}

func (recorder *sitePostCommitRecorder) NotifyAfterCommit(_ context.Context, trigger AlertPostCommitTrigger) {
	recorder.triggers = append(recorder.triggers, trigger)
}

func authorizedTestSiteClient(now int64) *testSiteClient {
	root := dto.UpstreamUser{
		UpstreamIdentity: dto.UpstreamIdentity{ID: 1, Username: "root", Role: 100, Status: 1},
		CreatedAt:        floorHour(now) - 2*3600 + 60,
	}
	return &testSiteClient{
		status: dto.UpstreamStatus{
			Version: "v-test", SystemName: "Test Upstream", QuotaPerUnit: "500000.0000000000",
			USDExchangeRate: "7.0000000000", DataExportEnabled: true,
		},
		self: root.UpstreamIdentity, root: root,
		snapshot:  dto.UpstreamUserSnapshot{Total: 1, Items: []dto.UpstreamUser{root}},
		channels:  dto.UpstreamChannelSnapshot{Total: 0, Items: []dto.UpstreamChannel{}},
		instances: []dto.UpstreamInstance{}, realtime: dto.UpstreamLogStat{RPM: 2, TPM: 3}, loginToken: "generated-token",
	}
}

func (client *testSiteClient) Status(context.Context, string) (dto.UpstreamStatus, error) {
	return client.status, client.statusErr
}

func (client *testSiteClient) Self(context.Context, string) (dto.UpstreamIdentity, error) {
	return client.self, client.selfErr
}

func (client *testSiteClient) GetUser(context.Context, string, int64) (dto.UpstreamUser, error) {
	return client.root, nil
}

func (client *testSiteClient) SnapshotUsers(context.Context, string) (dto.UpstreamUserSnapshot, error) {
	return client.snapshot, client.snapshotErr
}

func (client *testSiteClient) SnapshotChannels(context.Context, string) (dto.UpstreamChannelSnapshot, error) {
	return client.channels, client.channelsErr
}

func (client *testSiteClient) FlowHour(_ context.Context, _ string, hour int64) ([]dto.UpstreamFlowRow, error) {
	client.flowHours = append(client.flowHours, hour)
	return []dto.UpstreamFlowRow{}, client.flowErr
}

func (client *testSiteClient) DataHour(_ context.Context, _ string, hour int64) ([]dto.UpstreamDataRow, error) {
	client.dataHours = append(client.dataHours, hour)
	return []dto.UpstreamDataRow{}, client.dataErr
}

func (client *testSiteClient) Instances(context.Context, string) ([]dto.UpstreamInstance, error) {
	return client.instances, client.instancesErr
}

func (client *testSiteClient) LogStat(context.Context, string) (dto.UpstreamLogStat, error) {
	return client.realtime, client.realtimeErr
}

func (client *testSiteClient) PerformanceSummary(context.Context, string, int) (dto.UpstreamPerformanceSummary, error) {
	return dto.UpstreamPerformanceSummary{}, nil
}

func (client *testSiteClient) PerformanceHistory(context.Context, string, int) (dto.UpstreamPerformanceHistory, error) {
	return client.performance, client.performanceErr
}
func (client *testSiteClient) SnapshotTopups(context.Context, string) (dto.UpstreamTopupSnapshot, error) {
	return client.topups, client.topupsErr
}
func (client *testSiteClient) SnapshotRedemptions(context.Context, string) (dto.UpstreamRedemptionSnapshot, error) {
	return client.redemptions, client.redemptionsErr
}
func (client *testSiteClient) SnapshotUpstreamTasks(context.Context, string, int64, int64, []string) (dto.UpstreamTaskSnapshot, error) {
	return client.upstreamTasks, client.upstreamTasksErr
}

func (client *testSiteClient) LoginAndGenerateAccessToken(context.Context, string, string, string) (dto.UpstreamIdentity, string, error) {
	return client.self, client.loginToken, nil
}

func (client *testSiteClient) CloseIdleConnections() {}
