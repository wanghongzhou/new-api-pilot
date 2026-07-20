package contract_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/router"
)

type coreContractIdentityResolver struct {
	role string
}

func (resolver coreContractIdentityResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "9007199254740993", Role: resolver.role, Status: constant.UserStatusEnabled}, nil
}

type coreContractCustomerApplication struct {
	failure error
}

func (application *coreContractCustomerApplication) List(_ context.Context, query dto.CustomerListQuery) (common.PageData[dto.CustomerListItem], error) {
	if application.failure != nil {
		return common.PageData[dto.CustomerListItem]{}, application.failure
	}
	items := []dto.CustomerListItem{
		{ID: "9007199254740993", Name: "First", Status: dto.CustomerStatusUsing},
		{ID: "9007199254740994", Name: "Middle", Status: dto.CustomerStatusUsing},
		{ID: "9007199254740995", Name: "Last", Status: dto.CustomerStatusUsing},
	}
	start := (query.Page - 1) * query.PageSize
	if start < 0 || start >= len(items) {
		return common.NewPageData(query.Page, query.PageSize, int64(len(items)), []dto.CustomerListItem{}), nil
	}
	end := start + query.PageSize
	if end > len(items) {
		end = len(items)
	}
	return common.NewPageData(query.Page, query.PageSize, int64(len(items)), items[start:end]), nil
}

func (application *coreContractCustomerApplication) Create(_ context.Context, request dto.CustomerCreateRequest) (dto.CustomerDetail, error) {
	return dto.CustomerDetail{CustomerListItem: dto.CustomerListItem{
		ID: "9007199254740993", Name: request.Name, Status: request.Status,
	}}, nil
}

func (application *coreContractCustomerApplication) Get(context.Context, int64) (dto.CustomerDetail, error) {
	return dto.CustomerDetail{CustomerListItem: dto.CustomerListItem{ID: "9007199254740993", Name: "First", Status: dto.CustomerStatusUsing}}, nil
}

func (application *coreContractCustomerApplication) Update(ctx context.Context, id int64, _ dto.CustomerUpdateRequest) (dto.CustomerDetail, error) {
	return application.Get(ctx, id)
}

func (application *coreContractCustomerApplication) Delete(context.Context, int64) error { return nil }

func (application *coreContractCustomerApplication) Disable(ctx context.Context, id int64) (dto.CustomerDetail, error) {
	result, err := application.Get(ctx, id)
	result.Status = dto.CustomerStatusDisabled
	return result, err
}

func (application *coreContractCustomerApplication) Enable(context.Context, int64, string) (dto.CollectionRunItem, error) {
	return dto.CollectionRunItem{ID: "9007199254740996", TargetType: "customer", TargetID: "9007199254740993", Status: "pending"}, nil
}

func (application *coreContractCustomerApplication) Statistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return dto.StatisticsResponse{Scope: dto.StatisticsScopeCustomer, Granularity: dto.StatisticsGranularityHour}, nil
}

type coreContractAccountApplication struct{}

func (application *coreContractAccountApplication) List(_ context.Context, query dto.AccountListQuery) (common.PageData[dto.AccountListItem], error) {
	items := []dto.AccountListItem{{
		ID: "9007199254740995", SiteID: "9007199254740993", CustomerID: "9007199254740994", RemoteUserID: "9007199254740996",
		Quota: "9223372036854775000", UsedQuota: "9007199254740997", RequestCount: "9007199254741999",
	}}
	return common.NewPageData(query.Page, query.PageSize, 1, items), nil
}

func (application *coreContractAccountApplication) SearchRemoteUsers(context.Context, int64, dto.RemoteUserListQuery, string) (common.PageData[dto.RemoteUserItem], error) {
	return common.NewPageData(1, 20, 1, []dto.RemoteUserItem{{
		ID: "9007199254740996", Username: "remote-user", Quota: "1", UsedQuota: "2", RequestCount: "3", CreatedAt: 1,
	}}), nil
}

func (application *coreContractAccountApplication) Create(_ context.Context, request dto.AccountCreateRequest, _ string) (dto.AccountDetail, error) {
	return dto.AccountDetail{AccountListItem: dto.AccountListItem{
		ID: "9007199254740995", SiteID: request.SiteID, CustomerID: request.CustomerID, RemoteUserID: request.RemoteUserID,
	}}, nil
}

func (application *coreContractAccountApplication) Get(context.Context, int64) (dto.AccountDetail, error) {
	return dto.AccountDetail{AccountListItem: dto.AccountListItem{
		ID: "9007199254740995", SiteID: "9007199254740993", CustomerID: "9007199254740994", RemoteUserID: "9007199254740996",
	}}, nil
}

func (application *coreContractAccountApplication) Update(ctx context.Context, id int64, _ dto.AccountUpdateRequest) (dto.AccountDetail, error) {
	return application.Get(ctx, id)
}

func (application *coreContractAccountApplication) Delete(context.Context, int64) error { return nil }

func (application *coreContractAccountApplication) Archive(ctx context.Context, id int64) (dto.AccountDetail, error) {
	result, err := application.Get(ctx, id)
	result.ManagedStatus = dto.AccountManagedStatusArchived
	return result, err
}

func (application *coreContractAccountApplication) Restore(context.Context, int64, string) (dto.CollectionRunItem, error) {
	return dto.CollectionRunItem{ID: "9007199254740997", TargetType: "account", TargetID: "9007199254740995", Status: "pending"}, nil
}

func (application *coreContractAccountApplication) Refresh(ctx context.Context, id int64, _ string) (dto.AccountDetail, error) {
	return application.Get(ctx, id)
}

func (application *coreContractAccountApplication) Statistics(context.Context, int64, dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return dto.StatisticsResponse{Scope: dto.StatisticsScopeAccount, Granularity: dto.StatisticsGranularityHour}, nil
}

// a18DashboardApplication keeps the API contract fixture focused on the
// partial-realtime state rather than duplicating DashboardService unit tests.
type a18DashboardApplication struct{}

func (a18DashboardApplication) Summary(context.Context) (dto.DashboardSummary, error) {
	requestCount := "9007199254740993"
	quota := "9007199254740994"
	tokens := "9007199254740995"
	activeAccounts := "9007199254740996"
	rpm := "9007199254740997"
	tpm := "9007199254740998"
	asOf := int64(1_768_622_400)
	return dto.DashboardSummary{
		Today: dto.DashboardUsageSummary{
			UsageSummary: dto.UsageSummary{
				RequestCount: &requestCount, Quota: &quota, TokenUsed: &tokens, ActiveUsers: &activeAccounts,
				DataStatus: "partial", IsFinal: false, AsOf: &asOf,
			},
			SiteBreakdown: []dto.SiteQuotaBreakdown{},
		},
		ActiveAccountsToday:       &activeAccounts,
		ResourceStaleSiteIDs:      []string{},
		ResourceDataStatus:        "complete",
		StaleSiteIDs:              []string{"9007199254740994"},
		RealtimeDataStatus:        "partial",
		RealtimeCompleteSiteCount: 1,
		RealtimeExpectedSiteCount: 2,
		RPM:                       &rpm,
		TPM:                       &tpm,
	}, nil
}

func (a18DashboardApplication) Trend(context.Context, dto.DashboardTrendQuery) ([]dto.TrendPoint, error) {
	return []dto.TrendPoint{}, nil
}

func (a18DashboardApplication) Top(context.Context, dto.DashboardTopQuery) ([]dto.DashboardRankingItem, error) {
	return []dto.DashboardRankingItem{}, nil
}

func (a18DashboardApplication) Health(context.Context) (dto.DashboardHealth, error) {
	return dto.DashboardHealth{
		AuthExpiredSiteIDs: []string{}, StatisticsNotReadySiteIDs: []string{},
		LatestAlerts: []dto.AlertEventItem{}, Sites: []dto.DashboardSiteHealthItem{},
		Completeness: dto.Completeness{MissingSiteIDs: []string{}, MissingRanges: []dto.MissingRange{}},
	}, nil
}

func newCoreContractEngine(t *testing.T, role string, customers *coreContractCustomerApplication) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	accounts := &coreContractAccountApplication{}
	engine, err := router.New(router.Options{
		Config:             config.Config{AppEnv: config.EnvironmentTest},
		CustomerController: controller.NewCustomerController(customers, accounts),
		AccountController:  controller.NewAccountController(accounts),
		IdentityResolver:   coreContractIdentityResolver{role: role},
	})
	if err != nil {
		t.Fatalf("create core contract router: %v", err)
	}
	return engine
}

func newA18DashboardContractEngine(t *testing.T) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine, err := router.New(router.Options{
		Config:              config.Config{AppEnv: config.EnvironmentTest},
		DashboardController: controller.NewDashboardController(a18DashboardApplication{}),
		IdentityResolver:    coreContractIdentityResolver{role: constant.RoleViewer},
	})
	if err != nil {
		t.Fatalf("create A18 dashboard contract router: %v", err)
	}
	return engine
}

func coreContractRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("New-Api-User", "9007199254740993")
	request.Header.Set(middleware.RequestIDHeader, "core-contract-request")
	request.RemoteAddr = "198.51.100.20:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type coreContractEnvelope struct {
	Success     bool              `json:"success"`
	Message     string            `json:"message"`
	Code        string            `json:"code"`
	Data        json.RawMessage   `json:"data"`
	RequestID   string            `json:"request_id"`
	FieldErrors map[string]string `json:"field_errors"`
}

func decodeCoreContractEnvelope(t *testing.T, response *httptest.ResponseRecorder) coreContractEnvelope {
	t.Helper()
	var envelope coreContractEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode contract response: %v body=%s", err, response.Body.String())
	}
	if envelope.RequestID != "core-contract-request" {
		t.Fatalf("request ID contract = %q body=%s", envelope.RequestID, response.Body.String())
	}
	return envelope
}

func TestA18DashboardPartialRealtimeContract(t *testing.T) {
	response := coreContractRequest(newA18DashboardContractEngine(t), http.MethodGet, "/api/dashboard/summary", "")
	envelope := decodeCoreContractEnvelope(t, response)
	var data struct {
		RPM                       string   `json:"rpm"`
		TPM                       string   `json:"tpm"`
		RealtimeDataStatus        string   `json:"realtime_data_status"`
		RealtimeCompleteSiteCount int      `json:"realtime_complete_site_count"`
		RealtimeExpectedSiteCount int      `json:"realtime_expected_site_count"`
		StaleSiteIDs              []string `json:"stale_site_ids"`
	}
	if response.Code != http.StatusOK || !envelope.Success || json.Unmarshal(envelope.Data, &data) != nil ||
		data.RPM != "9007199254740997" || data.TPM != "9007199254740998" ||
		data.RealtimeDataStatus != "partial" || data.RealtimeCompleteSiteCount != 1 ||
		data.RealtimeExpectedSiteCount != 2 || len(data.StaleSiteIDs) != 1 || data.StaleSiteIDs[0] != "9007199254740994" {
		t.Fatalf("A18 dashboard partial realtime response=%d %#v data=%s", response.Code, envelope, envelope.Data)
	}
}

func TestA20A26A87APIEnvelopePaginationAndAuthorizationAcceptance(t *testing.T) {
	customers := &coreContractCustomerApplication{}
	viewer := newCoreContractEngine(t, constant.RoleViewer, customers)

	missing := coreContractRequest(viewer, http.MethodGet, "/api/missing", "")
	missingEnvelope := decodeCoreContractEnvelope(t, missing)
	if missing.Code != http.StatusNotFound || missingEnvelope.Success || missingEnvelope.Code != constant.CodeNotFound || string(missingEnvelope.Data) != "null" {
		t.Fatalf("A87 missing response = %d %#v", missing.Code, missingEnvelope)
	}

	for _, page := range []struct {
		value     int
		itemCount int
	}{
		{value: 1, itemCount: 1},
		{value: 2, itemCount: 1},
		{value: 3, itemCount: 1},
		{value: 4, itemCount: 0},
	} {
		response := coreContractRequest(viewer, http.MethodGet, "/api/customers?p="+strconv.Itoa(page.value)+"&page_size=1", "")
		envelope := decodeCoreContractEnvelope(t, response)
		var data struct {
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
			Total    int64            `json:"total"`
			Items    []map[string]any `json:"items"`
		}
		if response.Code != http.StatusOK || !envelope.Success || envelope.Code != "" || json.Unmarshal(envelope.Data, &data) != nil ||
			data.Page != page.value || data.PageSize != 1 || data.Total != 3 || len(data.Items) != page.itemCount {
			t.Fatalf("A87 page %d response = %d %#v data=%s", page.value, response.Code, envelope, envelope.Data)
		}
		for _, item := range data.Items {
			if _, isString := item["id"].(string); !isString {
				t.Fatalf("A87 customer ID is not a string: %#v", item)
			}
		}
	}

	accounts := coreContractRequest(viewer, http.MethodGet, "/api/accounts?p=1&page_size=20", "")
	accountsEnvelope := decodeCoreContractEnvelope(t, accounts)
	var accountPage struct {
		Items []map[string]any `json:"items"`
	}
	if accounts.Code != http.StatusOK || !accountsEnvelope.Success || json.Unmarshal(accountsEnvelope.Data, &accountPage) != nil || len(accountPage.Items) != 1 {
		t.Fatalf("A20 account list = %d %#v data=%s", accounts.Code, accountsEnvelope, accountsEnvelope.Data)
	}
	for key, want := range map[string]string{
		"id": "9007199254740995", "site_id": "9007199254740993", "customer_id": "9007199254740994",
		"remote_user_id": "9007199254740996", "quota": "9223372036854775000",
		"used_quota": "9007199254740997", "request_count": "9007199254741999",
	} {
		got, isString := accountPage.Items[0][key].(string)
		if !isString || got != want {
			t.Fatalf("A26 bigint field %s=%#v, want decimal string %q", key, accountPage.Items[0][key], want)
		}
	}

	forbiddenCreate := coreContractRequest(viewer, http.MethodPost, "/api/customers", `{"name":"Denied","status":"using"}`)
	if envelope := decodeCoreContractEnvelope(t, forbiddenCreate); forbiddenCreate.Code != http.StatusForbidden || envelope.Success || envelope.Code != constant.CodeForbidden {
		t.Fatalf("A20 viewer customer write = %d %#v", forbiddenCreate.Code, envelope)
	}
	forbiddenSearch := coreContractRequest(viewer, http.MethodGet, "/api/accounts/site/1/remote-users?p=1&page_size=20", "")
	if envelope := decodeCoreContractEnvelope(t, forbiddenSearch); forbiddenSearch.Code != http.StatusForbidden || envelope.Success || envelope.Code != constant.CodeForbidden {
		t.Fatalf("A20 viewer remote user search = %d %#v", forbiddenSearch.Code, envelope)
	}

	invalidPage := coreContractRequest(viewer, http.MethodGet, "/api/customers?p=0", "")
	if envelope := decodeCoreContractEnvelope(t, invalidPage); invalidPage.Code != http.StatusBadRequest || envelope.Code != constant.CodeValidationError || envelope.FieldErrors["p"] == "" {
		t.Fatalf("A87 invalid page = %d %#v", invalidPage.Code, envelope)
	}
	unknownQuery := coreContractRequest(viewer, http.MethodGet, "/api/accounts?drop=table", "")
	if envelope := decodeCoreContractEnvelope(t, unknownQuery); unknownQuery.Code != http.StatusBadRequest || envelope.Code != constant.CodeValidationError || envelope.FieldErrors["drop"] == "" {
		t.Fatalf("A87 unknown query = %d %#v", unknownQuery.Code, envelope)
	}

	customers.failure = errors.New("mysql password=core-secret must not leak")
	internal := coreContractRequest(viewer, http.MethodGet, "/api/customers", "")
	if envelope := decodeCoreContractEnvelope(t, internal); internal.Code != http.StatusInternalServerError || envelope.Success || envelope.Code != constant.CodeInternalError || strings.Contains(internal.Body.String(), "core-secret") {
		t.Fatalf("A87 internal failure response = %d %#v body=%s", internal.Code, envelope, internal.Body.String())
	}
}
