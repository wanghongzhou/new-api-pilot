package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSettingServiceNormalizesTypedPatchesAndSecretActions(t *testing.T) {
	settings := newPureSettingService(t)
	request := dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "collector.probe_interval_seconds", Value: json.RawMessage(`120`)},
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`7`)},
		{Key: "export.max_file_bytes", Value: json.RawMessage(`"8589934592"`)},
		{Key: "rate.fallback_usd_exchange_rate", Value: json.RawMessage(`"7.3000"`)},
		{Key: settingDingTalkWebhook, Value: json.RawMessage(`"https://oapi.dingtalk.com/robot/send?access_token=private"`)},
		{Key: settingDingTalkSecret, Value: json.RawMessage(`""`)},
	}}
	patches, fields := settings.normalizePatches(request)
	if fields != nil || len(patches) != len(request.Items) {
		t.Fatalf("normalizePatches() = %#v, %#v", patches, fields)
	}
	byKey := make(map[string]normalizedSettingPatch, len(patches))
	for _, patch := range patches {
		byKey[patch.Definition.Key] = patch
	}
	if byKey["collector.probe_interval_seconds"].Value != "120" ||
		byKey["collector.probe_interval_seconds"].Definition.ReadOnly ||
		byKey["collector.usage_delay_minutes"].Value != "7" ||
		byKey["export.max_file_bytes"].Value != "8589934592" ||
		byKey["rate.fallback_usd_exchange_rate"].Value != "7.3" ||
		byKey[settingDingTalkSecret].Action != settingPatchKeep {
		t.Fatalf("normalized patches = %#v", byKey)
	}
}

func TestSettingServiceRejectsInvalidIntervalUnsafeAndWrongJSONRepresentations(t *testing.T) {
	settings := newPureSettingService(t)
	request := dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "collector.probe_interval_seconds", Value: json.RawMessage(`90`)},
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`60`)},
		{Key: "export.max_file_bytes", Value: json.RawMessage(`8589934592`)},
		{Key: settingDingTalkWebhook, Value: json.RawMessage(`"http://oapi.dingtalk.com/robot/send?access_token=private"`)},
		{Key: settingPublicOrigin, Value: json.RawMessage(`"https://other.example"`)},
	}}
	_, fields := settings.normalizePatches(request)
	for _, key := range []string{"items[0].value", "items[1].value", "items[2].value", "items[3].value", "items[4].key"} {
		if fields[key] == "" {
			t.Fatalf("normalizePatches() fields = %#v, missing %s", fields, key)
		}
	}
}

func TestSettingServiceStrictScalarRepresentations(t *testing.T) {
	settings := newPureSettingService(t)
	tests := []struct {
		name string
		key  string
		raw  string
		want string
		ok   bool
	}{
		{name: "ordinary integer", key: "collector.usage_delay_minutes", raw: `5`, want: "5", ok: true},
		{name: "ordinary integer string", key: "collector.usage_delay_minutes", raw: `"5"`},
		{name: "ordinary integer fraction", key: "collector.usage_delay_minutes", raw: `5.0`},
		{name: "ordinary integer exponent", key: "collector.usage_delay_minutes", raw: `5e0`},
		{name: "ordinary integer sign", key: "collector.usage_delay_minutes", raw: `-5`},
		{name: "ordinary integer unsafe", key: "collector.usage_delay_minutes", raw: `9007199254740992`},
		{name: "byte integer string", key: "export.max_file_bytes", raw: `"8589934592"`, want: "8589934592", ok: true},
		{name: "byte integer number", key: "export.max_file_bytes", raw: `8589934592`},
		{name: "byte integer leading zero", key: "export.max_file_bytes", raw: `"08589934592"`},
		{name: "decimal canonicalized", key: "rate.fallback_usd_exchange_rate", raw: `"0007.3000"`, want: "7.3", ok: true},
		{name: "decimal number", key: "rate.fallback_usd_exchange_rate", raw: `7.3`},
		{name: "decimal exponent", key: "rate.fallback_usd_exchange_rate", raw: `"7.3e0"`},
		{name: "decimal sign", key: "rate.fallback_usd_exchange_rate", raw: `"+7.3"`},
		{name: "decimal whitespace", key: "rate.fallback_usd_exchange_rate", raw: `" 7.3"`},
		{name: "decimal non ASCII", key: "rate.fallback_usd_exchange_rate", raw: `"７.３"`},
		{name: "decimal empty fraction", key: "rate.fallback_usd_exchange_rate", raw: `"7."`},
		{name: "decimal zero", key: "rate.fallback_usd_exchange_rate", raw: `"0.000"`},
		{name: "decimal scale", key: "rate.fallback_usd_exchange_rate", raw: `"1.12345678901"`},
		{name: "decimal integer digits", key: "rate.fallback_usd_exchange_rate", raw: `"123456789012345678901"`},
	}
	definitions := settingDefinitionsByKey()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patches, fields := settings.normalizePatches(dto.SettingPatchRequest{Items: []dto.SettingPatchItem{{
				Key: test.key, Value: json.RawMessage(test.raw),
			}}})
			if !test.ok {
				if fields["items[0].value"] == "" || len(patches) != 0 {
					t.Fatalf("normalizePatches(%s) = %#v, %#v", test.raw, patches, fields)
				}
				return
			}
			if fields != nil || len(patches) != 1 || patches[0].Definition != definitions[test.key] || patches[0].Value != test.want {
				t.Fatalf("normalizePatches(%s) = %#v, %#v", test.raw, patches, fields)
			}
		})
	}
}

func TestNormalizeSettingIntegerEnforcesRepresentationAndSafeBoundary(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		requireString bool
		want          string
		ok            bool
	}{
		{name: "safe number maximum", raw: `9007199254740991`, want: "9007199254740991", ok: true},
		{name: "unsafe number", raw: `9007199254740992`},
		{name: "ordinary integer string", raw: `"5"`},
		{name: "int64 string maximum", raw: `"9223372036854775807"`, requireString: true, want: "9223372036854775807", ok: true},
		{name: "string integer number", raw: `5`, requireString: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := normalizeSettingInteger(json.RawMessage(test.raw), test.requireString)
			if got != test.want || ok != test.ok {
				t.Fatalf("normalizeSettingInteger(%s, %t) = %q, %t; want %q, %t", test.raw, test.requireString, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestSettingGroupsNeverExposeSensitiveValuesAndReportDecryptFailure(t *testing.T) {
	settings := newPureSettingService(t)
	rows := settingTestRows(t)
	webhook := rows[settingDingTalkWebhook]
	webhook.Value = "broken-ciphertext"
	rows[settingDingTalkWebhook] = webhook
	secretPlaintext := "private-signing-secret"
	secretCiphertext, err := settings.cipher.Encrypt([]byte(secretPlaintext), "setting:"+settingDingTalkSecret)
	if err != nil {
		t.Fatalf("encrypt test secret: %v", err)
	}
	secret := rows[settingDingTalkSecret]
	secret.Value = secretCiphertext
	rows[settingDingTalkSecret] = secret
	groups := settings.settingGroups(rows)
	encoded, err := common.Marshal(groups)
	if err != nil {
		t.Fatalf("marshal setting groups: %v", err)
	}
	for _, forbidden := range []string{"broken-ciphertext", secretCiphertext, secretPlaintext, "access_token=private"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("setting response leaked %q: %s", forbidden, encoded)
		}
	}
	webhookItem := findSettingItem(t, groups, settingDingTalkWebhook)
	secretItem := findSettingItem(t, groups, settingDingTalkSecret)
	if !webhookItem.Configured || !webhookItem.DecryptError || webhookItem.MaskedValue != settingSecretMask || webhookItem.Value != nil {
		t.Fatalf("webhook item = %#v", webhookItem)
	}
	if !secretItem.Configured || secretItem.DecryptError || secretItem.MaskedValue != settingSecretMask || secretItem.Value != nil {
		t.Fatalf("secret item = %#v", secretItem)
	}
	maxBytes := findSettingItem(t, groups, "export.max_file_bytes")
	if _, ok := maxBytes.Value.(string); !ok {
		t.Fatalf("max_file_bytes value = %#v, want decimal JSON string", maxBytes.Value)
	}
	publicOrigin := findSettingItem(t, groups, settingPublicOrigin)
	if !publicOrigin.ReadOnly || publicOrigin.Value != "https://pilot.example" || publicOrigin.UpdatedAt != nil {
		t.Fatalf("public origin item = %#v", publicOrigin)
	}
	probeInterval := findSettingItem(t, groups, "collector.probe_interval_seconds")
	if probeInterval.ReadOnly || probeInterval.Constraints["minimum"] != int64(60) || probeInterval.Constraints["maximum"] != int64(3600) {
		t.Fatalf("probe interval item = %#v", probeInterval)
	}
}

func TestSettingFinalValidationRequiresCompleteDingTalkAndCapacityOrdering(t *testing.T) {
	settings := newPureSettingService(t)
	rows := settingTestRows(t)
	values := settingValues(rows)
	values["export.max_active_per_user"] = "20"
	values["export.max_active_global"] = "10"
	if fields := settings.validateFinalValues(values); fields["export.max_active_per_user"] == "" {
		t.Fatalf("capacity fields = %#v", fields)
	}
	values["export.max_active_per_user"] = "3"
	values[settingDingTalkEnabled] = "true"
	webhook, err := settings.cipher.Encrypt(
		[]byte("https://oapi.dingtalk.com/robot/send?access_token=private"),
		"setting:"+settingDingTalkWebhook,
	)
	if err != nil {
		t.Fatalf("encrypt webhook: %v", err)
	}
	values[settingDingTalkWebhook] = webhook
	values[settingDingTalkSecret] = ""
	if fields := settings.validateFinalValues(values); fields[settingDingTalkSecret] == "" {
		t.Fatalf("incomplete DingTalk fields = %#v", fields)
	}
	secret, err := settings.cipher.Encrypt([]byte("private-secret"), "setting:"+settingDingTalkSecret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	values[settingDingTalkSecret] = secret
	if fields := settings.validateFinalValues(values); fields != nil {
		t.Fatalf("valid DingTalk fields = %#v", fields)
	}
}

func TestValidateSettingRowsRejectsOutOfRangePersistedValues(t *testing.T) {
	rows := settingTestRows(t)
	row := rows["collector.usage_delay_minutes"]
	row.Value = "60"
	rows[row.Key] = row
	if _, err := validateSettingRows(settingRowsSlice(rows)); !errorsIsSettingContract(err) {
		t.Fatalf("validateSettingRows() error = %v", err)
	}
}

func newPureSettingService(t *testing.T) *SettingService {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create setting cipher: %v", err)
	}
	settings, err := NewSettingService(SettingServiceOptions{
		Repository: model.NewSettingRepository(nil), Cipher: cipher,
		Clock:        testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
		PublicOrigin: "https://pilot.example",
	})
	if err != nil {
		t.Fatalf("create setting service: %v", err)
	}
	return settings
}

func settingTestRows(t *testing.T) map[string]model.PlatformSetting {
	t.Helper()
	defaults := map[string]string{
		"collector.probe_interval_seconds": "60", "collector.realtime_interval_seconds": "60",
		"collector.resource_interval_seconds": "60", "collector.usage_delay_minutes": "5",
		"collector.minute_retention_days": "90", "logs.retention_days": "90",
		"collector.probe_concurrency":         "20",
		"system_task_terminal_retention_days": "90",
		"collector.realtime_concurrency":      "10", "collector.resource_concurrency": "10",
		"collector.metadata_concurrency": "5", "collector.usage_concurrency": "5",
		"collector.backfill_concurrency": "2", "collector.manual_backfill_max_days": "366",
		"export.file_ttl_hours": "24", "export.max_active_per_user": "3",
		"export.max_active_global": "10", "export.max_file_bytes": "2147483648",
		"export.min_free_disk_bytes": "5368709120", "rate.fallback_quota_per_unit": "500000",
		"rate.fallback_usd_exchange_rate": "6.8", settingDingTalkEnabled: "false",
		settingDingTalkWebhook: "", settingDingTalkSecret: "",
	}
	result := make(map[string]model.PlatformSetting, len(settingDefinitions))
	for index, definition := range settingDefinitions {
		result[definition.Key] = model.PlatformSetting{
			ID: int64(index + 1), Key: definition.Key, Value: defaults[definition.Key],
			ValueType: definition.ValueType, Secret: definition.Secret, UpdatedAt: 100,
		}
	}
	return result
}

func settingValues(rows map[string]model.PlatformSetting) map[string]string {
	result := make(map[string]string, len(rows))
	for key, row := range rows {
		result[key] = row.Value
	}
	return result
}

func settingRowsSlice(rows map[string]model.PlatformSetting) []model.PlatformSetting {
	result := make([]model.PlatformSetting, 0, len(settingDefinitions))
	for _, definition := range settingDefinitions {
		result = append(result, rows[definition.Key])
	}
	return result
}

func findSettingItem(t *testing.T, groups []dto.SettingGroup, key string) dto.SettingItem {
	t.Helper()
	for _, group := range groups {
		for _, item := range group.Items {
			if item.Key == key {
				return item
			}
		}
	}
	t.Fatalf("setting item %s not found", key)
	return dto.SettingItem{}
}

func errorsIsSettingContract(err error) bool { return err == ErrSettingContract }
