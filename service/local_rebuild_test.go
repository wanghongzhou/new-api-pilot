package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestLocalRebuildCompletesAccountCreateRestoreAndCustomerEnable(t *testing.T) {
	t.Run("account_create", func(t *testing.T) {
		database := openSiteTestTransaction(t)
		clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
		cipher := accountTestCipher(t)
		site := createAccountReadySite(t, database, clock.Now().Unix(), cipher)
		customer := model.Customer{
			Name: "Local Rebuild Create", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
			CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
		}
		if err := model.NewCustomerRepository(database).Create(context.Background(), &customer); err != nil {
			t.Fatalf("create customer: %v", err)
		}
		remote := dto.UpstreamUser{
			UpstreamIdentity: dto.UpstreamIdentity{ID: 501, Username: "local-create", Role: 1, Status: 1, Group: "default"},
			CreatedAt:        clock.Now().Unix() - 2*3600,
		}
		client := &accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}
		client.root = remote
		accounts := newIntegrationAccountService(t, database, clock, cipher, client, nil)
		detail, err := accounts.Create(context.Background(), dto.AccountCreateRequest{
			SiteID: strconv.FormatInt(site.ID, 10), CustomerID: strconv.FormatInt(customer.ID, 10),
			RemoteUserID: strconv.FormatInt(remote.ID, 10),
		}, "req_local_create")
		if err != nil {
			t.Fatalf("create account: %v", err)
		}
		accountID, _ := strconv.ParseInt(detail.ID, 10, 64)
		run := loadLocalRebuildRun(t, database, constant.TaskTypeAccountRebuild, accountID)
		seedLocalRebuildFacts(t, database, site.ID, remote.ID, *run.StartTimestamp, *run.EndTimestamp, clock.Now().Unix())
		completeLocalRebuildRun(t, database, clock, run.ID, constant.TaskTypeAccountRebuild)

		var account model.Account
		if err := database.First(&account, accountID).Error; err != nil || account.StatisticsBackfillStatus != "none" {
			t.Fatalf("completed account = %#v, %v", account, err)
		}
		assertLocalRebuildMetrics(t, database, accountID, customer.ID, site.ID, *run.StartTimestamp, 1)
	})

	t.Run("account_restore", func(t *testing.T) {
		database := openSiteTestTransaction(t)
		clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
		cipher := accountTestCipher(t)
		site := createAccountReadySite(t, database, clock.Now().Unix(), cipher)
		customer := model.Customer{
			Name: "Local Rebuild Restore", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
			CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
		}
		if err := model.NewCustomerRepository(database).Create(context.Background(), &customer); err != nil {
			t.Fatalf("create customer: %v", err)
		}
		pauseAt := floorHour(clock.Now().Unix()) - 2*3600
		account := model.Account{
			SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 502, RemoteCreatedAt: pauseAt - 3600,
			Username: "local-restore", RemoteState: model.AccountRemoteStateNormal,
			ManagedStatus: model.AccountManagedStatusArchived, StatisticsPausedAt: &pauseAt,
			StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
		}
		if err := model.NewAccountRepository(database).Create(context.Background(), &account); err != nil {
			t.Fatalf("create archived account: %v", err)
		}
		client := &accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}
		accounts := newIntegrationAccountService(t, database, clock, cipher, client, nil)
		if _, err := accounts.Restore(context.Background(), account.ID, "req_local_restore"); err != nil {
			t.Fatalf("restore account: %v", err)
		}
		run := loadLocalRebuildRun(t, database, constant.TaskTypeAccountRebuild, account.ID)
		seedLocalRebuildFacts(t, database, site.ID, account.RemoteUserID, *run.StartTimestamp, *run.EndTimestamp, clock.Now().Unix())
		completeLocalRebuildRun(t, database, clock, run.ID, constant.TaskTypeAccountRebuild)

		if err := database.First(&account, account.ID).Error; err != nil ||
			account.ManagedStatus != model.AccountManagedStatusActive || account.StatisticsPausedAt != nil ||
			account.StatisticsBackfillStatus != "none" {
			t.Fatalf("restored account = %#v, %v", account, err)
		}
		assertLocalRebuildMetrics(t, database, account.ID, customer.ID, site.ID, *run.StartTimestamp, 1)
	})

	t.Run("customer_enable", func(t *testing.T) {
		database := openSiteTestTransaction(t)
		clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
		cipher := accountTestCipher(t)
		site := createAccountReadySite(t, database, clock.Now().Unix(), cipher)
		pauseAt := floorHour(clock.Now().Unix()) - 2*3600
		customer := model.Customer{
			Name: "Local Rebuild Enable", Status: dto.CustomerStatusDisabled, StatisticsPausedAt: &pauseAt,
			StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
		}
		if err := model.NewCustomerRepository(database).Create(context.Background(), &customer); err != nil {
			t.Fatalf("create disabled customer: %v", err)
		}
		account := model.Account{
			SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 503, RemoteCreatedAt: pauseAt - 3600,
			Username: "local-enable", RemoteState: model.AccountRemoteStateNormal,
			ManagedStatus: model.AccountManagedStatusActive, StatisticsPausedAt: &pauseAt,
			StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
		}
		if err := model.NewAccountRepository(database).Create(context.Background(), &account); err != nil {
			t.Fatalf("create paused account: %v", err)
		}
		customers, err := NewCustomerService(CustomerServiceOptions{Database: database, Clock: clock})
		if err != nil {
			t.Fatalf("create customer service: %v", err)
		}
		if _, err := customers.Enable(context.Background(), customer.ID, "req_local_enable"); err != nil {
			t.Fatalf("enable customer: %v", err)
		}
		run := loadLocalRebuildRun(t, database, constant.TaskTypeCustomerRebuild, customer.ID)
		seedLocalRebuildFacts(t, database, site.ID, account.RemoteUserID, *run.StartTimestamp, *run.EndTimestamp, clock.Now().Unix())
		completeLocalRebuildRun(t, database, clock, run.ID, constant.TaskTypeCustomerRebuild)

		if err := database.First(&customer, customer.ID).Error; err != nil ||
			customer.Status != dto.CustomerStatusUsing || customer.StatisticsPausedAt != nil ||
			customer.StatisticsBackfillStatus != "none" {
			t.Fatalf("enabled customer = %#v, %v", customer, err)
		}
		if err := database.First(&account, account.ID).Error; err != nil || account.StatisticsPausedAt != nil {
			t.Fatalf("enabled customer account = %#v, %v", account, err)
		}
		assertLocalRebuildMetrics(t, database, account.ID, customer.ID, site.ID, *run.StartTimestamp, 1)
	})
}

func TestLocalRebuildQueuesMissingUsageDependency(t *testing.T) {
	database := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, database, clock.Now().Unix(), cipher)
	customer := model.Customer{
		Name: "Local Rebuild Dependency", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(database).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 504,
		RemoteCreatedAt: clock.Now().Unix() - 3600, Username: "local-dependency",
		RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "pending", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewAccountRepository(database).Create(context.Background(), &account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	start, end := floorHour(account.RemoteCreatedAt), floorHour(clock.Now().Unix())
	run, err := newLocalRebuildRun("account", account.ID, constant.TaskTypeAccountRebuild,
		start, end, "req_local_dependency", clock.Now().Unix())
	if err != nil {
		t.Fatalf("build local run: %v", err)
	}
	if _, _, err := model.NewSiteRepository(database).CreateOrGetRun(context.Background(), &run); err != nil {
		t.Fatalf("create local run: %v", err)
	}
	repository := model.NewCollectionTaskRepository(database)
	if _, err := repository.MaterializeRunWindows(context.Background(), run.ID, clock.Now().Unix(), 1000); err != nil {
		t.Fatalf("materialize local run: %v", err)
	}
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{constant.TaskTypeAccountRebuild}, Now: clock.Now().Unix(),
		RequestID: "wrk_local_dependency", MaxWindow: 24, ScanLimit: 64,
	})
	if err != nil || len(claim.Windows) != 1 {
		t.Fatalf("claim local dependency = %#v, %v", claim, err)
	}
	local, err := NewLocalRebuildService(LocalRebuildServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create local rebuild service: %v", err)
	}
	_, err = local.PrepareWindow(context.Background(), LocalRebuildRequest{
		Run: claim.Run, Window: claim.Windows[0], RequestID: claim.RequestID,
	})
	if !errors.Is(err, model.ErrLocalRebuildDependencyPending) {
		t.Fatalf("prepare missing local window error = %v", err)
	}
	var dependency model.CollectionRun
	if err := database.Where("site_id = ? AND task_type = ? AND trigger_type = ?", site.ID,
		constant.TaskTypeUsageBackfill, constant.CollectionTriggerDependency).Take(&dependency).Error; err != nil {
		t.Fatalf("read usage dependency: %v", err)
	}
	if dependency.StartTimestamp == nil || *dependency.StartTimestamp != start ||
		dependency.EndTimestamp == nil || *dependency.EndTimestamp != end {
		t.Fatalf("usage dependency = %#v", dependency)
	}
}

func completeLocalRebuildRun(
	t *testing.T,
	database *gorm.DB,
	clock *testsupport.FakeClock,
	runID int64,
	taskType string,
) model.CollectionRun {
	t.Helper()
	repository := model.NewCollectionTaskRepository(database)
	materialized, err := repository.MaterializeRunWindows(context.Background(), runID, clock.Now().Unix(), 1000)
	if err != nil {
		t.Fatalf("materialize local rebuild: %v", err)
	}
	if materialized.Status == model.CollectionTaskStatusSuccess {
		return materialized
	}
	local, err := NewLocalRebuildService(LocalRebuildServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create local rebuild service: %v", err)
	}
	claim, err := repository.ClaimNext(context.Background(), model.CollectionTaskClaimOptions{
		TaskTypes: []string{taskType}, Now: clock.Now().Unix(),
		RequestID: fmt.Sprintf("wrk_local_%d", runID), MaxWindow: 24, ScanLimit: 64,
	})
	if err != nil {
		t.Fatalf("claim local rebuild: %v", err)
	}
	for _, window := range claim.Windows {
		mutation, err := local.PrepareWindow(context.Background(), LocalRebuildRequest{
			Run: claim.Run, Window: window, RequestID: claim.RequestID,
		})
		if err != nil {
			t.Fatalf("prepare local window %d: %v", window.ID, err)
		}
		materialized, err = repository.CompleteClaimedWindow(context.Background(), model.CompleteClaimedWindowRequest{
			RunID: claim.Run.ID, RequestID: claim.RequestID, Now: clock.Now().Unix(),
			Window: model.CollectionTaskWindowResult{
				WindowID: window.ID, AttemptCount: window.AttemptCount, Status: model.CollectionTaskStatusSuccess,
			},
			Mutation: mutation,
		})
		if err != nil {
			t.Fatalf("complete local window %d: %v", window.ID, err)
		}
	}
	if materialized.Status != model.CollectionTaskStatusSuccess {
		t.Fatalf("completed local run = %#v", materialized)
	}
	return materialized
}

func seedLocalRebuildFacts(t *testing.T, database *gorm.DB, siteID, remoteUserID, start, end, now int64) {
	t.Helper()
	for hour := start; hour < end; hour += 3600 {
		if err := database.Create(&model.CollectionWindow{
			SiteID: siteID, HourTS: hour, Status: model.CollectionWindowStatusComplete,
			FetchedRows: 1, UpdatedAt: now,
		}).Error; err != nil {
			t.Fatalf("create complete fact window: %v", err)
		}
		if err := database.Create(&model.UsageFactHourly{
			SiteID: siteID, RemoteUserID: remoteUserID, UsernameSnapshot: "local",
			ModelName: "local-model", ChannelID: 1, HourTS: hour,
			RequestCount: 1, Quota: 2, TokenUsed: 3, CollectedAt: now,
		}).Error; err != nil {
			t.Fatalf("create local fact: %v", err)
		}
	}
}

func loadLocalRebuildRun(t *testing.T, database *gorm.DB, taskType string, targetID int64) model.CollectionRun {
	t.Helper()
	var run model.CollectionRun
	if err := database.Where("task_type = ? AND target_id = ?", taskType, targetID).
		Order("id DESC").Take(&run).Error; err != nil {
		t.Fatalf("load local rebuild run: %v", err)
	}
	return run
}

func assertLocalRebuildMetrics(
	t *testing.T,
	database *gorm.DB,
	accountID, customerID, siteID, hour int64,
	wantRequests int64,
) {
	t.Helper()
	var account model.AccountStatHourly
	if err := database.Where("account_id = ? AND hour_ts = ?", accountID, hour).Take(&account).Error; err != nil ||
		account.RequestCount != wantRequests {
		t.Fatalf("account local metric = %#v, %v", account, err)
	}
	var customer model.CustomerStatHourly
	if err := database.Where("customer_id = ? AND site_id = ? AND hour_ts = ?", customerID, siteID, hour).
		Take(&customer).Error; err != nil || customer.RequestCount != wantRequests {
		t.Fatalf("customer local metric = %#v, %v", customer, err)
	}
}
