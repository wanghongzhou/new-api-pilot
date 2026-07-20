package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type accountTestClient struct {
	*testSiteClient
	listPages   map[int]dto.UpstreamUserPage
	searchPages map[int]dto.UpstreamUserPage
}

func (client *accountTestClient) ListUsersPage(_ context.Context, _ string, page int) (dto.UpstreamUserPage, error) {
	result, exists := client.listPages[page]
	if !exists {
		return dto.UpstreamUserPage{}, ErrUpstreamResponseInvalid
	}
	return result, nil
}

func (client *accountTestClient) SearchUsers(_ context.Context, _ string, _ string, page int) (dto.UpstreamUserPage, error) {
	result, exists := client.searchPages[page]
	if !exists {
		return dto.UpstreamUserPage{}, ErrUpstreamResponseInvalid
	}
	return result, nil
}

func TestAccountCreateCommitsBindingAndParentRunAtomically(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, tx, clock.Now().Unix(), cipher)
	customer := model.Customer{
		Name: "Account Create Customer", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(tx).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	remote := dto.UpstreamUser{
		UpstreamIdentity: dto.UpstreamIdentity{ID: 101, Username: "remote-user", DisplayName: "Remote User", Role: 1, Status: 1, Group: "vip"},
		Quota:            9_007_199_254_740_999, UsedQuota: 20, RequestCount: 30, CreatedAt: clock.Now().Unix() - 7200,
	}
	client := &accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}
	client.root = remote
	accounts := newIntegrationAccountService(t, tx, clock, cipher, client, nil)
	request := dto.AccountCreateRequest{
		SiteID: strconv.FormatInt(site.ID, 10), CustomerID: strconv.FormatInt(customer.ID, 10),
		RemoteUserID: strconv.FormatInt(remote.ID, 10), Remark: "fixed binding",
	}

	if _, err := accounts.Create(context.Background(), request, ""); !errors.Is(err, model.ErrCollectionRunContract) {
		t.Fatalf("invalid request ID create error = %v", err)
	}
	var rolledBack int64
	if err := tx.Model(&model.Account{}).Where("site_id = ? AND remote_user_id = ?", site.ID, remote.ID).Count(&rolledBack).Error; err != nil || rolledBack != 0 {
		t.Fatalf("rolled back account count = %d, %v", rolledBack, err)
	}

	detail, err := accounts.Create(context.Background(), request, "req_account_create")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if detail.SiteID != request.SiteID || detail.CustomerID != request.CustomerID || detail.RemoteUserID != request.RemoteUserID ||
		detail.Quota != "9007199254740999" || detail.ManagedStatus != dto.AccountManagedStatusActive {
		t.Fatalf("created account detail = %#v", detail)
	}
	accountID, _ := strconv.ParseInt(detail.ID, 10, 64)
	var run model.CollectionRun
	if err := tx.Where("target_type = 'account' AND target_id = ? AND task_type = ?", accountID, constant.TaskTypeAccountRebuild).
		First(&run).Error; err != nil {
		t.Fatalf("read account rebuild run: %v", err)
	}
	wantStart := floorHour(remote.CreatedAt)
	wantEnd := floorHour(clock.Now().Unix())
	if run.SiteID != nil || run.SiteConfigVersion != 0 || run.WindowsInitializedAt != nil || run.Status != "pending" ||
		run.StartTimestamp == nil || *run.StartTimestamp != wantStart || run.EndTimestamp == nil || *run.EndTimestamp != wantEnd {
		t.Fatalf("account rebuild run = %#v", run)
	}
	updatedCustomer, err := model.NewCustomerRepository(tx).FindByID(context.Background(), customer.ID)
	if err != nil || updatedCustomer.StatisticsBackfillStatus != "pending" {
		t.Fatalf("customer backfill status = %#v, %v", updatedCustomer, err)
	}
	if _, err := accounts.Create(context.Background(), request, "req_account_duplicate"); !errors.Is(err, ErrAccountAlreadyManaged) {
		t.Fatalf("duplicate account error = %v", err)
	}
}

func TestAccountListUsesBoundedQueriesAndPreservesMetadataContracts(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, tx, clock.Now().Unix(), cipher)
	quotaPerUnit := "500000"
	exchangeRate := "7.125"
	wantQuotaPerUnit := "500000.0000000000"
	wantExchangeRate := "7.1250000000"
	rateAt := clock.Now().Unix() - 60
	if err := tx.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"quota_per_unit": quotaPerUnit, "usd_exchange_rate": exchangeRate, "last_rate_at": rateAt,
	}).Error; err != nil {
		t.Fatalf("set account list site rate: %v", err)
	}
	customer := model.Customer{
		Name: "Bounded Account List", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(tx).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create account list customer: %v", err)
	}
	repository := model.NewAccountRepository(tx)
	accounts := make([]model.Account, 5)
	for index := range accounts {
		accounts[index] = model.Account{
			SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: int64(7000 + index),
			RemoteCreatedAt: clock.Now().Unix() - 7200, Username: "bounded-" + strconv.Itoa(index),
			RemoteStatus: 1, RemoteState: model.AccountRemoteStateNormal,
			Quota: int64(9_007_199_254_740_990 + index), UsedQuota: int64(index), RequestCount: int64(index + 1),
			ManagedStatus: model.AccountManagedStatusActive, StatisticsBackfillStatus: "none",
			CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix() + int64(index),
		}
		if err := repository.Create(context.Background(), &accounts[index]); err != nil {
			t.Fatalf("create bounded account %d: %v", index, err)
		}
	}
	run, err := newLocalRebuildRun("account", accounts[0].ID, constant.TaskTypeAccountRebuild,
		clock.Now().Unix()-7200, clock.Now().Unix(), "req_bounded_account_list", clock.Now().Unix())
	if err != nil {
		t.Fatalf("build account list run: %v", err)
	}
	run.Status = "running"
	run.TotalWindows = 4
	run.CompletedWindows = 1
	if err := tx.Create(&run).Error; err != nil {
		t.Fatalf("create account list run: %v", err)
	}

	callbackName := "test:bounded-account-list:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	queryCount := 0
	if err := tx.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queryCount++
	}); err != nil {
		t.Fatalf("register account list query counter: %v", err)
	}
	t.Cleanup(func() { _ = tx.Callback().Query().Remove(callbackName) })
	service := newIntegrationAccountService(t, tx, clock, cipher,
		&accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}, nil)
	page, err := service.List(context.Background(), dto.AccountListQuery{
		Page: 1, PageSize: 100, SiteID: strconv.FormatInt(site.ID, 10),
		SortBy: "updated_at", SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("list bounded accounts: %v", err)
	}
	if queryCount != 3 {
		t.Fatalf("account list SQL query count = %d, want 3 independent of page size", queryCount)
	}
	if page.Total != int64(len(accounts)) || len(page.Items) != len(accounts) {
		t.Fatalf("account list page = %#v", page)
	}
	items := make(map[string]dto.AccountListItem, len(page.Items))
	for _, item := range page.Items {
		items[item.ID] = item
		if item.SiteName != site.Name || item.CustomerName != customer.Name || item.Rate.Source != "site" ||
			item.Rate.QuotaPerUnit == nil || *item.Rate.QuotaPerUnit != wantQuotaPerUnit ||
			item.Rate.USDExchangeRate == nil || *item.Rate.USDExchangeRate != wantExchangeRate {
			t.Fatalf("account list metadata = %#v", item)
		}
		if _, parseErr := strconv.ParseInt(item.Quota, 10, 64); parseErr != nil {
			t.Fatalf("account quota is not a bigint JSON string: %#v", item)
		}
	}
	runItem := items[strconv.FormatInt(accounts[0].ID, 10)]
	if runItem.Backfill.RunID == nil || *runItem.Backfill.RunID != strconv.FormatInt(run.ID, 10) ||
		runItem.Backfill.Status != "running" || runItem.Backfill.TotalWindows != 4 || runItem.Backfill.CompletedWindows != 1 {
		t.Fatalf("account list backfill metadata = %#v", runItem.Backfill)
	}
}

func TestCustomerEnableAndAccountRestoreKeepObjectsPausedUntilWorkerCompletion(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, tx, clock.Now().Unix(), cipher)
	pauseAt := floorHour(clock.Now().Unix()) - 7200
	disabled := model.Customer{
		Name: "Disabled Customer", Status: dto.CustomerStatusDisabled, StatisticsPausedAt: &pauseAt,
		StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(tx).Create(context.Background(), &disabled); err != nil {
		t.Fatalf("create disabled customer: %v", err)
	}
	customers, err := NewCustomerService(CustomerServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	first, err := customers.Enable(context.Background(), disabled.ID, "req_customer_enable")
	if err != nil {
		t.Fatalf("enable customer: %v", err)
	}
	second, err := customers.Enable(context.Background(), disabled.ID, "req_customer_enable_repeat")
	if err != nil || second.ID != first.ID || !second.Deduplicated {
		t.Fatalf("repeat customer enable = %#v, %v", second, err)
	}
	clock.Advance(time.Hour)
	if _, err := customers.Enable(context.Background(), disabled.ID, "req_customer_enable_new_range"); !errors.Is(err, ErrEntityBackfillRunning) {
		t.Fatalf("different customer enable range error = %v", err)
	}
	stillDisabled, _ := model.NewCustomerRepository(tx).FindByID(context.Background(), disabled.ID)
	if stillDisabled.Status != dto.CustomerStatusDisabled || stillDisabled.StatisticsPausedAt == nil ||
		stillDisabled.StatisticsBackfillStatus != "pending" {
		t.Fatalf("customer opened before completion: %#v", stillDisabled)
	}

	using := model.Customer{
		Name: "Using Customer", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(tx).Create(context.Background(), &using); err != nil {
		t.Fatalf("create using customer: %v", err)
	}
	archived := model.Account{
		SiteID: site.ID, CustomerID: using.ID, RemoteUserID: 202, RemoteCreatedAt: pauseAt - 3600,
		Username: "archived", RemoteStatus: 1, RemoteState: model.AccountRemoteStateNormal,
		ManagedStatus: model.AccountManagedStatusArchived, StatisticsPausedAt: &pauseAt,
		StatisticsBackfillStatus: "none", CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewAccountRepository(tx).Create(context.Background(), &archived); err != nil {
		t.Fatalf("create archived account: %v", err)
	}
	client := &accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}
	accounts := newIntegrationAccountService(t, tx, clock, cipher, client, nil)
	restore, err := accounts.Restore(context.Background(), archived.ID, "req_account_restore")
	if err != nil {
		t.Fatalf("restore account: %v", err)
	}
	repeat, err := accounts.Restore(context.Background(), archived.ID, "req_account_restore_repeat")
	if err != nil || repeat.ID != restore.ID || !repeat.Deduplicated {
		t.Fatalf("repeat account restore = %#v, %v", repeat, err)
	}
	stillArchived, _ := model.NewAccountRepository(tx).FindByID(context.Background(), archived.ID)
	if stillArchived.ManagedStatus != model.AccountManagedStatusArchived || stillArchived.StatisticsPausedAt == nil ||
		stillArchived.StatisticsBackfillStatus != "pending" {
		t.Fatalf("account opened before completion: %#v", stillArchived)
	}
}

func TestRemoteUserSearchFiltersDeletedAndMarksManagedBindings(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, tx, clock.Now().Unix(), cipher)
	customer := model.Customer{
		Name: "Search Customer", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(tx).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create search customer: %v", err)
	}
	managed := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 301, RemoteCreatedAt: clock.Now().Unix() - 7200,
		Username: "alpha", RemoteStatus: 1, RemoteState: model.AccountRemoteStateNormal,
		ManagedStatus: model.AccountManagedStatusActive, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewAccountRepository(tx).Create(context.Background(), &managed); err != nil {
		t.Fatalf("create managed account: %v", err)
	}
	users := []dto.UpstreamUser{
		{UpstreamIdentity: dto.UpstreamIdentity{ID: 301, Username: "alpha", Status: 1}, CreatedAt: clock.Now().Unix() - 7200},
		{UpstreamIdentity: dto.UpstreamIdentity{ID: 302, Username: "deleted", Status: 1}, CreatedAt: clock.Now().Unix() - 7200, Deleted: true},
		{UpstreamIdentity: dto.UpstreamIdentity{ID: 303, Username: "gamma", Status: 1}, CreatedAt: clock.Now().Unix() - 3600, LastLoginAt: 123},
	}
	client := &accountTestClient{
		testSiteClient: authorizedTestSiteClient(clock.Now().Unix()),
		searchPages:    map[int]dto.UpstreamUserPage{1: {Page: 1, PageSize: 100, Total: 3, Items: users}},
	}
	accounts := newIntegrationAccountService(t, tx, clock, cipher, client, nil)
	page, err := accounts.SearchRemoteUsers(context.Background(), site.ID,
		dto.RemoteUserListQuery{Page: 1, PageSize: 20, Keyword: "a"}, "req_remote_search")
	if err != nil {
		t.Fatalf("search remote users: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 || !page.Items[0].AlreadyManaged ||
		page.Items[0].ManagedAccountID == nil || *page.Items[0].ManagedAccountID != strconv.FormatInt(managed.ID, 10) ||
		page.Items[0].ManagedCustomerName != customer.Name || page.Items[0].LastLoginAt != nil ||
		page.Items[1].LastLoginAt == nil || *page.Items[1].LastLoginAt != 123 {
		t.Fatalf("remote user page = %#v", page)
	}
}

func TestRemoteUserSearchRejectsFutureAndInvalidUsers(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, tx, clock.Now().Unix(), cipher)
	client := &accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}
	accounts := newIntegrationAccountService(t, tx, clock, cipher, client, nil)
	tests := []struct {
		name string
		user dto.UpstreamUser
	}{
		{
			name: "future created_at",
			user: dto.UpstreamUser{UpstreamIdentity: dto.UpstreamIdentity{ID: 401, Username: "future"}, CreatedAt: clock.Now().Unix() + 1},
		},
		{
			name: "invalid negative metric",
			user: dto.UpstreamUser{UpstreamIdentity: dto.UpstreamIdentity{ID: 402, Username: "invalid"}, CreatedAt: clock.Now().Unix() - 1, Quota: -1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client.searchPages = map[int]dto.UpstreamUserPage{
				1: {Page: 1, PageSize: 100, Total: 1, Items: []dto.UpstreamUser{test.user}},
			}
			_, err := accounts.SearchRemoteUsers(context.Background(), site.ID,
				dto.RemoteUserListQuery{Page: 1, PageSize: 20, Keyword: "user"}, "req_invalid_remote_search")
			if !errors.Is(err, ErrUpstreamResponseInvalid) {
				t.Fatalf("remote search error = %v", err)
			}
		})
	}
}

type fakeEntityStatisticsReader struct {
	customerQuery dto.StatisticsQuery
	accountQuery  dto.StatisticsQuery
}

func (reader *fakeEntityStatisticsReader) CustomerStatistics(_ context.Context, _ int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	reader.customerQuery = query
	return dto.StatisticsResponse{Scope: dto.StatisticsScopeCustomer}, nil
}

func (reader *fakeEntityStatisticsReader) AccountStatistics(_ context.Context, _ int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	reader.accountQuery = query
	return dto.StatisticsResponse{Scope: dto.StatisticsScopeAccount}, nil
}

func TestEntityStatisticsReaderReceivesFixedEntityScope(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	reader := &fakeEntityStatisticsReader{}
	customer := model.Customer{
		Name: "Statistics Customer", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewCustomerRepository(tx).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create statistics customer: %v", err)
	}
	customerService, _ := NewCustomerService(CustomerServiceOptions{Database: tx, Clock: clock, Statistics: reader})
	if _, err := customerService.Statistics(context.Background(), customer.ID, dto.StatisticsQuery{CustomerIDs: []int64{999}}); err != nil {
		t.Fatalf("customer statistics: %v", err)
	}
	if len(reader.customerQuery.CustomerIDs) != 1 || reader.customerQuery.CustomerIDs[0] != customer.ID {
		t.Fatalf("customer reader query = %#v", reader.customerQuery)
	}
	cipher := accountTestCipher(t)
	site := createAccountReadySite(t, tx, clock.Now().Unix(), cipher)
	account := model.Account{
		SiteID: site.ID, CustomerID: customer.ID, RemoteUserID: 501, RemoteCreatedAt: clock.Now().Unix() - 3600,
		Username: "statistics-account", RemoteState: model.AccountRemoteStateNormal,
		ManagedStatus: model.AccountManagedStatusActive, StatisticsBackfillStatus: "none",
		CreatedAt: clock.Now().Unix(), UpdatedAt: clock.Now().Unix(),
	}
	if err := model.NewAccountRepository(tx).Create(context.Background(), &account); err != nil {
		t.Fatalf("create statistics account: %v", err)
	}
	client := &accountTestClient{testSiteClient: authorizedTestSiteClient(clock.Now().Unix())}
	accountService := newIntegrationAccountService(t, tx, clock, cipher, client, reader)
	if _, err := accountService.Statistics(context.Background(), account.ID, dto.StatisticsQuery{AccountIDs: []int64{999}}); err != nil {
		t.Fatalf("account statistics: %v", err)
	}
	if len(reader.accountQuery.AccountIDs) != 1 || reader.accountQuery.AccountIDs[0] != account.ID {
		t.Fatalf("account reader query = %#v", reader.accountQuery)
	}
}

func TestGetUserMapsUpstream404ToUserNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	if _, err := client.GetUser(context.Background(), "req_user_not_found", 99); !errors.Is(err, ErrUpstreamUserNotFound) {
		t.Fatalf("GetUser 404 error = %v", err)
	}
}

func TestGetUserPreservesTypedIdentityConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"id":100,"username":"wrong-user","display_name":"","role":1,"status":1,"group":"","quota":0,"used_quota":0,"request_count":0,"created_at":1,"last_login_at":0,"DeletedAt":null}}`))
	}))
	defer server.Close()
	client := testClientForServer(t, server, true, testClientSettings{})
	_, err := client.GetUser(context.Background(), "req_user_identity_conflict", 99)
	var conflict *UpstreamUserIdentityConflictError
	if !errors.As(err, &conflict) || !errors.Is(err, ErrUpstreamUserIdentityConflict) ||
		conflict.ExpectedID != 99 || conflict.ActualID != 100 {
		t.Fatalf("GetUser wrong-ID error = %#v (%v)", conflict, err)
	}
}

func accountTestCipher(t *testing.T) *common.Cipher {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create account test cipher: %v", err)
	}
	return cipher
}

func createAccountReadySite(t *testing.T, tx *gorm.DB, now int64, cipher *common.Cipher) model.Site {
	t.Helper()
	rootID := int64(1)
	rootCreatedAt := now - 24*3600
	statisticsStart := floorHour(rootCreatedAt)
	source := "root_created_at"
	token, err := cipher.Encrypt([]byte("account-test-token"), siteTokenAAD(0))
	if err != nil {
		t.Fatalf("encrypt provisional account site token: %v", err)
	}
	site := newTestSite(now, "https://account-service.example")
	site.AuthStatus = constant.SiteAuthAuthorized
	site.StatisticsStatus = constant.SiteStatisticsReady
	site.DataExportEnabled = true
	site.RootUserID = &rootID
	site.RootCreatedAt = &rootCreatedAt
	site.StatisticsStartAt = &statisticsStart
	site.StatisticsStartSource = &source
	site.AccessTokenEncrypted = &token
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create account-ready site: %v", err)
	}
	token, err = cipher.Encrypt([]byte("account-test-token"), siteTokenAAD(site.ID))
	if err != nil {
		t.Fatalf("encrypt account site token: %v", err)
	}
	site.AccessTokenEncrypted = &token
	if err := model.NewSiteRepository(tx).Save(context.Background(), &site); err != nil {
		t.Fatalf("save account site token: %v", err)
	}
	if err := storeReadyCapabilities(context.Background(), model.NewSiteRepository(tx), site.ID, now); err != nil {
		t.Fatalf("store account site capabilities: %v", err)
	}
	return site
}

func newIntegrationAccountService(
	t *testing.T,
	tx *gorm.DB,
	clock common.Clock,
	cipher *common.Cipher,
	client SiteUpstreamClient,
	statistics EntityStatisticsReader,
) *AccountService {
	t.Helper()
	service, err := NewAccountService(AccountServiceOptions{
		Database: tx, ClientFactory: &testSiteClientFactory{authenticated: client, public: client},
		Cipher: cipher, Clock: clock, Statistics: statistics,
	})
	if err != nil {
		t.Fatalf("create account service: %v", err)
	}
	return service
}
