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

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const userRouterIntegrationLock = "new-api-pilot-platform-user-integration"

type userAPIHarness struct {
	handler    http.Handler
	repository *model.PlatformUserRepository
	users      *service.PlatformUserService
	auth       *service.AuthService
}

type userAPIEnvelope struct {
	Success     bool              `json:"success"`
	Code        string            `json:"code"`
	Data        json.RawMessage   `json:"data"`
	RequestID   string            `json:"request_id"`
	FieldErrors map[string]string `json:"field_errors"`
}

func TestUserAPIForcePasswordChangeAndSessionRotation(t *testing.T) {
	harness := newUserAPIHarness(t)
	ctx := context.Background()
	if _, err := harness.users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	admin, err := harness.repository.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}
	adminID := strconv.FormatInt(admin.ID, 10)

	login := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"admin","password":"bootstrap-pass"}`, nil, "")
	loginEnvelope := decodeUserAPIEnvelope(t, login)
	if login.Code != http.StatusOK || !loginEnvelope.Success {
		t.Fatalf("login = %d %#v body=%s", login.Code, loginEnvelope, login.Body.String())
	}
	var loginUser dto.LoginUser
	if err := json.Unmarshal(loginEnvelope.Data, &loginUser); err != nil || loginUser.ID != adminID || !loginUser.MustChangePassword {
		t.Fatalf("login user = %#v, %v", loginUser, err)
	}
	oldCookie := requireSessionCookie(t, login)

	blocked := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/", "", oldCookie, adminID)
	blockedEnvelope := decodeUserAPIEnvelope(t, blocked)
	if blocked.Code != http.StatusForbidden || blockedEnvelope.Code != constant.CodePasswordChangeRequired {
		t.Fatalf("force-password response = %d %#v", blocked.Code, blockedEnvelope)
	}
	wrongOriginalPassword := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/password", `{"original_password":"wrong-password","new_password":"changed-pass"}`, oldCookie, adminID)
	wrongOriginalEnvelope := decodeUserAPIEnvelope(t, wrongOriginalPassword)
	if wrongOriginalPassword.Code != http.StatusBadRequest || wrongOriginalEnvelope.Code != constant.CodeValidationError || wrongOriginalEnvelope.FieldErrors["original_password"] != "is incorrect" {
		t.Fatalf("wrong original password = %d %#v", wrongOriginalPassword.Code, wrongOriginalEnvelope)
	}

	changed := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/password", `{"original_password":"bootstrap-pass","new_password":"changed-pass"}`, oldCookie, adminID)
	changedEnvelope := decodeUserAPIEnvelope(t, changed)
	if changed.Code != http.StatusOK || !changedEnvelope.Success {
		t.Fatalf("change password = %d %#v body=%s", changed.Code, changedEnvelope, changed.Body.String())
	}
	newCookie := requireSessionCookie(t, changed)
	if newCookie.Value == oldCookie.Value {
		t.Fatal("password change did not rotate the session cookie")
	}

	stale := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/self", "", oldCookie, adminID)
	staleEnvelope := decodeUserAPIEnvelope(t, stale)
	if stale.Code != http.StatusUnauthorized || staleEnvelope.Code != constant.CodeAuthInvalid {
		t.Fatalf("stale session response = %d %#v", stale.Code, staleEnvelope)
	}
	fresh := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/self", "", newCookie, adminID)
	freshEnvelope := decodeUserAPIEnvelope(t, fresh)
	var current dto.LoginUser
	if fresh.Code != http.StatusOK || json.Unmarshal(freshEnvelope.Data, &current) != nil || current.MustChangePassword {
		t.Fatalf("fresh session response = %d %#v user=%#v", fresh.Code, freshEnvelope, current)
	}

	list := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/?p=1&page_size=1", "", newCookie, adminID)
	listEnvelope := decodeUserAPIEnvelope(t, list)
	var page common.PageData[dto.PlatformUserItem]
	if list.Code != http.StatusOK || json.Unmarshal(listEnvelope.Data, &page) != nil || page.Page != 1 || page.PageSize != 1 || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != adminID {
		t.Fatalf("platform user page = %d %#v data=%s", list.Code, page, string(listEnvelope.Data))
	}
	var rawPage struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listEnvelope.Data, &rawPage); err != nil {
		t.Fatalf("decode raw platform user page: %v", err)
	}
	if _, ok := rawPage.Items[0]["id"].(string); !ok {
		t.Fatalf("platform user id is not a JSON string: %#v", rawPage.Items[0]["id"])
	}

	unknown := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/", `{"username":"viewer-one","display_name":"Viewer","role":"viewer","password":"viewer-pass","unknown":true}`, newCookie, adminID)
	unknownEnvelope := decodeUserAPIEnvelope(t, unknown)
	if unknown.Code != http.StatusBadRequest || unknownEnvelope.Code != constant.CodeValidationError || unknownEnvelope.FieldErrors["body"] == "" {
		t.Fatalf("unknown-field response = %d %#v", unknown.Code, unknownEnvelope)
	}
	invalid := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/", `{"username":"BAD","display_name":"","role":"owner","password":"short"}`, newCookie, adminID)
	invalidEnvelope := decodeUserAPIEnvelope(t, invalid)
	if invalid.Code != http.StatusBadRequest || invalidEnvelope.Code != constant.CodeValidationError {
		t.Fatalf("invalid-user response = %d %#v", invalid.Code, invalidEnvelope)
	}
	for _, field := range []string{"username", "display_name", "role", "password"} {
		if invalidEnvelope.FieldErrors[field] == "" {
			t.Errorf("missing %s field error: %#v", field, invalidEnvelope.FieldErrors)
		}
	}
}

func TestViewerCanReadButCannotWritePlatformUsers(t *testing.T) {
	harness := newUserAPIHarness(t)
	ctx := context.Background()
	if _, err := harness.users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	viewer, err := harness.users.Create(ctx, dto.CreatePlatformUserRequest{
		Username: "viewer-one", DisplayName: "Viewer One", Role: constant.RoleViewer, Password: "viewer-pass",
	})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if _, err := harness.auth.ChangePassword(ctx, viewer.ID, dto.ChangePasswordRequest{
		OriginalPassword: "viewer-pass", NewPassword: "viewer-changed",
	}); err != nil {
		t.Fatalf("clear viewer password-change gate: %v", err)
	}
	viewerID := strconv.FormatInt(viewer.ID, 10)
	login := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"viewer-one","password":"viewer-changed"}`, nil, "")
	if login.Code != http.StatusOK {
		t.Fatalf("viewer login = %d body=%s", login.Code, login.Body.String())
	}
	cookie := requireSessionCookie(t, login)

	read := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/", "", cookie, viewerID)
	if envelope := decodeUserAPIEnvelope(t, read); read.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("viewer read = %d %#v", read.Code, envelope)
	}
	write := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/", `{"username":"other-viewer","display_name":"Other Viewer","role":"viewer","password":"viewer-pass"}`, cookie, viewerID)
	writeEnvelope := decodeUserAPIEnvelope(t, write)
	if write.Code != http.StatusForbidden || writeEnvelope.Code != constant.CodeForbidden {
		t.Fatalf("viewer write = %d %#v", write.Code, writeEnvelope)
	}
}

func TestLoginRateLimitReturnsRetryAfter(t *testing.T) {
	harness := newUserAPIHarness(t)
	if _, err := harness.users.EnsureBootstrapAdmin(context.Background(), "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		response := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"admin","password":"wrong-pass"}`, nil, "")
		envelope := decodeUserAPIEnvelope(t, response)
		if response.Code != http.StatusUnauthorized || envelope.Code != constant.CodeAuthInvalid {
			t.Fatalf("failed login %d = %d %#v", attempt, response.Code, envelope)
		}
	}
	limited := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"admin","password":"wrong-pass"}`, nil, "")
	limitedEnvelope := decodeUserAPIEnvelope(t, limited)
	if limited.Code != http.StatusTooManyRequests || limitedEnvelope.Code != constant.CodeLoginRateLimited || limited.Header().Get("Retry-After") != "900" {
		t.Fatalf("rate-limited login = %d headers=%v %#v", limited.Code, limited.Header(), limitedEnvelope)
	}
}

func TestDisabledUserLoginReportsDisabledOnlyAfterPasswordVerification(t *testing.T) {
	harness := newUserAPIHarness(t)
	ctx := context.Background()
	if _, err := harness.users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	user, err := harness.users.Create(ctx, dto.CreatePlatformUserRequest{
		Username: "disabled-user", DisplayName: "Disabled User", Role: constant.RoleViewer, Password: "viewer-pass",
	})
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}
	if err := harness.users.SetStatus(ctx, 0, user.ID, false); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	wrongPassword := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"disabled-user","password":"wrong-pass"}`, nil, "")
	if envelope := decodeUserAPIEnvelope(t, wrongPassword); wrongPassword.Code != http.StatusUnauthorized || envelope.Code != constant.CodeAuthInvalid {
		t.Fatalf("disabled user with wrong password = %d %#v", wrongPassword.Code, envelope)
	}

	correctPassword := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"disabled-user","password":"viewer-pass"}`, nil, "")
	if envelope := decodeUserAPIEnvelope(t, correctPassword); correctPassword.Code != http.StatusForbidden || envelope.Code != constant.CodeUserDisabled {
		t.Fatalf("disabled user with correct password = %d %#v", correctPassword.Code, envelope)
	}
}

func TestPlatformUserAdminLifecycleOverHTTP(t *testing.T) {
	harness := newUserAPIHarness(t)
	ctx := context.Background()
	if _, err := harness.users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	admin, err := harness.repository.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find bootstrap admin: %v", err)
	}
	adminID := strconv.FormatInt(admin.ID, 10)

	bootstrapLogin := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"admin","password":"bootstrap-pass"}`, nil, "")
	if envelope := decodeUserAPIEnvelope(t, bootstrapLogin); bootstrapLogin.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("bootstrap login = %d %#v", bootstrapLogin.Code, envelope)
	}
	bootstrapCookie := requireSessionCookie(t, bootstrapLogin)
	changedAdmin := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/password", `{"original_password":"bootstrap-pass","new_password":"admin-changed"}`, bootstrapCookie, adminID)
	if envelope := decodeUserAPIEnvelope(t, changedAdmin); changedAdmin.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("change bootstrap password = %d %#v", changedAdmin.Code, envelope)
	}
	adminCookie := requireSessionCookie(t, changedAdmin)

	lastAdminDowngrade := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/"+adminID, `{"username":"admin","display_name":"Administrator","role":"viewer"}`, adminCookie, adminID)
	if envelope := decodeUserAPIEnvelope(t, lastAdminDowngrade); lastAdminDowngrade.Code != http.StatusConflict || envelope.Code != constant.CodeLastAdmin {
		t.Fatalf("last admin downgrade = %d %#v", lastAdminDowngrade.Code, envelope)
	}
	selfReset := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/"+adminID+"/reset-password", `{"new_password":"self-reset"}`, adminCookie, adminID)
	if envelope := decodeUserAPIEnvelope(t, selfReset); selfReset.Code != http.StatusConflict || envelope.Code != constant.CodeConflict {
		t.Fatalf("self password reset = %d %#v", selfReset.Code, envelope)
	}

	created := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/", `{"username":"viewer-life","display_name":"Viewer Lifecycle","role":"viewer","password":"viewer-initial"}`, adminCookie, adminID)
	createdEnvelope := decodeUserAPIEnvelope(t, created)
	var createdUser dto.PlatformUserItem
	if created.Code != http.StatusOK || !createdEnvelope.Success || json.Unmarshal(createdEnvelope.Data, &createdUser) != nil || !createdUser.MustChangePassword || createdUser.Status != constant.UserStatusEnabled {
		t.Fatalf("create lifecycle user = %d %#v user=%#v", created.Code, createdEnvelope, createdUser)
	}

	firstLogin := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"viewer-life","password":"viewer-initial"}`, nil, "")
	firstCookie := requireSessionCookie(t, firstLogin)
	if envelope := decodeUserAPIEnvelope(t, firstLogin); firstLogin.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("first user login = %d %#v", firstLogin.Code, envelope)
	}
	blocked := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/", "", firstCookie, createdUser.ID)
	if envelope := decodeUserAPIEnvelope(t, blocked); blocked.Code != http.StatusForbidden || envelope.Code != constant.CodePasswordChangeRequired {
		t.Fatalf("first user bypassed password change = %d %#v", blocked.Code, envelope)
	}
	firstChange := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/password", `{"original_password":"viewer-initial","new_password":"viewer-changed"}`, firstCookie, createdUser.ID)
	if envelope := decodeUserAPIEnvelope(t, firstChange); firstChange.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("first user password change = %d %#v", firstChange.Code, envelope)
	}
	activeUserCookie := requireSessionCookie(t, firstChange)

	updated := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/"+createdUser.ID, `{"username":"viewer-renamed","display_name":"Viewer Renamed","role":"viewer"}`, adminCookie, adminID)
	updatedEnvelope := decodeUserAPIEnvelope(t, updated)
	var updatedUser dto.PlatformUserItem
	if updated.Code != http.StatusOK || !updatedEnvelope.Success || json.Unmarshal(updatedEnvelope.Data, &updatedUser) != nil || updatedUser.Username != "viewer-renamed" || updatedUser.DisplayName != "Viewer Renamed" {
		t.Fatalf("update lifecycle user = %d %#v user=%#v", updated.Code, updatedEnvelope, updatedUser)
	}

	reset := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/"+createdUser.ID+"/reset-password", `{"new_password":"viewer-reset"}`, adminCookie, adminID)
	if envelope := decodeUserAPIEnvelope(t, reset); reset.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("reset lifecycle user password = %d %#v", reset.Code, envelope)
	}
	stale := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/self", "", activeUserCookie, createdUser.ID)
	if envelope := decodeUserAPIEnvelope(t, stale); stale.Code != http.StatusUnauthorized || envelope.Code != constant.CodeAuthInvalid {
		t.Fatalf("reset did not invalidate old user session = %d %#v", stale.Code, envelope)
	}

	disabled := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/"+createdUser.ID+"/disable", "", adminCookie, adminID)
	if envelope := decodeUserAPIEnvelope(t, disabled); disabled.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("disable lifecycle user = %d %#v", disabled.Code, envelope)
	}
	wrongDisabledLogin := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"viewer-renamed","password":"wrong"}`, nil, "")
	if envelope := decodeUserAPIEnvelope(t, wrongDisabledLogin); wrongDisabledLogin.Code != http.StatusUnauthorized || envelope.Code != constant.CodeAuthInvalid {
		t.Fatalf("disabled user wrong password = %d %#v", wrongDisabledLogin.Code, envelope)
	}
	correctDisabledLogin := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"viewer-renamed","password":"viewer-reset"}`, nil, "")
	if envelope := decodeUserAPIEnvelope(t, correctDisabledLogin); correctDisabledLogin.Code != http.StatusForbidden || envelope.Code != constant.CodeUserDisabled {
		t.Fatalf("disabled user correct password = %d %#v", correctDisabledLogin.Code, envelope)
	}

	enabled := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/"+createdUser.ID+"/enable", "", adminCookie, adminID)
	if envelope := decodeUserAPIEnvelope(t, enabled); enabled.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("enable lifecycle user = %d %#v", enabled.Code, envelope)
	}
	resetLogin := performUserAPIRequest(harness.handler, http.MethodPost, "/api/user/login", `{"username":"viewer-renamed","password":"viewer-reset"}`, nil, "")
	if envelope := decodeUserAPIEnvelope(t, resetLogin); resetLogin.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("enabled user reset-password login = %d %#v", resetLogin.Code, envelope)
	}
	resetCookie := requireSessionCookie(t, resetLogin)
	stillBlocked := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/", "", resetCookie, createdUser.ID)
	if envelope := decodeUserAPIEnvelope(t, stillBlocked); stillBlocked.Code != http.StatusForbidden || envelope.Code != constant.CodePasswordChangeRequired {
		t.Fatalf("reset user bypassed password change = %d %#v", stillBlocked.Code, envelope)
	}
	finalChange := performUserAPIRequest(harness.handler, http.MethodPut, "/api/user/password", `{"original_password":"viewer-reset","new_password":"viewer-final"}`, resetCookie, createdUser.ID)
	if envelope := decodeUserAPIEnvelope(t, finalChange); finalChange.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("reset user password change = %d %#v", finalChange.Code, envelope)
	}

	filtered := performUserAPIRequest(harness.handler, http.MethodGet, "/api/user/?keyword=viewer-renamed&role=viewer&status=1&sort_by=username&sort_order=asc", "", adminCookie, adminID)
	filteredEnvelope := decodeUserAPIEnvelope(t, filtered)
	var page common.PageData[dto.PlatformUserItem]
	if filtered.Code != http.StatusOK || !filteredEnvelope.Success || json.Unmarshal(filteredEnvelope.Data, &page) != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != createdUser.ID || page.Items[0].MustChangePassword {
		t.Fatalf("filtered lifecycle user list = %d %#v page=%#v", filtered.Code, filteredEnvelope, page)
	}
}

func newUserAPIHarness(t *testing.T) userAPIHarness {
	t.Helper()
	database := openLockedUserRouterDatabase(t)
	clock := testsupport.NewFakeClock(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	repository := model.NewPlatformUserRepository(database.GORM)
	users := service.NewPlatformUserService(repository, clock)
	sessions, err := common.NewSessionStore([]byte("01234567890123456789012345678901"), false, clock)
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	auth, err := service.NewAuthService(repository, service.NewLoginLimiter(clock), clock)
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	authController := controller.NewAuthController(auth, users, sessions)
	userController := controller.NewPlatformUserController(users)
	resolver := middleware.SessionIdentityResolver{Store: sessions, Loader: auth}
	gin.SetMode(gin.TestMode)
	engine, err := New(Options{
		Config:         config.Config{AppEnv: config.EnvironmentTest},
		AuthController: authController, UserController: userController, IdentityResolver: resolver,
	})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	return userAPIHarness{handler: engine, repository: repository, users: users, auth: auth}
}

func openLockedUserRouterDatabase(t *testing.T) *model.Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	lockConnection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve test lock connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := lockConnection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", userRouterIntegrationLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = lockConnection.Close()
		_ = database.Close()
		t.Fatalf("acquire user router test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", userRouterIntegrationLock)
		_ = lockConnection.Close()
		_ = database.Close()
		t.Fatalf("run migrations: %v", err)
	}
	for _, statement := range []string{"DELETE FROM export_job", "DELETE FROM platform_user", "ALTER TABLE platform_user AUTO_INCREMENT = 1"} {
		if _, err := database.SQL.ExecContext(ctx, statement); err != nil {
			t.Fatalf("reset platform user fixtures with %q: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.SQL.ExecContext(cleanupContext, "DELETE FROM export_job")
		_, _ = database.SQL.ExecContext(cleanupContext, "DELETE FROM platform_user")
		_, _ = lockConnection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", userRouterIntegrationLock)
		_ = lockConnection.Close()
		_ = database.Close()
	})
	return database
}

func performUserAPIRequest(handler http.Handler, method, target, body string, cookie *http.Cookie, userID string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.RemoteAddr = "198.51.100.10:4321"
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if userID != "" {
		request.Header.Set("New-Api-User", userID)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeUserAPIEnvelope(t *testing.T, response *httptest.ResponseRecorder) userAPIEnvelope {
	t.Helper()
	var envelope userAPIEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode API response %d body=%q: %v", response.Code, response.Body.String(), err)
	}
	if envelope.RequestID == "" {
		t.Fatalf("API response %d has no request ID: %s", response.Code, response.Body.String())
	}
	return envelope
}

func requireSessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == common.SessionCookieName {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie: headers=%v", common.SessionCookieName, response.Header())
	return nil
}
