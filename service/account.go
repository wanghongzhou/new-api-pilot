package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"

	"gorm.io/gorm"
)

var (
	ErrAccountNotFound               = errors.New("account not found")
	ErrAccountInvalid                = errors.New("account is invalid")
	ErrAccountAlreadyManaged         = errors.New("remote user is already managed")
	ErrAccountInvalidState           = errors.New("account state does not allow this operation")
	ErrAccountDeleteRestricted       = errors.New("account has deletion dependencies")
	ErrAccountRemoteUserNotFound     = errors.New("upstream user not found")
	ErrAccountRemoteIdentityConflict = errors.New("upstream user identity conflicts with the selected binding")
)

type AccountService struct {
	db         *gorm.DB
	clients    SiteClientFactory
	cipher     *common.Cipher
	clock      common.Clock
	statistics EntityStatisticsReader
	postCommit PostCommitNotifier
}

type AccountServiceOptions struct {
	Database      *gorm.DB
	ClientFactory SiteClientFactory
	Cipher        *common.Cipher
	Clock         common.Clock
	Statistics    EntityStatisticsReader
	PostCommit    PostCommitNotifier
}

func NewAccountService(options AccountServiceOptions) (*AccountService, error) {
	if options.Database == nil || options.ClientFactory == nil || options.Cipher == nil || options.Clock == nil {
		return nil, errors.New("account service dependencies are required")
	}
	return &AccountService{
		db: options.Database, clients: options.ClientFactory, cipher: options.Cipher, clock: options.Clock,
		statistics: options.Statistics, postCommit: options.PostCommit,
	}, nil
}

func (service *AccountService) Create(ctx context.Context, request dto.AccountCreateRequest, requestID string) (dto.AccountDetail, error) {
	if request.Validate() != nil {
		return dto.AccountDetail{}, ErrAccountInvalid
	}
	siteID, customerID, remoteUserID, err := request.BindingIDs()
	if err != nil {
		return dto.AccountDetail{}, ErrAccountInvalid
	}
	originalSite, client, err := service.authenticatedSiteClient(ctx, siteID)
	if err != nil {
		return dto.AccountDetail{}, err
	}
	defer client.CloseIdleConnections()
	remote, err := client.GetUser(ctx, requestID, remoteUserID)
	if err != nil {
		if errors.Is(err, ErrUpstreamUserNotFound) {
			return dto.AccountDetail{}, ErrAccountRemoteUserNotFound
		}
		if errors.Is(err, ErrUpstreamUserIdentityConflict) {
			return dto.AccountDetail{}, ErrAccountRemoteIdentityConflict
		}
		return dto.AccountDetail{}, err
	}
	now := service.clock.Now().Unix()
	if err := validateExactRemoteUser(remote, remoteUserID, now); err != nil {
		return dto.AccountDetail{}, err
	}

	var accountID int64
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sites := model.NewSiteRepository(tx)
		currentSite, err := sites.FindByIDForUpdate(ctx, siteID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrSiteNotFound
			}
			return err
		}
		if currentSite.ConfigVersion != originalSite.ConfigVersion || currentSite.BaseURL != originalSite.BaseURL {
			return newSiteConfigChangedError(siteID, originalSite.ConfigVersion, currentSite.ConfigVersion)
		}
		if err := validateAccountSite(currentSite); err != nil {
			return err
		}
		ready, err := requiredCapabilitiesReady(ctx, sites, siteID)
		if err != nil {
			return err
		}
		if !ready {
			return ErrSiteCapabilitiesPending
		}
		customer, err := model.NewCustomerRepository(tx).FindByIDForUpdate(ctx, customerID)
		if err != nil {
			if model.IsNotFound(err) {
				return ErrCustomerNotFound
			}
			return err
		}
		if customer.Status != dto.CustomerStatusUsing {
			return ErrCustomerInvalidState
		}
		if _, err := model.NewAccountRepository(tx).FindBySiteAndRemoteUserForUpdate(ctx, siteID, remoteUserID); err == nil {
			return ErrAccountAlreadyManaged
		} else if !model.IsNotFound(err) {
			return err
		}
		observedAt := now
		account := model.Account{
			SiteID: siteID, CustomerID: customerID, RemoteUserID: remoteUserID,
			RemoteCreatedAt: remote.CreatedAt, Username: remote.Username, DisplayName: remote.DisplayName,
			RemoteGroup: remote.Group, RemoteStatus: int(remote.Status), RemoteState: model.AccountRemoteStateNormal,
			LastRemoteSeenAt: &observedAt, Quota: remote.Quota, UsedQuota: remote.UsedQuota,
			RequestCount: remote.RequestCount, ManagedStatus: model.AccountManagedStatusActive,
			StatisticsBackfillStatus: "pending", LastSyncedAt: &observedAt, Remark: request.Remark,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := model.NewAccountRepository(tx).Create(ctx, &account); err != nil {
			if model.IsDuplicateKey(err) {
				return ErrAccountAlreadyManaged
			}
			return err
		}
		accountID = account.ID
		scope, err := model.LockAccountOperationScope(ctx, tx, account.ID)
		if err != nil {
			return err
		}
		if scope.Site.ConfigVersion != originalSite.ConfigVersion {
			return newSiteConfigChangedError(siteID, originalSite.ConfigVersion, scope.Site.ConfigVersion)
		}
		if scope.Customer.Status != dto.CustomerStatusUsing {
			return ErrCustomerInvalidState
		}
		run, err := newLocalRebuildRun("account", account.ID, constant.TaskTypeAccountRebuild,
			remote.CreatedAt, floorHour(now), requestID, now)
		if err != nil {
			return err
		}
		if _, _, err := sites.CreateOrGetRun(ctx, &run); err != nil {
			return err
		}
		customerBackfillStatus, err := aggregateCustomerBackfillStatus(ctx, tx, customerID)
		if err != nil {
			return err
		}
		customerUpdatedAt := monotonicMutationTime(now, customer.UpdatedAt)
		return tx.WithContext(ctx).Model(&model.Customer{}).Where("id = ?", customerID).
			Updates(map[string]any{"statistics_backfill_status": customerBackfillStatus, "updated_at": customerUpdatedAt}).Error
	})
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	service.notifyAccountUserAfterCommit(ctx, siteID, accountID, now)
	return service.Get(ctx, accountID)
}

func (service *AccountService) Get(ctx context.Context, id int64) (dto.AccountDetail, error) {
	account, err := model.NewAccountRepository(service.db).FindByID(ctx, id)
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	item, err := service.listItemFromModel(ctx, account)
	if err != nil {
		return dto.AccountDetail{}, err
	}
	return dto.AccountDetail{
		AccountListItem: item, Remark: account.Remark, RemoteMissingCount: account.RemoteMissingCount,
		LastRemoteSeenAt: account.LastRemoteSeenAt, StatisticsPausedAt: account.StatisticsPausedAt,
		Completeness: entityCompleteness(account.StatisticsPausedAt != nil, "hour"),
		CreatedAt:    account.CreatedAt,
	}, nil
}

func (service *AccountService) List(ctx context.Context, query dto.AccountListQuery) (common.PageData[dto.AccountListItem], error) {
	filter := model.AccountFilter{
		Keyword: query.Keyword, RemoteStatus: query.RemoteStatus, RemoteState: query.RemoteState,
		ManagedStatus: query.ManagedStatus, SortBy: query.SortBy, SortOrder: query.SortOrder,
		TodayDateKey: beijingDateKey(service.clock.Now()), Offset: query.Offset(), Limit: query.PageSize,
	}
	if query.SiteID != "" {
		value, _ := strconv.ParseInt(query.SiteID, 10, 64)
		filter.SiteID = &value
	}
	if query.CustomerID != "" {
		value, _ := strconv.ParseInt(query.CustomerID, 10, 64)
		filter.CustomerID = &value
	}
	repository := model.NewAccountRepository(service.db)
	accounts, total, err := repository.List(ctx, filter)
	if err != nil {
		return common.PageData[dto.AccountListItem]{}, fmt.Errorf("list accounts: %w", err)
	}
	accountIDs := make([]int64, len(accounts))
	for index := range accounts {
		accountIDs[index] = accounts[index].ID
	}
	metadata, err := repository.LoadListMetadata(ctx, accountIDs)
	if err != nil {
		return common.PageData[dto.AccountListItem]{}, err
	}
	items := make([]dto.AccountListItem, 0, len(accounts))
	for _, account := range accounts {
		item, err := accountListItemFromMetadata(account, metadata[account.ID])
		if err != nil {
			return common.PageData[dto.AccountListItem]{}, err
		}
		items = append(items, item)
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *AccountService) Update(ctx context.Context, id int64, request dto.AccountUpdateRequest) (dto.AccountDetail, error) {
	if request.Validate() != nil {
		return dto.AccountDetail{}, ErrAccountInvalid
	}
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockAccountOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), scope.Account.UpdatedAt)
		return model.NewAccountRepository(tx).UpdateRemark(ctx, id, request.Remark, committedAt)
	})
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	service.notifyAccountLifecycleAfterCommit(ctx, id, committedAt)
	return service.Get(ctx, id)
}

func (service *AccountService) Delete(ctx context.Context, id int64) error {
	return accountServiceError(model.NewAccountRepository(service.db).DeleteByID(ctx, id))
}

func (service *AccountService) Archive(ctx context.Context, id int64) (dto.AccountDetail, error) {
	account, err := model.NewAccountRepository(service.db).FindByID(ctx, id)
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	if account.ManagedStatus == model.AccountManagedStatusArchived {
		return service.Get(ctx, id)
	}
	now := service.clock.Now().Unix()
	committedAt := int64(0)
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockAccountOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		committedAt = monotonicMutationTime(now, scope.Account.UpdatedAt)
		return model.NewAccountRepository(tx).Archive(ctx, id, floorHour(now), committedAt)
	})
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	service.notifyAccountLifecycleAfterCommit(ctx, id, committedAt)
	return service.Get(ctx, id)
}

func (service *AccountService) Restore(ctx context.Context, id int64, requestID string) (dto.CollectionRunItem, error) {
	var result dto.CollectionRunItem
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockAccountOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := validateAccountSite(scope.Site); err != nil {
			return err
		}
		ready, err := requiredCapabilitiesReady(ctx, model.NewSiteRepository(tx), scope.Site.ID)
		if err != nil {
			return err
		}
		if !ready {
			return ErrSiteCapabilitiesPending
		}
		if scope.Customer.Status == dto.CustomerStatusDisabled || scope.Account.RemoteState == model.AccountRemoteStateIdentityMismatch ||
			scope.Account.ManagedStatus != model.AccountManagedStatusArchived || scope.Account.StatisticsPausedAt == nil {
			return ErrAccountInvalidState
		}
		now := service.clock.Now().Unix()
		committedAt = monotonicMutationTime(now, scope.Account.UpdatedAt)
		start := floorHour(*scope.Account.StatisticsPausedAt)
		end := floorHour(now)
		if end < start {
			end = start
		}
		if existing, found, err := activeLocalRebuildRun(ctx, tx, "account", id, constant.TaskTypeAccountRebuild); err != nil {
			return err
		} else if found {
			if !sameRunRange(existing, start, end) {
				return ErrEntityBackfillRunning
			}
			result = collectionRunItemFromModel(existing, true)
			return nil
		}
		if err := model.NewAccountRepository(tx).BeginRestoreInTransaction(ctx, id, committedAt); err != nil {
			return err
		}
		run, err := newLocalRebuildRun("account", id, constant.TaskTypeAccountRebuild,
			*scope.Account.StatisticsPausedAt, floorHour(now), requestID, committedAt)
		if err != nil {
			return err
		}
		created, deduplicated, err := model.NewSiteRepository(tx).CreateOrGetRun(ctx, &run)
		if err != nil {
			return err
		}
		result = collectionRunItemFromModel(created, deduplicated)
		return nil
	})
	if err != nil {
		return dto.CollectionRunItem{}, accountServiceError(err)
	}
	service.notifyAccountLifecycleAfterCommit(ctx, id, committedAt)
	return result, nil
}

// CompleteRestore is the worker-only lifecycle completion boundary.
func (service *AccountService) CompleteRestore(ctx context.Context, id int64) error {
	committedAt := int64(0)
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := model.LockAccountOperationScope(ctx, tx, id)
		if err != nil {
			return err
		}
		committedAt = monotonicMutationTime(service.clock.Now().Unix(), scope.Account.UpdatedAt)
		return model.NewAccountRepository(tx).CompleteRestoreInTransaction(ctx, id, committedAt)
	})
	if err != nil {
		return accountServiceError(err)
	}
	service.notifyAccountLifecycleAfterCommit(ctx, id, committedAt)
	return nil
}

func (service *AccountService) Statistics(ctx context.Context, id int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	if _, err := model.NewAccountRepository(service.db).FindByID(ctx, id); err != nil {
		return dto.StatisticsResponse{}, accountServiceError(err)
	}
	if service.statistics == nil {
		return dto.StatisticsResponse{}, ErrEntityStatisticsUnavailable
	}
	query.AccountIDs = []int64{id}
	return service.statistics.AccountStatistics(ctx, id, query)
}

func (service *AccountService) Refresh(ctx context.Context, id int64, requestID string) (dto.AccountDetail, error) {
	account, err := model.NewAccountRepository(service.db).FindByID(ctx, id)
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	_, client, err := service.authenticatedSiteClient(ctx, account.SiteID)
	if err != nil {
		return dto.AccountDetail{}, err
	}
	defer client.CloseIdleConnections()
	remote, err := client.GetUser(ctx, requestID, account.RemoteUserID)
	if err != nil {
		if errors.Is(err, ErrUpstreamUserNotFound) {
			return dto.AccountDetail{}, ErrAccountRemoteUserNotFound
		}
		if errors.Is(err, ErrUpstreamUserIdentityConflict) {
			return dto.AccountDetail{}, ErrAccountRemoteIdentityConflict
		}
		return dto.AccountDetail{}, err
	}
	now := service.clock.Now().Unix()
	if remote.ID != account.RemoteUserID || remote.CreatedAt <= 0 || remote.CreatedAt > now || remote.Deleted {
		if remote.ID == account.RemoteUserID && remote.Deleted {
			return dto.AccountDetail{}, ErrAccountRemoteUserNotFound
		}
		return service.markIdentityMismatch(ctx, account, now)
	}
	if remote.CreatedAt != account.RemoteCreatedAt {
		return service.markIdentityMismatch(ctx, account, now)
	}
	committed, applied, err := model.NewAccountRepository(service.db).ApplyAuthoritativeRemoteSnapshot(ctx, id,
		model.AuthoritativeAccountRemoteSnapshot{
			RemoteCreatedAt: remote.CreatedAt, Username: remote.Username, DisplayName: remote.DisplayName,
			RemoteGroup: remote.Group, RemoteStatus: int(remote.Status), Quota: remote.Quota,
			UsedQuota: remote.UsedQuota, RequestCount: remote.RequestCount, ObservedAt: now, UpdatedAt: now,
		})
	if err != nil {
		if errors.Is(err, model.ErrAccountIdentityMismatch) {
			return service.markIdentityMismatch(ctx, account, now)
		}
		return dto.AccountDetail{}, accountServiceError(err)
	}
	if applied {
		service.notifyCommittedAccountUser(ctx, committed)
	}
	return service.Get(ctx, id)
}

func (service *AccountService) SearchRemoteUsers(ctx context.Context, siteID int64, query dto.RemoteUserListQuery, requestID string) (common.PageData[dto.RemoteUserItem], error) {
	_, client, err := service.authenticatedSiteClient(ctx, siteID)
	if err != nil {
		return common.PageData[dto.RemoteUserItem]{}, err
	}
	defer client.CloseIdleConnections()
	remoteClient, ok := client.(SiteRemoteUserClient)
	if !ok {
		return common.PageData[dto.RemoteUserItem]{}, ErrSiteIncompatible
	}
	users, err := loadAllRemoteUsers(ctx, remoteClient, requestID, query.Keyword, service.clock.Now().Unix())
	if err != nil {
		return common.PageData[dto.RemoteUserItem]{}, err
	}
	selectable := make([]dto.UpstreamUser, 0, len(users))
	for _, user := range users {
		if !user.Deleted {
			selectable = append(selectable, user)
		}
	}
	total := int64(len(selectable))
	start := query.Offset()
	if start > len(selectable) {
		start = len(selectable)
	}
	end := start + query.PageSize
	if end > len(selectable) {
		end = len(selectable)
	}
	pageUsers := selectable[start:end]
	ids := make([]int64, len(pageUsers))
	for index := range pageUsers {
		ids[index] = pageUsers[index].ID
	}
	bindings, err := model.NewAccountRepository(service.db).FindManagedBindings(ctx, siteID, ids)
	if err != nil {
		return common.PageData[dto.RemoteUserItem]{}, err
	}
	items := make([]dto.RemoteUserItem, 0, len(pageUsers))
	for _, user := range pageUsers {
		item := remoteUserItemFromUpstream(user)
		if binding, found := bindings[user.ID]; found {
			item.AlreadyManaged = true
			accountID := strconv.FormatInt(binding.AccountID, 10)
			item.ManagedAccountID = &accountID
			item.ManagedCustomerName = binding.CustomerName
		}
		items = append(items, item)
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *AccountService) listItemFromModel(ctx context.Context, account model.Account) (dto.AccountListItem, error) {
	site, err := model.NewSiteRepository(service.db).FindByID(ctx, account.SiteID)
	if err != nil {
		return dto.AccountListItem{}, fmt.Errorf("read account site: %w", err)
	}
	customer, err := model.NewCustomerRepository(service.db).FindByID(ctx, account.CustomerID)
	if err != nil {
		return dto.AccountListItem{}, fmt.Errorf("read account customer: %w", err)
	}
	backfill, err := (&CustomerService{db: service.db, clock: service.clock}).localBackfillSummary(ctx, "account", account.ID, account.StatisticsBackfillStatus)
	if err != nil {
		return dto.AccountListItem{}, err
	}
	rate := dto.RateInfo{QuotaPerUnit: site.QuotaPerUnit, USDExchangeRate: site.USDExchangeRate, Source: "unavailable", UpdatedAt: site.LastRateAt}
	if site.QuotaPerUnit != nil && site.USDExchangeRate != nil {
		rate.Source = "site"
	}
	return newAccountListItem(account, site.Name, customer.Name, rate, backfill), nil
}

func accountListItemFromMetadata(account model.Account, metadata model.AccountListMetadata) (dto.AccountListItem, error) {
	if metadata.AccountID != account.ID {
		return dto.AccountListItem{}, model.ErrAccountListContract
	}
	backfill, err := accountBackfillFromMetadata(account.StatisticsBackfillStatus, metadata)
	if err != nil {
		return dto.AccountListItem{}, err
	}
	rate := dto.RateInfo{
		QuotaPerUnit: metadata.QuotaPerUnit, USDExchangeRate: metadata.USDExchangeRate,
		Source: "unavailable", UpdatedAt: metadata.LastRateAt,
	}
	if metadata.QuotaPerUnit != nil && metadata.USDExchangeRate != nil {
		rate.Source = "site"
	}
	return newAccountListItem(account, metadata.SiteName, metadata.CustomerName, rate, backfill), nil
}

func accountBackfillFromMetadata(status string, metadata model.AccountListMetadata) (dto.BackfillSummary, error) {
	if metadata.LatestRunID == nil {
		return dto.BackfillSummary{Status: normalizedBackfillStatus(status)}, nil
	}
	if metadata.LatestRunStatus == nil || metadata.LatestRunTotalWindows == nil ||
		metadata.LatestRunCompletedWindows == nil || metadata.LatestRunFailedWindows == nil {
		return dto.BackfillSummary{}, model.ErrAccountListContract
	}
	run := model.CollectionRun{
		ID: *metadata.LatestRunID, Status: *metadata.LatestRunStatus,
		StartTimestamp: metadata.LatestRunStartTimestamp, EndTimestamp: metadata.LatestRunEndTimestamp,
		TotalWindows: *metadata.LatestRunTotalWindows, CompletedWindows: *metadata.LatestRunCompletedWindows,
		FailedWindows: *metadata.LatestRunFailedWindows,
	}
	return backfillSummaryFromRun(run), nil
}

func newAccountListItem(
	account model.Account,
	siteName, customerName string,
	rate dto.RateInfo,
	backfill dto.BackfillSummary,
) dto.AccountListItem {
	return dto.AccountListItem{
		ID: strconv.FormatInt(account.ID, 10), SiteID: strconv.FormatInt(account.SiteID, 10), SiteName: siteName,
		CustomerID: strconv.FormatInt(account.CustomerID, 10), CustomerName: customerName,
		RemoteUserID: strconv.FormatInt(account.RemoteUserID, 10), RemoteCreatedAt: account.RemoteCreatedAt,
		Username: account.Username, DisplayName: account.DisplayName, RemoteGroup: account.RemoteGroup,
		RemoteStatus: account.RemoteStatus, RemoteState: account.RemoteState, ManagedStatus: account.ManagedStatus,
		Quota: strconv.FormatInt(account.Quota, 10), UsedQuota: strconv.FormatInt(account.UsedQuota, 10),
		RequestCount: strconv.FormatInt(account.RequestCount, 10), Rate: rate, LastSyncedAt: account.LastSyncedAt,
		Today: missingUsageSummary(), Backfill: backfill, UpdatedAt: account.UpdatedAt,
	}
}

func (service *AccountService) authenticatedSiteClient(ctx context.Context, siteID int64) (model.Site, SiteUpstreamClient, error) {
	site, err := model.NewSiteRepository(service.db).FindByID(ctx, siteID)
	if err != nil {
		if model.IsNotFound(err) {
			return model.Site{}, nil, ErrSiteNotFound
		}
		return model.Site{}, nil, err
	}
	if err := validateAccountSite(site); err != nil {
		return model.Site{}, nil, err
	}
	ready, err := requiredCapabilitiesReady(ctx, model.NewSiteRepository(service.db), site.ID)
	if err != nil {
		return model.Site{}, nil, err
	}
	if !ready {
		return model.Site{}, nil, ErrSiteCapabilitiesPending
	}
	plaintext, err := service.cipher.Decrypt(*site.AccessTokenEncrypted, siteTokenAAD(site.ID))
	if err != nil {
		return model.Site{}, nil, ErrSiteIncompatible
	}
	client, err := service.clients.NewAuthenticated(site.BaseURL, site.BaseURL, string(plaintext), *site.RootUserID)
	if err != nil {
		return model.Site{}, nil, err
	}
	return site, client, nil
}

func validateAccountSite(site model.Site) error {
	if !site.DataExportEnabled {
		return ErrSiteExportDisabled
	}
	if site.ManagementStatus != constant.SiteManagementActive || site.AuthStatus != constant.SiteAuthAuthorized ||
		site.StatisticsEndAt != nil || site.RootUserID == nil || site.AccessTokenEncrypted == nil {
		return ErrSiteInvalidState
	}
	return nil
}

func validateExactRemoteUser(user dto.UpstreamUser, expectedID, now int64) error {
	if user.ID != expectedID {
		return ErrAccountRemoteIdentityConflict
	}
	if user.Deleted {
		return ErrAccountRemoteUserNotFound
	}
	if user.CreatedAt <= 0 || user.CreatedAt > now {
		return ErrAccountRemoteIdentityConflict
	}
	return nil
}

func aggregateCustomerBackfillStatus(ctx context.Context, tx *gorm.DB, customerID int64) (string, error) {
	accountIDs := tx.WithContext(ctx).Model(&model.Account{}).Select("id").Where("customer_id = ?", customerID)
	var running int64
	err := tx.WithContext(ctx).Model(&model.CollectionRun{}).
		Where("status = ? AND ((target_type = ? AND target_id = ?) OR (target_type = ? AND target_id IN (?)))",
			"running", "customer", customerID, "account", accountIDs).
		Count(&running).Error
	if err != nil {
		return "", err
	}
	if running > 0 {
		return "running", nil
	}
	return "pending", nil
}

func (service *AccountService) markIdentityMismatch(ctx context.Context, account model.Account, now int64) (dto.AccountDetail, error) {
	committed, applied, err := model.NewAccountRepository(service.db).MarkIdentityMismatch(ctx, account.ID, now, floorHour(now), now)
	if err != nil {
		return dto.AccountDetail{}, accountServiceError(err)
	}
	if applied {
		service.notifyCommittedAccountUser(ctx, committed)
	}
	return service.Get(ctx, account.ID)
}

func (service *AccountService) notifyAccountLifecycleAfterCommit(ctx context.Context, id int64, observedAt int64) {
	if service.postCommit == nil || observedAt <= 0 {
		return
	}
	service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
		Source: AlertSampleSourceLifecycle, ScopeType: "account", ScopeID: id, ObservedAt: observedAt,
	})
}

func (service *AccountService) notifyAccountUserAfterCommit(
	ctx context.Context,
	siteID int64,
	accountID int64,
	observedAt int64,
) {
	if service.postCommit == nil || observedAt <= 0 {
		return
	}
	service.postCommit.NotifyAfterCommit(ctx, AlertPostCommitTrigger{
		Source: AlertSampleSourceUser, SiteID: siteID, AccountID: accountID, ObservedAt: observedAt,
	})
}

func (service *AccountService) notifyCommittedAccountUser(ctx context.Context, account model.Account) {
	if account.LastSyncedAt == nil {
		return
	}
	service.notifyAccountUserAfterCommit(ctx, account.SiteID, account.ID, *account.LastSyncedAt)
}

func loadAllRemoteUsers(ctx context.Context, client SiteRemoteUserClient, requestID, keyword string, now int64) ([]dto.UpstreamUser, error) {
	load := func(page int) (dto.UpstreamUserPage, error) {
		if keyword == "" {
			return client.ListUsersPage(ctx, requestID, page)
		}
		return client.SearchUsers(ctx, requestID, keyword, page)
	}
	first, err := load(1)
	if err != nil {
		return nil, err
	}
	if first.Total < 0 || first.Total > 1_000_000 {
		return nil, ErrUpstreamResponseInvalid
	}
	items := make([]dto.UpstreamUser, 0, minInt64ToInt(first.Total))
	seen := make(map[int64]struct{}, minInt64ToInt(first.Total))
	appendPage := func(page dto.UpstreamUserPage) error {
		for _, user := range page.Items {
			if !validRemoteSearchUser(user, now) {
				return ErrUpstreamResponseInvalid
			}
			if _, duplicate := seen[user.ID]; duplicate {
				return ErrUpstreamResponseInvalid
			}
			seen[user.ID] = struct{}{}
			items = append(items, user)
		}
		if int64(len(items)) > first.Total {
			return ErrUpstreamResponseInvalid
		}
		return nil
	}
	if err := appendPage(first); err != nil {
		return nil, err
	}
	for page := 2; int64(len(items)) < first.Total; page++ {
		next, err := load(page)
		if err != nil {
			return nil, err
		}
		if next.Total != first.Total || len(next.Items) == 0 {
			return nil, ErrUpstreamResponseInvalid
		}
		if err := appendPage(next); err != nil {
			return nil, err
		}
	}
	if int64(len(items)) != first.Total {
		return nil, ErrUpstreamResponseInvalid
	}
	return items, nil
}

func validRemoteSearchUser(user dto.UpstreamUser, now int64) bool {
	return user.ID > 0 && user.CreatedAt > 0 && user.CreatedAt <= now && user.LastLoginAt >= 0 &&
		user.Quota >= 0 && user.UsedQuota >= 0 && user.RequestCount >= 0 &&
		validUpstreamString(user.Username, 1, 255) && validUpstreamString(user.DisplayName, 0, 255) &&
		validUpstreamString(user.Group, 0, 128)
}

func remoteUserItemFromUpstream(user dto.UpstreamUser) dto.RemoteUserItem {
	item := dto.RemoteUserItem{
		ID: strconv.FormatInt(user.ID, 10), Username: user.Username, DisplayName: user.DisplayName,
		Role: int(user.Role), Status: int(user.Status), Group: user.Group,
		Quota: strconv.FormatInt(user.Quota, 10), UsedQuota: strconv.FormatInt(user.UsedQuota, 10),
		RequestCount: strconv.FormatInt(user.RequestCount, 10), CreatedAt: user.CreatedAt,
	}
	if user.LastLoginAt > 0 {
		value := user.LastLoginAt
		item.LastLoginAt = &value
	}
	return item
}

func accountServiceError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case model.IsNotFound(err):
		return ErrAccountNotFound
	case errors.Is(err, model.ErrDeleteHasDependencies):
		return ErrAccountDeleteRestricted
	case errors.Is(err, model.ErrAccountIdentityMismatch), errors.Is(err, model.ErrAccountRestoreContract),
		errors.Is(err, model.ErrRebuildRunNotReady), errors.Is(err, model.ErrCustomerDisabled):
		return ErrAccountInvalidState
	case model.IsDuplicateKey(err):
		return ErrAccountAlreadyManaged
	default:
		return err
	}
}
