package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestAccountCreateMapsRealUpstreamIdentityFailures(t *testing.T) {
	tx := openSiteRouterTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	server, upstreamURL, allowedPrefix := newRoutableAccountUpstreamServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/41":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"id":42,"username":"wrong-user","display_name":"Wrong","role":1,"status":1,"group":"default","quota":0,"used_quota":0,"request_count":0,"created_at":1752397200,"last_login_at":0,"DeletedAt":null}}`))
		case "/api/user/43":
			http.Error(writer, "not found", http.StatusNotFound)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	cipher := newRouterAccountCipher(t)
	site, customer := createRouterAccountScope(t, tx, clock.Now().Unix(), upstreamURL, cipher)
	factory := service.NewConfiguredSiteClientFactory(service.SiteClientFactoryOptions{
		AllowedCIDRs: []netip.Prefix{allowedPrefix},
		ConnectTimeout: service.UpstreamConnectTimeout, HeaderTimeout: service.UpstreamResponseHeaderTimeout,
		RequestTimeout: service.UpstreamRequestTimeout, ExportTimeout: service.UpstreamExportTimeout,
	})
	accountService, err := service.NewAccountService(service.AccountServiceOptions{
		Database: tx, ClientFactory: factory, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create account service: %v", err)
	}
	customerService, err := service.NewCustomerService(service.CustomerServiceOptions{Database: tx, Clock: clock})
	if err != nil {
		t.Fatalf("create customer service: %v", err)
	}
	handler := newActualEntityRouteEngine(customerService, accountService)

	tests := []struct {
		name         string
		remoteUserID string
		wantStatus   int
		wantCode     string
	}{
		{name: "wrong ID", remoteUserID: "41", wantStatus: http.StatusConflict, wantCode: constant.CodeConflict},
		{name: "not found", remoteUserID: "43", wantStatus: http.StatusUnprocessableEntity, wantCode: constant.CodeUpstreamUserNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"site_id":"` + strconv.FormatInt(site.ID, 10) + `","customer_id":"` +
				strconv.FormatInt(customer.ID, 10) + `","remote_user_id":"` + test.remoteUserID + `"}`
			response := performEntityRequest(handler, http.MethodPost, "/api/accounts", body)
			assertEntityError(t, response, test.wantStatus, test.wantCode, "")
		})
	}
	var accountCount int64
	if err := tx.Model(&model.Account{}).Where("site_id = ?", site.ID).Count(&accountCount).Error; err != nil || accountCount != 0 {
		t.Fatalf("identity failures persisted accounts = %d, %v", accountCount, err)
	}
}

func newRoutableAccountUpstreamServer(t *testing.T, handler http.Handler) (*httptest.Server, string, netip.Prefix) {
	t.Helper()
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen for account upstream: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	var address netip.Addr
	interfaces, err := net.Interfaces()
	if err != nil {
		server.Close()
		t.Fatalf("list network interfaces: %v", err)
	}
	for _, networkInterface := range interfaces {
		addresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, candidate := range addresses {
			ip, _, parseErr := net.ParseCIDR(candidate.String())
			if parseErr != nil || ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			address, _ = netip.AddrFromSlice(ip.To4())
			break
		}
		if address.IsValid() {
			break
		}
	}
	if !address.IsValid() {
		server.Close()
		t.Fatal("no non-loopback IPv4 address is available for the real upstream test")
	}
	port := listener.Addr().(*net.TCPAddr).Port
	url := "http://" + net.JoinHostPort(address.String(), strconv.Itoa(port))
	return server, url, netip.PrefixFrom(address, 32)
}

func TestAccountCreateConfigVersionRaceReturnsTypedHTTPParams(t *testing.T) {
	database := openCommittedAccountRouterDatabase(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher := newRouterAccountCipher(t)
	site, customer := createRouterAccountScope(t, database.GORM, clock.Now().Unix(), "https://config-race.example", cipher)
	t.Cleanup(func() { cleanupRouterAccountScope(database.GORM, site.ID, customer.ID) })
	client := &blockingAccountClient{
		started: make(chan struct{}), release: make(chan struct{}),
		user: dto.UpstreamUser{
			UpstreamIdentity: dto.UpstreamIdentity{ID: 51, Username: "config-race-user", Status: 1},
			CreatedAt:        clock.Now().Unix() - 3600,
		},
	}
	factory := &fixedAccountClientFactory{client: client}
	accountService, err := service.NewAccountService(service.AccountServiceOptions{
		Database: database.GORM, ClientFactory: factory, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create race account service: %v", err)
	}
	customerService, err := service.NewCustomerService(service.CustomerServiceOptions{Database: database.GORM, Clock: clock})
	if err != nil {
		t.Fatalf("create race customer service: %v", err)
	}
	handler := newActualEntityRouteEngine(customerService, accountService)
	body := `{"site_id":"` + strconv.FormatInt(site.ID, 10) + `","customer_id":"` +
		strconv.FormatInt(customer.ID, 10) + `","remote_user_id":"51"}`
	responseChannel := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responseChannel <- performEntityRequest(handler, http.MethodPost, "/api/accounts", body)
	}()
	select {
	case <-client.started:
	case <-time.After(5 * time.Second):
		t.Fatal("account request did not reach upstream GetUser")
	}
	expectedVersion := site.ConfigVersion
	actualVersion := expectedVersion + 1
	result := database.GORM.Model(&model.Site{}).Where("id = ? AND config_version = ?", site.ID, expectedVersion).
		Update("config_version", actualVersion)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("bump concurrent site version rows=%d err=%v", result.RowsAffected, result.Error)
	}
	close(client.release)
	var response *httptest.ResponseRecorder
	select {
	case response = <-responseChannel:
	case <-time.After(10 * time.Second):
		t.Fatal("account request did not finish after releasing upstream")
	}
	envelope := decodeSiteEnvelope(t, response)
	if response.Code != http.StatusConflict || envelope.Success || envelope.Code != constant.CodeSiteConfigChanged || len(envelope.Params) != 3 {
		t.Fatalf("config race response = %d %#v body=%s", response.Code, envelope, response.Body.String())
	}
	var siteID string
	var expected, actual int
	if err := json.Unmarshal(envelope.Params["site_id"], &siteID); err != nil {
		t.Fatalf("decode site_id param: %v", err)
	}
	if err := json.Unmarshal(envelope.Params["expected_config_version"], &expected); err != nil {
		t.Fatalf("decode expected config version: %v", err)
	}
	if err := json.Unmarshal(envelope.Params["actual_config_version"], &actual); err != nil {
		t.Fatalf("decode actual config version: %v", err)
	}
	if siteID != strconv.FormatInt(site.ID, 10) || expected != expectedVersion || actual != actualVersion {
		t.Fatalf("config race params site=%q expected=%d actual=%d", siteID, expected, actual)
	}
	var accountCount int64
	if err := database.GORM.Model(&model.Account{}).Where("site_id = ?", site.ID).Count(&accountCount).Error; err != nil || accountCount != 0 {
		t.Fatalf("config race persisted accounts = %d, %v", accountCount, err)
	}
}

type fixedAccountClientFactory struct {
	client service.SiteUpstreamClient
}

func (factory *fixedAccountClientFactory) NewPublic(string) (service.SiteUpstreamClient, error) {
	return factory.client, nil
}

func (factory *fixedAccountClientFactory) NewAuthenticated(string, string, string, int64) (service.SiteUpstreamClient, error) {
	return factory.client, nil
}

type blockingAccountClient struct {
	service.SiteUpstreamClient
	started chan struct{}
	release chan struct{}
	once    sync.Once
	user    dto.UpstreamUser
}

func (client *blockingAccountClient) GetUser(ctx context.Context, _ string, _ int64) (dto.UpstreamUser, error) {
	client.once.Do(func() { close(client.started) })
	select {
	case <-client.release:
		return client.user, nil
	case <-ctx.Done():
		return dto.UpstreamUser{}, ctx.Err()
	}
}

func (client *blockingAccountClient) CloseIdleConnections() {}

func newActualEntityRouteEngine(customers *service.CustomerService, accounts *service.AccountService) http.Handler {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RequestID())
	registerCustomerRoutes(engine, controller.NewCustomerController(customers, accounts),
		controller.NewAccountController(accounts), entityRouteIdentityResolver{role: constant.RoleAdmin})
	return engine
}

func newRouterAccountCipher(t *testing.T) *common.Cipher {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create router account cipher: %v", err)
	}
	return cipher
}

func createRouterAccountScope(t *testing.T, db *gorm.DB, now int64, baseURL string, cipher *common.Cipher) (model.Site, model.Customer) {
	t.Helper()
	rootID := int64(1)
	rootCreatedAt := now - 24*3600
	statisticsStart := rootCreatedAt - rootCreatedAt%3600
	statisticsSource := "root_created_at"
	site := model.Site{
		Name: "Account Error Site", BaseURL: baseURL, ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, RootUserID: &rootID, RootCreatedAt: &rootCreatedAt,
		DataExportEnabled: true, StatisticsStartAt: &statisticsStart, StatisticsStartSource: &statisticsSource,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewSiteRepository(db).Create(context.Background(), &site); err != nil {
		t.Fatalf("create router account site: %v", err)
	}
	token, err := cipher.Encrypt([]byte("router-account-token"), "site:"+strconv.FormatInt(site.ID, 10)+":access_token")
	if err != nil {
		t.Fatalf("encrypt router account token: %v", err)
	}
	site.AccessTokenEncrypted = &token
	if err := model.NewSiteRepository(db).Save(context.Background(), &site); err != nil {
		t.Fatalf("save router account token: %v", err)
	}
	capabilities := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, model.SiteCapability{
			SiteID: site.ID, CapabilityKey: key, Status: status, MessageCode: string(constant.MessageCapabilityOK),
			MessageParams: []byte(`{"site_id":"` + strconv.FormatInt(site.ID, 10) + `","capability_key":"` + key + `"}`), CheckedAt: now,
		})
	}
	if err := model.NewSiteRepository(db).ReplaceCapabilities(context.Background(), site.ID, capabilities); err != nil {
		t.Fatalf("store router account capabilities: %v", err)
	}
	customer := model.Customer{
		Name: "Account Error Customer", Status: dto.CustomerStatusUsing, StatisticsBackfillStatus: "none",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := model.NewCustomerRepository(db).Create(context.Background(), &customer); err != nil {
		t.Fatalf("create router account customer: %v", err)
	}
	return site, customer
}

func openCommittedAccountRouterDatabase(t *testing.T) *model.Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open committed account router database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve committed account router lock: %v", err)
	}
	var acquired sql.NullInt64
	const lockName = "new-api-pilot-site-service-integration"
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", lockName).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire committed account router lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
	})
	return database
}

func cleanupRouterAccountScope(db *gorm.DB, siteID, customerID int64) {
	_ = db.Where("site_id = ?", siteID).Delete(&model.Account{}).Error
	_ = db.Where("site_id = ?", siteID).Delete(&model.SiteCapability{}).Error
	_ = db.Where("id = ?", customerID).Delete(&model.Customer{}).Error
	_ = db.Where("id = ?", siteID).Delete(&model.Site{}).Error
}
