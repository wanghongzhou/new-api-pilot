package testsupport

import "testing"

func TestPlatformSettingSeedsMatchRequiredBaseline(t *testing.T) {
	if len(platformSettingSeeds) != 37 {
		t.Fatalf("platform setting seed count = %d, want 37", len(platformSettingSeeds))
	}
	seen := make(map[string]struct{}, len(platformSettingSeeds))
	for _, setting := range platformSettingSeeds {
		if _, exists := seen[setting.key]; exists {
			t.Fatalf("duplicate platform setting seed %q", setting.key)
		}
		seen[setting.key] = struct{}{}
	}
	if _, exists := seen["system_task_terminal_retention_days"]; !exists {
		t.Fatal("platform setting seed system_task_terminal_retention_days is missing")
	}
	for _, key := range []string{"fast_task.history_retention_seconds", "fast_task.history_count", "upstream.allowed_host_suffixes", "upstream.allowed_cidrs", "upstream.max_inflight_per_origin"} {
		if _, exists := seen[key]; !exists {
			t.Fatalf("platform setting seed %s is missing", key)
		}
	}
}
