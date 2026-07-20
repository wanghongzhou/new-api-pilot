package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

type coreAuthFixture struct {
	FixtureID      string `json:"fixture_id"`
	PasswordPolicy struct {
		Valid []struct {
			Value string `json:"value"`
		} `json:"valid"`
		Invalid []struct {
			Value string `json:"value"`
		} `json:"invalid"`
	} `json:"password_policy"`
}

func TestA01A02A03AuthenticationLifecycleAcceptance(t *testing.T) {
	fixture := loadCoreAuthFixture(t)
	if fixture.FixtureID != "F01" || len(fixture.PasswordPolicy.Valid) == 0 || len(fixture.PasswordPolicy.Invalid) == 0 {
		t.Fatalf("invalid F01 authentication fixture: %#v", fixture)
	}

	for _, candidate := range fixture.PasswordPolicy.Valid {
		if fields := (dto.CreatePlatformUserRequest{
			Username: "fixture-viewer", DisplayName: "Fixture Viewer", Role: constant.RoleViewer, Password: candidate.Value,
		}).Validate(); fields != nil {
			t.Fatalf("F01 valid create password rejected: %#v", fields)
		}
		if fields := (dto.ChangePasswordRequest{OriginalPassword: "current-password", NewPassword: candidate.Value}).Validate(); fields != nil {
			t.Fatalf("F01 valid change password rejected: %#v", fields)
		}
		if fields := (dto.ResetPasswordRequest{NewPassword: candidate.Value}).Validate(); fields != nil {
			t.Fatalf("F01 valid reset password rejected: %#v", fields)
		}
	}
	for _, candidate := range fixture.PasswordPolicy.Invalid {
		createFields := (dto.CreatePlatformUserRequest{
			Username: "fixture-viewer", DisplayName: "Fixture Viewer", Role: constant.RoleViewer, Password: candidate.Value,
		}).Validate()
		if createFields == nil || createFields["password"] == "" {
			t.Fatalf("F01 invalid create password accepted: %#v", candidate)
		}
		changeFields := (dto.ChangePasswordRequest{OriginalPassword: "current-password", NewPassword: candidate.Value}).Validate()
		if changeFields == nil || changeFields["new_password"] == "" {
			t.Fatalf("F01 invalid change password accepted: %#v", candidate)
		}
		resetFields := (dto.ResetPasswordRequest{NewPassword: candidate.Value}).Validate()
		if resetFields == nil || resetFields["new_password"] == "" {
			t.Fatalf("F01 invalid reset password accepted: %#v", candidate)
		}
	}
	if fields := (dto.LoginRequest{Username: "admin", Password: fixture.PasswordPolicy.Invalid[0].Value}).Validate(); fields != nil {
		t.Fatalf("non-empty login password was incorrectly subjected to the password-setting policy: %#v", fields)
	}

	database := openCoreAcceptanceTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_768_622_400, 0))
	repository := model.NewPlatformUserRepository(database)
	users := service.NewPlatformUserService(repository, clock)
	bootstrapPassword := fixture.PasswordPolicy.Valid[0].Value
	bootstrap, err := users.EnsureBootstrapAdmin(context.Background(), bootstrapPassword)
	if err != nil || !bootstrap.Created || bootstrap.GeneratedPassword != "" {
		t.Fatalf("bootstrap admin = %#v, %v", bootstrap, err)
	}
	admin, err := repository.FindByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("load bootstrap admin: %v", err)
	}
	if admin.Role != constant.RoleAdmin || admin.Status != constant.UserStatusEnabled || !admin.MustChangePassword || admin.PasswordHash == bootstrapPassword {
		t.Fatalf("bootstrap admin contract = %#v", admin)
	}
	if err := common.CheckPassword(admin.PasswordHash, bootstrapPassword); err != nil {
		t.Fatalf("bootstrap password hash does not verify: %v", err)
	}

	loginLimiter := service.NewLoginLimiter(clock)
	auth, err := service.NewAuthService(repository, loginLimiter, clock)
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	loggedIn, err := auth.Login(context.Background(), "192.0.2.10", dto.LoginRequest{Username: "admin", Password: bootstrapPassword})
	if err != nil || loggedIn.ID != admin.ID || !loggedIn.MustChangePassword {
		t.Fatalf("bootstrap login = %#v, %v", loggedIn, err)
	}

	sessions, err := common.NewSessionStore([]byte("0123456789abcdef0123456789abcdef"), false, clock)
	if err != nil {
		t.Fatalf("create session store: %v", err)
	}
	writer := httptest.NewRecorder()
	if err := sessions.Write(writer, service.IdentityFromUser(loggedIn)); err != nil {
		t.Fatalf("write bootstrap session: %v", err)
	}
	var cookie *http.Cookie
	for _, candidate := range writer.Result().Cookies() {
		if candidate.Name == common.SessionCookieName {
			cookie = candidate
			break
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure {
		t.Fatalf("bootstrap session cookie contract = %#v", cookie)
	}

	secondAdmin, err := users.Create(context.Background(), dto.CreatePlatformUserRequest{
		Username: "core-second-admin", DisplayName: "Core Second Admin", Role: constant.RoleAdmin, Password: bootstrapPassword,
	})
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	if _, err := users.Update(context.Background(), admin.ID, dto.UpdatePlatformUserRequest{
		Username: admin.Username, DisplayName: admin.DisplayName, Role: constant.RoleViewer,
	}); err != nil {
		t.Fatalf("downgrade original admin after creating replacement: %v", err)
	}
	if secondAdmin.ID <= 0 {
		t.Fatalf("second admin did not receive an ID: %#v", secondAdmin)
	}

	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	request.AddCookie(cookie)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	resolver := middleware.SessionIdentityResolver{Store: sessions, Loader: auth}
	if _, err := resolver.ResolveIdentity(context); !errors.Is(err, middleware.ErrIdentityInvalid) {
		t.Fatalf("old session remained valid after role downgrade: %v", err)
	}
}

func loadCoreAuthFixture(t *testing.T) coreAuthFixture {
	t.Helper()
	contents, err := os.ReadFile(testsupport.DesignFixturePath("f01-auth.json"))
	if err != nil {
		t.Fatalf("read F01 auth fixture: %v", err)
	}
	var fixture coreAuthFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode F01 auth fixture: %v", err)
	}
	return fixture
}
