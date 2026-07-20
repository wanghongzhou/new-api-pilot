package router

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/middleware"
)

func TestOperationalEndpointsAndAPIEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readiness := common.NewReadiness()
	metrics := common.NewMetrics()
	engine, err := New(Options{
		Config: config.Config{
			AppEnv:              config.EnvironmentTest,
			MetricsAllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		},
		Readiness: readiness,
		Metrics:   metrics,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response := performRequest(engine, http.MethodGet, "/healthz", "127.0.0.1:1000")
	if response.Code != http.StatusOK || response.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("health response = %d headers=%v", response.Code, response.Header())
	}
	response = performRequest(engine, http.MethodGet, "/readyz", "127.0.0.1:1000")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-ready status = %d", response.Code)
	}
	readiness.SetInitialized(true)
	response = performRequest(engine, http.MethodGet, "/readyz", "127.0.0.1:1000")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("scheduler-not-ready status = %d", response.Code)
	}
	readiness.SetSchedulerReady(true)
	response = performRequest(engine, http.MethodGet, "/readyz", "127.0.0.1:1000")
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d body=%s", response.Code, response.Body.String())
	}

	response = performRequest(engine, http.MethodGet, "/metrics", "127.0.0.1:1000")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "new_api_pilot_ready") {
		t.Fatalf("metrics status = %d body=%s", response.Code, response.Body.String())
	}
	response = performRequest(engine, http.MethodGet, "/metrics", "192.0.2.1:1000")
	if response.Code != http.StatusForbidden {
		t.Fatalf("restricted metrics status = %d", response.Code)
	}

	response = performRequest(engine, http.MethodGet, "/api/missing", "127.0.0.1:1000")
	var envelope common.APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode API envelope: %v", err)
	}
	if response.Code != http.StatusNotFound || envelope.Code != constant.CodeNotFound || envelope.RequestID == "" {
		t.Fatalf("missing API response = %d %#v", response.Code, envelope)
	}
}

func TestAccessLogCoversUserRoutesRegisteredByNew(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	engine, err := New(Options{
		Config:           config.Config{AppEnv: config.EnvironmentTest},
		AuthController:   controller.NewAuthController(nil, nil, nil),
		UserController:   controller.NewPlatformUserController(nil),
		IdentityResolver: fixedRouterIdentityResolver{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response := performRequest(engine, http.MethodPost, "/api/user/login", "127.0.0.1:1000")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty login status = %d body=%s", response.Code, response.Body.String())
	}
	if logged := output.String(); !strings.Contains(logged, "method=POST route=/api/user/login status=400") {
		t.Fatalf("user route was not access logged: %q", logged)
	}
}

func TestNewRegistersStableFeatureRoutesWithAuthenticationAndRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newEngine := func(role string) *gin.Engine {
		t.Helper()
		engine, err := New(Options{
			Config:               config.Config{AppEnv: config.EnvironmentTest},
			CustomerController:   controller.NewCustomerController(&fakeCustomerApplication{}, &fakeAccountApplication{}),
			AccountController:    controller.NewAccountController(&fakeAccountApplication{}),
			StatisticsController: controller.NewStatisticsController(&fakeStatisticsApplication{}),
			DashboardController:  controller.NewDashboardController(dashboardRouteApplication{}),
			AlertController:      controller.NewAlertController(nil),
			IdentityResolver:     entityRouteIdentityResolver{role: role},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		return engine
	}
	viewer := newEngine(constant.RoleViewer)

	registered := make(map[string]struct{})
	for _, route := range viewer.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"GET /api/customers", "GET /api/accounts", "GET /api/statistics/global",
		"GET /api/dashboard/summary", "GET /api/alerts/summary", "GET /api/alert-rules",
	} {
		if _, exists := registered[route]; !exists {
			t.Errorf("stable route %s was not registered", route)
		}
	}

	response := performAuthenticatedRequest(viewer, http.MethodGet, "/api/customers", "1")
	if response.Code != http.StatusOK {
		t.Fatalf("viewer read status = %d body=%s", response.Code, response.Body.String())
	}
	response = performAuthenticatedRequest(viewer, http.MethodPost, "/api/customers", "1")
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer write status = %d body=%s", response.Code, response.Body.String())
	}
	response = performRequest(viewer, http.MethodGet, "/api/dashboard/summary", "127.0.0.1:1000")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("registered unauthenticated status = %d body=%s", response.Code, response.Body.String())
	}
	response = performAuthenticatedRequest(viewer, http.MethodGet, "/api/not-registered", "1")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown authenticated status = %d body=%s", response.Code, response.Body.String())
	}

	admin := newEngine(constant.RoleAdmin)
	response = performAuthenticatedRequest(admin, http.MethodPost, "/api/customers", "1")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("admin write routing status = %d body=%s", response.Code, response.Body.String())
	}
}

type fixedRouterIdentityResolver struct{}

func (fixedRouterIdentityResolver) ResolveIdentity(*gin.Context) (middleware.Identity, error) {
	return middleware.Identity{}, middleware.ErrIdentityMissing
}

func performRequest(handler http.Handler, method, target, remoteAddress string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.RemoteAddr = remoteAddress
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func performAuthenticatedRequest(handler http.Handler, method, target, userID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.RemoteAddr = "127.0.0.1:1000"
	request.Header.Set("New-Api-User", userID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
