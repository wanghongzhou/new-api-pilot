package service

import (
	"context"
	"testing"
	"time"

	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

type scheduledMaintenanceCall struct {
	dateKey int
	start   int64
	end     int64
}

type scheduledMaintenanceRepository struct {
	finalizeResult model.ResourceMaintenanceBatchResult
	repairCalls    []scheduledMaintenanceCall
}

func (repository *scheduledMaintenanceRepository) ProcessAuthorizationPricingIntent(context.Context, int64) (model.AuthorizationPricingProcessResult, error) {
	return model.AuthorizationPricingProcessResult{}, nil
}

func (repository *scheduledMaintenanceRepository) RedactCollectionRunErrors(context.Context, int, int64, int, int64) (model.DataMaintenanceBatchResult, error) {
	return model.DataMaintenanceBatchResult{}, nil
}

func (repository *scheduledMaintenanceRepository) CleanupMetadataDiagnosticRuns(context.Context, int, int64, int, int64) (model.DataMaintenanceBatchResult, error) {
	return model.DataMaintenanceBatchResult{}, nil
}

func (repository *scheduledMaintenanceRepository) RepairResourceRollupGaps(_ context.Context, dateKey int, start, end int64, _ int, _ int64) (model.ResourceMaintenanceBatchResult, error) {
	repository.repairCalls = append(repository.repairCalls, scheduledMaintenanceCall{dateKey: dateKey, start: start, end: end})
	return model.ResourceMaintenanceBatchResult{}, nil
}

func (repository *scheduledMaintenanceRepository) FinalizeResourceDaily(context.Context, int, int64, int64, int, int64) (model.ResourceMaintenanceBatchResult, error) {
	return repository.finalizeResult, nil
}

func TestRunScheduledMaintenanceRepairsClosedResourceHours(t *testing.T) {
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	tests := []struct {
		name        string
		now         time.Time
		finalized   bool
		wantStart   time.Time
		wantEnd     time.Time
		wantDateKey int
	}{
		{
			name: "daytime uses current day after previous day is finalized",
			now:  time.Date(2026, 8, 18, 18, 37, 0, 0, beijing), finalized: true,
			wantStart: time.Date(2026, 8, 18, 0, 0, 0, 0, beijing),
			wantEnd:   time.Date(2026, 8, 18, 18, 0, 0, 0, beijing), wantDateKey: 20260818,
		},
		{
			name:      "early morning includes previous day final hour",
			now:       time.Date(2026, 8, 18, 1, 30, 0, 0, beijing),
			wantStart: time.Date(2026, 8, 17, 23, 0, 0, 0, beijing),
			wantEnd:   time.Date(2026, 8, 18, 1, 0, 0, 0, beijing), wantDateKey: 20260818,
		},
		{
			name:      "unfinished previous day expands recovery range",
			now:       time.Date(2026, 8, 18, 18, 37, 0, 0, beijing),
			wantStart: time.Date(2026, 8, 17, 0, 0, 0, 0, beijing),
			wantEnd:   time.Date(2026, 8, 18, 18, 0, 0, 0, beijing), wantDateKey: 20260818,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &scheduledMaintenanceRepository{
				finalizeResult: model.ResourceMaintenanceBatchResult{Complete: test.finalized},
			}
			maintenance, err := NewDataMaintenanceService(repository, testsupport.NewFakeClock(test.now))
			if err != nil {
				t.Fatalf("create maintenance service: %v", err)
			}
			if _, err := maintenance.RunScheduledMaintenance(context.Background()); err != nil {
				t.Fatalf("run scheduled maintenance: %v", err)
			}
			if len(repository.repairCalls) != 1 {
				t.Fatalf("repair calls = %d, want 1", len(repository.repairCalls))
			}
			call := repository.repairCalls[0]
			if call.dateKey != test.wantDateKey || call.start != test.wantStart.Unix() || call.end != test.wantEnd.Unix() {
				t.Fatalf("repair call = %+v, want date_key=%d start=%d end=%d", call, test.wantDateKey, test.wantStart.Unix(), test.wantEnd.Unix())
			}
			if call.end > test.now.Unix()-test.now.Unix()%3600 {
				t.Fatalf("repair included the open current hour: end=%d now=%d", call.end, test.now.Unix())
			}
		})
	}
}
