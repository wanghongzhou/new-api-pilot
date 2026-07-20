package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

const exportServiceIntegrationLock = "new-api-pilot-export-service-integration"

type exportCreateIntegrationResult struct {
	item dto.ExportJobItem
	err  error
}

func TestMySQLExportDownloadIsOwnerScopedAndHandlesExpiredMissingAndUnsafeFiles(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 8, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = database.Close() }()
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	lockConnection := acquireExportServiceIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", exportServiceIntegrationLock)
		_ = lockConnection.Close()
	}()

	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	owner := createExportIntegrationUser(t, tx, "viewer")
	otherAdmin := createExportIntegrationUser(t, tx, "admin")
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	exportDirectory := t.TempDir()
	exports, err := NewExportService(ExportServiceOptions{
		Database: tx, Clock: testsupport.NewFakeClock(now), ExportDir: exportDirectory,
	})
	if err != nil {
		t.Fatalf("NewExportService: %v", err)
	}

	successName := "statistics-global-1-2-1.csv"
	if err := os.WriteFile(filepath.Join(exportDirectory, successName), []byte("owner-data"), 0o600); err != nil {
		t.Fatalf("write success artifact: %v", err)
	}
	success := createExportIntegrationSuccessJob(t, tx, owner.ID, successName, successName, now.Unix()+3600)
	download, err := exports.OpenDownload(ctx, strconv.FormatInt(owner.ID, 10), success.ID)
	if err != nil {
		t.Fatalf("owner download: %v", err)
	}
	data, readErr := io.ReadAll(download.File)
	closeErr := download.File.Close()
	if readErr != nil || closeErr != nil || string(data) != "owner-data" {
		t.Fatalf("download data=%q read=%v close=%v", data, readErr, closeErr)
	}
	if download.Name != successName || download.ContentType != "text/csv; charset=utf-8" || download.Size != int64(len(data)) ||
		!strings.HasPrefix(download.ContentDisposition, "attachment;") || strings.ContainsAny(download.ContentDisposition, "\r\n") {
		t.Fatalf("download metadata = %#v", download)
	}
	if _, err := exports.OpenDownload(ctx, strconv.FormatInt(otherAdmin.ID, 10), success.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("other admin download error = %v", err)
	}
	if _, err := exports.Get(ctx, strconv.FormatInt(otherAdmin.ID, 10), success.ID); !errors.Is(err, ErrExportNotFound) {
		t.Fatalf("other admin detail error = %v", err)
	}
	otherJobs, err := exports.List(ctx, strconv.FormatInt(otherAdmin.ID, 10), dto.ExportListQuery{
		Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc",
	})
	if err != nil || otherJobs.Total != 0 || len(otherJobs.Items) != 0 {
		t.Fatalf("other admin list = %#v, %v", otherJobs, err)
	}

	expiredName := "statistics-global-1-2-2.csv"
	if err := os.WriteFile(filepath.Join(exportDirectory, expiredName), []byte("expired"), 0o600); err != nil {
		t.Fatalf("write expired artifact: %v", err)
	}
	expired := createExportIntegrationSuccessJob(t, tx, owner.ID, expiredName, expiredName, now.Unix())
	if _, err := exports.OpenDownload(ctx, strconv.FormatInt(owner.ID, 10), expired.ID); !errors.Is(err, ErrExportExpired) {
		t.Fatalf("expired download error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(exportDirectory, expiredName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired artifact remains: %v", err)
	}
	var expiredState model.ExportJob
	if err := tx.First(&expiredState, expired.ID).Error; err != nil || expiredState.Status != dto.ExportStatusExpired || expiredState.FilePath != nil {
		t.Fatalf("expired state = %#v, %v", expiredState, err)
	}

	missingName := "statistics-global-1-2-3.csv"
	missing := createExportIntegrationSuccessJob(t, tx, owner.ID, missingName, missingName, now.Unix()+3600)
	if _, err := exports.OpenDownload(ctx, strconv.FormatInt(owner.ID, 10), missing.ID); !errors.Is(err, ErrExportFileMissing) {
		t.Fatalf("missing download error = %v", err)
	}
	assertExportMarkedMissing(t, tx, missing.ID)

	outsidePath := filepath.Join(filepath.Dir(exportDirectory), "outside-export.csv")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	defer func() { _ = os.Remove(outsidePath) }()
	traversal := createExportIntegrationSuccessJob(t, tx, owner.ID, "../outside-export.csv", missingName, now.Unix()+3600)
	if _, err := exports.OpenDownload(ctx, strconv.FormatInt(owner.ID, 10), traversal.ID); !errors.Is(err, ErrExportFileMissing) {
		t.Fatalf("traversal download error = %v", err)
	}
	outsideData, err := os.ReadFile(outsidePath)
	if err != nil || string(outsideData) != "outside" {
		t.Fatalf("outside path changed: %q, %v", outsideData, err)
	}
	assertExportMarkedMissing(t, tx, traversal.ID)

	if runtime.GOOS != "windows" {
		target := filepath.Join(exportDirectory, "target.csv")
		linkName := "statistics-global-1-2-4.csv"
		if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
			t.Fatalf("write symlink target: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(exportDirectory, linkName)); err != nil {
			t.Fatalf("create symlink: %v", err)
		}
		linked := createExportIntegrationSuccessJob(t, tx, owner.ID, linkName, linkName, now.Unix()+3600)
		if _, err := exports.OpenDownload(ctx, strconv.FormatInt(owner.ID, 10), linked.ID); !errors.Is(err, ErrExportFileMissing) {
			t.Fatalf("symlink download error = %v", err)
		}
		targetData, err := os.ReadFile(target)
		if err != nil || string(targetData) != "target" {
			t.Fatalf("symlink target changed: %q, %v", targetData, err)
		}
		assertExportMarkedMissing(t, tx, linked.ID)
	}

	retainedName := "statistics-global-1-2-5.csv"
	if err := os.Mkdir(filepath.Join(exportDirectory, retainedName), 0o700); err != nil {
		t.Fatalf("create undeletable artifact fixture: %v", err)
	}
	retained := createExportIntegrationSuccessJob(t, tx, owner.ID, retainedName, retainedName, now.Unix())
	if _, err := exports.OpenDownload(ctx, strconv.FormatInt(owner.ID, 10), retained.ID); !errors.Is(err, ErrExportExpired) {
		t.Fatalf("retained expired download error = %v", err)
	}
	var retainedState model.ExportJob
	if err := tx.First(&retainedState, retained.ID).Error; err != nil || retainedState.Status != dto.ExportStatusExpired ||
		retainedState.FilePath == nil || *retainedState.FilePath != retainedName {
		t.Fatalf("retained expired state = %#v, %v", retainedState, err)
	}
}

func TestMySQLExportCreateSerializesDeduplicationAndActiveLimits(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 8, MaxOpen: 24, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = database.Close() }()
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	lockConnection := acquireExportServiceIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", exportServiceIntegrationLock)
		_ = lockConnection.Close()
	}()

	settingKeys := []string{"export.max_active_per_user", "export.max_active_global"}
	var originalSettings []model.PlatformSetting
	if err := database.GORM.Where("setting_key IN ?", settingKeys).Find(&originalSettings).Error; err != nil || len(originalSettings) != 2 {
		t.Fatalf("read original export limits = %#v, %v", originalSettings, err)
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for _, setting := range originalSettings {
			_ = database.GORM.WithContext(cleanup).Model(&model.PlatformSetting{}).
				Where("id = ?", setting.ID).Updates(map[string]any{"setting_value": setting.Value, "updated_at": setting.UpdatedAt}).Error
		}
	}()
	if err := database.GORM.Model(&model.PlatformSetting{}).Where("setting_key = ?", "export.max_active_per_user").
		Update("setting_value", "3").Error; err != nil {
		t.Fatalf("set per-user limit: %v", err)
	}
	if err := database.GORM.Model(&model.PlatformSetting{}).Where("setting_key = ?", "export.max_active_global").
		Update("setting_value", "4").Error; err != nil {
		t.Fatalf("set global limit: %v", err)
	}
	if err := database.GORM.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}

	ownerOne := createCommittedExportIntegrationUser(t, database.GORM, "quota-one")
	ownerTwo := createCommittedExportIntegrationUser(t, database.GORM, "quota-two")
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = database.GORM.WithContext(cleanup).Where("user_id IN ?", []int64{ownerOne.ID, ownerTwo.ID}).Delete(&model.ExportJob{}).Error
		_ = database.GORM.WithContext(cleanup).Where("id IN ?", []int64{ownerOne.ID, ownerTwo.ID}).Delete(&model.PlatformUser{}).Error
	}()

	now := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	exports, err := NewExportService(ExportServiceOptions{
		Database: database.GORM, Clock: testsupport.NewFakeClock(now), ExportDir: t.TempDir(),
		DiskFree: func(string) (uint64, error) { return math.MaxUint64, nil },
	})
	if err != nil {
		t.Fatalf("NewExportService: %v", err)
	}
	ownerOneID := strconv.FormatInt(ownerOne.ID, 10)
	sameRequest := exportIntegrationCreateRequest(now.Add(-24*time.Hour), 0)
	runConcurrent := func(ownerID string, requests []dto.ExportCreateRequest) []exportCreateIntegrationResult {
		start := make(chan struct{})
		results := make(chan exportCreateIntegrationResult, len(requests))
		for _, request := range requests {
			request := request
			go func() {
				<-start
				item, createErr := exports.Create(ctx, ownerID, request)
				results <- exportCreateIntegrationResult{item: item, err: createErr}
			}()
		}
		close(start)
		collected := make([]exportCreateIntegrationResult, 0, len(requests))
		for range requests {
			collected = append(collected, <-results)
		}
		return collected
	}

	dedupRequests := make([]dto.ExportCreateRequest, 8)
	for index := range dedupRequests {
		dedupRequests[index] = sameRequest
	}
	dedupResults := runConcurrent(ownerOneID, dedupRequests)
	jobID := ""
	created := 0
	for _, result := range dedupResults {
		if result.err != nil {
			t.Fatalf("deduplicated create error: %v", result.err)
		}
		if jobID == "" {
			jobID = result.item.ID
		}
		if result.item.ID != jobID {
			t.Fatalf("deduplicated IDs differ: %q and %q", jobID, result.item.ID)
		}
		if !result.item.Deduplicated {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("physical creates = %d, want 1", created)
	}
	var firstJob model.ExportJob
	if err := database.GORM.First(&firstJob, jobID).Error; err != nil {
		t.Fatalf("read deduplicated job: %v", err)
	}
	wantPrefix := ownerOneID + ":csv:global:"
	if firstJob.ActiveKey == nil || !strings.HasPrefix(*firstJob.ActiveKey, wantPrefix) {
		t.Fatalf("active key = %v, want prefix %q", firstJob.ActiveKey, wantPrefix)
	}

	distinct := make([]dto.ExportCreateRequest, 8)
	for index := range distinct {
		distinct[index] = exportIntegrationCreateRequest(now.Add(-48*time.Hour), index+1)
	}
	distinctResults := runConcurrent(ownerOneID, distinct)
	successes, limited := countExportCreateResults(t, distinctResults)
	if successes != 2 || limited != 6 {
		t.Fatalf("per-user results success=%d limited=%d, want 2/6", successes, limited)
	}

	ownerTwoRequests := make([]dto.ExportCreateRequest, 6)
	for index := range ownerTwoRequests {
		ownerTwoRequests[index] = exportIntegrationCreateRequest(now.Add(-72*time.Hour), index+20)
	}
	globalResults := runConcurrent(strconv.FormatInt(ownerTwo.ID, 10), ownerTwoRequests)
	successes, limited = countExportCreateResults(t, globalResults)
	if successes != 1 || limited != 5 {
		t.Fatalf("global results success=%d limited=%d, want 1/5", successes, limited)
	}
	var active int64
	if err := database.GORM.Model(&model.ExportJob{}).Where("status IN ?", []string{"pending", "running"}).Count(&active).Error; err != nil {
		t.Fatalf("count active exports: %v", err)
	}
	if active != 4 {
		t.Fatalf("global active exports = %d, want 4", active)
	}
}

func exportIntegrationCreateRequest(start time.Time, discriminator int) dto.ExportCreateRequest {
	windowStart := start.Add(time.Duration(discriminator) * time.Hour)
	return dto.ExportCreateRequest{
		Format: dto.ExportFormatCSV, StatisticsType: dto.StatisticsScopeGlobal,
		Filters: dto.ExportFilters{
			StartTimestamp: windowStart.Unix(), EndTimestamp: windowStart.Add(time.Hour).Unix(),
			Granularity: dto.StatisticsGranularityHour,
			SiteIDs:     []string{}, CustomerIDs: []string{}, AccountIDs: []string{}, ModelNames: []string{}, ChannelKeys: []string{},
			SortBy: "bucket_start", SortOrder: "asc",
		},
	}
}

func countExportCreateResults(t *testing.T, results []exportCreateIntegrationResult) (int, int) {
	t.Helper()
	successes, limited := 0, 0
	for _, result := range results {
		switch {
		case result.err == nil:
			successes++
		case errors.Is(result.err, ErrExportLimitReached):
			limited++
		default:
			t.Fatalf("unexpected export create error: %v", result.err)
		}
	}
	return successes, limited
}

func createCommittedExportIntegrationUser(t *testing.T, database *gorm.DB, suffix string) model.PlatformUser {
	t.Helper()
	now := time.Now().UnixNano()
	user := model.PlatformUser{
		Username:     "export-quota-" + suffix + "-" + strconv.FormatInt(now, 10),
		PasswordHash: "integration-only", DisplayName: "Export Quota", Role: "viewer", Status: 1,
		SessionVersion: 1, CreatedAt: now / int64(time.Second), UpdatedAt: now / int64(time.Second),
	}
	if err := database.Create(&user).Error; err != nil || user.ID <= 0 {
		t.Fatalf("create quota user = %d, %v", user.ID, err)
	}
	return user
}

func acquireExportServiceIntegrationLock(t *testing.T, ctx context.Context, database *sql.DB) *sql.Conn {
	t.Helper()
	connection, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve export integration lock connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", exportServiceIntegrationLock).Scan(&acquired); err != nil ||
		!acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		t.Fatalf("acquire export integration lock = %v, %v", acquired, err)
	}
	return connection
}

func createExportIntegrationUser(t *testing.T, tx *gorm.DB, role string) model.PlatformUser {
	t.Helper()
	now := time.Now().UnixNano()
	user := model.PlatformUser{
		Username:     "export-download-" + role + "-" + strconv.FormatInt(now, 10),
		PasswordHash: "integration-only", DisplayName: "Export " + role, Role: role, Status: 1,
		SessionVersion: 1, CreatedAt: now / int64(time.Second), UpdatedAt: now / int64(time.Second),
	}
	if err := tx.Create(&user).Error; err != nil || user.ID <= 0 {
		t.Fatalf("create %s fixture user = %d, %v", role, user.ID, err)
	}
	return user
}

func createExportIntegrationSuccessJob(
	t *testing.T,
	tx *gorm.DB,
	ownerID int64,
	filePath string,
	fileName string,
	expiresAt int64,
) model.ExportJob {
	t.Helper()
	filters, err := json.Marshal(dto.ExportFilters{
		StartTimestamp: 1_767_196_800, EndTimestamp: 1_767_200_400,
		Granularity: dto.StatisticsGranularityHour, SiteIDs: []string{}, CustomerIDs: []string{},
		AccountIDs: []string{}, ModelNames: []string{}, ChannelKeys: []string{},
		SortBy: "bucket_start", SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("marshal filters: %v", err)
	}
	now := expiresAt - 3600
	finishedAt := now
	job := model.ExportJob{
		UserID: ownerID, Format: dto.ExportFormatCSV, StatisticsType: dto.StatisticsScopeGlobal,
		Filters: filters, FilterHash: strings.Repeat("a", 64), Status: dto.ExportStatusSuccess, Progress: 100,
		NextAttemptAt: now, FilePath: &filePath, FileName: &fileName, FileSize: 10, RowCount: 1,
		ExpiresAt: &expiresAt, FinishedAt: &finishedAt, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&job).Error; err != nil {
		t.Fatalf("create export job: %v", err)
	}
	return job
}

func assertExportMarkedMissing(t *testing.T, tx *gorm.DB, id int64) {
	t.Helper()
	var job model.ExportJob
	if err := tx.First(&job, id).Error; err != nil {
		t.Fatalf("read missing export: %v", err)
	}
	if job.Status != dto.ExportStatusFailed || job.FilePath != nil || job.FileSize != 0 ||
		job.ErrorCode != "EXPORT_FILE_MISSING" || len(job.ErrorParams) == 0 {
		t.Fatalf("missing export state = %#v", job)
	}
}
