package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/controller"
	"new-api-pilot/dto"
	"new-api-pilot/middleware"
	"new-api-pilot/model"
	"new-api-pilot/router"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const a46Now = int64(1_768_665_599)

type a46Envelope struct {
	Success     bool              `json:"success"`
	Code        string            `json:"code"`
	Data        json.RawMessage   `json:"data"`
	RequestID   string            `json:"request_id"`
	FieldErrors map[string]string `json:"field_errors"`
}

func TestA46SettingsAPIContract(t *testing.T) {
	ctx := context.Background()
	tx := openA46AcceptanceTransaction(t)
	if err := testsupport.ResetPlatformSettings(ctx, tx, a46Now); err != nil {
		t.Fatalf("reset A46 settings: %v", err)
	}
	admin := newA46Engine(t, tx, constant.RoleAdmin, config.EnvironmentTest)
	viewer := newA46Engine(t, tx, constant.RoleViewer, config.EnvironmentTest)

	read := a46Request(viewer, http.MethodGet, "/api/settings", "")
	readEnvelope := decodeA46Envelope(t, read)
	if read.Code != http.StatusOK || !readEnvelope.Success {
		t.Fatalf("A46 settings GET = %d %#v body=%s", read.Code, readEnvelope, read.Body.String())
	}
	for _, forbidden := range []string{"setting_value", "v1:", "private-a46"} {
		if strings.Contains(read.Body.String(), forbidden) {
			t.Fatalf("A46 GET leaked %q: %s", forbidden, read.Body.String())
		}
	}

	viewerWrite := a46Request(viewer, http.MethodPut, "/api/settings",
		`{"items":[{"key":"collector.usage_delay_minutes","value":4}]}`)
	if envelope := decodeA46Envelope(t, viewerWrite); viewerWrite.Code != http.StatusForbidden ||
		envelope.Code != constant.CodeForbidden {
		t.Fatalf("A46 viewer write = %d %#v", viewerWrite.Code, envelope)
	}

	invalidItems := []string{
		`{"key":"collector.usage_delay_minutes","value":"4"}`,
		`{"key":"collector.usage_delay_minutes","value":4.0}`,
		`{"key":"collector.usage_delay_minutes","value":4e0}`,
		`{"key":"collector.usage_delay_minutes","value":9007199254740992}`,
		`{"key":"export.max_file_bytes","value":2147483648}`,
		`{"key":"rate.fallback_usd_exchange_rate","value":7.3}`,
		`{"key":"rate.fallback_usd_exchange_rate","value":"-1"}`,
		`{"key":"rate.fallback_usd_exchange_rate","value":"7.3e0"}`,
		`{"key":"rate.fallback_usd_exchange_rate","value":"1.12345678901"}`,
	}
	for _, invalid := range invalidItems {
		before := readA46SettingRows(t, tx)
		response := a46Request(admin, http.MethodPut, "/api/settings",
			`{"items":[{"key":"export.file_ttl_hours","value":48},`+invalid+`]}`)
		envelope := decodeA46Envelope(t, response)
		if response.Code != http.StatusBadRequest || envelope.Code != constant.CodeValidationError ||
			envelope.FieldErrors["items[1].value"] == "" {
			t.Fatalf("A46 invalid scalar %s = %d %#v", invalid, response.Code, envelope)
		}
		assertA46SettingRowsEqual(t, before, readA46SettingRows(t, tx))
	}

	valid := a46Request(admin, http.MethodPut, "/api/settings", `{"items":[
  {"key":"collector.usage_delay_minutes","value":4},
  {"key":"notification.dingtalk.webhook","value":"https://robot.a46.example/robot/send?access_token=private-a46-token"},
  {"key":"notification.dingtalk.secret","value":"private-a46-secret"},
  {"key":"notification.dingtalk.enabled","value":true}
]}`)
	validEnvelope := decodeA46Envelope(t, valid)
	if valid.Code != http.StatusOK || !validEnvelope.Success {
		t.Fatalf("A46 valid atomic PUT = %d %#v body=%s", valid.Code, validEnvelope, valid.Body.String())
	}
	rows := readA46SettingRows(t, tx)
	for _, key := range []string{"notification.dingtalk.webhook", "notification.dingtalk.secret"} {
		if !strings.HasPrefix(rows[key].Value, "v1:") || strings.Contains(rows[key].Value, "private-a46") {
			t.Fatalf("A46 sensitive row %s is not encrypted: %#v", key, rows[key])
		}
	}

	read = a46Request(viewer, http.MethodGet, "/api/settings", "")
	readEnvelope = decodeA46Envelope(t, read)
	if read.Code != http.StatusOK || !readEnvelope.Success || strings.Contains(read.Body.String(), "private-a46") ||
		strings.Contains(read.Body.String(), rows["notification.dingtalk.webhook"].Value) {
		t.Fatalf("A46 sensitive GET = %d %#v body=%s", read.Code, readEnvelope, read.Body.String())
	}
	var groups []dto.SettingGroup
	if err := json.Unmarshal(readEnvelope.Data, &groups); err != nil {
		t.Fatalf("decode A46 settings response: %v", err)
	}
	for _, key := range []string{"notification.dingtalk.webhook", "notification.dingtalk.secret"} {
		item := findA46SettingItem(t, groups, key)
		if !item.Secret || !item.Configured || item.DecryptError || item.MaskedValue != "********" || item.Value != nil {
			t.Fatalf("A46 secret response item %s = %#v", key, item)
		}
	}

	production := newA46Engine(t, tx, constant.RoleAdmin, config.EnvironmentProduction)
	beforeProduction := readA46SettingRows(t, tx)
	sloForbidden := a46Request(production, http.MethodPut, "/api/settings",
		`{"items":[{"key":"collector.usage_delay_minutes","value":20}]}`)
	if envelope := decodeA46Envelope(t, sloForbidden); sloForbidden.Code != http.StatusUnprocessableEntity ||
		envelope.Code != constant.CodeSLOConfigForbidden {
		t.Fatalf("A46 production SLO rejection = %d %#v", sloForbidden.Code, envelope)
	}
	assertA46SettingRowsEqual(t, beforeProduction, readA46SettingRows(t, tx))
}

func openA46AcceptanceTransaction(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		if strings.TrimSpace(os.Getenv("ACCEPTANCE_ID")) == "A46" {
			t.Fatal("A46 requires TEST_DATABASE_DSN")
		}
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 10, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open A46 database: %v", err)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("run A46 migrations: %v", err)
	}
	tx := database.GORM.Begin()
	if tx.Error != nil {
		_ = database.Close()
		t.Fatalf("begin A46 transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		_ = database.Close()
	})
	return tx
}

func newA46Engine(t *testing.T, database *gorm.DB, role, environment string) http.Handler {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create A46 cipher: %v", err)
	}
	settings, err := service.NewSettingService(service.SettingServiceOptions{
		Repository: model.NewSettingRepository(database), Cipher: cipher,
		Clock: testsupport.NewFakeClock(time.Unix(a46Now, 0)), AppEnv: environment,
		PublicOrigin: "https://pilot.a46.example", DingTalkHosts: []string{"robot.a46.example"},
	})
	if err != nil {
		t.Fatalf("create A46 settings service: %v", err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Recovery())
	router.RegisterSettingRoutes(engine, controller.NewSettingController(settings), coreContractIdentityResolver{role: role})
	return engine
}

func a46Request(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("New-Api-User", "9007199254740993")
	request.Header.Set(middleware.RequestIDHeader, "a46-settings-request")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeA46Envelope(t *testing.T, response *httptest.ResponseRecorder) a46Envelope {
	t.Helper()
	var envelope a46Envelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode A46 response: %v body=%s", err, response.Body.String())
	}
	if envelope.RequestID != "a46-settings-request" {
		t.Fatalf("A46 request ID = %q body=%s", envelope.RequestID, response.Body.String())
	}
	return envelope
}

func readA46SettingRows(t *testing.T, database *gorm.DB) map[string]model.PlatformSetting {
	t.Helper()
	var rows []model.PlatformSetting
	if err := database.Order("setting_key ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read A46 settings: %v", err)
	}
	result := make(map[string]model.PlatformSetting, len(rows))
	for _, row := range rows {
		result[row.Key] = row
	}
	return result
}

func assertA46SettingRowsEqual(t *testing.T, want, got map[string]model.PlatformSetting) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("A46 setting rows changed after rejected patch:\nwant=%#v\ngot=%#v", want, got)
	}
}

func findA46SettingItem(t *testing.T, groups []dto.SettingGroup, key string) dto.SettingItem {
	t.Helper()
	for _, group := range groups {
		for _, item := range group.Items {
			if item.Key == key {
				return item
			}
		}
	}
	t.Fatalf("A46 setting item %s is absent", key)
	return dto.SettingItem{}
}
