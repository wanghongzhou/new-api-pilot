package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
)

type entityRouteIdentityResolver struct {
	role string
}

func (resolver entityRouteIdentityResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{ID: "1", Role: resolver.role, Status: constant.UserStatusEnabled}, nil
}

type fakeCustomerApplication struct {
	lastStatistics dto.StatisticsQuery
}

func (application *fakeCustomerApplication) List(context.Context, dto.CustomerListQuery) (common.PageData[dto.CustomerListItem], error) {
	return common.NewPageData(1, 20, 1, []dto.CustomerListItem{{ID: "9007199254740993", Name: "Customer", Status: dto.CustomerStatusUsing}}), nil
}

func (application *fakeCustomerApplication) Create(context.Context, dto.CustomerCreateRequest) (dto.CustomerDetail, error) {
	return dto.CustomerDetail{CustomerListItem: dto.CustomerListItem{ID: "9007199254740993", Name: "Customer", Status: dto.CustomerStatusUsing}}, nil
}

func (application *fakeCustomerApplication) Get(context.Context, int64) (dto.CustomerDetail, error) {
	return dto.CustomerDetail{CustomerListItem: dto.CustomerListItem{ID: "1", Name: "Customer", Status: dto.CustomerStatusUsing}}, nil
}

func (application *fakeCustomerApplication) Update(context.Context, int64, dto.CustomerUpdateRequest) (dto.CustomerDetail, error) {
	return application.Get(context.Background(), 1)
}

func (application *fakeCustomerApplication) Delete(context.Context, int64) error { return nil }

func (application *fakeCustomerApplication) Disable(context.Context, int64) (dto.CustomerDetail, error) {
	detail, _ := application.Get(context.Background(), 1)
	detail.Status = dto.CustomerStatusDisabled
	return detail, nil
}

func (application *fakeCustomerApplication) Enable(context.Context, int64, string) (dto.CollectionRunItem, error) {
	return dto.CollectionRunItem{ID: "44", TargetType: "customer", TargetID: "1", Status: "pending"}, nil
}

func (application *fakeCustomerApplication) Statistics(_ context.Context, _ int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.lastStatistics = query
	return dto.StatisticsResponse{Scope: dto.StatisticsScopeCustomer, Granularity: query.Granularity}, nil
}

type fakeAccountApplication struct {
	lastStatistics dto.StatisticsQuery
}

func (application *fakeAccountApplication) List(context.Context, dto.AccountListQuery) (common.PageData[dto.AccountListItem], error) {
	return common.NewPageData(1, 20, 1, []dto.AccountListItem{{
		ID: "9007199254740995", SiteID: "1", CustomerID: "2", RemoteUserID: "9007199254740997",
		Quota: "9007199254740999", UsedQuota: "1", RequestCount: "2",
	}}), nil
}

func (application *fakeAccountApplication) SearchRemoteUsers(context.Context, int64, dto.RemoteUserListQuery, string) (common.PageData[dto.RemoteUserItem], error) {
	return common.NewPageData(1, 20, 1, []dto.RemoteUserItem{{ID: "9007199254740997", Quota: "3", UsedQuota: "4", RequestCount: "5"}}), nil
}

func (application *fakeAccountApplication) Create(context.Context, dto.AccountCreateRequest, string) (dto.AccountDetail, error) {
	return dto.AccountDetail{AccountListItem: dto.AccountListItem{ID: "1", SiteID: "2", CustomerID: "3", RemoteUserID: "4"}}, nil
}

func (application *fakeAccountApplication) Get(context.Context, int64) (dto.AccountDetail, error) {
	return dto.AccountDetail{AccountListItem: dto.AccountListItem{ID: "1", SiteID: "2", CustomerID: "3", RemoteUserID: "4"}}, nil
}

func (application *fakeAccountApplication) Update(context.Context, int64, dto.AccountUpdateRequest) (dto.AccountDetail, error) {
	return application.Get(context.Background(), 1)
}

func (application *fakeAccountApplication) Delete(context.Context, int64) error { return nil }

func (application *fakeAccountApplication) Archive(context.Context, int64) (dto.AccountDetail, error) {
	detail, _ := application.Get(context.Background(), 1)
	detail.ManagedStatus = dto.AccountManagedStatusArchived
	return detail, nil
}

func (application *fakeAccountApplication) Restore(context.Context, int64, string) (dto.CollectionRunItem, error) {
	return dto.CollectionRunItem{ID: "55", TargetType: "account", TargetID: "1", Status: "pending"}, nil
}

func (application *fakeAccountApplication) Refresh(context.Context, int64, string) (dto.AccountDetail, error) {
	return application.Get(context.Background(), 1)
}

func (application *fakeAccountApplication) Statistics(_ context.Context, _ int64, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	application.lastStatistics = query
	return dto.StatisticsResponse{Scope: dto.StatisticsScopeAccount, Granularity: query.Granularity}, nil
}

func TestCustomerAccountRoutesPermissionAndStrictContracts(t *testing.T) {
	customers := &fakeCustomerApplication{}
	accounts := &fakeAccountApplication{}
	admin := newEntityRouteEngine(customers, accounts, constant.RoleAdmin)
	viewer := newEntityRouteEngine(customers, accounts, constant.RoleViewer)

	list := performEntityRequest(viewer, http.MethodGet, "/api/customers?p=1&page_size=20", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"9007199254740993"`) {
		t.Fatalf("viewer customer list = %d %s", list.Code, list.Body.String())
	}
	accountList := performEntityRequest(viewer, http.MethodGet, "/api/accounts?p=1&page_size=20", "")
	if accountList.Code != http.StatusOK || !strings.Contains(accountList.Body.String(), `"quota":"9007199254740999"`) {
		t.Fatalf("viewer account list = %d %s", accountList.Code, accountList.Body.String())
	}

	forbidden := performEntityRequest(viewer, http.MethodPost, "/api/customers", `{"name":"Blocked","status":"using"}`)
	assertEntityError(t, forbidden, http.StatusForbidden, constant.CodeForbidden, "")
	remoteForbidden := performEntityRequest(viewer, http.MethodGet, "/api/accounts/site/1/remote-users?p=1&page_size=20", "")
	assertEntityError(t, remoteForbidden, http.StatusForbidden, constant.CodeForbidden, "")

	unknownJSON := performEntityRequest(admin, http.MethodPost, "/api/customers", `{"name":"Invalid","status":"using","unknown":true}`)
	assertEntityError(t, unknownJSON, http.StatusBadRequest, constant.CodeValidationError, "body")
	duplicateJSON := performEntityRequest(admin, http.MethodPost, "/api/accounts", `{"site_id":"1","site_id":"2","customer_id":"3","remote_user_id":"4"}`)
	assertEntityError(t, duplicateJSON, http.StatusBadRequest, constant.CodeValidationError, "body")

	duplicateQuery := performEntityRequest(viewer, http.MethodGet, "/api/accounts?p=1&p=2", "")
	assertEntityError(t, duplicateQuery, http.StatusBadRequest, constant.CodeValidationError, "p")
	unknownQuery := performEntityRequest(viewer, http.MethodGet, "/api/customers?drop=table", "")
	assertEntityError(t, unknownQuery, http.StatusBadRequest, constant.CodeValidationError, "drop")

	nonEmptyAction := performEntityRequest(admin, http.MethodPost, "/api/accounts/1/archive", `{}`)
	assertEntityError(t, nonEmptyAction, http.StatusBadRequest, constant.CodeValidationError, "body")
	unexpectedActionQuery := performEntityRequest(admin, http.MethodPost, "/api/accounts/1/archive?unexpected=1", "")
	assertEntityError(t, unexpectedActionQuery, http.StatusBadRequest, constant.CodeValidationError, "unexpected")
	emptyAction := performEntityRequest(admin, http.MethodPost, "/api/accounts/1/archive", "")
	if emptyAction.Code != http.StatusOK {
		t.Fatalf("empty archive body = %d %s", emptyAction.Code, emptyAction.Body.String())
	}

	statistics := performEntityRequest(viewer, http.MethodGet,
		"/api/customers/7/stats?start_timestamp=1710000000&end_timestamp=1710003600&granularity=hour&site_ids=2,2,3&p=1&page_size=20&sort_by=quota&sort_order=asc", "")
	if statistics.Code != http.StatusOK || customers.lastStatistics.StartTimestamp != 1710000000 ||
		len(customers.lastStatistics.SiteIDs) != 2 || customers.lastStatistics.SiteIDs[0] != 2 ||
		customers.lastStatistics.SiteIDs[1] != 3 || customers.lastStatistics.SortBy != "quota" {
		t.Fatalf("customer statistics = %d query=%#v body=%s", statistics.Code, customers.lastStatistics, statistics.Body.String())
	}
	misalignedStats := performEntityRequest(viewer, http.MethodGet,
		"/api/customers/7/stats?start_timestamp=1710000001&end_timestamp=1710003600&granularity=hour", "")
	assertEntityError(t, misalignedStats, http.StatusBadRequest, constant.CodeValidationError, "range")
	overflowStats := performEntityRequest(viewer, http.MethodGet,
		"/api/accounts/1/stats?start_timestamp=1710000000&end_timestamp=1710003600&granularity=hour&p=9223372036854775807&page_size=100", "")
	assertEntityError(t, overflowStats, http.StatusBadRequest, constant.CodeValidationError, "p")
	duplicateStatsKey := performEntityRequest(viewer, http.MethodGet,
		"/api/accounts/1/stats?start_timestamp=1&start_timestamp=2&end_timestamp=3&granularity=hour", "")
	assertEntityError(t, duplicateStatsKey, http.StatusBadRequest, constant.CodeValidationError, "start_timestamp")
}

func newEntityRouteEngine(customers *fakeCustomerApplication, accounts *fakeAccountApplication, role string) http.Handler {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RequestID())
	registerCustomerRoutes(engine, controller.NewCustomerController(customers, accounts),
		controller.NewAccountController(accounts), entityRouteIdentityResolver{role: role})
	return engine
}

func performEntityRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("New-Api-User", "1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertEntityError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code, field string) {
	t.Helper()
	var envelope struct {
		Success     bool              `json:"success"`
		Code        string            `json:"code"`
		FieldErrors map[string]string `json:"field_errors"`
		RequestID   string            `json:"request_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, recorder.Body.String())
	}
	if recorder.Code != status || envelope.Success || envelope.Code != code || envelope.RequestID == "" {
		t.Fatalf("error envelope = %d %#v body=%s", recorder.Code, envelope, recorder.Body.String())
	}
	if field != "" && envelope.FieldErrors[field] == "" {
		t.Fatalf("missing field error %q: %#v", field, envelope.FieldErrors)
	}
}
