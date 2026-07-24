package service_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const platformUserIntegrationLock = "new-api-pilot-platform-user-integration"

func TestBootstrapAdminIsIdempotent(t *testing.T) {
	_, repository, users := newPlatformUserIntegrationHarness(t)
	ctx := context.Background()

	first, err := users.EnsureBootstrapAdmin(ctx, "bootstrap-pass")
	if err != nil || !first.Created || first.GeneratedPassword != "" {
		t.Fatalf("first bootstrap = %#v, %v", first, err)
	}
	second, err := users.EnsureBootstrapAdmin(ctx, "different-pass")
	if err != nil || second.Created {
		t.Fatalf("second bootstrap = %#v, %v", second, err)
	}

	count, err := repository.Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("platform user count = %d, %v", count, err)
	}
	admin, err := repository.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find bootstrap admin: %v", err)
	}
	if admin.Role != constant.RoleAdmin || admin.Status != constant.UserStatusEnabled || !admin.MustChangePassword || admin.SessionVersion != 1 {
		t.Fatalf("bootstrap admin = %#v", admin)
	}
	if err := common.CheckPassword(admin.PasswordHash, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap password does not match: %v", err)
	}
}

func TestConcurrentAdminDowngradePreservesOneEnabledAdmin(t *testing.T) {
	_, repository, users := newPlatformUserIntegrationHarness(t)
	ctx := context.Background()
	if _, err := users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	first, err := repository.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find first admin: %v", err)
	}
	second, err := users.Create(ctx, dto.CreatePlatformUserRequest{
		Username: "second-admin", DisplayName: "Second Admin", Role: constant.RoleAdmin, Password: "second-pass",
	})
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}

	type result struct {
		id  int64
		err error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for _, candidate := range []model.PlatformUser{first, second} {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, updateErr := users.Update(ctx, candidate.ID, dto.UpdatePlatformUserRequest{
				Username: candidate.Username, DisplayName: candidate.DisplayName, Role: constant.RoleViewer,
			})
			results <- result{id: candidate.ID, err: updateErr}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	succeeded := 0
	rejected := 0
	for outcome := range results {
		switch {
		case outcome.err == nil:
			succeeded++
		case errors.Is(outcome.err, service.ErrLastAdmin):
			rejected++
		default:
			t.Fatalf("downgrade user %d error = %v", outcome.id, outcome.err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent downgrade succeeded=%d rejected=%d", succeeded, rejected)
	}
	status := constant.UserStatusEnabled
	page, err := users.List(ctx, dto.PlatformUserListQuery{
		Page: 1, PageSize: 20, Role: constant.RoleAdmin, Status: &status, SortBy: "username", SortOrder: "asc",
	})
	if err != nil || page.Total != 1 {
		t.Fatalf("enabled admin page = %#v, %v", page, err)
	}
}

func TestUserMutationsEnforceUniquenessAndRotateSessions(t *testing.T) {
	_, repository, users := newPlatformUserIntegrationHarness(t)
	ctx := context.Background()
	if _, err := users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	admin, err := repository.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}
	viewer, err := users.Create(ctx, dto.CreatePlatformUserRequest{
		Username: "viewer-one", DisplayName: "Viewer One", Role: constant.RoleViewer, Password: "viewer-pass",
	})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if _, err := users.Create(ctx, dto.CreatePlatformUserRequest{
		Username: "viewer-one", DisplayName: "Duplicate", Role: constant.RoleViewer, Password: "viewer-pass",
	}); !errors.Is(err, service.ErrUsernameConflict) {
		t.Fatalf("duplicate username error = %v", err)
	}

	viewer, err = users.Update(ctx, viewer.ID, dto.UpdatePlatformUserRequest{
		Username: "viewer-renamed", DisplayName: "Viewer Renamed", Role: constant.RoleAdmin,
	})
	if err != nil || viewer.SessionVersion != 2 {
		t.Fatalf("role update user = %#v, %v", viewer, err)
	}
	if err := users.SetStatus(ctx, admin.ID, viewer.ID, false); err != nil {
		t.Fatalf("disable viewer: %v", err)
	}
	viewer = mustFindUser(t, repository, viewer.ID)
	if viewer.Status != constant.UserStatusDisabled || viewer.SessionVersion != 3 {
		t.Fatalf("disabled user = %#v", viewer)
	}
	if err := users.SetStatus(ctx, admin.ID, viewer.ID, true); err != nil {
		t.Fatalf("enable viewer: %v", err)
	}
	viewer = mustFindUser(t, repository, viewer.ID)
	if viewer.Status != constant.UserStatusEnabled || viewer.SessionVersion != 4 {
		t.Fatalf("enabled user = %#v", viewer)
	}
	if err := users.ResetPassword(ctx, admin.ID, viewer.ID, "rotated-pass"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	viewer = mustFindUser(t, repository, viewer.ID)
	if !viewer.MustChangePassword || viewer.SessionVersion != 5 {
		t.Fatalf("password-reset user = %#v", viewer)
	}
	if err := common.CheckPassword(viewer.PasswordHash, "rotated-pass"); err != nil {
		t.Fatalf("reset password does not match: %v", err)
	}
}

func TestPlatformUserListTreatsLikeMetacharactersLiterally(t *testing.T) {
	_, _, users := newPlatformUserIntegrationHarness(t)
	ctx := context.Background()
	for _, request := range []dto.CreatePlatformUserRequest{
		{Username: "percent-user", DisplayName: "Percent 100%", Role: constant.RoleViewer, Password: "viewer-pass"},
		{Username: "under-user", DisplayName: "Under_score", Role: constant.RoleViewer, Password: "viewer-pass"},
		{Username: "slash-user", DisplayName: `Back\slash`, Role: constant.RoleViewer, Password: "viewer-pass"},
		{Username: "ordinary-user", DisplayName: "Ordinary", Role: constant.RoleViewer, Password: "viewer-pass"},
	} {
		if _, err := users.Create(ctx, request); err != nil {
			t.Fatalf("create %s: %v", request.Username, err)
		}
	}
	for keyword, wantUsername := range map[string]string{"%": "percent-user", "_": "under-user", `\`: "slash-user"} {
		page, err := users.List(ctx, dto.PlatformUserListQuery{
			Page: 1, PageSize: 20, Keyword: keyword, SortBy: "username", SortOrder: "asc",
		})
		if err != nil {
			t.Fatalf("list keyword %q: %v", keyword, err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Username != wantUsername {
			t.Fatalf("list keyword %q = %#v", keyword, page)
		}
	}
}

func TestPlatformUserListSortsByStatus(t *testing.T) {
	_, repository, users := newPlatformUserIntegrationHarness(t)
	ctx := context.Background()
	if _, err := users.EnsureBootstrapAdmin(ctx, "bootstrap-pass"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	viewer, err := users.Create(ctx, dto.CreatePlatformUserRequest{
		Username: "status-viewer", DisplayName: "Status Viewer", Role: constant.RoleViewer, Password: "viewer-pass",
	})
	if err != nil {
		t.Fatalf("create status viewer: %v", err)
	}
	admin, err := repository.FindByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find bootstrap admin: %v", err)
	}
	if err := users.SetStatus(ctx, admin.ID, viewer.ID, false); err != nil {
		t.Fatalf("disable status viewer: %v", err)
	}

	ascending, err := users.List(ctx, dto.PlatformUserListQuery{
		Page: 1, PageSize: 20, SortBy: "status", SortOrder: "asc",
	})
	if err != nil || len(ascending.Items) != 2 || ascending.Items[0].Status != constant.UserStatusEnabled || ascending.Items[1].Status != constant.UserStatusDisabled {
		t.Fatalf("ascending status page = %#v, %v", ascending, err)
	}
	descending, err := users.List(ctx, dto.PlatformUserListQuery{
		Page: 1, PageSize: 20, SortBy: "status", SortOrder: "desc",
	})
	if err != nil || len(descending.Items) != 2 || descending.Items[0].Status != constant.UserStatusDisabled || descending.Items[1].Status != constant.UserStatusEnabled {
		t.Fatalf("descending status page = %#v, %v", descending, err)
	}
}

func newPlatformUserIntegrationHarness(t *testing.T) (*model.Database, *model.PlatformUserRepository, *service.PlatformUserService) {
	t.Helper()
	database := openLockedIntegrationDatabase(t)
	repository := model.NewPlatformUserRepository(database.GORM)
	clock := testsupport.NewFakeClock(time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC))
	return database, repository, service.NewPlatformUserService(repository, clock)
}

func openLockedIntegrationDatabase(t *testing.T) *model.Database {
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
	if err := lockConnection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", platformUserIntegrationLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = lockConnection.Close()
		_ = database.Close()
		t.Fatalf("acquire platform user test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", platformUserIntegrationLock)
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
		_, _ = lockConnection.ExecContext(cleanupContext, "SELECT RELEASE_LOCK(?)", platformUserIntegrationLock)
		_ = lockConnection.Close()
		_ = database.Close()
	})
	return database
}

func mustFindUser(t *testing.T, repository *model.PlatformUserRepository, id int64) model.PlatformUser {
	t.Helper()
	user, err := repository.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("find user %d: %v", id, err)
	}
	return user
}
