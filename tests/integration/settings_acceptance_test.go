package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const a69Now = int64(1_768_665_599)

func TestA69SettingsH15Acceptance(t *testing.T) {
	requireA69Database(t)
	ctx := context.Background()
	tx := openCoreAcceptanceTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(a69Now, 0))
	if err := testsupport.ResetPlatformSettings(ctx, tx, a69Now); err != nil {
		t.Fatalf("reset A69 settings: %v", err)
	}
	development := newA69SettingService(t, tx, clock, config.EnvironmentTest)

	groups, err := development.Update(ctx, dto.SettingPatchRequest{Items: []dto.SettingPatchItem{{
		Key: "collector.usage_delay_minutes", Value: json.RawMessage(`20`),
	}}})
	if err != nil {
		t.Fatalf("A69 test delay update: %v", err)
	}
	assertA69SLOStatus(t, groups, false, []dto.SettingSLOReasonCode{dto.SettingSLOReasonUsageDelayTooHigh})

	groups, err = development.Update(ctx, dto.SettingPatchRequest{Items: []dto.SettingPatchItem{{
		Key: "collector.usage_concurrency", Value: json.RawMessage(`1`),
	}}})
	if err != nil {
		t.Fatalf("A69 test concurrency update: %v", err)
	}
	assertA69SLOStatus(t, groups, false, []dto.SettingSLOReasonCode{
		dto.SettingSLOReasonUsageDelayTooHigh, dto.SettingSLOReasonUsageConcurrencyTooLow,
	})

	if err := testsupport.ResetPlatformSettings(ctx, tx, a69Now); err != nil {
		t.Fatalf("reset A69 production settings: %v", err)
	}
	production := newA69SettingService(t, tx, clock, config.EnvironmentProduction)
	for _, patch := range []dto.SettingPatchItem{
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`20`)},
		{Key: "collector.usage_concurrency", Value: json.RawMessage(`1`)},
	} {
		before := readA69SettingRows(t, tx)
		_, updateErr := production.Update(ctx, dto.SettingPatchRequest{Items: []dto.SettingPatchItem{patch}})
		if !errors.Is(updateErr, service.ErrSettingSLOForbidden) {
			t.Fatalf("A69 production patch %#v error = %v", patch, updateErr)
		}
		assertA69SettingRowsEqual(t, before, readA69SettingRows(t, tx))
	}

	groups, err = development.Get(ctx)
	if err != nil {
		t.Fatalf("A69 reset settings GET: %v", err)
	}
	assertA69SLOStatus(t, groups, true, nil)
}

func requireA69Database(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) == "" &&
		strings.TrimSpace(os.Getenv("ACCEPTANCE_ID")) == "A69" {
		t.Fatalf("A69 requires TEST_DATABASE_DSN")
	}
}

func newA69SettingService(
	t *testing.T,
	database *gorm.DB,
	clock common.Clock,
	environment string,
) *service.SettingService {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create A69 cipher: %v", err)
	}
	settings, err := service.NewSettingService(service.SettingServiceOptions{
		Repository: model.NewSettingRepository(database), Cipher: cipher, Clock: clock,
		AppEnv: environment, PublicOrigin: "https://pilot.a69.example",
	})
	if err != nil {
		t.Fatalf("create A69 setting service: %v", err)
	}
	return settings
}

func assertA69SLOStatus(
	t *testing.T,
	groups []dto.SettingGroup,
	wantEligible bool,
	wantReasons []dto.SettingSLOReasonCode,
) {
	t.Helper()
	for _, group := range groups {
		if group.Key != "collector" {
			continue
		}
		if group.H15SLOEligible != wantEligible || !reflect.DeepEqual(group.H15SLOReasonCodes, wantReasons) {
			t.Fatalf("A69 collector SLO status = eligible:%t reasons:%#v, want eligible:%t reasons:%#v",
				group.H15SLOEligible, group.H15SLOReasonCodes, wantEligible, wantReasons)
		}
		return
	}
	t.Fatal("A69 collector settings group is absent")
}

func readA69SettingRows(t *testing.T, database *gorm.DB) map[string]model.PlatformSetting {
	t.Helper()
	var rows []model.PlatformSetting
	if err := database.Order("setting_key ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read A69 settings: %v", err)
	}
	result := make(map[string]model.PlatformSetting, len(rows))
	for _, row := range rows {
		result[row.Key] = row
	}
	return result
}

func assertA69SettingRowsEqual(t *testing.T, want, got map[string]model.PlatformSetting) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("A69 production rejection changed settings:\nwant=%#v\ngot=%#v", want, got)
	}
}
