package worker

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestPollErrorClassification(t *testing.T) {
	for _, code := range []uint16{1205, 1213} {
		if fatalPollError("test", fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: code})) {
			t.Fatalf("transient error %d stopped polling", code)
		}
	}
	for _, err := range []error{errors.New("database unavailable"), &mysql.MySQLError{Number: 1146}} {
		if !fatalPollError("test", err) {
			t.Fatal("permanent failure was swallowed")
		}
	}
}

func TestExecutorPollingRecoversAfterTransactionConflict(t *testing.T) {
	for _, code := range []uint16{1205, 1213} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			database := openWorkerTestDatabase(t)
			clock := testsupport.NewFakeClock(time.Unix(1752400800, 0))
			executor, err := NewExecutor(ExecutorOptions{
				Repository: model.NewCollectionTaskRepository(database.GORM),
				Settings:   model.NewCollectorSettingRepository(database.GORM), Clock: clock,
			})
			if err != nil {
				t.Fatal(err)
			}
			var attempts atomic.Int32
			observed := make(chan int32, 16)
			const callback = "test:poll_conflict"
			if err := database.GORM.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				n := attempts.Add(1)
				if n == 1 {
					tx.AddError(&mysql.MySQLError{Number: code})
				}
				select {
				case observed <- n:
				default:
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer database.GORM.Callback().Query().Remove(callback)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- executor.Run(ctx, ctx) }()
			deadline := time.After(5 * time.Second)
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for attempts.Load() < 2 {
				select {
				case err := <-done:
					t.Fatalf("polling stopped before recovery: %v", err)
				case <-deadline:
					t.Fatal("polling did not recover")
				case <-observed:
				case <-tick.C:
					clock.Advance(time.Second)
				}
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("executor did not stop")
			}
		})
	}
}
