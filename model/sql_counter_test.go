package model

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"new-api-pilot/constant"
)

type testSQLCounter struct {
	total        atomic.Int64
	exec         atomic.Int64
	query        atomic.Int64
	row          atomic.Int64
	create       atomic.Int64
	update       atomic.Int64
	delete       atomic.Int64
	begin        atomic.Int64
	commit       atomic.Int64
	rollback     atomic.Int64
	statementsMu sync.Mutex
	statements   []string
}

type testSQLCounterSnapshot struct {
	Total    int64
	Exec     int64
	Query    int64
	Row      int64
	Create   int64
	Update   int64
	Delete   int64
	Begin    int64
	Commit   int64
	Rollback int64
}

func (counter *testSQLCounter) record(kind, statement string) {
	counter.total.Add(1)
	switch kind {
	case "exec":
		counter.exec.Add(1)
	case "query":
		counter.query.Add(1)
	case "row":
		counter.row.Add(1)
	case "begin":
		counter.begin.Add(1)
	case "commit":
		counter.commit.Add(1)
	case "rollback":
		counter.rollback.Add(1)
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
	if kind == "exec" {
		switch {
		case strings.HasPrefix(normalized, "insert "):
			counter.create.Add(1)
		case strings.HasPrefix(normalized, "update "):
			counter.update.Add(1)
		case strings.HasPrefix(normalized, "delete "):
			counter.delete.Add(1)
		}
	}
	counter.statementsMu.Lock()
	counter.statements = append(counter.statements, normalized)
	counter.statementsMu.Unlock()
}

func (counter *testSQLCounter) snapshot() testSQLCounterSnapshot {
	return testSQLCounterSnapshot{
		Total: counter.total.Load(), Exec: counter.exec.Load(), Query: counter.query.Load(), Row: counter.row.Load(),
		Create: counter.create.Load(), Update: counter.update.Load(), Delete: counter.delete.Load(),
		Begin: counter.begin.Load(), Commit: counter.commit.Load(), Rollback: counter.rollback.Load(),
	}
}

func (counter *testSQLCounter) countStatementsContaining(fragments ...string) int64 {
	counter.statementsMu.Lock()
	defer counter.statementsMu.Unlock()
	var count int64
	for _, statement := range counter.statements {
		matches := true
		for _, fragment := range fragments {
			if !strings.Contains(statement, strings.ToLower(fragment)) {
				matches = false
				break
			}
		}
		if matches {
			count++
		}
	}
	return count
}

type testCountingConnPool struct {
	gorm.ConnPool
	counter *testSQLCounter
}

func (pool *testCountingConnPool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	pool.counter.record("exec", query)
	return pool.ConnPool.ExecContext(ctx, query, args...)
}

func (pool *testCountingConnPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	pool.counter.record("query", query)
	return pool.ConnPool.QueryContext(ctx, query, args...)
}

func (pool *testCountingConnPool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	pool.counter.record("row", query)
	return pool.ConnPool.QueryRowContext(ctx, query, args...)
}

func (pool *testCountingConnPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	beginner, ok := pool.ConnPool.(gorm.TxBeginner)
	if !ok {
		return nil, fmt.Errorf("counted connection pool does not support transactions")
	}
	transaction, err := beginner.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	pool.counter.record("begin", "BEGIN")
	return &testCountingTx{Tx: transaction, counter: pool.counter}, nil
}

type testCountingTx struct {
	*sql.Tx
	counter *testSQLCounter
}

func (transaction *testCountingTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	transaction.counter.record("exec", query)
	return transaction.Tx.ExecContext(ctx, query, args...)
}

func (transaction *testCountingTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	transaction.counter.record("query", query)
	return transaction.Tx.QueryContext(ctx, query, args...)
}

func (transaction *testCountingTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	transaction.counter.record("row", query)
	return transaction.Tx.QueryRowContext(ctx, query, args...)
}

func (transaction *testCountingTx) Commit() error {
	if err := transaction.Tx.Commit(); err != nil {
		return err
	}
	transaction.counter.record("commit", "COMMIT")
	return nil
}

func (transaction *testCountingTx) Rollback() error {
	if err := transaction.Tx.Rollback(); err != nil {
		return err
	}
	transaction.counter.record("rollback", "ROLLBACK")
	return nil
}

func newTestSQLCountingDB(database *gorm.DB) (*gorm.DB, *testSQLCounter) {
	counted := database.Session(&gorm.Session{NewDB: true, Context: context.Background()})
	counter := &testSQLCounter{}
	pool := &testCountingConnPool{ConnPool: counted.Statement.ConnPool, counter: counter}
	counted.Config.ConnPool = pool
	counted.Statement.ConnPool = pool
	return counted, counter
}

func TestSQLCounterCoversGORMOperationsAndRawScan(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	counted, counter := newTestSQLCountingDB(database.GORM)
	now := int64(1_752_400_800)
	err := counted.Transaction(func(transaction *gorm.DB) error {
		site := Site{
			Name: "Run run-sql-counter", BaseURL: "https://run-sql-counter.example", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, CreatedAt: now, UpdatedAt: now,
		}
		if err := transaction.Create(&site).Error; err != nil {
			return fmt.Errorf("count Create: %w", err)
		}
		var found Site
		if err := transaction.First(&found, site.ID).Error; err != nil {
			return fmt.Errorf("count Query: %w", err)
		}
		beforeRaw := counter.snapshot()
		var rawID int64
		if err := transaction.Raw("SELECT id FROM site WHERE id = ?", site.ID).Scan(&rawID).Error; err != nil || rawID != site.ID {
			return fmt.Errorf("count Raw Scan id=%d: %w", rawID, err)
		}
		afterRaw := counter.snapshot()
		if afterRaw.Query != beforeRaw.Query+1 || afterRaw.Total != beforeRaw.Total+1 {
			return fmt.Errorf("Raw Scan count delta total=%d query=%d, want 1/1",
				afterRaw.Total-beforeRaw.Total, afterRaw.Query-beforeRaw.Query)
		}
		var rowID int64
		if err := transaction.Raw("SELECT id FROM site WHERE id = ?", site.ID).Row().Scan(&rowID); err != nil || rowID != site.ID {
			return fmt.Errorf("count Row id=%d: %w", rowID, err)
		}
		if err := transaction.Model(&site).Update("remark", "counted").Error; err != nil {
			return fmt.Errorf("count Update: %w", err)
		}
		if err := transaction.Delete(&site).Error; err != nil {
			return fmt.Errorf("count Delete: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("exercise SQL counter: %v", err)
	}
	snapshot := counter.snapshot()
	t.Logf("SQL counter operations: total=%d exec=%d query=%d row=%d create=%d update=%d delete=%d begin=%d commit=%d rollback=%d",
		snapshot.Total, snapshot.Exec, snapshot.Query, snapshot.Row, snapshot.Create, snapshot.Update, snapshot.Delete,
		snapshot.Begin, snapshot.Commit, snapshot.Rollback)
	if snapshot.Create != 1 || snapshot.Update != 1 || snapshot.Delete != 1 {
		t.Fatalf("SQL counter mutations = create:%d update:%d delete:%d, want 1 each",
			snapshot.Create, snapshot.Update, snapshot.Delete)
	}
	if snapshot.Query < 2 || snapshot.Row != 1 || snapshot.Exec != 3 {
		t.Fatalf("SQL counter methods = exec:%d query:%d row:%d, want 3/>=2/1", snapshot.Exec, snapshot.Query, snapshot.Row)
	}
	if snapshot.Begin != 1 || snapshot.Commit != 1 || snapshot.Rollback != 0 {
		t.Fatalf("SQL counter transaction = begin:%d commit:%d rollback:%d, want 1/1/0",
			snapshot.Begin, snapshot.Commit, snapshot.Rollback)
	}
}
