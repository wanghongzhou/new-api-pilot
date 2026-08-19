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
	finalizeErr    error
	repairResult   model.ResourceMaintenanceBatchResult
	repairCalls    []scheduledMaintenanceCall
	finalizeCalls  int
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
	return repository.repairResult, nil
}

func (repository *scheduledMaintenanceRepository) FinalizeResourceDaily(context.Context, int, int64, int64, int, int64) (model.ResourceMaintenanceBatchResult, error) {
	repository.finalizeCalls++
	return repository.finalizeResult, repository.finalizeErr
}

func TestRunScheduledMaintenanceKeepsIncompleteDailyInputsRetryable(t *testing.T) {
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	repository := &scheduledMaintenanceRepository{
		repairResult: model.ResourceMaintenanceBatchResult{Complete: true},
		finalizeErr:  model.ErrResourceDailyInputsIncomplete,
	}
	maintenance, err := NewDataMaintenanceService(repository, testsupport.NewFakeClock(time.Date(2026, 8, 19, 9, 40, 0, 0, beijing)))
	if err != nil {
		t.Fatalf("create maintenance service: %v", err)
	}
	if _, err := maintenance.RunScheduledMaintenance(context.Background()); err != nil {
		t.Fatalf("incomplete daily inputs must remain retryable: %v", err)
	}
	if repository.finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", repository.finalizeCalls)
	}
}

func TestRunScheduledMaintenanceRepairsClosedResourceHours(t *testing.T) {
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	tests := []struct {
		name           string
		now            time.Time
		repairComplete bool
		wantStart      time.Time
		wantEnd        time.Time
		wantDateKey    int
	}{
		{
			name: "daytime repairs previous day before finalization",
			now:  time.Date(2026, 8, 18, 18, 37, 0, 0, beijing), repairComplete: true,
			wantStart: time.Date(2026, 8, 17, 0, 0, 0, 0, beijing),
			wantEnd:   time.Date(2026, 8, 18, 18, 0, 0, 0, beijing), wantDateKey: 20260818,
		},
		{
			name:      "early morning includes previous day final hour",
			now:       time.Date(2026, 8, 18, 1, 30, 0, 0, beijing),
			wantStart: time.Date(2026, 8, 17, 23, 0, 0, 0, beijing),
			wantEnd:   time.Date(2026, 8, 18, 1, 0, 0, 0, beijing), wantDateKey: 20260818,
		},
		{
			name:      "incomplete repair remains normal startup progress",
			now:       time.Date(2026, 8, 18, 18, 37, 0, 0, beijing),
			wantStart: time.Date(2026, 8, 17, 0, 0, 0, 0, beijing),
			wantEnd:   time.Date(2026, 8, 18, 18, 0, 0, 0, beijing), wantDateKey: 20260818,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &scheduledMaintenanceRepository{
				repairResult:   model.ResourceMaintenanceBatchResult{Complete: test.repairComplete},
				finalizeResult: model.ResourceMaintenanceBatchResult{Complete: true},
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
			wantFinalizeCalls := 0
			if test.now.Hour() >= 3 && test.repairComplete {
				wantFinalizeCalls = 1
			}
			if repository.finalizeCalls != wantFinalizeCalls {
				t.Fatalf("finalize calls = %d, want %d", repository.finalizeCalls, wantFinalizeCalls)
			}
		})
	}
}
