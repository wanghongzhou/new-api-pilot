package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSettingServiceEncryptsSecretsAndAppliesAtomicPatchContracts(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	settings, cipher := newIntegrationSettingService(t, tx, clock, config.EnvironmentTest)
	before := readSettingRows(t, tx)
	webhookPlaintext := "https://oapi.dingtalk.com/robot/send?access_token=private-token"
	secretPlaintext := "private-signing-secret"
	groups, err := settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`4`)},
		{Key: settingDingTalkWebhook, Value: settingJSONRawString(webhookPlaintext)},
		{Key: settingDingTalkSecret, Value: settingJSONRawString(secretPlaintext)},
		{Key: settingDingTalkEnabled, Value: json.RawMessage(`true`)},
	}})
	if err != nil {
		t.Fatalf("valid setting update: %v", err)
	}
	after := readSettingRows(t, tx)
	for _, key := range []string{settingDingTalkWebhook, settingDingTalkSecret} {
		if after[key].Value == "" || after[key].Value == before[key].Value ||
			after[key].Value == webhookPlaintext || after[key].Value == secretPlaintext ||
			!strings.HasPrefix(after[key].Value, "v1:") {
			t.Fatalf("sensitive setting %s was not encrypted", key)
		}
		plaintext, decryptErr := cipher.Decrypt(after[key].Value, "setting:"+key)
		if decryptErr != nil || string(plaintext) == "" {
			t.Fatalf("decrypt %s: %q, %v", key, plaintext, decryptErr)
		}
		if after[key].UpdatedAt <= before[key].UpdatedAt {
			t.Fatalf("updated_at for %s = %d, before %d", key, after[key].UpdatedAt, before[key].UpdatedAt)
		}
	}
	encoded, err := common.Marshal(groups)
	if err != nil {
		t.Fatalf("marshal setting response: %v", err)
	}
	for _, forbidden := range []string{webhookPlaintext, secretPlaintext, after[settingDingTalkWebhook].Value, after[settingDingTalkSecret].Value} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("setting response leaked sensitive value %q", forbidden)
		}
	}

	atomicBefore := readSettingRows(t, tx)
	_, err = settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`7`)},
		{Key: settingDingTalkSecret, Clear: true},
	}})
	var validation *SettingValidationError
	if !errors.As(err, &validation) || validation.Fields[settingDingTalkSecret] == "" {
		t.Fatalf("incomplete DingTalk atomic error = %v, %#v", err, validation)
	}
	atomicAfter := readSettingRows(t, tx)
	assertSettingRowsEqual(t, atomicBefore, atomicAfter)

	secretBeforeKeep := atomicAfter[settingDingTalkSecret]
	_, err = settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: settingDingTalkSecret, Value: json.RawMessage(`""`)},
		{Key: "collector.usage_concurrency", Value: json.RawMessage(`6`)},
	}})
	if err != nil {
		t.Fatalf("empty sensitive keep update: %v", err)
	}
	kept := readSettingRows(t, tx)[settingDingTalkSecret]
	if kept.Value != secretBeforeKeep.Value || kept.UpdatedAt != secretBeforeKeep.UpdatedAt {
		t.Fatalf("empty sensitive input changed stored secret: before=%#v after=%#v", secretBeforeKeep, kept)
	}

	_, err = settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: settingDingTalkEnabled, Value: json.RawMessage(`false`)},
		{Key: settingDingTalkWebhook, Clear: true},
		{Key: settingDingTalkSecret, Clear: true},
	}})
	if err != nil {
		t.Fatalf("disable and clear DingTalk: %v", err)
	}
	cleared := readSettingRows(t, tx)
	if cleared[settingDingTalkWebhook].Value != "" || cleared[settingDingTalkSecret].Value != "" {
		t.Fatalf("explicit clear did not clear ciphertexts")
	}
}

func TestSettingServiceProductionSLOAndCapacityFailuresRollbackEveryItem(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	settings, _ := newIntegrationSettingService(t, tx, clock, config.EnvironmentProduction)
	before := readSettingRows(t, tx)
	_, err := settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`6`)},
		{Key: "export.file_ttl_hours", Value: json.RawMessage(`48`)},
	}})
	if !errors.Is(err, ErrSettingSLOForbidden) {
		t.Fatalf("production SLO error = %v", err)
	}
	assertSettingRowsEqual(t, before, readSettingRows(t, tx))

	_, err = settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "export.max_active_per_user", Value: json.RawMessage(`20`)},
		{Key: "export.max_active_global", Value: json.RawMessage(`10`)},
		{Key: "export.file_ttl_hours", Value: json.RawMessage(`72`)},
	}})
	var validation *SettingValidationError
	if !errors.As(err, &validation) || validation.Fields["export.max_active_per_user"] == "" {
		t.Fatalf("capacity ordering error = %v, %#v", err, validation)
	}
	assertSettingRowsEqual(t, before, readSettingRows(t, tx))
}

func TestSettingServiceInvalidScalarMixedBatchChangesNoValueOrTimestamp(t *testing.T) {
	tx := openSiteTestTransaction(t)
	settings, _ := newIntegrationSettingService(
		t,
		tx,
		testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
		config.EnvironmentTest,
	)
	tests := []struct {
		name string
		item dto.SettingPatchItem
	}{
		{name: "ordinary integer string", item: dto.SettingPatchItem{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`"4"`)}},
		{name: "ordinary integer exponent", item: dto.SettingPatchItem{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`4e0`)}},
		{name: "byte integer number", item: dto.SettingPatchItem{Key: "export.max_file_bytes", Value: json.RawMessage(`8589934592`)}},
		{name: "decimal number", item: dto.SettingPatchItem{Key: "rate.fallback_usd_exchange_rate", Value: json.RawMessage(`7.3`)}},
		{name: "decimal exponent string", item: dto.SettingPatchItem{Key: "rate.fallback_usd_exchange_rate", Value: json.RawMessage(`"7.3e0"`)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := readSettingRows(t, tx)
			_, err := settings.Update(context.Background(), dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
				{Key: "export.file_ttl_hours", Value: json.RawMessage(`48`)},
				test.item,
			}})
			var validation *SettingValidationError
			if !errors.As(err, &validation) || validation.Fields["items[1].value"] == "" {
				t.Fatalf("invalid scalar error = %v, %#v", err, validation)
			}
			assertSettingRowsEqual(t, before, readSettingRows(t, tx))
		})
	}
}

func TestSettingServiceGETSurvivesSecretDecryptFailureWithoutLeakingStorage(t *testing.T) {
	tx := openSiteTestTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(1_752_400_800, 0))
	settings, _ := newIntegrationSettingService(t, tx, clock, config.EnvironmentTest)
	const broken = "v1:broken:private-ciphertext"
	if err := tx.Model(&model.PlatformSetting{}).Where("setting_key = ?", settingDingTalkWebhook).
		Update("setting_value", broken).Error; err != nil {
		t.Fatalf("store broken setting ciphertext: %v", err)
	}
	groups, err := settings.Get(context.Background())
	if err != nil {
		t.Fatalf("GET settings with broken ciphertext: %v", err)
	}
	item := findSettingItem(t, groups, settingDingTalkWebhook)
	if !item.Configured || !item.DecryptError || item.Value != nil || item.MaskedValue != settingSecretMask {
		t.Fatalf("broken secret item = %#v", item)
	}
	encoded, err := common.Marshal(groups)
	if err != nil {
		t.Fatalf("marshal broken setting response: %v", err)
	}
	if strings.Contains(string(encoded), broken) || findSettingItem(t, groups, "collector.usage_delay_minutes").Value == nil {
		t.Fatalf("GET response leaked ciphertext or dropped non-secret settings: %s", encoded)
	}
}

func newIntegrationSettingService(
	t *testing.T,
	tx *gorm.DB,
	clock common.Clock,
	appEnv string,
) (*SettingService, *common.Cipher) {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create integration setting cipher: %v", err)
	}
	settings, err := NewSettingService(SettingServiceOptions{
		Repository: model.NewSettingRepository(tx), Cipher: cipher, Clock: clock,
		AppEnv: appEnv, PublicOrigin: "https://pilot.example",
	})
	if err != nil {
		t.Fatalf("create integration setting service: %v", err)
	}
	return settings, cipher
}

func readSettingRows(t *testing.T, tx *gorm.DB) map[string]model.PlatformSetting {
	t.Helper()
	var rows []model.PlatformSetting
	if err := tx.Order("setting_key ASC").Find(&rows).Error; err != nil {
		t.Fatalf("read platform settings: %v", err)
	}
	result := make(map[string]model.PlatformSetting, len(rows))
	for _, row := range rows {
		result[row.Key] = row
	}
	return result
}

func assertSettingRowsEqual(
	t *testing.T,
	want, got map[string]model.PlatformSetting,
) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("setting row counts = %d, want %d", len(got), len(want))
	}
	for key, expected := range want {
		actual := got[key]
		if actual != expected {
			t.Fatalf("setting %s changed atomically: before=%#v after=%#v", key, expected, actual)
		}
	}
}

func settingJSONRawString(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
