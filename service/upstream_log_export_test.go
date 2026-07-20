package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

func TestGenerateUpstreamLogExportCSVAndXLSX(t *testing.T) {
	database := openUpstreamLogExportDatabase(t)
	now := int64(2_100_100_000)
	site := model.Site{Name: "Log Export", BaseURL: "https://log-export-" + time.Now().Format("150405.000000000") + ".example",
		ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
		AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK,
		DataExportEnabled: true, CreatedAt: now, UpdatedAt: now}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create log export site: %v", err)
	}
	fact := model.UpstreamLogFact{SiteID: site.ID, ConfigVersion: 1, UpstreamLogKey: strings.Repeat("b", 64), CreatedAt: now - 10,
		Type: 2, RemoteUserID: 3, Username: "alice", ModelName: "gpt", TokenID: 4, TokenName: "key", ChannelID: 5,
		UseGroup: "vip", RequestID: "req", UpstreamRequestID: "up", Quota: 6, PromptTokens: 7, CompletionTokens: 8,
		UseTimeSeconds: 9, ContentRedacted: "[redacted]", CollectedAt: now}
	if err := database.GORM.Create(&fact).Error; err != nil {
		t.Fatalf("create log export fact: %v", err)
	}
	query := dto.LogQuery{Page: 1, PageSize: 100, SiteIDs: []int64{site.ID}, StartTimestamp: now - 100, EndTimestamp: now}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		path := filepath.Join(t.TempDir(), "logs."+format)
		result, err := GenerateUpstreamLogExport(context.Background(), UpstreamLogExportOptions{Database: database.GORM,
			Query: query, Format: format, TemporaryPath: path, DataSnapshotAt: now, ExportedAt: now,
			MaxFileBytes: 1 << 20, MinFreeBytes: 1, DiskFree: func(string) (uint64, error) { return 1 << 30, nil }})
		if err != nil || result.RowCount != 1 || result.FileSize <= 0 {
			t.Fatalf("%s log export = %+v, %v", format, result, err)
		}
		if format == dto.ExportFormatCSV {
			contents, readErr := os.ReadFile(path)
			if readErr != nil || !strings.Contains(string(contents), "[redacted]") || strings.Contains(string(contents), "203.0.113") {
				t.Fatalf("csv log export = %s, %v", contents, readErr)
			}
		} else {
			book, openErr := excelize.OpenFile(path)
			if openErr != nil {
				t.Fatalf("open xlsx log export: %v", openErr)
			}
			rows, rowsErr := book.GetRows(book.GetSheetName(0))
			_ = book.Close()
			if rowsErr != nil || len(rows) != 2 || !strings.Contains(strings.Join(rows[1], "|"), "[redacted]") {
				t.Fatalf("xlsx log export rows = %#v, %v", rows, rowsErr)
			}
		}
	}
}

func openUpstreamLogExportDatabase(t *testing.T) *model.Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 5, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open log export database: %v", err)
	}
	connection, err := database.SQL.Conn(ctx)
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", "new_api_pilot_upstream_log_export_test").Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		_ = database.Close()
		t.Fatalf("acquire log export lock: %v %#v", err, acquired)
	}
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("run log export migrations: %v", err)
	}
	tx := database.GORM.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		t.Fatalf("begin log export test transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
		_, _ = connection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", "new_api_pilot_upstream_log_export_test")
		_ = connection.Close()
		_ = database.Close()
	})
	return &model.Database{GORM: tx, SQL: database.SQL}
}
