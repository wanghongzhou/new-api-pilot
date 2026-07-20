package integration_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestA11A21A32A33A34A35A54A80A81AccountCustomerAcceptance(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	now := int64(1_768_622_400)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCoreSiteClient(now)
	factory := &coreSiteClientFactory{client: client}
	accounts, err := service.NewAccountService(service.AccountServiceOptions{
		Database: database, ClientFactory: factory, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create account service: %v", err)
	}
	customers, err := service.NewCustomerService(service.CustomerServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	site := createCoreAuthorizedSite(t, database, cipher, now)

	remoteID := int64(701)
	remote := dto.UpstreamUser{
		UpstreamIdentity: dto.UpstreamIdentity{ID: remoteID, Username: "managed-user", DisplayName: "Managed User", Role: 1, Status: 1},
		CreatedAt:        now - 7200, Quota: 100, UsedQuota: 10, RequestCount: 2,
	}
	client.users[remoteID] = remote

	notUsing := createCoreCustomer(t, database, now, dto.CustomerStatusSigning)
	_, err = accounts.Create(context.Background(), dto.AccountCreateRequest{
		SiteID: strconv.FormatInt(site.ID, 10), CustomerID: strconv.FormatInt(notUsing.ID, 10), RemoteUserID: strconv.FormatInt(remoteID, 10),
	}, "a21-customer-not-using")
	if !errors.Is(err, service.ErrCustomerInvalidState) {
		t.Fatalf("A21 non-using customer account creation error = %v", err)
	}
	var rejectedCount int64
	if err := database.Model(&model.Account{}).Where("site_id = ? AND customer_id = ?", site.ID, notUsing.ID).Count(&rejectedCount).Error; err != nil || rejectedCount != 0 {
		t.Fatalf("A21 created an account for a non-using customer: count=%d err=%v", rejectedCount, err)
	}

	using := createCoreCustomer(t, database, now, dto.CustomerStatusUsing)
	created, err := accounts.Create(context.Background(), dto.AccountCreateRequest{
		SiteID: strconv.FormatInt(site.ID, 10), CustomerID: strconv.FormatInt(using.ID, 10), RemoteUserID: strconv.FormatInt(remoteID, 10), Remark: "fixed binding",
	}, "a35-create")
	if err != nil || created.SiteID != strconv.FormatInt(site.ID, 10) || created.CustomerID != strconv.FormatInt(using.ID, 10) || created.RemoteUserID != strconv.FormatInt(remoteID, 10) {
		t.Fatalf("A35 account creation = %#v, %v", created, err)
	}
	accountID, err := strconv.ParseInt(created.ID, 10, 64)
	if err != nil || accountID <= 0 {
		t.Fatalf("A35 created account ID = %q, %v", created.ID, err)
	}
	persisted, err := model.NewAccountRepository(database).FindByID(context.Background(), accountID)
	if err != nil || persisted.SiteID != site.ID || persisted.CustomerID != using.ID || persisted.RemoteUserID != remoteID || persisted.RemoteCreatedAt != remote.CreatedAt {
		t.Fatalf("A35 persisted fixed binding = %#v, err=%v", persisted, err)
	}
	if _, err := accounts.Create(context.Background(), dto.AccountCreateRequest{
		SiteID: strconv.FormatInt(site.ID, 10), CustomerID: strconv.FormatInt(using.ID, 10), RemoteUserID: strconv.FormatInt(remoteID, 10),
	}, "a35-duplicate"); !errors.Is(err, service.ErrAccountAlreadyManaged) {
		t.Fatalf("A35 duplicate site/user binding error = %v", err)
	}
	if database.Migrator().HasTable("customer_account") {
		t.Fatal("A35 unexpected customer_account binding table exists")
	}

	repository := model.NewAccountRepository(database)
	firstMissing, applied, err := repository.MarkMissing(context.Background(), accountID, now+1, now+1)
	if err != nil || !applied || firstMissing.RemoteMissingCount != 1 || firstMissing.RemoteState != model.AccountRemoteStateNormal {
		t.Fatalf("A11/A34 first complete absence = %#v applied=%t err=%v", firstMissing, applied, err)
	}
	secondMissing, applied, err := repository.MarkMissing(context.Background(), accountID, now+2, now+2)
	if err != nil || !applied || secondMissing.RemoteMissingCount != 2 || secondMissing.RemoteState != model.AccountRemoteStateMissing {
		t.Fatalf("A11/A34 second complete absence = %#v applied=%t err=%v", secondMissing, applied, err)
	}
	recovered, applied, err := repository.ApplyAuthoritativeRemoteSnapshot(context.Background(), accountID, model.AuthoritativeAccountRemoteSnapshot{
		RemoteCreatedAt: remote.CreatedAt, Username: "reappeared-user", DisplayName: "Reappeared", RemoteGroup: "",
		RemoteStatus: 1, Quota: 101, UsedQuota: 11, RequestCount: 3, ObservedAt: now + 3, UpdatedAt: now + 3,
	})
	if err != nil || !applied || recovered.RemoteState != model.AccountRemoteStateNormal || recovered.RemoteMissingCount != 0 || recovered.Username != "reappeared-user" {
		t.Fatalf("A11/A34 authoritative reappearance = %#v applied=%t err=%v", recovered, applied, err)
	}

	clock.Advance(time.Hour)
	changedIdentity := client.users[remoteID]
	changedIdentity.CreatedAt++
	client.users[remoteID] = changedIdentity
	if _, err := accounts.Refresh(context.Background(), accountID, "a54-identity-mismatch"); err != nil {
		t.Fatalf("A54 refresh identity mismatch: %v", err)
	}
	mismatched, err := repository.FindByID(context.Background(), accountID)
	if err != nil || mismatched.RemoteState != model.AccountRemoteStateIdentityMismatch || mismatched.StatisticsPausedAt == nil {
		t.Fatalf("A54 identity mismatch state = %#v, err=%v", mismatched, err)
	}

	softDeletedID := int64(702)
	softDeleted := remote
	softDeleted.ID = softDeletedID
	softDeleted.Username = "soft-deleted-user"
	softDeleted.Deleted = true
	client.snapshot = dto.UpstreamUserSnapshot{Total: 1, Items: []dto.UpstreamUser{softDeleted}}
	search, err := accounts.SearchRemoteUsers(context.Background(), site.ID, dto.RemoteUserListQuery{Page: 1, PageSize: 20}, "a80-search")
	if err != nil || search.Total != 0 || len(search.Items) != 0 {
		t.Fatalf("A80 soft-deleted remote user search = %#v, %v", search, err)
	}
	softDeletedAccount := createCoreAccount(t, database, site.ID, using.ID, softDeletedID, now)
	firstSoftMissing, applied, err := repository.MarkMissing(context.Background(), softDeletedAccount.ID, now+4, now+4)
	if err != nil || !applied || firstSoftMissing.RemoteState != model.AccountRemoteStateNormal {
		t.Fatalf("A80 first soft-delete absence = %#v applied=%t err=%v", firstSoftMissing, applied, err)
	}
	secondSoftMissing, applied, err := repository.MarkMissing(context.Background(), softDeletedAccount.ID, now+5, now+5)
	if err != nil || !applied || secondSoftMissing.RemoteState != model.AccountRemoteStateMissing {
		t.Fatalf("A80 second soft-delete absence = %#v applied=%t err=%v", secondSoftMissing, applied, err)
	}

	// A32 starts after the initial account backfill has already completed. Use a
	// normal, unpaused account rather than the active initial rebuild from A35.
	archivedFixture := createCoreAccount(t, database, site.ID, using.ID, 703, now)
	archiveID := archivedFixture.ID
	if _, err := accounts.Archive(context.Background(), archiveID); err != nil {
		t.Fatalf("A32 archive account: %v", err)
	}
	restoreRun, err := accounts.Restore(context.Background(), archiveID, "a32-restore")
	if err != nil || restoreRun.Status != "pending" || restoreRun.TargetType != "account" || restoreRun.TargetID != strconv.FormatInt(archiveID, 10) {
		t.Fatalf("A32 restore run = %#v, %v", restoreRun, err)
	}
	archivedDuringRestore, err := repository.FindByID(context.Background(), archiveID)
	if err != nil || archivedDuringRestore.ManagedStatus != model.AccountManagedStatusArchived || archivedDuringRestore.StatisticsPausedAt == nil {
		t.Fatalf("A32 restore prematurely opened statistics = %#v, err=%v", archivedDuringRestore, err)
	}
	duplicateRestore, err := accounts.Restore(context.Background(), archiveID, "a32-restore-duplicate")
	if err != nil || !duplicateRestore.Deduplicated || duplicateRestore.ID != restoreRun.ID {
		t.Fatalf("A32 duplicate restore = %#v, %v", duplicateRestore, err)
	}

	customerLifecycle := createCoreCustomer(t, database, now, dto.CustomerStatusUsing)
	if _, err := customers.Disable(context.Background(), customerLifecycle.ID); err != nil {
		t.Fatalf("A33 disable customer: %v", err)
	}
	customerRestore, err := customers.Enable(context.Background(), customerLifecycle.ID, "a33-enable")
	if err != nil || customerRestore.Status != "pending" || customerRestore.TargetType != "customer" {
		t.Fatalf("A33 enable customer run = %#v, %v", customerRestore, err)
	}
	disabledCustomer, err := model.NewCustomerRepository(database).FindByID(context.Background(), customerLifecycle.ID)
	if err != nil || disabledCustomer.Status != dto.CustomerStatusDisabled || disabledCustomer.StatisticsPausedAt == nil {
		t.Fatalf("A33 customer became available before its rebuild completed = %#v, err=%v", disabledCustomer, err)
	}

	refreshRemoteID := int64(704)
	refreshRemote := remote
	refreshRemote.ID = refreshRemoteID
	refreshRemote.Username = "before-sync"
	client.users[refreshRemoteID] = refreshRemote
	refreshedDetail, err := accounts.Create(context.Background(), dto.AccountCreateRequest{
		SiteID: strconv.FormatInt(site.ID, 10), CustomerID: strconv.FormatInt(using.ID, 10), RemoteUserID: strconv.FormatInt(refreshRemoteID, 10),
	}, "a81-create")
	if err != nil {
		t.Fatalf("A81 create refresh account: %v", err)
	}
	refreshAccountID, _ := strconv.ParseInt(refreshedDetail.ID, 10, 64)
	clock.Advance(time.Hour)
	refreshRemote.Username = "after-sync"
	refreshRemote.DisplayName = "After Sync"
	refreshRemote.Quota = 200
	client.users[refreshRemoteID] = refreshRemote
	if _, err := accounts.Refresh(context.Background(), refreshAccountID, "a81-user-rename"); err != nil {
		t.Fatalf("A81 account refresh: %v", err)
	}
	refreshed, err := repository.FindByID(context.Background(), refreshAccountID)
	if err != nil || refreshed.Username != "after-sync" || refreshed.DisplayName != "After Sync" || refreshed.Quota != 200 {
		t.Fatalf("A81 authoritative user refresh = %#v, err=%v", refreshed, err)
	}
}
