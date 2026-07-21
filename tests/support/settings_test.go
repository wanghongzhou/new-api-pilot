package testsupport

import "testing"

func TestPlatformSettingSeedsMatchRequiredBaseline(t *testing.T) {
	if len(platformSettingSeeds) != 26 {
		t.Fatalf("platform setting seed count = %d, want 26", len(platformSettingSeeds))
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
}
