package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type recordingSitePostCommit struct{ triggers []AlertPostCommitTrigger }

func (recorder *recordingSitePostCommit) NotifyAfterCommit(_ context.Context, trigger AlertPostCommitTrigger) {
	recorder.triggers = append(recorder.triggers, trigger)
}

func TestValidatePeriodicCommitRejectsChangedSnapshotAndLifecycle(t *testing.T) {
	rootID := int64(42)
	token := "encrypted-token"
	original := model.Site{
		ID:                   1,
		BaseURL:              "https://periodic.example.test",
		ConfigVersion:        7,
		ManagementStatus:     constant.SiteManagementActive,
		AuthStatus:           constant.SiteAuthAuthorized,
		RootUserID:           &rootID,
		AccessTokenEncrypted: &token,
	}
	matchingRootID := rootID
	matchingToken := token
	matching := original
	matching.RootUserID = &matchingRootID
	matching.AccessTokenEncrypted = &matchingToken
	if err := validatePeriodicCommit(matching, original, original.ConfigVersion); err != nil {
		t.Fatalf("matching snapshot rejected: %v", err)
	}

	endedAt := int64(1_752_400_800)
	otherRootID := rootID + 1
	otherToken := token + "-rotated"
	tests := []struct {
		name     string
		expected int
		mutate   func(*model.Site, *model.Site)
	}{
		{name: "expected config version", expected: original.ConfigVersion + 1},
		{name: "original config version", expected: original.ConfigVersion, mutate: func(_ *model.Site, snapshot *model.Site) { snapshot.ConfigVersion++ }},
		{name: "current config version", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.ConfigVersion++ }},
		{name: "base url", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.BaseURL = "https://changed.example.test" }},
		{name: "management disabled", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.ManagementStatus = constant.SiteManagementDisabled }},
		{name: "authorization expired", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.AuthStatus = constant.SiteAuthExpired }},
		{name: "statistics ended", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.StatisticsEndAt = &endedAt }},
		{name: "root user", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.RootUserID = &otherRootID }},
		{name: "access token", expected: original.ConfigVersion, mutate: func(current, _ *model.Site) { current.AccessTokenEncrypted = &otherToken }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := original
			snapshot := original
			if test.mutate != nil {
				test.mutate(&current, &snapshot)
			}
			if err := validatePeriodicCommit(current, snapshot, test.expected); !errors.Is(err, model.ErrSiteRunConfigChanged) {
				t.Fatalf("validatePeriodicCommit() error = %v, want %v", err, model.ErrSiteRunConfigChanged)
			}
		})
	}
}

func TestValidatePeriodicProbeSnapshotRejectsDisabledOrEndedSite(t *testing.T) {
	site := model.Site{ID: 1, ConfigVersion: 3, ManagementStatus: constant.SiteManagementActive}
	if err := validatePeriodicProbeSnapshot(site, site.ConfigVersion); err != nil {
		t.Fatalf("active site rejected: %v", err)
	}

	endedAt := int64(1_752_400_800)
	tests := []struct {
		name     string
		expected int
		mutate   func(*model.Site)
	}{
		{name: "config version", expected: site.ConfigVersion + 1},
		{name: "management disabled", expected: site.ConfigVersion, mutate: func(site *model.Site) { site.ManagementStatus = constant.SiteManagementDisabled }},
		{name: "statistics ended", expected: site.ConfigVersion, mutate: func(site *model.Site) { site.StatisticsEndAt = &endedAt }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := site
			if test.mutate != nil {
				test.mutate(&current)
			}
			if err := validatePeriodicProbeSnapshot(current, test.expected); !errors.Is(err, model.ErrSiteRunConfigChanged) {
				t.Fatalf("validatePeriodicProbeSnapshot() error = %v, want %v", err, model.ErrSiteRunConfigChanged)
			}
		})
	}
}

func TestPeriodicProbeRejectsStaleSiteSnapshotWithoutWritingProbeState(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})

	tests := []struct {
		name   string
		mutate func(*model.Site)
	}{
		{name: "config version", mutate: func(site *model.Site) { site.ConfigVersion++ }},
		{name: "base url", mutate: func(site *model.Site) { site.BaseURL += "/changed" }},
		{name: "root user", mutate: func(site *model.Site) { value := *site.RootUserID + 1; site.RootUserID = &value }},
		{name: "access token", mutate: func(site *model.Site) {
			value := *site.AccessTokenEncrypted + "-rotated"
			site.AccessTokenEncrypted = &value
		}},
		{name: "management disabled", mutate: func(site *model.Site) { site.ManagementStatus = constant.SiteManagementDisabled }},
		{name: "statistics ended", mutate: func(site *model.Site) { value := clock.Now().Unix(); site.StatisticsEndAt = &value }},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rootID := int64(100 + index)
			token := "encrypted-probe-token"
			site := newTestSite(clock.Now().Unix(), "https://periodic-probe-fence-"+test.name+".example.test")
			site.ManagementStatus = constant.SiteManagementActive
			site.RootUserID = &rootID
			site.AccessTokenEncrypted = &token
			if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
				t.Fatalf("create probe site: %v", err)
			}
			original := site
			changed := site
			test.mutate(&changed)
			if err := model.NewSiteRepository(tx).Save(context.Background(), &changed); err != nil {
				t.Fatalf("mutate probe site: %v", err)
			}
			if _, err := sites.probeWithSnapshot(context.Background(), original, original.ConfigVersion,
				"req_periodic_probe_fence", true); !errors.Is(err, model.ErrSiteRunConfigChanged) {
				t.Fatalf("probeWithSnapshot() error = %v, want %v", err, model.ErrSiteRunConfigChanged)
			}
			persisted, err := model.NewSiteRepository(tx).FindByID(context.Background(), site.ID)
			if err != nil {
				t.Fatalf("read probe site: %v", err)
			}
			if persisted.LastProbeAt != nil || persisted.LastProbeSuccessAt != nil || persisted.ProbeFailCount != 0 || persisted.Version != "" {
				t.Fatalf("stale probe response wrote state: %#v", persisted)
			}
		})
	}
}

func TestPeriodicSiteTasksCommitMetadataBehindConfigFence(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	now := clock.Now().Unix()
	client := authorizedTestSiteClient(now)
	client.channels = dto.UpstreamChannelSnapshot{Total: 1, Items: []dto.UpstreamChannel{{ID: 7, Name: "periodic-channel", Status: 1}}}
	isMaster := true
	cpu, memory, disk := 25.0, 40.0, 55.0
	diskTotal, diskUsed := int64(1_000), int64(550)
	client.instances = []dto.UpstreamInstance{{
		NodeName: "periodic-node", Status: "online", StaleAfterSeconds: 90,
		StartedAt: now - 3600, LastSeenAt: now, IsMaster: &isMaster,
		RuntimeVersion: "go1.25", GOOS: "linux", GOARCH: "amd64", Hostname: "periodic-host",
		CPUPercent: &cpu, MemoryPercent: &memory, StorageUsedPercent: &disk,
		StorageTotalBytes: &diskTotal, StorageUsedBytes: &diskUsed,
	}}
	client.realtime = dto.UpstreamLogStat{RPM: 12, TPM: 34}
	client.performance = dto.UpstreamPerformanceHistory{Models: []dto.UpstreamPerformanceModelHistory{{
		ModelName: "gpt-periodic", SeriesSchema: "official_average_v1",
		Groups: []dto.UpstreamPerformanceGroupHistory{{Group: "default", Series: []dto.UpstreamPerformanceBucket{{
			Timestamp: now - 3600, AvgTTFTMS: "50.0000000000", AvgLatencyMS: "100.0000000000",
			SuccessRate: "1.0000000000", AvgTPS: "25.0000000000",
		}}}},
	}}}
	client.topups = dto.UpstreamTopupSnapshot{Total: 1, MaxID: 11, Items: []dto.UpstreamTopup{{ID: 11, UserID: 1, Amount: 100, Money: "10.25", PaymentMethod: "stripe", PaymentProvider: "stripe", CreateTime: now - 10, Status: "success"}}}
	client.redemptions = dto.UpstreamRedemptionSnapshot{Total: 1, MaxID: 12, Items: []dto.UpstreamRedemption{{ID: 12, Name: "periodic", Status: 1, Quota: 50, CreatedTime: now - 10, ExpiredTime: now + 3600}}}
	client.upstreamTasks = dto.UpstreamTaskSnapshot{Items: []dto.UpstreamTask{{ID: 13, CreatedAt: now - 20, UpdatedAt: now - 10, TaskID: "task_periodic", Platform: "video", UserID: 1, Group: "default", ChannelID: 7, Quota: 50, Action: "generate", Status: "IN_PROGRESS", SubmitTime: now - 20, StartTime: now - 15, Progress: "50%", Properties: dto.UpstreamTaskProperties{Model: "video-model"}}}}
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	postCommit := &recordingSitePostCommit{}
	sites.postCommit = postCommit
	site := newTestSite(now, "https://periodic.example.test")
	site.ManagementStatus = constant.SiteManagementActive
	site.OnlineStatus = constant.SiteOnlineOnline
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	site.DataExportEnabled = true
	rootID := client.root.ID
	rootCreatedAt := client.root.CreatedAt
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreatedAt
	statisticsStart := floorHour(rootCreatedAt)
	site.StatisticsStartAt = &statisticsStart
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create periodic site: %v", err)
	}
	encrypted, err := sites.cipher.Encrypt([]byte("periodic-token"), siteTokenAAD(site.ID))
	if err != nil {
		t.Fatalf("encrypt periodic token: %v", err)
	}
	site.AccessTokenEncrypted = &encrypted
	if err := tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", encrypted).Error; err != nil {
		t.Fatalf("store periodic token: %v", err)
	}
	customer := model.Customer{Name: "Periodic Customer", Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&customer).Error; err != nil {
		t.Fatalf("create periodic customer: %v", err)
	}
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: client.root.ID, RemoteCreatedAt: client.root.CreatedAt,
		Username: "old-root", RemoteState: model.AccountRemoteStateMissing, RemoteMissingCount: 2,
		ManagedStatus: model.AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("create periodic account: %v", err)
	}

	for _, taskType := range []string{
		constant.TaskTypeSiteProbe,
		constant.TaskTypeRealtimeStat,
		constant.TaskTypeChannelSync,
		constant.TaskTypeUserSync,
		constant.TaskTypeResourceSnapshot,
		constant.TaskTypePerformanceSync,
		constant.TaskTypeTopupSync,
		constant.TaskTypeRedemptionSync,
		constant.TaskTypeUpstreamTaskSync,
	} {
		if _, _, err := sites.ExecutePeriodicSiteTask(context.Background(), taskType, site.ID, site.ConfigVersion, "req_periodic_"+taskType); err != nil {
			t.Fatalf("execute %s: %v", taskType, err)
		}
	}
	channelTriggers := make([]AlertPostCommitTrigger, 0, 1)
	for _, trigger := range postCommit.triggers {
		if trigger.Source == AlertSampleSourceChannel {
			channelTriggers = append(channelTriggers, trigger)
		}
	}
	if len(channelTriggers) != 1 || channelTriggers[0].SiteID != site.ID || channelTriggers[0].HourTS != floorHour(channelTriggers[0].ObservedAt) {
		t.Fatalf("channel post-commit triggers = %#v", channelTriggers)
	}
	var current model.Site
	if err := tx.First(&current, site.ID).Error; err != nil || current.CurrentRPM != 12 || current.CurrentTPM != 34 ||
		current.LastRealtimeStatAt == nil || current.MonitoringStartAt == nil {
		t.Fatalf("periodic site state = %#v, %v", current, err)
	}
	var channel model.SiteChannel
	if err := tx.Where("site_id = ? AND remote_channel_id = 7", site.ID).First(&channel).Error; err != nil || channel.Name != "periodic-channel" {
		t.Fatalf("periodic channel = %#v, %v", channel, err)
	}
	var instance model.SiteInstance
	if err := tx.Where("site_id = ? AND node_name = ?", site.ID, "periodic-node").First(&instance).Error; err != nil ||
		instance.CurrentStatus != "online" {
		t.Fatalf("periodic instance = %#v, %v", instance, err)
	}
	var siteMinute model.SiteStatusMinutely
	if err := tx.Where("site_id = ? AND minute_ts = ?", site.ID, floorMinute(now)).First(&siteMinute).Error; err != nil ||
		siteMinute.InstanceCount != 1 || siteMinute.OnlineInstanceCount != 1 {
		t.Fatalf("periodic site minute = %#v, %v", siteMinute, err)
	}
	var performance model.SitePerformanceMetricBucket
	if err := tx.Where("site_id = ? AND model_name = ? AND bucket_ts = ?", site.ID, "gpt-periodic", now-3600).First(&performance).Error; err != nil ||
		performance.MetricSource != model.PerformanceMetricSourceOfficialAverage || performance.RequestCount != nil {
		t.Fatalf("periodic performance = %#v, %v", performance, err)
	}
	var topup model.SiteTopupOrder
	if err := tx.Where("site_id=? AND remote_id=11", site.ID).Take(&topup).Error; err != nil || topup.Money != "10.2500000000" {
		t.Fatalf("periodic topup=%#v err=%v", topup, err)
	}
	var redemption model.SiteRedemption
	if err := tx.Where("site_id=? AND remote_id=12", site.ID).Take(&redemption).Error; err != nil || redemption.Name != "periodic" {
		t.Fatalf("periodic redemption=%#v err=%v", redemption, err)
	}
	var upstreamTask model.SiteUpstreamTask
	if err := tx.Where("site_id=? AND remote_id=13", site.ID).Take(&upstreamTask).Error; err != nil || upstreamTask.RemoteStatus != "IN_PROGRESS" {
		t.Fatalf("periodic upstream task=%#v err=%v", upstreamTask, err)
	}
	var synced model.Account
	if err := tx.First(&synced, account.ID).Error; err != nil || synced.RemoteState != model.AccountRemoteStateNormal ||
		synced.RemoteMissingCount != 0 || synced.Username != client.root.Username {
		t.Fatalf("periodic account = %#v, %v", synced, err)
	}
	for _, taskType := range []string{constant.TaskTypeRealtimeStat, constant.TaskTypeTopupSync, constant.TaskTypeRedemptionSync, constant.TaskTypeUpstreamTaskSync} {
		if _, _, err := sites.ExecutePeriodicSiteTask(context.Background(), taskType, site.ID, site.ConfigVersion+1, "req_periodic_stale_"+taskType); !errors.Is(err, model.ErrSiteRunConfigChanged) {
			t.Fatalf("stale %s fence error = %v", taskType, err)
		}
	}
}

func TestPeriodicAuthenticatedTasksExpireAuthorizationForUnauthorizedAndForbidden(t *testing.T) {
	testCases := []struct {
		name     string
		taskType string
		cause    error
		setError func(*testSiteClient, error)
	}{
		{name: "realtime_unauthorized", taskType: constant.TaskTypeRealtimeStat, cause: ErrUpstreamAuthExpired,
			setError: func(client *testSiteClient, err error) { client.realtimeErr = err }},
		{name: "resource_forbidden", taskType: constant.TaskTypeResourceSnapshot, cause: ErrUpstreamPermissionDenied,
			setError: func(client *testSiteClient, err error) { client.instancesErr = err }},
		{name: "user_unauthorized", taskType: constant.TaskTypeUserSync, cause: ErrUpstreamAuthExpired,
			setError: func(client *testSiteClient, err error) { client.snapshotErr = err }},
		{name: "channel_forbidden", taskType: constant.TaskTypeChannelSync, cause: ErrUpstreamPermissionDenied,
			setError: func(client *testSiteClient, err error) { client.channelsErr = err }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tx := openSiteTestTransaction(t)
			clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
			client := authorizedTestSiteClient(clock.Now().Unix())
			testCase.setError(client, testCase.cause)
			sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
			site := newTestSite(clock.Now().Unix(), "https://auth-failure-"+testCase.name+".example.test")
			site.AuthStatus = constant.SiteAuthAuthorized
			site.StatisticsStatus = constant.SiteStatisticsReady
			site.DataExportEnabled = true
			rootID := client.root.ID
			rootCreatedAt := client.root.CreatedAt
			statisticsStart := floorHour(rootCreatedAt)
			site.RootUserID = &rootID
			site.RootCreatedAt = &rootCreatedAt
			site.StatisticsStartAt = &statisticsStart
			if err := tx.Create(&site).Error; err != nil {
				t.Fatalf("create site: %v", err)
			}
			encrypted, err := sites.cipher.Encrypt([]byte("periodic-token"), siteTokenAAD(site.ID))
			if err != nil {
				t.Fatalf("encrypt token: %v", err)
			}
			if err := tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", encrypted).Error; err != nil {
				t.Fatalf("store token: %v", err)
			}

			if _, _, err := sites.ExecutePeriodicSiteTask(
				context.Background(), testCase.taskType, site.ID, site.ConfigVersion, "req_auth_failure",
			); !errors.Is(err, testCase.cause) {
				t.Fatalf("periodic auth failure = %v, want %v", err, testCase.cause)
			}
			persisted, err := sites.sites.FindByID(context.Background(), site.ID)
			if err != nil || persisted.AuthStatus != constant.SiteAuthExpired ||
				persisted.ConfigVersion != site.ConfigVersion+1 || persisted.StatisticsStatus != constant.SiteStatisticsError {
				t.Fatalf("expired site = %#v, %v", persisted, err)
			}
		})
	}
}

func TestPeriodicCredentialDecryptFailureExpiresAuthorization(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{authenticated: client, public: client})
	site := newTestSite(clock.Now().Unix(), "https://decrypt-failure.example.test")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	rootID := client.root.ID
	rootCreatedAt := client.root.CreatedAt
	statisticsStart := floorHour(rootCreatedAt)
	invalidCiphertext := "invalid-ciphertext"
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreatedAt
	site.StatisticsStartAt = &statisticsStart
	site.AccessTokenEncrypted = &invalidCiphertext
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, _, err := sites.ExecutePeriodicSiteTask(
		context.Background(), constant.TaskTypeRealtimeStat, site.ID, site.ConfigVersion, "req_decrypt_failure",
	); !errors.Is(err, ErrUpstreamAuthExpired) {
		t.Fatalf("decrypt auth failure = %v", err)
	}
	persisted, err := sites.sites.FindByID(context.Background(), site.ID)
	if err != nil || persisted.AuthStatus != constant.SiteAuthExpired || persisted.ConfigVersion != site.ConfigVersion+1 {
		t.Fatalf("expired decrypt site = %#v, %v", persisted, err)
	}
}

func TestConcurrentAuthorizationFailuresBumpFenceExactlyOnce(t *testing.T) {
	database := openAlertConcurrentDatabase(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	site := newTestSite(clock.Now().Unix(), "https://concurrent-auth-expiry.example.test")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	repository := model.NewSiteRepository(database.GORM)
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create concurrent expiration site: %v", err)
	}
	t.Cleanup(func() { _ = database.GORM.Delete(&model.Site{}, site.ID).Error })

	const workers = 8
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsByWorker <- expireSiteAuthorization(
				context.Background(), repository, clock, nil, site.ID, site.ConfigVersion,
			)
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent expiration error: %v", err)
		}
	}
	persisted, err := repository.FindByID(context.Background(), site.ID)
	if err != nil || persisted.ConfigVersion != site.ConfigVersion+1 || persisted.AuthStatus != constant.SiteAuthExpired {
		t.Fatalf("concurrent expired site = %#v, %v", persisted, err)
	}
}

func TestPeriodicResourceSnapshotRetainsKnownNodesAndRequiresThreeNonOnline(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	client := authorizedTestSiteClient(clock.Now().Unix())
	factory := &testSiteClientFactory{authenticated: client, public: client}
	sites := newIntegrationSiteService(t, tx, clock, factory)
	site := newTestSite(clock.Now().Unix(), "https://periodic-resource.example.test")
	site.ManagementStatus = constant.SiteManagementActive
	site.OnlineStatus = constant.SiteOnlineOnline
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	rootID := client.root.ID
	rootCreatedAt := client.root.CreatedAt
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreatedAt
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create periodic resource site: %v", err)
	}
	encrypted, err := sites.cipher.Encrypt([]byte("periodic-resource-token"), siteTokenAAD(site.ID))
	if err != nil {
		t.Fatalf("encrypt periodic resource token: %v", err)
	}
	if err := tx.Model(&model.Site{}).Where("id = ?", site.ID).Update("access_token_encrypted", encrypted).Error; err != nil {
		t.Fatalf("store periodic resource token: %v", err)
	}

	cpu, memory, disk := 25.0, 40.0, 55.0
	setOnlineSnapshot := func() {
		now := clock.Now().Unix()
		client.instances = []dto.UpstreamInstance{{
			NodeName: "retained-node", Status: "online", StaleAfterSeconds: 90,
			StartedAt: now - 3600, LastSeenAt: now, Hostname: "retained-host",
			CPUPercent: &cpu, MemoryPercent: &memory, StorageUsedPercent: &disk,
		}}
	}
	execute := func(requestID string) (int64, int64, error) {
		return sites.ExecutePeriodicSiteTask(
			context.Background(), constant.TaskTypeResourceSnapshot, site.ID, site.ConfigVersion, requestID,
		)
	}
	assertStatus := func(minute int64, wantStatus string, wantSiteCount int) {
		t.Helper()
		var instance model.SiteInstance
		if err := tx.Where("site_id = ? AND node_name = ?", site.ID, "retained-node").First(&instance).Error; err != nil ||
			instance.CurrentStatus != wantStatus {
			t.Fatalf("retained instance at %d = %#v, %v", minute, instance, err)
		}
		var sample model.SiteInstanceStatusMinutely
		if err := tx.Where("site_id = ? AND node_name = ? AND minute_ts = ?", site.ID, "retained-node", minute).
			First(&sample).Error; err != nil || sample.Status != wantStatus {
			t.Fatalf("retained sample at %d = %#v, %v", minute, sample, err)
		}
		var siteSample model.SiteStatusMinutely
		if err := tx.Where("site_id = ? AND minute_ts = ?", site.ID, minute).First(&siteSample).Error; err != nil ||
			siteSample.InstanceCount != wantSiteCount {
			t.Fatalf("site resource sample at %d = %#v, %v", minute, siteSample, err)
		}
		if wantSiteCount == 0 && (sample.CPUPercent != nil || sample.MemoryPercent != nil || sample.DiskUsedPercent != nil ||
			sample.DiskTotalBytes != nil || sample.DiskUsedBytes != nil) {
			t.Fatalf("missing retained node at %d kept resource metrics: %#v", minute, sample)
		}
	}
	assertRetired := func(want bool) {
		t.Helper()
		var instance model.SiteInstance
		if err := tx.Where("site_id = ? AND node_name = ?", site.ID, "retained-node").First(&instance).Error; err != nil {
			t.Fatalf("load retained instance retirement state: %v", err)
		}
		if (instance.RetiredAt != nil) != want {
			t.Fatalf("retained instance retired=%t, want %t: %#v", instance.RetiredAt != nil, want, instance)
		}
	}

	setOnlineSnapshot()
	if _, _, err := execute("req_resource_online_initial"); err != nil {
		t.Fatalf("initial resource snapshot: %v", err)
	}
	assertStatus(floorMinute(clock.Now().Unix()), "online", 1)

	client.instances = []dto.UpstreamInstance{}
	clock.Advance(time.Minute)
	firstMissingMinute := floorMinute(clock.Now().Unix())
	if fetched, written, err := execute("req_resource_missing_1"); err != nil || fetched != 0 || written != 1 {
		t.Fatalf("first missing snapshot = fetched:%d written:%d err:%v", fetched, written, err)
	}
	assertRetired(true)
	var retiredSampleCount int64
	if err := tx.Model(&model.SiteInstanceStatusMinutely{}).
		Where("site_id = ? AND node_name = ? AND minute_ts = ?", site.ID, "retained-node", firstMissingMinute).
		Count(&retiredSampleCount).Error; err != nil || retiredSampleCount != 0 {
		t.Fatalf("retired node wrote minute samples = %d, %v", retiredSampleCount, err)
	}
	var retiredSiteSample model.SiteStatusMinutely
	if err := tx.Where("site_id = ? AND minute_ts = ?", site.ID, firstMissingMinute).First(&retiredSiteSample).Error; err != nil ||
		retiredSiteSample.InstanceCount != 0 {
		t.Fatalf("retired site sample = %#v, %v", retiredSiteSample, err)
	}

	clock.Advance(time.Minute)
	failedMinute := floorMinute(clock.Now().Unix())
	instanceFailure := errors.New("instances unavailable")
	sites.clients = &testSiteClientFactory{
		authenticated: &failingInstancesSiteClient{testSiteClient: client, err: instanceFailure},
		public:        client,
	}
	if _, _, err := execute("req_resource_failed"); !errors.Is(err, instanceFailure) {
		t.Fatalf("failed resource snapshot error = %v", err)
	}
	for _, table := range []any{&model.SiteInstanceStatusMinutely{}, &model.SiteStatusMinutely{}} {
		var count int64
		if err := tx.Model(table).Where("site_id = ? AND minute_ts = ?", site.ID, failedMinute).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed resource snapshot wrote %T rows = %d, %v", table, count, err)
		}
	}
	sites.clients = factory

	setNonOnlineSnapshot := func() {
		now := clock.Now().Unix()
		client.instances = []dto.UpstreamInstance{{
			NodeName: "retained-node", Status: "stale", StaleAfterSeconds: 90,
			StartedAt: now - 3600, LastSeenAt: now - 180, Hostname: "retained-host",
		}}
	}
	for attempt := 1; attempt <= 3; attempt++ {
		clock.Advance(time.Minute)
		setNonOnlineSnapshot()
		minute := floorMinute(clock.Now().Unix())
		if fetched, written, err := execute("req_resource_non_online_" + strconv.Itoa(attempt)); err != nil || fetched != 1 || written != 2 {
			t.Fatalf("non-online snapshot %d = fetched:%d written:%d err:%v", attempt, fetched, written, err)
		}
		wantStatus := "stale"
		if attempt == 3 {
			wantStatus = "offline"
		}
		assertStatus(minute, wantStatus, 1)
		assertRetired(false)
	}

	clock.Advance(time.Minute)
	setOnlineSnapshot()
	if _, _, err := execute("req_resource_online_reset"); err != nil {
		t.Fatalf("online reset snapshot: %v", err)
	}
	assertStatus(floorMinute(clock.Now().Unix()), "online", 1)

	client.instances = []dto.UpstreamInstance{}
	clock.Advance(time.Minute)
	missingAfterResetMinute := floorMinute(clock.Now().Unix())
	if fetched, written, err := execute("req_resource_missing_after_reset"); err != nil || fetched != 0 || written != 1 {
		t.Fatalf("missing snapshot after online reset = fetched:%d written:%d err:%v", fetched, written, err)
	}
	assertRetired(true)
	var missingAfterResetSamples int64
	if err := tx.Model(&model.SiteInstanceStatusMinutely{}).
		Where("site_id = ? AND node_name = ? AND minute_ts = ?", site.ID, "retained-node", missingAfterResetMinute).
		Count(&missingAfterResetSamples).Error; err != nil || missingAfterResetSamples != 0 {
		t.Fatalf("retired node after reset wrote minute samples = %d, %v", missingAfterResetSamples, err)
	}
}

type failingInstancesSiteClient struct {
	*testSiteClient
	err error
}

func (client *failingInstancesSiteClient) Instances(context.Context, string) ([]dto.UpstreamInstance, error) {
	return nil, client.err
}
