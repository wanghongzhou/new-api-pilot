package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"new-api-pilot/constant"

	"gorm.io/gorm"
)

func TestCustomerRepositoryCRUDFilterLockAndDeleteDependencies(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_400_800)
	repository := NewCustomerRepository(database.GORM)

	site := Site{
		Name: "B4 Site " + suffix, BaseURL: "https://b4-customer-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	customer := Customer{
		Name: "B4 Customer " + suffix, Contact: "ops@example.test", Status: "using",
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.Create(ctx, &customer); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	literal := Customer{
		Name: "B4 Literal %_\\ " + suffix, Contact: "literal%_\\@example.test", Status: "communicating",
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now + 1,
	}
	if err := repository.Create(ctx, &literal); err != nil {
		t.Fatalf("create literal customer: %v", err)
	}
	decoy := Customer{
		Name: "B4 Literal xyz " + suffix, Contact: "literalXYZ@example.test", Status: "signing",
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now + 2,
	}
	if err := repository.Create(ctx, &decoy); err != nil {
		t.Fatalf("create decoy customer: %v", err)
	}
	var enableRunID int64
	t.Cleanup(func() {
		if enableRunID > 0 {
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id = ?", enableRunID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE id = ?", enableRunID)
		}
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE site_id = ?", site.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer_stat_hourly WHERE customer_id IN (?, ?, ?)", customer.ID, literal.ID, decoy.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer_stat_daily WHERE customer_id IN (?, ?, ?)", customer.ID, literal.ID, decoy.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id IN (?, ?, ?)", customer.ID, literal.ID, decoy.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})

	loaded, err := repository.FindByID(ctx, customer.ID)
	if err != nil || loaded.Name != customer.Name {
		t.Fatalf("find customer = %#v, %v", loaded, err)
	}
	if err := repository.WithTransaction(ctx, func(transaction *CustomerRepository) error {
		locked, lockErr := transaction.FindByIDForUpdate(ctx, customer.ID)
		if lockErr == nil && locked.ID != customer.ID {
			return fmt.Errorf("locked wrong customer %d", locked.ID)
		}
		return lockErr
	}); err != nil {
		t.Fatalf("lock customer: %v", err)
	}
	loaded.Name = "B4 Updated " + suffix
	loaded.Contact = "updated@example.test"
	loaded.UpdatedAt = now + 3
	if err := repository.UpdateProfile(ctx, &loaded); err != nil {
		t.Fatalf("update customer profile: %v", err)
	}
	loaded, err = repository.FindByID(ctx, customer.ID)
	if err != nil || loaded.Name != "B4 Updated "+suffix || loaded.CreatedAt != now {
		t.Fatalf("saved customer = %#v, %v", loaded, err)
	}

	items, total, err := repository.List(ctx, CustomerFilter{
		Keyword: "%_\\", SortBy: "updated_at", SortOrder: "desc", Limit: 20,
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != literal.ID {
		t.Fatalf("literal LIKE list total=%d items=%#v err=%v", total, items, err)
	}
	items, total, err = repository.List(ctx, CustomerFilter{
		Statuses: []string{"communicating", "signing"}, SortBy: "name", SortOrder: "asc", Limit: 20,
	})
	if err != nil || total < 2 || len(items) < 2 {
		t.Fatalf("status list total=%d items=%#v err=%v", total, items, err)
	}
	if _, _, err := repository.List(ctx, CustomerFilter{SortBy: "name; DROP TABLE customer", Limit: 20}); err == nil {
		t.Fatal("unsafe customer sort was accepted")
	}

	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 7001, RemoteCreatedAt: now - 3600,
		Username: "customer-user", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	accountRepository := NewAccountRepository(database.GORM)
	if err := accountRepository.Create(ctx, &account); err != nil {
		t.Fatalf("create account dependency: %v", err)
	}
	archivedAccount := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 7002, RemoteCreatedAt: now - 1800,
		Username: "archived-customer-user", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusArchived, StatisticsPausedAt: int64Pointer(now - 100),
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := accountRepository.Create(ctx, &archivedAccount); err != nil {
		t.Fatalf("create archived account dependency: %v", err)
	}
	pauseAt := now + 100
	if err := repository.WithTransaction(ctx, func(transaction *CustomerRepository) error {
		return transaction.DisableInTransaction(ctx, customer.ID, pauseAt, now+100)
	}); err != nil {
		t.Fatalf("disable customer lifecycle: %v", err)
	}
	disabled, err := repository.FindByID(ctx, customer.ID)
	activeAfterDisable, activeErr := accountRepository.FindByID(ctx, account.ID)
	archivedAfterDisable, archivedErr := accountRepository.FindByID(ctx, archivedAccount.ID)
	if err != nil || activeErr != nil || archivedErr != nil || disabled.Status != "disabled" || disabled.StatisticsPausedAt == nil ||
		*disabled.StatisticsPausedAt != pauseAt || activeAfterDisable.StatisticsPausedAt == nil ||
		*activeAfterDisable.StatisticsPausedAt != pauseAt || archivedAfterDisable.StatisticsPausedAt == nil ||
		*archivedAfterDisable.StatisticsPausedAt != now-100 {
		t.Fatalf("disabled lifecycle customer=%#v active=%#v archived=%#v errors=%v/%v/%v",
			disabled, activeAfterDisable, archivedAfterDisable, err, activeErr, archivedErr)
	}
	disabled.Name = "must-not-bypass-lifecycle"
	disabled.Status = "using"
	if err := repository.UpdateProfile(ctx, &disabled); !errors.Is(err, ErrCustomerLifecycleContract) {
		t.Fatalf("disabled profile bypass error = %v", err)
	}
	if err := repository.BeginEnable(ctx, customer.ID, now+101); err != nil {
		t.Fatalf("begin customer enable: %v", err)
	}
	enabling, err := repository.FindByID(ctx, customer.ID)
	activeDuringEnable, activeErr := accountRepository.FindByID(ctx, account.ID)
	if err != nil || activeErr != nil || enabling.Status != "disabled" || enabling.StatisticsBackfillStatus != "pending" ||
		enabling.StatisticsPausedAt == nil || activeDuringEnable.StatisticsPausedAt == nil ||
		*activeDuringEnable.StatisticsPausedAt != pauseAt {
		t.Fatalf("enable cleared pause customer=%#v account=%#v errors=%v/%v", enabling, activeDuringEnable, err, activeErr)
	}
	enableRun := createFenceLocalRun(t, database, "customer", customer.ID, constant.TaskTypeCustomerRebuild,
		site.ID, pauseAt, "success", now+102)
	enableRunID = enableRun.ID
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", enableRun.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("finish enable run key: %v", err)
	}
	if err := repository.CompleteEnable(ctx, customer.ID, now+103); err != nil {
		t.Fatalf("complete customer enable: %v", err)
	}
	enabled, err := repository.FindByID(ctx, customer.ID)
	activeAfterEnable, activeErr := accountRepository.FindByID(ctx, account.ID)
	archivedAfterEnable, archivedErr := accountRepository.FindByID(ctx, archivedAccount.ID)
	if err != nil || activeErr != nil || archivedErr != nil || enabled.Status != "using" || enabled.StatisticsPausedAt != nil ||
		activeAfterEnable.StatisticsPausedAt != nil || archivedAfterEnable.StatisticsPausedAt == nil ||
		*archivedAfterEnable.StatisticsPausedAt != now-100 {
		t.Fatalf("completed enable customer=%#v active=%#v archived=%#v errors=%v/%v/%v",
			enabled, activeAfterEnable, archivedAfterEnable, err, activeErr, archivedErr)
	}
	canDelete, dependencies, err := repository.CanDelete(ctx, customer.ID)
	if err != nil || canDelete || dependencies.Accounts != 2 {
		t.Fatalf("customer account dependencies = %#v, canDelete=%t err=%v", dependencies, canDelete, err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete with account dependency error = %v", err)
	}
	if err := accountRepository.DeleteByID(ctx, account.ID); err != nil {
		t.Fatalf("delete empty account: %v", err)
	}
	if err := accountRepository.DeleteByID(ctx, archivedAccount.ID); err != nil {
		t.Fatalf("delete empty archived account: %v", err)
	}

	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO customer_stat_hourly
  (customer_id, site_id, hour_ts, request_count, quota, token_used, active_users, data_status,
   last_calculated_at, created_at, updated_at)
VALUES (?, ?, ?, 1, 2, 3, 1, 'complete', ?, ?, ?)`, customer.ID, site.ID, now-3600, now, now, now); err != nil {
		t.Fatalf("create customer statistic dependency: %v", err)
	}
	canDelete, dependencies, err = repository.CanDelete(ctx, customer.ID)
	if err != nil || canDelete || dependencies.HourlyStats != 1 {
		t.Fatalf("customer statistic dependencies = %#v, canDelete=%t err=%v", dependencies, canDelete, err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete with statistic dependency error = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, "DELETE FROM customer_stat_hourly WHERE customer_id = ?", customer.ID); err != nil {
		t.Fatalf("delete customer statistic: %v", err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); err != nil {
		t.Fatalf("delete dependency-free customer: %v", err)
	}
	if _, err := repository.FindByID(ctx, customer.ID); !IsNotFound(err) {
		t.Fatalf("deleted customer find error = %v", err)
	}
}

func TestCustomerEnableRequiresLatestCustomerAndAccountRuns(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_900_000)
	site := Site{
		Name: "B4 Enable Matrix Site " + suffix, BaseURL: "https://b4-enable-matrix-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create enable matrix site: %v", err)
	}
	customer := Customer{Name: "B4 Enable Matrix Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create enable matrix customer: %v", err)
	}
	archivedPause := now - 7200
	mismatchPause := now - 3600
	active := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8401, RemoteCreatedAt: now - 10_800,
		Username: "enable-active", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	archived := active
	archived.ID = 0
	archived.RemoteUserID = 8402
	archived.Username = "enable-archived"
	archived.ManagedStatus = AccountManagedStatusArchived
	archived.StatisticsPausedAt = &archivedPause
	mismatch := active
	mismatch.ID = 0
	mismatch.RemoteUserID = 8403
	mismatch.Username = "enable-mismatch"
	mismatch.RemoteState = AccountRemoteStateIdentityMismatch
	mismatch.StatisticsPausedAt = &mismatchPause
	for _, account := range []*Account{&active, &archived, &mismatch} {
		if err := database.GORM.Create(account).Error; err != nil {
			t.Fatalf("create enable account %s: %v", account.Username, err)
		}
	}
	runIDs := make([]int64, 0, 2)
	t.Cleanup(func() {
		if len(runIDs) > 0 {
			_ = database.GORM.Where("run_id IN ?", runIDs).Delete(&CollectionRunWindow{}).Error
			_ = database.GORM.Where("id IN ?", runIDs).Delete(&CollectionRun{}).Error
		}
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id IN (?, ?, ?)", active.ID, archived.ID, mismatch.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})
	repository := NewCustomerRepository(database.GORM)
	pauseAt := now
	if err := repository.Disable(ctx, customer.ID, pauseAt, now+1); err != nil {
		t.Fatalf("disable enable-matrix customer: %v", err)
	}
	if err := repository.BeginEnable(ctx, customer.ID, now+2); err != nil {
		t.Fatalf("begin enable-matrix customer: %v", err)
	}
	assertB4CustomerEnableBoundary(t, database, customer.ID, active.ID, pauseAt)
	if err := repository.CompleteEnable(ctx, customer.ID, now+3); !errors.Is(err, ErrCustomerEnableNotReady) {
		t.Fatalf("complete enable without run error = %v", err)
	}
	customerRun := createFenceLocalRun(t, database, "customer", customer.ID, constant.TaskTypeCustomerRebuild,
		site.ID, pauseAt, "pending", now+3)
	runIDs = append(runIDs, customerRun.ID)
	for index, status := range []string{"pending", "running", "failed"} {
		setB4RunStatus(t, database, customerRun.ID, status, now+int64(index)+4)
		if err := repository.CompleteEnable(ctx, customer.ID, now+int64(index)+4); !errors.Is(err, ErrCustomerEnableNotReady) {
			t.Fatalf("complete latest customer %s run error = %v", status, err)
		}
		assertB4CustomerEnableBoundary(t, database, customer.ID, active.ID, pauseAt)
	}
	setB4RunStatus(t, database, customerRun.ID, "success", now+7)
	accountRun := createFenceLocalRun(t, database, "account", active.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt, "pending", now+8)
	runIDs = append(runIDs, accountRun.ID)
	for index, status := range []string{"pending", "running", "failed"} {
		setB4RunStatus(t, database, accountRun.ID, status, now+int64(index)+8)
		if err := repository.CompleteEnable(ctx, customer.ID, now+int64(index)+8); !errors.Is(err, ErrCustomerEnableNotReady) {
			t.Fatalf("complete latest account %s run error = %v", status, err)
		}
		assertB4CustomerEnableBoundary(t, database, customer.ID, active.ID, pauseAt)
	}
	setB4RunStatus(t, database, accountRun.ID, "success", now+11)
	if err := repository.CompleteEnable(ctx, customer.ID, now+12); err != nil {
		t.Fatalf("complete fully successful enable: %v", err)
	}
	enabled, err := repository.FindByID(ctx, customer.ID)
	activeAfter, activeErr := NewAccountRepository(database.GORM).FindByID(ctx, active.ID)
	archivedAfter, archivedErr := NewAccountRepository(database.GORM).FindByID(ctx, archived.ID)
	mismatchAfter, mismatchErr := NewAccountRepository(database.GORM).FindByID(ctx, mismatch.ID)
	if err != nil || activeErr != nil || archivedErr != nil || mismatchErr != nil || enabled.Status != "using" ||
		enabled.StatisticsPausedAt != nil || enabled.StatisticsBackfillStatus != "none" || activeAfter.StatisticsPausedAt != nil ||
		archivedAfter.StatisticsPausedAt == nil || *archivedAfter.StatisticsPausedAt != archivedPause ||
		mismatchAfter.StatisticsPausedAt == nil || *mismatchAfter.StatisticsPausedAt != mismatchPause {
		t.Fatalf("completed matrix enable customer=%#v active=%#v archived=%#v mismatch=%#v errors=%v/%v/%v/%v",
			enabled, activeAfter, archivedAfter, mismatchAfter, err, activeErr, archivedErr, mismatchErr)
	}
}

func TestCustomerEnableRunWriterCannotOvertakeCompletion(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_940_000)
	site := Site{
		Name: "B4 Customer Run Fence Site " + suffix, BaseURL: "https://b4-customer-run-fence-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create customer run-fence site: %v", err)
	}
	customer := Customer{Name: "B4 Customer Run Fence " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create customer run-fence customer: %v", err)
	}
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8601, RemoteCreatedAt: now - 7200,
		Username: "customer-run-fence", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&account).Error; err != nil {
		t.Fatalf("create customer run-fence account: %v", err)
	}
	pauseAt := now - 3600
	if err := NewCustomerRepository(database.GORM).Disable(ctx, customer.ID, pauseAt, now+1); err != nil {
		t.Fatalf("disable customer run-fence customer: %v", err)
	}
	if err := NewCustomerRepository(database.GORM).BeginEnable(ctx, customer.ID, now+2); err != nil {
		t.Fatalf("begin customer run-fence enable: %v", err)
	}
	authoritative := createFenceLocalRun(t, database, "customer", customer.ID, constant.TaskTypeCustomerRebuild,
		site.ID, pauseAt, "success", now+2)
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", authoritative.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("clear authoritative customer run key: %v", err)
	}
	end := pauseAt + 3600
	activeKey, err := CollectionRunActiveKey(constant.TaskTypeCustomerRebuild, "customer", customer.ID, &pauseAt, &end)
	if err != nil {
		t.Fatalf("build competing customer run key: %v", err)
	}
	competingRun := CollectionRun{
		TaskType: constant.TaskTypeCustomerRebuild, TargetType: "customer", TargetID: customer.ID,
		TriggerType: constant.CollectionTriggerDependency, StartTimestamp: &pauseAt, EndTimestamp: &end,
		Scope: []byte("{}"), ActiveKey: &activeKey, Status: "pending",
		Priority: constant.CollectionPriorityLocalRebuild, NextAttemptAt: now + 3,
		CreatedRequestID: "req_customer_run_fence_writer", LastRequestID: "req_customer_run_fence_writer",
		CreatedAt: now + 3, UpdatedAt: now + 3,
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id = ?", authoritative.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE target_type = 'customer' AND target_id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id = ?", account.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})

	completionDB := gormOnDedicatedConnection(t, database)
	writerDB := gormOnDedicatedConnection(t, database)
	writerStarted := make(chan struct{})
	writerDone := make(chan error, 1)
	err = completionDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, lockErr := LockCustomerOperationScope(ctx, tx, customer.ID); lockErr != nil {
			return lockErr
		}
		go func() {
			writerDone <- writerDB.WithContext(ctx).Transaction(func(writer *gorm.DB) error {
				close(writerStarted)
				scope, lockErr := LockCustomerOperationScope(ctx, writer, customer.ID)
				if lockErr != nil {
					return lockErr
				}
				if scope.Customer.Status != "disabled" {
					return ErrCustomerLifecycleContract
				}
				return writer.Create(&competingRun).Error
			})
		}()
		<-writerStarted
		select {
		case writerErr := <-writerDone:
			return fmt.Errorf("run writer crossed customer operation scope before completion: %w", writerErr)
		case <-time.After(100 * time.Millisecond):
		}
		var newerCount int64
		if countErr := database.GORM.Model(&CollectionRun{}).
			Where("target_type = 'customer' AND target_id = ? AND id > ?", customer.ID, authoritative.ID).
			Count(&newerCount).Error; countErr != nil {
			return countErr
		}
		if newerCount != 0 {
			return fmt.Errorf("newer customer rebuild run became visible before scope release")
		}
		return NewCustomerRepository(tx).CompleteEnableInTransaction(ctx, customer.ID, now+4)
	})
	if err != nil {
		t.Fatalf("complete enable under run writer contention: %v", err)
	}
	if writerErr := <-writerDone; !errors.Is(writerErr, ErrCustomerLifecycleContract) {
		t.Fatalf("customer run writer post-fence recheck error = %v", writerErr)
	}
	var latest CollectionRun
	if err := database.GORM.Where("target_type = 'customer' AND target_id = ? AND task_type = ?",
		customer.ID, constant.TaskTypeCustomerRebuild).Order("id DESC").First(&latest).Error; err != nil {
		t.Fatalf("read latest customer run after fenced completion: %v", err)
	}
	enabled, customerErr := NewCustomerRepository(database.GORM).FindByID(ctx, customer.ID)
	active, accountErr := NewAccountRepository(database.GORM).FindByID(ctx, account.ID)
	if customerErr != nil || accountErr != nil || latest.ID != authoritative.ID || enabled.Status != "using" ||
		enabled.StatisticsPausedAt != nil || active.StatisticsPausedAt != nil {
		t.Fatalf("fenced customer completion latest=%#v customer=%#v account=%#v errors=%v/%v",
			latest, enabled, active, customerErr, accountErr)
	}
}

func assertB4CustomerEnableBoundary(t *testing.T, database *Database, customerID, activeAccountID, pauseAt int64) {
	t.Helper()
	customer, customerErr := NewCustomerRepository(database.GORM).FindByID(context.Background(), customerID)
	account, accountErr := NewAccountRepository(database.GORM).FindByID(context.Background(), activeAccountID)
	if customerErr != nil || accountErr != nil || customer.Status != "disabled" || customer.StatisticsPausedAt == nil ||
		*customer.StatisticsPausedAt != pauseAt || account.ManagedStatus != AccountManagedStatusActive ||
		account.StatisticsPausedAt == nil || *account.StatisticsPausedAt != pauseAt {
		t.Fatalf("enable boundary customer=%#v account=%#v errors=%v/%v", customer, account, customerErr, accountErr)
	}
}

func TestCustomerDeleteMetadataBlockersAndCleanup(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_700_000)
	site := Site{
		Name: "B4 Customer Delete Site " + suffix, BaseURL: "https://b4-customer-delete-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create customer delete site: %v", err)
	}
	customer := Customer{Name: "B4 Metadata Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create metadata customer: %v", err)
	}
	run := createFenceLocalRun(t, database, "customer", customer.ID, "customer_rebuild", site.ID, now-7200, "pending", now)
	ruleKey := "b4_customer_delete_" + suffix
	ruleID := createB4AlertRule(t, database, ruleKey, now)
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_delivery WHERE alert_event_id IN (SELECT id FROM alert_event WHERE rule_id = ?)", ruleID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_event WHERE rule_id = ?", ruleID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_rule WHERE id = ?", ruleID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id = ?", run.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE id = ?", run.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})
	repository := NewCustomerRepository(database.GORM)
	dependencies, err := repository.DeletionDependencies(ctx, customer.ID)
	if err != nil || dependencies.ActiveRuns != 1 {
		t.Fatalf("customer active run dependencies = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete customer with active run = %v", err)
	}
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", run.ID).
		Updates(map[string]any{"status": "failed", "active_key": nil, "updated_at": now + 1}).Error; err != nil {
		t.Fatalf("finish customer run: %v", err)
	}
	collectionTargetKey := fmt.Sprintf("%d/%d", site.ID, run.ID)
	collectionAlertID := createB4AlertEvent(t, database, ruleID, "backfill_failed", &site.ID,
		"collection", collectionTargetKey, "firing", true, now)
	dependencies, err = repository.DeletionDependencies(ctx, customer.ID)
	if err != nil || dependencies.ActiveAlerts != 1 {
		t.Fatalf("customer collection alert dependencies = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete customer with collection alert = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE alert_event
SET status = 'resolved', active_key = NULL, resolved_at = ?, updated_at = ? WHERE id = ?`, now+2, now+2, collectionAlertID); err != nil {
		t.Fatalf("resolve collection history alert: %v", err)
	}
	dependencies, err = repository.DeletionDependencies(ctx, customer.ID)
	if err != nil || dependencies.AlertHistory != 1 {
		t.Fatalf("customer collection alert history = %#v, %v", dependencies, err)
	}
	if _, err := database.SQL.ExecContext(ctx, "DELETE FROM alert_event WHERE id = ?", collectionAlertID); err != nil {
		t.Fatalf("remove collection alert history fixture: %v", err)
	}
	activeAlertID := createB4AlertEvent(t, database, ruleID, ruleKey, nil,
		"customer", fmt.Sprintf("%d", customer.ID), "firing", true, now)
	dependencies, err = repository.DeletionDependencies(ctx, customer.ID)
	if err != nil || dependencies.ActiveAlerts != 1 {
		t.Fatalf("customer active alert dependencies = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete customer with active alert = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE alert_event
SET status = 'resolved', active_key = NULL, resolved_at = ?, updated_at = ? WHERE id = ?`, now+2, now+2, activeAlertID); err != nil {
		t.Fatalf("resolve customer history alert: %v", err)
	}
	dependencies, err = repository.DeletionDependencies(ctx, customer.ID)
	if err != nil || dependencies.AlertHistory != 1 {
		t.Fatalf("customer alert history dependencies = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, customer.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete customer with alert history = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, "DELETE FROM alert_event WHERE id = ?", activeAlertID); err != nil {
		t.Fatalf("remove customer alert history fixture: %v", err)
	}
	clearableAlertID := createB4AlertEvent(t, database, ruleID, ruleKey, nil,
		"customer", fmt.Sprintf("%d", customer.ID), "resolved", false, now)
	clearableCollectionAlertID := createB4AlertEvent(t, database, ruleID, "backfill_failed", &site.ID,
		"collection", collectionTargetKey, "resolved", false, now)
	if err := repository.DeleteByID(ctx, customer.ID); err != nil {
		t.Fatalf("delete customer with only clearable metadata: %v", err)
	}
	assertB4RowCount(t, database, "customer", "id = ?", customer.ID, 0)
	assertB4RowCount(t, database, "collection_run", "id = ?", run.ID, 0)
	assertB4RowCount(t, database, "collection_run_window", "run_id = ?", run.ID, 0)
	assertB4RowCount(t, database, "alert_event", "id = ?", clearableAlertID, 0)
	assertB4RowCount(t, database, "alert_event", "id = ?", clearableCollectionAlertID, 0)
}
