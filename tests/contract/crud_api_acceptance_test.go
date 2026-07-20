package contract_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/router"
)

func TestA76CRUDRouteAndDTOAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	customers := &coreContractCustomerApplication{}
	accounts := &coreContractAccountApplication{}
	engine, err := router.New(router.Options{
		Config:             config.Config{AppEnv: config.EnvironmentTest},
		AuthController:     controller.NewAuthController(nil, nil, nil),
		UserController:     controller.NewPlatformUserController(nil),
		SiteController:     controller.NewSiteController(nil),
		CustomerController: controller.NewCustomerController(customers, accounts),
		AccountController:  controller.NewAccountController(accounts),
		IdentityResolver:   coreContractIdentityResolver{role: constant.RoleAdmin},
	})
	if err != nil {
		t.Fatalf("create A76 router: %v", err)
	}
	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, expected := range []string{
		"POST /api/user/login", "POST /api/user/logout", "GET /api/user/self", "PUT /api/user/password",
		"GET /api/user/", "POST /api/user/", "PUT /api/user/:id", "POST /api/user/:id/enable",
		"POST /api/user/:id/disable", "POST /api/user/:id/reset-password",
		"GET /api/sites", "POST /api/sites", "PUT /api/sites/:id", "DELETE /api/sites/:id",
		"GET /api/customers", "POST /api/customers", "PUT /api/customers/:id", "DELETE /api/customers/:id",
		"GET /api/accounts", "POST /api/accounts", "PUT /api/accounts/:id", "DELETE /api/accounts/:id",
		"POST /api/accounts/:id/archive", "POST /api/accounts/:id/restore",
	} {
		if _, exists := routes[expected]; !exists {
			t.Errorf("A76 route is missing: %s", expected)
		}
	}
	if _, exists := routes["DELETE /api/user/:id"]; exists {
		t.Fatal("A76 platform-user physical delete route must not exist")
	}

	validAccount := dto.AccountCreateRequest{SiteID: "9007199254740993", CustomerID: "9007199254740994", RemoteUserID: "9007199254740995"}
	if fields := validAccount.Validate(); fields != nil {
		t.Fatalf("A76 valid fixed account binding rejected: %#v", fields)
	}
	for _, invalid := range []dto.AccountCreateRequest{
		{SiteID: "01", CustomerID: "2", RemoteUserID: "3"},
		{SiteID: "1", CustomerID: "-2", RemoteUserID: "3"},
		{SiteID: "1", CustomerID: "2", RemoteUserID: "0"},
	} {
		if fields := invalid.Validate(); fields == nil {
			t.Fatalf("A76 non-canonical account binding accepted: %#v", invalid)
		}
	}
	if fields := (dto.CreatePlatformUserRequest{
		Username: "new-admin", DisplayName: "New Admin", Role: constant.RoleAdmin, Password: "Correct-Horse-2026!",
	}).Validate(); fields != nil {
		t.Fatalf("A76 valid platform user request rejected: %#v", fields)
	}
	if fields := (dto.CreatePlatformUserRequest{
		Username: "INVALID", DisplayName: "", Role: "owner", Password: "short",
	}).Validate(); fields == nil || fields["username"] == "" || fields["display_name"] == "" || fields["role"] == "" || fields["password"] == "" {
		t.Fatalf("A76 invalid platform user fields = %#v", fields)
	}

	createdCustomer := coreContractRequest(engine, http.MethodPost, "/api/customers", `{"name":"CRUD customer","status":"using"}`)
	if envelope := decodeCoreContractEnvelope(t, createdCustomer); createdCustomer.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("A76 admin customer create = %d %#v", createdCustomer.Code, envelope)
	}
	updatedCustomer := coreContractRequest(engine, http.MethodPut, "/api/customers/9007199254740993", `{"name":"CRUD customer","status":"using"}`)
	if envelope := decodeCoreContractEnvelope(t, updatedCustomer); updatedCustomer.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("A76 admin customer update = %d %#v", updatedCustomer.Code, envelope)
	}
	deletedCustomer := coreContractRequest(engine, http.MethodDelete, "/api/customers/9007199254740993", "")
	if envelope := decodeCoreContractEnvelope(t, deletedCustomer); deletedCustomer.Code != http.StatusOK || !envelope.Success || string(envelope.Data) != "null" {
		t.Fatalf("A76 admin customer delete = %d %#v", deletedCustomer.Code, envelope)
	}
	createdAccount := coreContractRequest(engine, http.MethodPost, "/api/accounts", `{"site_id":"9007199254740993","customer_id":"9007199254740994","remote_user_id":"9007199254740995"}`)
	if envelope := decodeCoreContractEnvelope(t, createdAccount); createdAccount.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("A76 admin account create = %d %#v", createdAccount.Code, envelope)
	}
	invalidActionBody := coreContractRequest(engine, http.MethodPost, "/api/accounts/9007199254740995/archive", `{}`)
	if envelope := decodeCoreContractEnvelope(t, invalidActionBody); invalidActionBody.Code != http.StatusBadRequest || envelope.Code != constant.CodeValidationError || envelope.FieldErrors["body"] == "" {
		t.Fatalf("A76 archive non-empty action body = %d %#v", invalidActionBody.Code, envelope)
	}

	for _, entity := range []any{model.Site{}, model.Customer{}, model.Account{}} {
		entityType := reflect.TypeOf(entity)
		if _, exists := entityType.FieldByName("DeletedAt"); exists {
			t.Fatalf("A76 %s unexpectedly has a soft-delete field", entityType.Name())
		}
	}
}
