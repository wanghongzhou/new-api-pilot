package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestSiteRoutesViewerReadAdminWriteContract(t *testing.T) {
	tx := openSiteRouterTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	factory := service.NewConfiguredSiteClientFactory(service.SiteClientFactoryOptions{
		AllowedHostSuffixes: []string{"example.test"},
		ConnectTimeout:      service.UpstreamConnectTimeout, HeaderTimeout: service.UpstreamResponseHeaderTimeout,
		RequestTimeout: service.UpstreamRequestTimeout, ExportTimeout: service.UpstreamExportTimeout,
	})
	siteService, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(tx), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create site service: %v", err)
	}
	siteController := controller.NewSiteController(siteService)
	admin := newSiteRoleEngine(t, siteController, constant.RoleAdmin)
	viewer := newSiteRoleEngine(t, siteController, constant.RoleViewer)

	created := performSiteRequest(admin, http.MethodPost, "/api/sites", `{"name":"Primary","base_url":"https://tenant.example.test","remark":""}`)
	createdEnvelope := decodeSiteEnvelope(t, created)
	if created.Code != http.StatusOK || !createdEnvelope.Success {
		t.Fatalf("admin create = %d %#v body=%s", created.Code, createdEnvelope, created.Body.String())
	}
	var createdData map[string]any
	if err := json.Unmarshal(createdEnvelope.Data, &createdData); err != nil {
		t.Fatalf("decode created site: %v", err)
	}
	if _, ok := createdData["id"].(string); !ok {
		t.Fatalf("site ID is not a JSON string: %#v", createdData["id"])
	}

	read := performSiteRequest(viewer, http.MethodGet, "/api/sites?p=1&page_size=20&management_status=active", "")
	readEnvelope := decodeSiteEnvelope(t, read)
	if read.Code != http.StatusOK || !readEnvelope.Success {
		t.Fatalf("viewer list = %d %#v body=%s", read.Code, readEnvelope, read.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(readEnvelope.Data, &page); err != nil || len(page.Items) != 1 {
		t.Fatalf("site page = %#v, %v", page, err)
	}

	forbidden := performSiteRequest(viewer, http.MethodPost, "/api/sites", `{"name":"Blocked","base_url":"https://blocked.example.test","remark":""}`)
	forbiddenEnvelope := decodeSiteEnvelope(t, forbidden)
	if forbidden.Code != http.StatusForbidden || forbiddenEnvelope.Code != constant.CodeForbidden {
		t.Fatalf("viewer write = %d %#v", forbidden.Code, forbiddenEnvelope)
	}

	unknownField := performSiteRequest(admin, http.MethodPost, "/api/sites", `{"name":"Invalid","base_url":"https://invalid.example.test","remark":"","unknown":true}`)
	unknownEnvelope := decodeSiteEnvelope(t, unknownField)
	if unknownField.Code != http.StatusBadRequest || unknownEnvelope.Code != constant.CodeValidationError || unknownEnvelope.FieldErrors["body"] == "" {
		t.Fatalf("unknown field = %d %#v", unknownField.Code, unknownEnvelope)
	}

	invalidQuery := performSiteRequest(viewer, http.MethodGet, "/api/sites?sort_by=DROP%20TABLE%20site", "")
	invalidEnvelope := decodeSiteEnvelope(t, invalidQuery)
	if invalidQuery.Code != http.StatusBadRequest || invalidEnvelope.Code != constant.CodeValidationError || invalidEnvelope.FieldErrors["sort_by"] == "" {
		t.Fatalf("invalid sort = %d %#v", invalidQuery.Code, invalidEnvelope)
	}
}

func TestSiteDeleteRestrictedResponseIncludesDependencyTypesWithoutIDs(t *testing.T) {
	tx := openSiteRouterTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	factory := service.NewConfiguredSiteClientFactory(service.SiteClientFactoryOptions{
		AllowedHostSuffixes: []string{"example.test"},
		ConnectTimeout:      service.UpstreamConnectTimeout, HeaderTimeout: service.UpstreamResponseHeaderTimeout,
		RequestTimeout: service.UpstreamRequestTimeout, ExportTimeout: service.UpstreamExportTimeout,
	})
	siteService, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(tx), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create site service: %v", err)
	}
	admin := newSiteRoleEngine(t, controller.NewSiteController(siteService), constant.RoleAdmin)
	created := performSiteRequest(admin, http.MethodPost, "/api/sites", `{"name":"Restricted","base_url":"https://restricted.example.test","remark":""}`)
	createdEnvelope := decodeSiteEnvelope(t, created)
	if created.Code != http.StatusOK || !createdEnvelope.Success {
		t.Fatalf("create restricted site = %d %#v body=%s", created.Code, createdEnvelope, created.Body.String())
	}
	var createdData map[string]any
	if err := json.Unmarshal(createdEnvelope.Data, &createdData); err != nil {
		t.Fatalf("decode restricted site: %v", err)
	}
	createdSiteID, ok := createdData["id"].(string)
	if !ok {
		t.Fatalf("restricted site ID is not a JSON string: %#v", createdData["id"])
	}
	siteID, err := strconv.ParseInt(createdSiteID, 10, 64)
	if err != nil {
		t.Fatalf("parse restricted site ID %q: %v", createdSiteID, err)
	}
	now := clock.Now().Unix()
	customer := model.Customer{
		Name: "Delete Restricted Customer", Status: "using", StatisticsBackfillStatus: "none",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&customer).Error; err != nil {
		t.Fatalf("create delete blocker customer: %v", err)
	}
	account := model.Account{
		SiteID: siteID, CustomerID: customer.ID, RemoteUserID: 9_000_001,
		RemoteCreatedAt: now - 3600, Username: "delete-blocker", RemoteStatus: 1,
		RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
		StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&account).Error; err != nil {
		t.Fatalf("create delete blocker account: %v", err)
	}
	if err := tx.Exec(`INSERT INTO usage_fact_hourly
  (site_id, remote_user_id, username_snapshot, model_name, channel_id, hour_ts,
   request_count, quota, token_used, collected_at)
VALUES (?, ?, 'delete-blocker', 'model', 1, ?, 1, 1, 1, ?)`,
		siteID, account.RemoteUserID, now-now%3600, now).Error; err != nil {
		t.Fatalf("create delete blocker fact: %v", err)
	}

	restrictedDelete := performSiteRequest(admin, http.MethodDelete, "/api/sites/"+createdSiteID, "")
	restrictedEnvelope := decodeSiteEnvelope(t, restrictedDelete)
	if restrictedDelete.Code != http.StatusConflict || restrictedEnvelope.Success ||
		restrictedEnvelope.Code != constant.CodeDeleteRestricted || string(restrictedEnvelope.Data) != "null" {
		t.Fatalf("restricted delete = %d %#v body=%s", restrictedDelete.Code, restrictedEnvelope, restrictedDelete.Body.String())
	}
	if len(restrictedEnvelope.Params) != 1 {
		t.Fatalf("restricted delete params = %#v", restrictedEnvelope.Params)
	}
	var dependencyTypes []string
	if err := json.Unmarshal(restrictedEnvelope.Params["dependency_types"], &dependencyTypes); err != nil {
		t.Fatalf("decode restricted dependency types: %v, params=%#v", err, restrictedEnvelope.Params)
	}
	if strings.Join(dependencyTypes, ",") != "account,usage_fact" {
		t.Fatalf("restricted dependency types = %#v", dependencyTypes)
	}
	paramsJSON, err := json.Marshal(restrictedEnvelope.Params)
	if err != nil {
		t.Fatalf("encode restricted params: %v", err)
	}
	for _, sensitiveID := range []int64{siteID, customer.ID, account.ID} {
		if strings.Contains(string(paramsJSON), strconv.FormatInt(sensitiveID, 10)) {
			t.Fatalf("restricted params leak ID %d: %s", sensitiveID, paramsJSON)
		}
	}
}

func TestSiteBackfillHTTPRejectsFutureEndAndAcceptsCurrentHourBoundary(t *testing.T) {
	tx := openSiteRouterTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	if err := testsupport.ResetPlatformSettings(context.Background(), tx, clock.Now().Unix()); err != nil {
		t.Fatalf("reset backfill boundary settings: %v", err)
	}
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create backfill boundary cipher: %v", err)
	}
	factory := service.NewConfiguredSiteClientFactory(service.SiteClientFactoryOptions{
		AllowedHostSuffixes: []string{"example.test"},
		ConnectTimeout:      service.UpstreamConnectTimeout, HeaderTimeout: service.UpstreamResponseHeaderTimeout,
		RequestTimeout: service.UpstreamRequestTimeout, ExportTimeout: service.UpstreamExportTimeout,
	})
	siteService, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(tx), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create backfill boundary site service: %v", err)
	}
	now := clock.Now().Unix()
	currentHour := now - now%3600
	statisticsStart := currentHour - 2*3600
	site := model.Site{
		Name: "Backfill Boundary", BaseURL: "https://backfill-boundary.example.test", ConfigVersion: 1,
		ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
		HealthStatus: constant.SiteHealthOK, DataExportEnabled: true,
		StatisticsStartAt: &statisticsStart, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&site).Error; err != nil {
		t.Fatalf("create backfill boundary site: %v", err)
	}
	capabilities := make([]model.SiteCapability, 0, len(constant.SiteCapabilityKeys()))
	for _, key := range constant.SiteCapabilityKeys() {
		status := constant.CapabilityStatusPassed
		if key == constant.CapabilityFlowDataConsistency {
			status = constant.CapabilityStatusSkipped
		}
		capabilities = append(capabilities, model.SiteCapability{
			SiteID: site.ID, CapabilityKey: key, Status: status,
			MessageCode: string(constant.MessageCapabilityOK),
			MessageParams: []byte(`{"site_id":"` + strconv.FormatInt(site.ID, 10) +
				`","capability_key":"` + key + `"}`), CheckedAt: now,
		})
	}
	if err := model.NewSiteRepository(tx).ReplaceCapabilities(context.Background(), site.ID, capabilities); err != nil {
		t.Fatalf("store backfill boundary capabilities: %v", err)
	}
	admin := newSiteRoleEngine(t, controller.NewSiteController(siteService), constant.RoleAdmin)
	start := currentHour - 3600
	futureEnd := currentHour + 3600
	future := performSiteRequest(admin, http.MethodPost, "/api/sites/"+strconv.FormatInt(site.ID, 10)+"/backfill",
		`{"start_timestamp":`+strconv.FormatInt(start, 10)+`,"end_timestamp":`+strconv.FormatInt(futureEnd, 10)+`}`)
	futureEnvelope := decodeSiteEnvelope(t, future)
	if future.Code != http.StatusBadRequest || futureEnvelope.Success ||
		futureEnvelope.Code != constant.CodeValidationError || futureEnvelope.FieldErrors["range"] == "" {
		t.Fatalf("future backfill HTTP response = %d %#v body=%s", future.Code, futureEnvelope, future.Body.String())
	}
	current := performSiteRequest(admin, http.MethodPost, "/api/sites/"+strconv.FormatInt(site.ID, 10)+"/backfill",
		`{"start_timestamp":`+strconv.FormatInt(start, 10)+`,"end_timestamp":`+strconv.FormatInt(currentHour, 10)+`}`)
	currentEnvelope := decodeSiteEnvelope(t, current)
	if current.Code != http.StatusOK || !currentEnvelope.Success {
		t.Fatalf("current-hour boundary HTTP response = %d %#v body=%s", current.Code, currentEnvelope, current.Body.String())
	}
}

type siteAPIEnvelope struct {
	Success     bool                       `json:"success"`
	Code        string                     `json:"code"`
	Data        json.RawMessage            `json:"data"`
	RequestID   string                     `json:"request_id"`
	FieldErrors map[string]string          `json:"field_errors"`
	Params      map[string]json.RawMessage `json:"params"`
}

type fixedSiteIdentityResolver struct {
	role string
}

func (resolver fixedSiteIdentityResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: resolver.role, Status: constant.UserStatusEnabled}, nil
}

func newSiteRoleEngine(t *testing.T, siteController *controller.SiteController, role string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine, err := New(Options{
		Config: config.Config{AppEnv: config.EnvironmentTest}, SiteController: siteController,
		IdentityResolver: fixedSiteIdentityResolver{role: role},
	})
	if err != nil {
		t.Fatalf("create %s site router: %v", role, err)
	}
	return engine
}

func performSiteRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("New-Api-User", "1")
	request.RemoteAddr = "198.51.100.20:4321"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeSiteEnvelope(t *testing.T, response *httptest.ResponseRecorder) siteAPIEnvelope {
	t.Helper()
	var envelope siteAPIEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode site API response %d body=%q: %v", response.Code, response.Body.String(), err)
	}
	if envelope.RequestID == "" {
		t.Fatalf("site API response has no request ID: %s", response.Body.String())
	}
	return envelope
}

func openSiteRouterTestTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open site router database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve site router lock: %v", err)
	}
	var acquired sql.NullInt64
	const lockName = "new-api-pilot-site-service-integration"
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", lockName).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire site router lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	tx := database.GORM.Begin()
	if tx.Error != nil {
		t.Fatalf("begin site router transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", lockName)
		_ = connection.Close()
		_ = database.Close()
	})
	return tx
}
