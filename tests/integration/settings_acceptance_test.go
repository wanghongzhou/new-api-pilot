package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const a69Now = int64(1_768_665_599)

func TestA69SettingsWithoutH15GateAndWithNinetyDayLogRetention(t *testing.T) {
	requireA69Database(t)
	ctx := context.Background()
	tx := openCoreAcceptanceTransaction(t)
	clock := testsupport.NewFakeClock(time.Unix(a69Now, 0))
	if err := testsupport.ResetPlatformSettings(ctx, tx, a69Now); err != nil {
		t.Fatalf("reset A69 settings: %v", err)
	}
	settings := newA69SettingService(t, tx, clock)

	groups, err := settings.Update(ctx, dto.SettingPatchRequest{Items: []dto.SettingPatchItem{
		{Key: "collector.usage_delay_minutes", Value: json.RawMessage(`20`)},
		{Key: "collector.usage_concurrency", Value: json.RawMessage(`1`)},
		{Key: "collector.manual_backfill_max_days", Value: json.RawMessage(`400`)},
	}})
	if err != nil {
		t.Fatalf("A69 update settings without H+15 gate: %v", err)
	}
	encoded, err := json.Marshal(groups)
	if err != nil {
		t.Fatalf("A69 marshal settings: %v", err)
	}
	if strings.Contains(string(encoded), "h15_slo") {
		t.Fatalf("A69 settings response still exposes H+15 eligibility: %s", encoded)
	}
	for key, want := range map[string]any{
		"collector.usage_delay_minutes":      int64(20),
		"collector.usage_concurrency":        int64(1),
		"collector.manual_backfill_max_days": int64(400),
		"logs.retention_days":                int64(90),
		"rate.fallback_quota_per_unit":       "500000",
		"rate.fallback_usd_exchange_rate":    "6.8",
	} {
		item := findA69SettingItem(t, groups, key)
		if item.Value != want {
			t.Fatalf("A69 setting %s = %#v, want %#v", key, item.Value, want)
		}
	}
}

func findA69SettingItem(t *testing.T, groups []dto.SettingGroup, key string) dto.SettingItem {
	t.Helper()
	for _, group := range groups {
		for _, item := range group.Items {
			if item.Key == key {
				return item
			}
		}
	}
	t.Fatalf("A69 setting %s is absent", key)
	return dto.SettingItem{}
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
) *service.SettingService {
	t.Helper()
	cipher, err := common.NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("create A69 cipher: %v", err)
	}
	settings, err := service.NewSettingService(service.SettingServiceOptions{
		Repository: model.NewSettingRepository(database), Cipher: cipher, Clock: clock,
		PublicOrigin: "https://pilot.a69.example",
	})
	if err != nil {
		t.Fatalf("create A69 setting service: %v", err)
	}
	return settings
}
