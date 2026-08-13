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

func TestExportProgressUsesProcessedRowsAndCapsRunningWork(t *testing.T) {
	tests := []struct {
		name             string
		processed, total int64
		want             int64
	}{
		{name: "unknown total", processed: 10, total: 0, want: 0},
		{name: "nothing processed", processed: 0, total: 100, want: 0},
		{name: "small first page", processed: 1, total: 2, want: 47},
		{name: "large early page", processed: 95, total: 1000, want: 9},
		{name: "minimum visible progress", processed: 1, total: 1000, want: 1},
		{name: "complete generation waits for commit", processed: 2, total: 2, want: 95},
		{name: "processed exceeds total", processed: 3, total: 2, want: 95},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exportProgress(test.processed, test.total); got != test.want {
				t.Fatalf("exportProgress(%d, %d) = %d, want %d", test.processed, test.total, got, test.want)
			}
		})
	}
}
