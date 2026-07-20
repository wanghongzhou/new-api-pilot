package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"new-api-pilot/constant"

	mysqlgorm "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAccountRepositoryBindingStatesFiltersAndDeleteDependencies(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_400_800)

	site := Site{
		Name: "B4 Account Site " + suffix, BaseURL: "https://b4-account-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create site: %v", err)
	}
	customer := Customer{
		Name: "B4 Account Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := NewCustomerRepository(database.GORM).Create(ctx, &customer); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	repository := NewAccountRepository(database.GORM)
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8001, RemoteCreatedAt: now - 7200,
		Username: "literal%_\\user", DisplayName: "Literal Display", RemoteGroup: "vip", RemoteStatus: 1,
		RemoteState: AccountRemoteStateNormal, Quota: 9_223_372_036_854_000_000, UsedQuota: 77, RequestCount: 88,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.Create(ctx, &account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	decoy := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8002, RemoteCreatedAt: now - 3600,
		Username: "literalXYZuser", RemoteStatus: 0, RemoteState: AccountRemoteStateMissing,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now + 1,
	}
	if err := repository.Create(ctx, &decoy); err != nil {
		t.Fatalf("create decoy account: %v", err)
	}
	var restoreRunID int64
	t.Cleanup(func() {
		if restoreRunID > 0 {
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id = ?", restoreRunID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE id = ?", restoreRunID)
		}
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account_stat_hourly WHERE account_id IN (?, ?)", account.ID, decoy.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account_stat_daily WHERE account_id IN (?, ?)", account.ID, decoy.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE site_id = ?", site.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})

	duplicate := account
	duplicate.ID = 0
	duplicate.CustomerID = customer.ID
	duplicate.CreatedAt = now + 1
	duplicate.UpdatedAt = now + 1
	if err := repository.Create(ctx, &duplicate); !IsDuplicateKey(err) {
		t.Fatalf("duplicate site/remote binding error = %v", err)
	}

	loaded, err := repository.FindBySiteAndRemoteUser(ctx, site.ID, account.RemoteUserID)
	if err != nil || loaded.ID != account.ID {
		t.Fatalf("find binding = %#v, %v", loaded, err)
	}
	if err := repository.WithTransaction(ctx, func(transaction *AccountRepository) error {
		locked, lockErr := transaction.FindBySiteAndRemoteUserForUpdate(ctx, site.ID, account.RemoteUserID)
		if lockErr == nil && locked.ID != account.ID {
			return fmt.Errorf("locked wrong account %d", locked.ID)
		}
		return lockErr
	}); err != nil {
		t.Fatalf("lock binding: %v", err)
	}

	originalSiteID, originalCustomerID := loaded.SiteID, loaded.CustomerID
	originalRemoteUserID, originalRemoteCreatedAt := loaded.RemoteUserID, loaded.RemoteCreatedAt
	if err := repository.UpdateRemark(ctx, loaded.ID, "updated remark", now+2); err != nil {
		t.Fatalf("update account remark: %v", err)
	}
	loaded, err = repository.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if loaded.SiteID != originalSiteID || loaded.CustomerID != originalCustomerID ||
		loaded.RemoteUserID != originalRemoteUserID || loaded.RemoteCreatedAt != originalRemoteCreatedAt {
		t.Fatalf("immutable binding changed: %#v", loaded)
	}
	if loaded.Remark != "updated remark" || loaded.Username != account.Username {
		t.Fatalf("explicit mutable field update = %#v", loaded)
	}

	items, total, err := repository.List(ctx, AccountFilter{
		Keyword: "%_\\", SiteID: &site.ID, RemoteStatus: intPointer(1), RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, SortBy: "quota", SortOrder: "desc", Limit: 20,
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != account.ID {
		t.Fatalf("escaped account search total=%d items=%#v err=%v", total, items, err)
	}
	if _, applied, err := repository.ApplyAuthoritativeRemoteSnapshot(ctx, account.ID, AuthoritativeAccountRemoteSnapshot{
		RemoteCreatedAt: account.RemoteCreatedAt,
		Username:        "literal%_\\user", DisplayName: "Remote", RemoteGroup: "group", RemoteStatus: 1,
		Quota: 100, UsedQuota: 200, RequestCount: 300, ObservedAt: now + 3, UpdatedAt: now + 3,
	}); err != nil || !applied {
		t.Fatalf("apply remote snapshot: applied=%t err=%v", applied, err)
	}
	items, total, err = repository.List(ctx, AccountFilter{
		Keyword: "%_\\", SiteID: &site.ID, SortBy: "updated_at", SortOrder: "desc", Limit: 20,
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != account.ID {
		t.Fatalf("literal account list total=%d items=%#v err=%v", total, items, err)
	}
	if _, _, err := repository.List(ctx, AccountFilter{SortBy: "quota; DELETE FROM account", Limit: 20}); err == nil {
		t.Fatal("unsafe account sort was accepted")
	}

	if err := repository.Archive(ctx, account.ID, now, now+4); err != nil {
		t.Fatalf("archive account: %v", err)
	}
	archived, err := repository.FindByID(ctx, account.ID)
	if err != nil || archived.ManagedStatus != AccountManagedStatusArchived || archived.StatisticsPausedAt == nil ||
		*archived.StatisticsPausedAt != now || archived.StatisticsBackfillStatus != "none" {
		t.Fatalf("archived account = %#v, %v", archived, err)
	}
	bindings, err := repository.FindManagedBindings(ctx, site.ID, []int64{account.RemoteUserID, decoy.RemoteUserID, 999999})
	if err != nil || len(bindings) != 2 || bindings[account.RemoteUserID].AccountID != account.ID ||
		bindings[account.RemoteUserID].CustomerName != customer.Name || bindings[account.RemoteUserID].ManagedStatus != AccountManagedStatusArchived {
		t.Fatalf("managed bindings = %#v, %v", bindings, err)
	}
	if err := repository.CompleteRestore(ctx, account.ID, now+5); !errors.Is(err, ErrRebuildRunNotReady) {
		t.Fatalf("complete archived restore without run error = %v", err)
	}
	stillArchived, err := repository.FindByID(ctx, account.ID)
	if err != nil || stillArchived.StatisticsPausedAt == nil || *stillArchived.StatisticsPausedAt != now {
		t.Fatalf("archived pause was cleared: %#v, %v", stillArchived, err)
	}
	if err := repository.BeginRestore(ctx, account.ID, now+5); err != nil {
		t.Fatalf("begin restore account: %v", err)
	}
	restored, err := repository.FindByID(ctx, account.ID)
	if err != nil || restored.ManagedStatus != AccountManagedStatusArchived || restored.StatisticsBackfillStatus != "pending" || restored.StatisticsPausedAt == nil {
		t.Fatalf("restoring account = %#v, %v", restored, err)
	}
	restoreRun := createFenceLocalRun(t, database, "account", account.ID, constant.TaskTypeAccountRebuild,
		site.ID, now, "success", now+5)
	restoreRunID = restoreRun.ID
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", restoreRun.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("finish restore run key: %v", err)
	}
	if err := repository.CompleteRestore(ctx, account.ID, now+6); err != nil {
		t.Fatalf("complete restore: %v", err)
	}
	firstAbsence, applied, err := repository.MarkMissing(ctx, account.ID, now+7, now+7)
	if err != nil || !applied || firstAbsence.RemoteState != AccountRemoteStateNormal || firstAbsence.RemoteMissingCount != 1 {
		t.Fatalf("first absence account = %#v, applied=%t err=%v", firstAbsence, applied, err)
	}
	missing, applied, err := repository.MarkMissing(ctx, account.ID, now+8, now+8)
	if err != nil || missing.RemoteState != AccountRemoteStateMissing || missing.RemoteMissingCount != 2 {
		t.Fatalf("missing account = %#v, applied=%t err=%v", missing, applied, err)
	}
	mismatch, applied, err := repository.MarkIdentityMismatch(ctx, account.ID, now+9, now+3600, now+9)
	if err != nil || mismatch.RemoteState != AccountRemoteStateIdentityMismatch || mismatch.StatisticsPausedAt == nil || *mismatch.StatisticsPausedAt != now+3600 {
		t.Fatalf("identity mismatch account = %#v, applied=%t err=%v", mismatch, applied, err)
	}
	conflictingDisplay, applied, err := repository.ApplyAuthoritativeRemoteSnapshot(ctx, account.ID, AuthoritativeAccountRemoteSnapshot{
		RemoteCreatedAt: account.RemoteCreatedAt + 1, Username: "must-not-recover", RemoteStatus: 1,
		ObservedAt: now + 10, UpdatedAt: now + 10,
	})
	if err != nil || !applied || conflictingDisplay.RemoteState != AccountRemoteStateIdentityMismatch ||
		conflictingDisplay.StatisticsPausedAt == nil || conflictingDisplay.Username != "must-not-recover" {
		t.Fatalf("identity mismatch display refresh = %#v, applied=%t err=%v", conflictingDisplay, applied, err)
	}
	if err := repository.BeginRestore(ctx, account.ID, now+10); !errors.Is(err, ErrAccountIdentityMismatch) {
		t.Fatalf("identity mismatch restore error = %v", err)
	}
	mismatch, err = repository.FindByID(ctx, account.ID)
	if err != nil || mismatch.RemoteState != AccountRemoteStateIdentityMismatch || mismatch.Username != "must-not-recover" || mismatch.StatisticsPausedAt == nil {
		t.Fatalf("identity mismatch isolation changed: %#v, %v", mismatch, err)
	}
	isolated, applied, err := repository.ApplyAuthoritativeRemoteSnapshot(ctx, account.ID, AuthoritativeAccountRemoteSnapshot{
		RemoteCreatedAt: account.RemoteCreatedAt, Username: "exact-original", RemoteStatus: 1,
		ObservedAt: now + 11, UpdatedAt: now + 11,
	})
	if err != nil || !applied || isolated.RemoteState != AccountRemoteStateIdentityMismatch ||
		isolated.RemoteMissingCount != mismatch.RemoteMissingCount || isolated.StatisticsPausedAt == nil || isolated.Username != "exact-original" {
		t.Fatalf("exact mismatch refresh = %#v, applied=%t err=%v", isolated, applied, err)
	}
	if err := repository.CompleteRestore(ctx, account.ID, now+12); !errors.Is(err, ErrAccountIdentityMismatch) {
		t.Fatalf("complete identity mismatch restore error = %v", err)
	}

	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO account_stat_hourly
  (account_id, hour_ts, request_count, quota, token_used, data_status, last_calculated_at, created_at, updated_at)
VALUES (?, ?, 1, 2, 3, 'complete', ?, ?, ?)`, account.ID, now-3600, now, now, now); err != nil {
		t.Fatalf("create account statistic dependency: %v", err)
	}
	canDelete, dependencies, err := repository.CanDelete(ctx, account.ID)
	if err != nil || canDelete || dependencies.HourlyStats != 1 {
		t.Fatalf("account dependencies = %#v canDelete=%t err=%v", dependencies, canDelete, err)
	}
	if err := repository.DeleteByID(ctx, account.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete account with statistics error = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, "DELETE FROM account_stat_hourly WHERE account_id = ?", account.ID); err != nil {
		t.Fatalf("delete account statistic: %v", err)
	}
	if err := repository.DeleteByID(ctx, account.ID); err != nil {
		t.Fatalf("delete dependency-free account: %v", err)
	}
	if err := repository.DeleteByID(ctx, decoy.ID); err != nil {
		t.Fatalf("delete decoy account: %v", err)
	}
}

func intPointer(value int) *int { return &value }

func TestAccountMissingObservationMonotonicAcrossConnections(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_500_000)
	site := Site{
		Name: "B4 Observation Site " + suffix, BaseURL: "https://b4-observation-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create observation site: %v", err)
	}
	customer := Customer{Name: "B4 Observation Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create observation customer: %v", err)
	}
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8101, RemoteCreatedAt: now - 7200,
		Username: "observation-user", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&account).Error; err != nil {
		t.Fatalf("create observation account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id = ?", account.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})

	firstRepository := accountRepositoryOnDedicatedConnection(t, database)
	secondRepository := accountRepositoryOnDedicatedConnection(t, database)
	first, applied, err := firstRepository.MarkMissing(ctx, account.ID, now+100, now+100)
	if err != nil || !applied || first.RemoteMissingCount != 1 || first.RemoteState != AccountRemoteStateNormal {
		t.Fatalf("first missing observation = %#v applied=%t err=%v", first, applied, err)
	}
	second, applied, err := firstRepository.MarkMissing(ctx, account.ID, now+200, now+200)
	if err != nil || !applied || second.RemoteMissingCount != 2 || second.RemoteState != AccountRemoteStateMissing {
		t.Fatalf("second missing observation = %#v applied=%t err=%v", second, applied, err)
	}

	type observationResult struct {
		account Account
		applied bool
		err     error
	}
	results := make(chan observationResult, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for _, request := range []struct {
		repository *AccountRepository
		observedAt int64
	}{{firstRepository, now + 150}, {secondRepository, now + 300}} {
		wait.Add(1)
		go func(repository *AccountRepository, observedAt int64) {
			defer wait.Done()
			<-start
			observed, wasApplied, observeErr := repository.MarkMissing(ctx, account.ID, observedAt, observedAt)
			results <- observationResult{account: observed, applied: wasApplied, err: observeErr}
		}(request.repository, request.observedAt)
	}
	close(start)
	wait.Wait()
	close(results)
	appliedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent observation error = %v", result.err)
		}
		if result.applied {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("concurrent applied count = %d, want 1", appliedCount)
	}
	final, err := NewAccountRepository(database.GORM).FindByID(ctx, account.ID)
	if err != nil || final.RemoteMissingCount != 3 || final.RemoteState != AccountRemoteStateMissing ||
		final.LastSyncedAt == nil || *final.LastSyncedAt != now+300 {
		t.Fatalf("concurrent final account = %#v, %v", final, err)
	}
	duplicate, applied, err := secondRepository.MarkMissing(ctx, account.ID, now+300, now+301)
	if err != nil || applied || duplicate.RemoteMissingCount != 3 || duplicate.RemoteState != AccountRemoteStateMissing {
		t.Fatalf("duplicate observation = %#v applied=%t err=%v", duplicate, applied, err)
	}
	staleSnapshot, applied, err := firstRepository.ApplyAuthoritativeRemoteSnapshot(ctx, account.ID, AuthoritativeAccountRemoteSnapshot{
		RemoteCreatedAt: account.RemoteCreatedAt, Username: "stale", ObservedAt: now + 250, UpdatedAt: now + 302,
	})
	if err != nil || applied || staleSnapshot.RemoteMissingCount != 3 || staleSnapshot.RemoteState != AccountRemoteStateMissing {
		t.Fatalf("stale exact snapshot = %#v applied=%t err=%v", staleSnapshot, applied, err)
	}
	recovered, applied, err := secondRepository.ApplyAuthoritativeRemoteSnapshot(ctx, account.ID, AuthoritativeAccountRemoteSnapshot{
		RemoteCreatedAt: account.RemoteCreatedAt, Username: "recovered", RemoteStatus: 1,
		ObservedAt: now + 400, UpdatedAt: now + 400,
	})
	if err != nil || !applied || recovered.RemoteMissingCount != 0 || recovered.RemoteState != AccountRemoteStateNormal || recovered.Username != "recovered" {
		t.Fatalf("new exact snapshot = %#v applied=%t err=%v", recovered, applied, err)
	}
}

func TestAccountRestoreLatestRunStatesCustomerGuardAndFence(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_800_000)
	site := Site{
		Name: "B4 Restore Site " + suffix, BaseURL: "https://b4-restore-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create restore site: %v", err)
	}
	customer := Customer{Name: "B4 Restore Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create restore customer: %v", err)
	}
	pauseAt := now - 3600
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8301, RemoteCreatedAt: now - 7200,
		Username: "restore-state-user", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusArchived, StatisticsPausedAt: &pauseAt,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	guardedAccount := account
	guardedAccount.ID = 0
	guardedAccount.RemoteUserID = 8302
	guardedAccount.Username = "restore-disabled-customer"
	fenceAccount := account
	fenceAccount.ID = 0
	fenceAccount.RemoteUserID = 8303
	fenceAccount.Username = "restore-fence-user"
	for _, candidate := range []*Account{&account, &guardedAccount, &fenceAccount} {
		if err := database.GORM.Create(candidate).Error; err != nil {
			t.Fatalf("create restore account %s: %v", candidate.Username, err)
		}
	}
	runIDs := make([]int64, 0, 4)
	t.Cleanup(func() {
		if len(runIDs) > 0 {
			_ = database.GORM.Where("run_id IN ?", runIDs).Delete(&CollectionRunWindow{}).Error
			_ = database.GORM.Where("id IN ?", runIDs).Delete(&CollectionRun{}).Error
		}
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id IN (?, ?, ?)", account.ID, guardedAccount.ID, fenceAccount.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})
	repository := NewAccountRepository(database.GORM)
	if err := repository.BeginRestore(ctx, account.ID, now+1); err != nil {
		t.Fatalf("begin account restore: %v", err)
	}
	assertB4AccountRestoreBoundary(t, repository, account.ID, pauseAt, "pending")
	if err := repository.CompleteRestore(ctx, account.ID, now+2); !errors.Is(err, ErrRebuildRunNotReady) {
		t.Fatalf("complete without run error = %v", err)
	}
	staleSuccess := createFenceLocalRun(t, database, "account", account.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt-3600, "success", now-1)
	runIDs = append(runIDs, staleSuccess.ID)
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", staleSuccess.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("clear stale success key: %v", err)
	}
	if err := repository.CompleteRestore(ctx, account.ID, now+2); !errors.Is(err, ErrRebuildRunNotReady) {
		t.Fatalf("stale success crossed pause fence: %v", err)
	}
	run := createFenceLocalRun(t, database, "account", account.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt, "pending", now+2)
	runIDs = append(runIDs, run.ID)
	for index, status := range []string{"pending", "running", "failed"} {
		setB4RunStatus(t, database, run.ID, status, now+int64(index)+3)
		if err := repository.CompleteRestore(ctx, account.ID, now+int64(index)+3); !errors.Is(err, ErrRebuildRunNotReady) {
			t.Fatalf("complete latest %s run error = %v", status, err)
		}
		assertB4AccountRestoreBoundary(t, repository, account.ID, pauseAt, "pending")
	}
	setB4RunStatus(t, database, run.ID, "success", now+6)
	newerFailed := createFenceLocalRun(t, database, "account", account.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt, "failed", now+7)
	runIDs = append(runIDs, newerFailed.ID)
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", newerFailed.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("clear newer failed run key: %v", err)
	}
	if err := repository.CompleteRestore(ctx, account.ID, now+8); !errors.Is(err, ErrRebuildRunNotReady) {
		t.Fatalf("newer failed run did not override old success: %v", err)
	}
	setB4RunStatus(t, database, newerFailed.ID, "success", now+9)
	if err := repository.CompleteRestore(ctx, account.ID, now+10); err != nil {
		t.Fatalf("complete latest successful restore: %v", err)
	}
	completed, err := repository.FindByID(ctx, account.ID)
	if err != nil || completed.ManagedStatus != AccountManagedStatusActive || completed.StatisticsPausedAt != nil || completed.StatisticsBackfillStatus != "none" {
		t.Fatalf("completed restore = %#v, %v", completed, err)
	}

	if err := database.GORM.Model(&Customer{}).Where("id = ?", customer.ID).Update("status", "disabled").Error; err != nil {
		t.Fatalf("disable restore customer fixture: %v", err)
	}
	if err := repository.BeginRestore(ctx, guardedAccount.ID, now+11); !errors.Is(err, ErrCustomerDisabled) {
		t.Fatalf("begin restore under disabled customer error = %v", err)
	}
	guardRun := createFenceLocalRun(t, database, "account", guardedAccount.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt, "success", now+11)
	runIDs = append(runIDs, guardRun.ID)
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", guardRun.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("clear disabled-customer run key: %v", err)
	}
	if err := repository.CompleteRestore(ctx, guardedAccount.ID, now+11); !errors.Is(err, ErrCustomerDisabled) {
		t.Fatalf("complete restore under disabled customer error = %v", err)
	}
	assertB4AccountRestoreBoundary(t, repository, guardedAccount.ID, pauseAt, "none")
	if err := database.GORM.Model(&Customer{}).Where("id = ?", customer.ID).Update("status", "using").Error; err != nil {
		t.Fatalf("enable restore customer fixture: %v", err)
	}

	if err := repository.BeginRestore(ctx, fenceAccount.ID, now+12); err != nil {
		t.Fatalf("begin fenced restore: %v", err)
	}
	fenceRun := createFenceLocalRun(t, database, "account", fenceAccount.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt, "success", now+13)
	runIDs = append(runIDs, fenceRun.ID)
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", fenceRun.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("clear fence run key: %v", err)
	}
	firstConnectionRepository := accountRepositoryOnDedicatedConnection(t, database)
	secondConnectionRepository := accountRepositoryOnDedicatedConnection(t, database)
	writerStarted := make(chan struct{})
	writerDone := make(chan error, 1)
	err = firstConnectionRepository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, lockErr := findAccountForUpdate(tx, fenceAccount.ID); lockErr != nil {
			return lockErr
		}
		go func() {
			writerDone <- secondConnectionRepository.db.WithContext(ctx).Transaction(func(writer *gorm.DB) error {
				close(writerStarted)
				locked, lockErr := findAccountForUpdate(writer, fenceAccount.ID)
				if lockErr != nil {
					return lockErr
				}
				if locked.ManagedStatus != AccountManagedStatusArchived {
					return ErrAccountRestoreContract
				}
				return errors.New("writer unexpectedly crossed restore fence")
			})
		}()
		<-writerStarted
		time.Sleep(50 * time.Millisecond)
		return NewAccountRepository(tx).CompleteRestoreInTransaction(ctx, fenceAccount.ID, now+14)
	})
	if err != nil {
		t.Fatalf("complete fenced restore: %v", err)
	}
	if writerErr := <-writerDone; !errors.Is(writerErr, ErrAccountRestoreContract) {
		t.Fatalf("concurrent writer fence error = %v", writerErr)
	}
}

func TestAccountCustomerLifecycleLockOrderAcrossConnections(t *testing.T) {
	database := openLockedSiteRunDatabase(t)

	t.Run("disable and begin restore", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		now := int64(1_752_910_000)
		site := Site{
			Name: "B4 Lock Begin Site " + suffix, BaseURL: "https://b4-lock-begin-" + suffix + ".example", ConfigVersion: 1,
			ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
			StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
		}
		if err := database.GORM.Create(&site).Error; err != nil {
			t.Fatalf("create lock-order site: %v", err)
		}
		customer := Customer{Name: "B4 Lock Begin Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
		if err := database.GORM.Create(&customer).Error; err != nil {
			t.Fatalf("create lock-order customer: %v", err)
		}
		pauseAt := now - 3600
		account := Account{
			SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8501, RemoteCreatedAt: now - 7200,
			Username: "lock-begin", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
			ManagedStatus: AccountManagedStatusArchived, StatisticsPausedAt: &pauseAt,
			StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
		}
		if err := database.GORM.Create(&account).Error; err != nil {
			t.Fatalf("create lock-order account: %v", err)
		}
		t.Cleanup(func() {
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id = ?", account.ID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
		})

		accountRepository := NewAccountRepository(gormOnDedicatedConnection(t, database))
		customerRepository := NewCustomerRepository(gormOnDedicatedConnection(t, database))
		results := runB4ConcurrentLifecycleOperations(ctx,
			func() error { return accountRepository.BeginRestore(ctx, account.ID, now+1) },
			func() error { return customerRepository.Disable(ctx, customer.ID, now, now+2) },
		)
		if results.second != nil {
			t.Fatalf("concurrent customer disable error = %v", results.second)
		}
		if results.first != nil && !errors.Is(results.first, ErrCustomerDisabled) {
			t.Fatalf("concurrent account begin restore error = %v", results.first)
		}
		disabled, customerErr := NewCustomerRepository(database.GORM).FindByID(ctx, customer.ID)
		archived, accountErr := NewAccountRepository(database.GORM).FindByID(ctx, account.ID)
		if customerErr != nil || accountErr != nil || disabled.Status != "disabled" || archived.ManagedStatus != AccountManagedStatusArchived ||
			archived.StatisticsPausedAt == nil || *archived.StatisticsPausedAt != pauseAt {
			t.Fatalf("serialized begin/disable customer=%#v account=%#v errors=%v/%v", disabled, archived, customerErr, accountErr)
		}
		wantBackfill := "none"
		if results.first == nil {
			wantBackfill = "pending"
		}
		if archived.StatisticsBackfillStatus != wantBackfill {
			t.Fatalf("serialized account backfill status = %s, want %s", archived.StatisticsBackfillStatus, wantBackfill)
		}
	})

	t.Run("enable and complete restore", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		now := int64(1_752_920_000)
		site := Site{
			Name: "B4 Lock Complete Site " + suffix, BaseURL: "https://b4-lock-complete-" + suffix + ".example", ConfigVersion: 1,
			ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
			StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
		}
		if err := database.GORM.Create(&site).Error; err != nil {
			t.Fatalf("create complete lock-order site: %v", err)
		}
		customer := Customer{Name: "B4 Lock Complete Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
		if err := database.GORM.Create(&customer).Error; err != nil {
			t.Fatalf("create complete lock-order customer: %v", err)
		}
		accountPause := now - 3600
		account := Account{
			SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8502, RemoteCreatedAt: now - 7200,
			Username: "lock-complete", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
			ManagedStatus: AccountManagedStatusArchived, StatisticsPausedAt: &accountPause,
			StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
		}
		if err := database.GORM.Create(&account).Error; err != nil {
			t.Fatalf("create complete lock-order account: %v", err)
		}
		if err := NewAccountRepository(database.GORM).BeginRestore(ctx, account.ID, now+1); err != nil {
			t.Fatalf("begin complete lock-order restore: %v", err)
		}
		if err := NewCustomerRepository(database.GORM).Disable(ctx, customer.ID, now, now+2); err != nil {
			t.Fatalf("disable complete lock-order customer: %v", err)
		}
		if err := NewCustomerRepository(database.GORM).BeginEnable(ctx, customer.ID, now+3); err != nil {
			t.Fatalf("begin complete lock-order enable: %v", err)
		}
		accountRun := createFenceLocalRun(t, database, "account", account.ID, constant.TaskTypeAccountRebuild,
			site.ID, accountPause, "success", now+3)
		customerRun := createFenceLocalRun(t, database, "customer", customer.ID, constant.TaskTypeCustomerRebuild,
			site.ID, now, "success", now+3)
		for _, run := range []CollectionRun{accountRun, customerRun} {
			if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", run.ID).Update("active_key", nil).Error; err != nil {
				t.Fatalf("clear complete lock-order run %d key: %v", run.ID, err)
			}
		}
		t.Cleanup(func() {
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id IN (?, ?)", accountRun.ID, customerRun.ID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE id IN (?, ?)", accountRun.ID, customerRun.ID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id = ?", account.ID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
			_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
		})

		accountRepository := NewAccountRepository(gormOnDedicatedConnection(t, database))
		customerRepository := NewCustomerRepository(gormOnDedicatedConnection(t, database))
		results := runB4ConcurrentLifecycleOperations(ctx,
			func() error { return accountRepository.CompleteRestore(ctx, account.ID, now+4) },
			func() error { return customerRepository.CompleteEnable(ctx, customer.ID, now+4) },
		)
		if results.second != nil {
			t.Fatalf("concurrent customer complete enable error = %v", results.second)
		}
		if results.first != nil && !errors.Is(results.first, ErrCustomerDisabled) {
			t.Fatalf("concurrent account complete restore error = %v", results.first)
		}
		enabled, customerErr := NewCustomerRepository(database.GORM).FindByID(ctx, customer.ID)
		finalAccount, accountErr := NewAccountRepository(database.GORM).FindByID(ctx, account.ID)
		if customerErr != nil || accountErr != nil || enabled.Status != "using" || enabled.StatisticsPausedAt != nil {
			t.Fatalf("serialized complete customer=%#v account=%#v errors=%v/%v", enabled, finalAccount, customerErr, accountErr)
		}
		if results.first == nil {
			if finalAccount.ManagedStatus != AccountManagedStatusActive || finalAccount.StatisticsPausedAt != nil {
				t.Fatalf("successful serialized restore account = %#v", finalAccount)
			}
		} else if finalAccount.ManagedStatus != AccountManagedStatusArchived || finalAccount.StatisticsPausedAt == nil {
			t.Fatalf("guarded serialized restore account = %#v", finalAccount)
		}
	})
}

func TestAccountRestoreRunWriterCannotOvertakeCompletion(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_930_000)
	site := Site{
		Name: "B4 Run Fence Site " + suffix, BaseURL: "https://b4-run-fence-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create run-fence site: %v", err)
	}
	customer := Customer{Name: "B4 Run Fence Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create run-fence customer: %v", err)
	}
	pauseAt := now - 3600
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8503, RemoteCreatedAt: now - 7200,
		Username: "run-fence", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusArchived, StatisticsPausedAt: &pauseAt,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&account).Error; err != nil {
		t.Fatalf("create run-fence account: %v", err)
	}
	if err := NewAccountRepository(database.GORM).BeginRestore(ctx, account.ID, now+1); err != nil {
		t.Fatalf("begin run-fence restore: %v", err)
	}
	authoritative := createFenceLocalRun(t, database, "account", account.ID, constant.TaskTypeAccountRebuild,
		site.ID, pauseAt, "success", now+2)
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", authoritative.ID).Update("active_key", nil).Error; err != nil {
		t.Fatalf("clear authoritative run key: %v", err)
	}
	end := pauseAt + 3600
	activeKey, err := CollectionRunActiveKey(constant.TaskTypeAccountRebuild, "account", account.ID, &pauseAt, &end)
	if err != nil {
		t.Fatalf("build competing run key: %v", err)
	}
	competingRun := CollectionRun{
		TaskType: constant.TaskTypeAccountRebuild, TargetType: "account", TargetID: account.ID,
		TriggerType: constant.CollectionTriggerDependency, StartTimestamp: &pauseAt, EndTimestamp: &end,
		Scope: []byte("{}"), ActiveKey: &activeKey, Status: "pending",
		Priority: constant.CollectionPriorityLocalRebuild, NextAttemptAt: now + 3,
		CreatedRequestID: "req_run_fence_writer", LastRequestID: "req_run_fence_writer",
		CreatedAt: now + 3, UpdatedAt: now + 3,
	}
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id = ?", authoritative.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE target_type = 'account' AND target_id = ?", account.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id = ?", account.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})

	completionDB := gormOnDedicatedConnection(t, database)
	writerDB := gormOnDedicatedConnection(t, database)
	writerStarted := make(chan struct{})
	writerDone := make(chan error, 1)
	err = completionDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, lockErr := LockAccountOperationScope(ctx, tx, account.ID); lockErr != nil {
			return lockErr
		}
		go func() {
			writerDone <- writerDB.WithContext(ctx).Transaction(func(writer *gorm.DB) error {
				close(writerStarted)
				scope, lockErr := LockAccountOperationScope(ctx, writer, account.ID)
				if lockErr != nil {
					return lockErr
				}
				if scope.Account.ManagedStatus != AccountManagedStatusArchived {
					return ErrAccountRestoreContract
				}
				return writer.Create(&competingRun).Error
			})
		}()
		<-writerStarted
		select {
		case writerErr := <-writerDone:
			return fmt.Errorf("run writer crossed account operation scope before completion: %w", writerErr)
		case <-time.After(100 * time.Millisecond):
		}
		var newerCount int64
		if countErr := database.GORM.Model(&CollectionRun{}).
			Where("target_type = 'account' AND target_id = ? AND id > ?", account.ID, authoritative.ID).
			Count(&newerCount).Error; countErr != nil {
			return countErr
		}
		if newerCount != 0 {
			return fmt.Errorf("newer account rebuild run became visible before scope release")
		}
		return NewAccountRepository(tx).CompleteRestoreInTransaction(ctx, account.ID, now+4)
	})
	if err != nil {
		t.Fatalf("complete restore under run writer contention: %v", err)
	}
	if writerErr := <-writerDone; !errors.Is(writerErr, ErrAccountRestoreContract) {
		t.Fatalf("run writer post-fence recheck error = %v", writerErr)
	}
	var latest CollectionRun
	if err := database.GORM.Where("target_type = 'account' AND target_id = ? AND task_type = ?",
		account.ID, constant.TaskTypeAccountRebuild).Order("id DESC").First(&latest).Error; err != nil {
		t.Fatalf("read latest run after fenced completion: %v", err)
	}
	completed, err := NewAccountRepository(database.GORM).FindByID(ctx, account.ID)
	if err != nil || latest.ID != authoritative.ID || completed.ManagedStatus != AccountManagedStatusActive || completed.StatisticsPausedAt != nil {
		t.Fatalf("fenced completion latest=%#v account=%#v err=%v", latest, completed, err)
	}
}

type b4ConcurrentLifecycleResults struct {
	first  error
	second error
}

func runB4ConcurrentLifecycleOperations(
	ctx context.Context,
	first func() error,
	second func() error,
) b4ConcurrentLifecycleResults {
	start := make(chan struct{})
	type operationResult struct {
		index int
		err   error
	}
	results := make(chan operationResult, 2)
	go func() {
		<-start
		results <- operationResult{index: 0, err: first()}
	}()
	go func() {
		<-start
		results <- operationResult{index: 1, err: second()}
	}()
	close(start)
	var combined b4ConcurrentLifecycleResults
	for range 2 {
		select {
		case result := <-results:
			if result.index == 0 {
				combined.first = result.err
			} else {
				combined.second = result.err
			}
		case <-ctx.Done():
			if combined.first == nil {
				combined.first = ctx.Err()
			}
			if combined.second == nil {
				combined.second = ctx.Err()
			}
			return combined
		}
	}
	return combined
}

func assertB4AccountRestoreBoundary(
	t *testing.T,
	repository *AccountRepository,
	accountID, pauseAt int64,
	backfillStatus string,
) {
	t.Helper()
	account, err := repository.FindByID(context.Background(), accountID)
	if err != nil || account.ManagedStatus != AccountManagedStatusArchived || account.StatisticsPausedAt == nil ||
		*account.StatisticsPausedAt != pauseAt || account.StatisticsBackfillStatus != backfillStatus {
		t.Fatalf("restore boundary account = %#v, %v", account, err)
	}
}

func setB4RunStatus(t *testing.T, database *Database, runID int64, status string, updatedAt int64) {
	t.Helper()
	updates := map[string]any{"status": status, "updated_at": updatedAt}
	if status == "success" || status == "failed" {
		updates["active_key"] = nil
	}
	if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", runID).Updates(updates).Error; err != nil {
		t.Fatalf("set run %d status %s: %v", runID, status, err)
	}
}

func accountRepositoryOnDedicatedConnection(t *testing.T, database *Database) *AccountRepository {
	t.Helper()
	return NewAccountRepository(gormOnDedicatedConnection(t, database))
}

func gormOnDedicatedConnection(t *testing.T, database *Database) *gorm.DB {
	t.Helper()
	connection, err := database.SQL.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve dedicated MySQL connection: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	db, err := gorm.Open(mysqlgorm.New(mysqlgorm.Config{
		Conn: connection, SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open GORM on dedicated connection: %v", err)
	}
	return db
}

func TestAccountDeleteMetadataBlockersAndCleanupRollback(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := int64(1_752_600_000)
	site := Site{
		Name: "B4 Delete Site " + suffix, BaseURL: "https://b4-delete-" + suffix + ".example", ConfigVersion: 1,
		ManagementStatus: "active", OnlineStatus: "online", AuthStatus: "authorized",
		StatisticsStatus: "ready", HealthStatus: "ok", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create delete site: %v", err)
	}
	customer := Customer{Name: "B4 Delete Customer " + suffix, Status: "using", StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create delete customer: %v", err)
	}
	account := Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 8201, RemoteCreatedAt: now - 3600,
		Username: "delete-user", RemoteStatus: 1, RemoteState: AccountRemoteStateNormal,
		ManagedStatus: AccountManagedStatusActive, StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&account).Error; err != nil {
		t.Fatalf("create delete account: %v", err)
	}
	accountRun := createFenceLocalRun(t, database, "account", account.ID, "account_rebuild", site.ID, now-7200, "pending", now)
	customerRun := createFenceLocalRun(t, database, "customer", customer.ID, "customer_rebuild", site.ID, now-7200, "pending", now)
	ruleID := createB4AlertRule(t, database, "b4_account_delete_"+suffix, now)
	t.Cleanup(func() {
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_delivery WHERE alert_event_id IN (SELECT id FROM alert_event WHERE rule_id = ?)", ruleID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_event WHERE rule_id = ?", ruleID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM alert_rule WHERE id = ?", ruleID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run_window WHERE run_id IN (?, ?)", accountRun.ID, customerRun.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM collection_run WHERE id IN (?, ?)", accountRun.ID, customerRun.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM account WHERE id = ?", account.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM customer WHERE id = ?", customer.ID)
		_, _ = database.SQL.ExecContext(context.Background(), "DELETE FROM site WHERE id = ?", site.ID)
	})
	repository := NewAccountRepository(database.GORM)
	dependencies, err := repository.DeletionDependencies(ctx, account.ID)
	if err != nil || dependencies.ActiveRuns != 2 {
		t.Fatalf("polymorphic active runs = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, account.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete with active polymorphic runs = %v", err)
	}
	for _, run := range []CollectionRun{accountRun, customerRun} {
		if err := database.GORM.Model(&CollectionRun{}).Where("id = ?", run.ID).
			Updates(map[string]any{"status": "success", "active_key": nil, "updated_at": now + 1}).Error; err != nil {
			t.Fatalf("finish run %d: %v", run.ID, err)
		}
	}

	activeAlertID := createB4AlertEvent(t, database, ruleID, "b4_account_delete_"+suffix, &site.ID,
		"account", fmt.Sprintf("%d", account.ID), "firing", true, now)
	dependencies, err = repository.DeletionDependencies(ctx, account.ID)
	if err != nil || dependencies.ActiveAlerts != 1 {
		t.Fatalf("active alert dependencies = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, account.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete with firing alert = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE alert_event
SET status = 'resolved', active_key = NULL, resolved_at = ?, updated_at = ? WHERE id = ?`, now+2, now+2, activeAlertID); err != nil {
		t.Fatalf("resolve valuable alert: %v", err)
	}
	dependencies, err = repository.DeletionDependencies(ctx, account.ID)
	if err != nil || dependencies.AlertHistory != 1 {
		t.Fatalf("valuable alert history dependencies = %#v, %v", dependencies, err)
	}
	if err := repository.DeleteByID(ctx, account.ID); !errors.Is(err, ErrDeleteHasDependencies) {
		t.Fatalf("delete with alert history = %v", err)
	}
	if _, err := database.SQL.ExecContext(ctx, "DELETE FROM alert_event WHERE id = ?", activeAlertID); err != nil {
		t.Fatalf("remove valuable alert fixture: %v", err)
	}
	clearableAlertID := createB4AlertEvent(t, database, ruleID, "b4_account_delete_"+suffix, &site.ID,
		"account", fmt.Sprintf("%d", account.ID), "resolved", false, now)

	lockConnection, err := database.SQL.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve alert lock connection: %v", err)
	}
	lockTransaction, err := lockConnection.BeginTx(ctx, nil)
	if err != nil {
		_ = lockConnection.Close()
		t.Fatalf("begin alert lock transaction: %v", err)
	}
	var lockedAlertID int64
	if err := lockTransaction.QueryRowContext(ctx, "SELECT id FROM alert_event WHERE id = ? FOR UPDATE", clearableAlertID).Scan(&lockedAlertID); err != nil {
		_ = lockTransaction.Rollback()
		_ = lockConnection.Close()
		t.Fatalf("lock clearable alert: %v", err)
	}
	deleteContext, cancelDelete := context.WithTimeout(ctx, 300*time.Millisecond)
	deleteErr := repository.DeleteByID(deleteContext, account.ID)
	cancelDelete()
	_ = lockTransaction.Rollback()
	_ = lockConnection.Close()
	if deleteErr == nil || errors.Is(deleteErr, ErrDeleteHasDependencies) {
		t.Fatalf("forced metadata cleanup error = %v", deleteErr)
	}
	assertB4RowCount(t, database, "account", "id = ?", account.ID, 1)
	assertB4RowCount(t, database, "collection_run", "id = ?", accountRun.ID, 1)
	assertB4RowCount(t, database, "collection_run_window", "run_id = ?", accountRun.ID, 1)
	assertB4RowCount(t, database, "alert_event", "id = ?", clearableAlertID, 1)
	if err := repository.DeleteByID(ctx, account.ID); err != nil {
		t.Fatalf("delete after cleanup failure removed: %v", err)
	}
	assertB4RowCount(t, database, "account", "id = ?", account.ID, 0)
	assertB4RowCount(t, database, "collection_run", "id = ?", accountRun.ID, 0)
	assertB4RowCount(t, database, "collection_run_window", "run_id = ?", accountRun.ID, 0)
	assertB4RowCount(t, database, "alert_event", "id = ?", clearableAlertID, 0)
	// A terminal customer-scoped run is shared metadata and is not owned by one account.
	assertB4RowCount(t, database, "collection_run", "id = ?", customerRun.ID, 1)
}

func createB4AlertRule(t *testing.T, database *Database, ruleKey string, now int64) int64 {
	t.Helper()
	result, err := database.SQL.ExecContext(context.Background(), `INSERT INTO alert_rule
  (rule_key, name, enabled, level, metric, compare_operator, threshold_value, for_times,
   scope_type, scope_id, created_at, updated_at)
VALUES (?, ?, 1, 'critical', 'account.remote_exists', '==', 0, 1, 'global', 0, ?, ?)`, ruleKey, ruleKey, now, now)
	if err != nil {
		t.Fatalf("create B4 alert rule: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read B4 alert rule ID: %v", err)
	}
	return id
}

func createB4AlertEvent(
	t *testing.T,
	database *Database,
	ruleID int64,
	ruleKey string,
	siteID *int64,
	targetType, targetKey, status string,
	fired bool,
	now int64,
) int64 {
	t.Helper()
	var activeKey any
	var firstFiredAt any
	var lastFiredAt any
	var resolvedAt any
	if status != "resolved" {
		activeKey = ruleKey + ":" + targetType + ":" + targetKey
	}
	if fired {
		firstFiredAt = now
		lastFiredAt = now
	}
	if status == "resolved" {
		resolvedAt = now
	}
	result, err := database.SQL.ExecContext(context.Background(), `INSERT INTO alert_event
  (rule_id, rule_key, site_id, target_type, target_key, active_key, level, status, consecutive_count,
   message_code, message, first_observed_at, first_fired_at, last_fired_at, resolved_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'critical', ?, 1, 'account_missing', 'b4 test alert', ?, ?, ?, ?, ?, ?)`,
		ruleID, ruleKey, siteID, targetType, targetKey, activeKey, status, now, firstFiredAt, lastFiredAt, resolvedAt, now, now)
	if err != nil {
		t.Fatalf("create B4 alert event: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read B4 alert event ID: %v", err)
	}
	return id
}

func assertB4RowCount(t *testing.T, database *Database, table, predicate string, argument any, expected int64) {
	t.Helper()
	var count int64
	if err := database.SQL.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE "+predicate, argument).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != expected {
		t.Fatalf("%s count = %d, want %d", table, count, expected)
	}
}
