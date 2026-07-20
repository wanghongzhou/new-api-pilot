package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

const applicationIntegrationLock = "new-api-pilot-application-integration"

func TestBootstrapApplicationRegistersSettingsNotificationsAndCompositeRuntime(t *testing.T) {
	database, tx := openApplicationTestTransaction(t)
	cipher, err := common.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create application cipher: %v", err)
	}
	now := time.Unix(1_752_400_800, 0)
	app, _, err := bootstrapApplication(context.Background(), applicationOptions{
		Config: config.Config{
			AppEnv: config.EnvironmentTest, SessionSecret: []byte("0123456789abcdef0123456789abcdef"),
			BootstrapAdminSecret: "S7rong!Pilot#Bootstrap2026", PublicOrigin: "https://pilot.test",
			DingTalkAllowedHosts: []string{"oapi.dingtalk.com"},
			ExportDir:            t.TempDir(),
		},
		Database: &model.Database{GORM: tx, SQL: database.SQL}, Cipher: cipher,
		Clock: testsupport.NewFakeClock(now),
	})
	if err != nil {
		t.Fatalf("bootstrap application: %v", err)
	}
	engine, ok := app.Handler.(*gin.Engine)
	if !ok {
		t.Fatalf("application handler type = %T", app.Handler)
	}
	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		http.MethodGet + " /api/settings",
		http.MethodPut + " /api/settings",
		http.MethodPost + " /api/notifications/dingtalk/test",
		http.MethodPost + " /api/statistics/export",
		http.MethodGet + " /api/statistics/exports",
		http.MethodGet + " /api/statistics/exports/:id",
		http.MethodGet + " /api/statistics/exports/:id/download",
	} {
		if !routes[route] {
			t.Errorf("bootstrap route %q is not registered", route)
		}
	}
	group, ok := app.runtime.(*runtimeGroup)
	if !ok || len(group.components) != 5 {
		t.Fatalf("application runtime = %#v", app.runtime)
	}
	assertApplicationReadiness(t, app.Handler, http.StatusServiceUnavailable)
}

func TestBootstrapApplicationExportLifecycleOwnershipAndDownload(t *testing.T) {
	database, transaction := openApplicationTestTransaction(t)
	if err := transaction.Rollback().Error; err != nil {
		t.Fatalf("release application fixture transaction: %v", err)
	}
	now := time.Unix(1_752_400_800, 0)
	clock := testsupport.NewFakeClock(now)
	exportDir := t.TempDir()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	password := "S7rong!Export#2026"
	passwordHash, err := common.HashPassword(password)
	if err != nil {
		t.Fatalf("hash export owner password: %v", err)
	}
	users := []model.PlatformUser{
		{
			Username: "export-owner-" + suffix, PasswordHash: passwordHash, DisplayName: "Export Owner",
			Role: constant.RoleViewer, Status: constant.UserStatusEnabled, SessionVersion: 1,
			CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
		},
		{
			Username: "export-other-" + suffix, PasswordHash: passwordHash, DisplayName: "Export Other",
			Role: constant.RoleViewer, Status: constant.UserStatusEnabled, SessionVersion: 1,
			CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
		},
	}
	if err := database.GORM.Create(&users).Error; err != nil {
		t.Fatalf("create export route users: %v", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ids := []int64{users[0].ID, users[1].ID}
		_ = database.GORM.WithContext(cleanup).Where("user_id IN ?", ids).Delete(&model.ExportJob{}).Error
		_ = database.GORM.WithContext(cleanup).Where("id IN ?", ids).Delete(&model.PlatformUser{}).Error
	})
	cipher, err := common.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create application cipher: %v", err)
	}
	app, _, err := bootstrapApplication(context.Background(), applicationOptions{
		Config: config.Config{
			AppEnv: config.EnvironmentTest, SessionSecret: []byte("0123456789abcdef0123456789abcdef"),
			PublicOrigin:         "https://pilot.test",
			DingTalkAllowedHosts: []string{"oapi.dingtalk.com"}, ExportDir: exportDir,
			RedisDSN: "redis://redis:6379/15", RedisDB: 15, RedisTimeout: time.Second,
			FastTaskRetention: time.Hour, FastTaskHistoryCount: 10,
		},
		Database: database, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("bootstrap export application: %v", err)
	}
	assertApplicationReadiness(t, app.Handler, http.StatusServiceUnavailable)

	ownerCookie := loginApplicationUser(t, app.Handler, users[0], password)
	otherCookie := loginApplicationUser(t, app.Handler, users[1], password)
	start := now.Unix() - now.Unix()%3600 - 3600
	createBody, err := json.Marshal(dto.ExportCreateRequest{
		Format: dto.ExportFormatCSV, StatisticsType: dto.StatisticsScopeGlobal,
		Filters: dto.ExportFilters{
			StartTimestamp: start, EndTimestamp: start + 3600, Granularity: dto.StatisticsGranularityHour,
			SiteIDs: []string{}, CustomerIDs: []string{}, AccountIDs: []string{}, ModelNames: []string{},
			ChannelKeys: []string{}, SortBy: "bucket_start", SortOrder: "asc",
		},
	})
	if err != nil {
		t.Fatalf("marshal export request: %v", err)
	}
	created := performApplicationAPIRequest(
		app.Handler, http.MethodPost, "/api/statistics/export", createBody, ownerCookie,
		strconv.FormatInt(users[0].ID, 10),
	)
	createdEnvelope := decodeApplicationAPIEnvelope(t, created)
	var item dto.ExportJobItem
	if created.Code != http.StatusOK || !createdEnvelope.Success || json.Unmarshal(createdEnvelope.Data, &item) != nil {
		t.Fatalf("create export = %d %#v body=%s", created.Code, createdEnvelope, created.Body.String())
	}
	jobID, err := strconv.ParseInt(item.ID, 10, 64)
	if err != nil || jobID <= 0 {
		t.Fatalf("created export ID = %q, %v", item.ID, err)
	}

	temporaryName := ".bootstrap-recovery.tmp"
	if err := os.WriteFile(filepath.Join(exportDir, temporaryName), []byte("partial"), 0o600); err != nil {
		t.Fatalf("write abandoned export artifact: %v", err)
	}
	claimToken := strings.Repeat("a", 64)
	if err := database.GORM.Model(&model.ExportJob{}).Where("id = ?", jobID).Updates(map[string]any{
		"status": dto.ExportStatusRunning, "attempt_count": 1, "progress": 10,
		"claim_token": claimToken, "heartbeat_at": now.Unix(), "lease_expires_at": now.Add(5 * time.Minute).Unix(),
		"file_path": temporaryName, "updated_at": now.Unix(),
	}).Error; err != nil {
		t.Fatalf("prepare abandoned export claim: %v", err)
	}

	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	started, stopped := false, false
	t.Cleanup(func() {
		cancelRuntime()
		if started && !stopped {
			stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = app.Stop(stopContext)
		}
	})
	if err := app.Start(runtimeContext); err != nil {
		t.Fatalf("start export application: %v", err)
	}
	started = true
	assertApplicationReadiness(t, app.Handler, http.StatusOK)
	if !app.RuntimeReady() {
		t.Fatal("application runtime was not ready after export recovery")
	}
	var recovered model.ExportJob
	if err := database.GORM.First(&recovered, jobID).Error; err != nil {
		t.Fatalf("read recovered export: %v", err)
	}
	if recovered.Status != dto.ExportStatusPending || recovered.AttemptCount != 1 ||
		recovered.ClaimToken != nil || recovered.FilePath != nil {
		t.Fatalf("startup processed pending export before ready or failed recovery: %#v", recovered)
	}
	if _, err := os.Stat(filepath.Join(exportDir, temporaryName)); !os.IsNotExist(err) {
		t.Fatalf("startup recovery retained abandoned artifact: %v", err)
	}

	clock.Advance(time.Second)
	completed := waitForApplicationExportStatus(t, database.GORM, jobID, dto.ExportStatusSuccess)
	if completed.FilePath == nil || completed.FileName == nil || completed.ClaimToken != nil {
		t.Fatalf("completed export state = %#v", completed)
	}

	otherDetail := performApplicationAPIRequest(
		app.Handler, http.MethodGet, "/api/statistics/exports/"+item.ID, nil, otherCookie,
		strconv.FormatInt(users[1].ID, 10),
	)
	otherEnvelope := decodeApplicationAPIEnvelope(t, otherDetail)
	if otherDetail.Code != http.StatusNotFound || otherEnvelope.Code != constant.CodeNotFound {
		t.Fatalf("cross-owner export detail = %d %#v", otherDetail.Code, otherEnvelope)
	}
	ownerDetail := performApplicationAPIRequest(
		app.Handler, http.MethodGet, "/api/statistics/exports/"+item.ID, nil, ownerCookie,
		strconv.FormatInt(users[0].ID, 10),
	)
	if envelope := decodeApplicationAPIEnvelope(t, ownerDetail); ownerDetail.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("owner export detail = %d %#v", ownerDetail.Code, envelope)
	}
	download := performApplicationAPIRequest(
		app.Handler, http.MethodGet, "/api/statistics/exports/"+item.ID+"/download", nil, ownerCookie,
		strconv.FormatInt(users[0].ID, 10),
	)
	if download.Code != http.StatusOK || download.Body.Len() == 0 ||
		!strings.Contains(download.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("owner export download = %d headers=%v bytes=%d", download.Code, download.Header(), download.Body.Len())
	}

	stopContext, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
	if err := app.Stop(stopContext); err != nil {
		cancelStop()
		t.Fatalf("stop export application: %v", err)
	}
	cancelStop()
	stopped = true
	cancelRuntime()
	assertApplicationReadiness(t, app.Handler, http.StatusServiceUnavailable)
	if app.RuntimeReady() {
		t.Fatal("stopped application still reported runtime ready")
	}
	entries, err := os.ReadDir(exportDir)
	if err != nil {
		t.Fatalf("read export directory after shutdown: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary export artifact remained after shutdown: %s", entry.Name())
		}
	}
}

type applicationAPIEnvelope struct {
	Success bool            `json:"success"`
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
}

func loginApplicationUser(t *testing.T, handler http.Handler, user model.PlatformUser, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user.Username, "password": password})
	response := performApplicationAPIRequest(handler, http.MethodPost, "/api/user/login", body, nil, "")
	envelope := decodeApplicationAPIEnvelope(t, response)
	if response.Code != http.StatusOK || !envelope.Success {
		t.Fatalf("login application user %s = %d %#v body=%s", user.Username, response.Code, envelope, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == common.SessionCookieName {
			return cookie
		}
	}
	t.Fatalf("login application user %s did not set a session cookie", user.Username)
	return nil
}

func performApplicationAPIRequest(
	handler http.Handler,
	method, target string,
	body []byte,
	cookie *http.Cookie,
	userID string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.RemoteAddr = "127.0.0.1:1000"
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
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

func decodeApplicationAPIEnvelope(t *testing.T, response *httptest.ResponseRecorder) applicationAPIEnvelope {
	t.Helper()
	var envelope applicationAPIEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode application API envelope: %v body=%s", err, response.Body.String())
	}
	return envelope
}

func waitForApplicationExportStatus(
	t *testing.T,
	database *gorm.DB,
	jobID int64,
	status string,
) model.ExportJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var job model.ExportJob
		if err := database.First(&job, jobID).Error; err != nil {
			t.Fatalf("read export %d: %v", jobID, err)
		}
		if job.Status == status {
			return job
		}
		if job.Status == dto.ExportStatusFailed {
			t.Fatalf("export %d failed while waiting for %s: %#v", jobID, status, job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("export %d did not reach %s", jobID, status)
	return model.ExportJob{}
}

func openApplicationTestTransaction(t *testing.T) (*model.Database, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open application test database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatalf("reserve application test lock: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", applicationIntegrationLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire application test lock = %v, %v", acquired, err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", applicationIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run application test migrations: %v", err)
	}
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", applicationIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("run application test seeds: %v", err)
	}
	tx := database.GORM.Begin()
	if tx.Error != nil {
		t.Fatalf("begin application test transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = connection.ExecContext(cleanup, "SELECT RELEASE_LOCK(?)", applicationIntegrationLock)
		_ = connection.Close()
		_ = database.Close()
	})
	return database, tx
}
