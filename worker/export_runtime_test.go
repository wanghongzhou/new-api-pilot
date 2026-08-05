package worker

import (
	"testing"
	"time"

	"gorm.io/gorm"

	testsupport "new-api-pilot/tests/support"
)

func TestNewExportRuntimeDefaultsToFiveSecondPollInterval(t *testing.T) {
	runtime, err := NewExportRuntime(ExportRuntimeOptions{
		Database:  &gorm.DB{},
		Clock:     testsupport.NewFakeClock(time.Unix(1_752_400_800, 0)),
		ExportDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewExportRuntime: %v", err)
	}
	if runtime.pollInterval != 5*time.Second {
		t.Fatalf("poll interval = %s, want 5s", runtime.pollInterval)
	}
}
