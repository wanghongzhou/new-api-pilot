package worker

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const exportRuntimeIntegrationLock = "new-api-pilot-export-runtime-integration"

func TestMySQLExportRuntimeRetriesFirstFailureAndFinalizesSecondFailure(t *testing.T) {
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
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	lockConnection := acquireExportRuntimeIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", exportRuntimeIntegrationLock)
		_ = lockConnection.Close()
	}()

	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	now := time.Unix(1_752_400_800, 0)
	owner := model.PlatformUser{
		Username:     "export-runtime-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		PasswordHash: "integration-only", DisplayName: "Export Runtime", Role: "viewer", Status: 1,
		SessionVersion: 1, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := tx.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	clock := testsupport.NewFakeClock(now)
	directory := t.TempDir()
	runtime, err := NewExportRuntime(ExportRuntimeOptions{Database: tx, Clock: clock, ExportDir: directory})
	if err != nil {
		t.Fatalf("NewExportRuntime: %v", err)
	}

	job := exportRuntimePendingJob(owner.ID, "retry-budget", now.Unix())
	if err := runtime.repository.Create(ctx, &job); err != nil {
		t.Fatalf("create retry job: %v", err)
	}
	first, err := runtime.repository.Claim(ctx, now.Unix(), "attempt-one", now.Add(5*time.Minute).Unix())
	if err != nil || first == nil || first.Job.AttemptCount != 1 {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	temporaryName := ".attempt-one.tmp"
	if err := os.WriteFile(filepath.Join(directory, temporaryName), []byte("partial"), 0o600); err != nil {
		t.Fatalf("write partial file: %v", err)
	}
	if err := runtime.finishFailure(ctx, *first, "attempt-one", errors.Join(service.ErrStatisticsRead, errors.New("read failed")), temporaryName); err != nil {
		t.Fatalf("first finishFailure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, temporaryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file remains after failure: %v", err)
	}
	var retrying model.ExportJob
	if err := tx.First(&retrying, job.ID).Error; err != nil {
		t.Fatalf("read retrying job: %v", err)
	}
	if retrying.Status != dto.ExportStatusPending || retrying.AttemptCount != 1 || retrying.ActiveKey == nil ||
		retrying.NextAttemptAt != now.Add(time.Minute).Unix() || retrying.ClaimToken != nil ||
		retrying.ErrorCode != string(constant.MessageExportSnapshotFailed) || retrying.FinishedAt != nil {
		t.Fatalf("first failure state = %#v", retrying)
	}

	clock.Advance(time.Minute)
	second, err := runtime.repository.Claim(ctx, clock.Now().Unix(), "attempt-two", clock.Now().Add(5*time.Minute).Unix())
	if err != nil || second == nil || second.Job.AttemptCount != 2 {
		t.Fatalf("second claim = %#v, %v", second, err)
	}
	if err := runtime.finishFailure(ctx, *second, "attempt-two", errors.Join(service.ErrStatisticsRead, errors.New("read failed again")), ""); err != nil {
		t.Fatalf("second finishFailure: %v", err)
	}
	var failed model.ExportJob
	if err := tx.First(&failed, job.ID).Error; err != nil {
		t.Fatalf("read failed job: %v", err)
	}
	if failed.Status != dto.ExportStatusFailed || failed.AttemptCount != 2 || failed.ActiveKey != nil ||
		failed.ClaimToken != nil || failed.FinishedAt == nil || *failed.FinishedAt != clock.Now().Unix() ||
		failed.ErrorCode != string(constant.MessageExportSnapshotFailed) {
		t.Fatalf("second failure state = %#v", failed)
	}

	largeJob := exportRuntimePendingJob(owner.ID, "file-too-large", clock.Now().Unix())
	if err := runtime.repository.Create(ctx, &largeJob); err != nil {
		t.Fatalf("create large job: %v", err)
	}
	largeClaim, err := runtime.repository.Claim(ctx, clock.Now().Unix(), "large-attempt", clock.Now().Add(5*time.Minute).Unix())
	if err != nil || largeClaim == nil {
		t.Fatalf("large Claim = %#v, %v", largeClaim, err)
	}
	observed := largeClaim.Settings.MaxFileBytes + 12_345
	if err := runtime.finishFailure(ctx, *largeClaim, "large-attempt", &service.ExportFileTooLargeError{
		ObservedBytes: observed, LimitBytes: largeClaim.Settings.MaxFileBytes,
	}, ""); err != nil {
		t.Fatalf("large finishFailure: %v", err)
	}
	var largeFailed model.ExportJob
	if err := tx.First(&largeFailed, largeJob.ID).Error; err != nil {
		t.Fatalf("read large failure: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(largeFailed.ErrorParams, &params); err != nil {
		t.Fatalf("decode large error params: %v", err)
	}
	if largeFailed.Status != dto.ExportStatusFailed || largeFailed.AttemptCount != 1 ||
		largeFailed.ErrorCode != string(constant.MessageExportFileTooLarge) ||
		params["file_bytes"] != strconv.FormatInt(observed, 10) ||
		params["limit_bytes"] != strconv.FormatInt(largeClaim.Settings.MaxFileBytes, 10) {
		t.Fatalf("large failure state=%#v params=%#v", largeFailed, params)
	}
}

func TestMySQLExportRuntimeTakeoverAndTTLRetryArtifactCleanup(t *testing.T) {
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
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	lockConnection := acquireExportRuntimeIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", exportRuntimeIntegrationLock)
		_ = lockConnection.Close()
	}()
	tx := database.GORM.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatalf("begin fixture transaction: %v", tx.Error)
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	now := time.Unix(1_752_400_800, 0)
	owner := model.PlatformUser{
		Username:     "export-cleanup-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		PasswordHash: "integration-only", DisplayName: "Export Cleanup", Role: "viewer", Status: 1,
		SessionVersion: 1, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := tx.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	directory := t.TempDir()
	runtime, err := NewExportRuntime(ExportRuntimeOptions{
		Database: tx, Clock: testsupport.NewFakeClock(now), ExportDir: directory,
	})
	if err != nil {
		t.Fatalf("NewExportRuntime: %v", err)
	}

	regularName := ".takeover-regular.tmp"
	if err := os.WriteFile(filepath.Join(directory, regularName), []byte("partial"), 0o600); err != nil {
		t.Fatalf("write regular takeover artifact: %v", err)
	}
	regular := exportRuntimeRunningJob(owner.ID, "takeover-regular", regularName, now.Unix())
	if err := runtime.repository.Create(ctx, &regular); err != nil {
		t.Fatalf("create regular takeover job: %v", err)
	}
	count, err := runtime.Takeover(ctx)
	if err != nil || count != 1 {
		t.Fatalf("regular Takeover = %d, %v", count, err)
	}
	assertExportRuntimeArtifactState(t, tx, regular.ID, dto.ExportStatusPending, 1, nil)
	if _, err := os.Stat(filepath.Join(directory, regularName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("regular takeover artifact remains: %v", err)
	}

	blockedName := ".takeover-blocked.tmp"
	blockedPath := filepath.Join(directory, blockedName)
	if err := os.Mkdir(blockedPath, 0o700); err != nil {
		t.Fatalf("create blocked takeover artifact: %v", err)
	}
	blocked := exportRuntimeRunningJob(owner.ID, "takeover-blocked", blockedName, now.Unix())
	if err := runtime.repository.Create(ctx, &blocked); err != nil {
		t.Fatalf("create blocked takeover job: %v", err)
	}
	if _, err := runtime.Takeover(ctx); !errors.Is(err, service.ErrExportContract) {
		t.Fatalf("blocked Takeover error = %v", err)
	}
	assertExportRuntimeArtifactState(t, tx, blocked.ID, dto.ExportStatusPending, 1, &blockedName)
	if err := os.Remove(blockedPath); err != nil {
		t.Fatalf("remove blocked directory: %v", err)
	}
	if err := os.WriteFile(blockedPath, []byte("retry"), 0o600); err != nil {
		t.Fatalf("replace blocked artifact: %v", err)
	}
	count, err = runtime.Takeover(ctx)
	if err != nil || count != 1 {
		t.Fatalf("retried Takeover = %d, %v", count, err)
	}
	assertExportRuntimeArtifactState(t, tx, blocked.ID, dto.ExportStatusPending, 1, nil)

	expiredName := "statistics-global-1-2-expired.csv"
	if err := os.WriteFile(filepath.Join(directory, expiredName), []byte("expired"), 0o600); err != nil {
		t.Fatalf("write expired artifact: %v", err)
	}
	expired := exportRuntimeSuccessfulJob(owner.ID, expiredName, now.Add(-time.Second).Unix(), now.Unix())
	if err := runtime.repository.Create(ctx, &expired); err != nil {
		t.Fatalf("create expired job: %v", err)
	}
	count, err = runtime.CleanupOnce(ctx)
	if err != nil || count != 1 {
		t.Fatalf("regular CleanupOnce = %d, %v", count, err)
	}
	assertExportRuntimeArtifactState(t, tx, expired.ID, dto.ExportStatusExpired, 1, nil)

	blockedExpiredName := "statistics-global-1-2-blocked.csv"
	blockedExpiredPath := filepath.Join(directory, blockedExpiredName)
	if err := os.Mkdir(blockedExpiredPath, 0o700); err != nil {
		t.Fatalf("create blocked expired artifact: %v", err)
	}
	blockedExpired := exportRuntimeSuccessfulJob(owner.ID, blockedExpiredName, now.Add(-time.Second).Unix(), now.Unix())
	if err := runtime.repository.Create(ctx, &blockedExpired); err != nil {
		t.Fatalf("create blocked expired job: %v", err)
	}
	if _, err := runtime.CleanupOnce(ctx); !errors.Is(err, service.ErrExportContract) {
		t.Fatalf("blocked CleanupOnce error = %v", err)
	}
	assertExportRuntimeArtifactState(t, tx, blockedExpired.ID, dto.ExportStatusExpired, 1, &blockedExpiredName)
	if err := os.Remove(blockedExpiredPath); err != nil {
		t.Fatalf("remove blocked expired directory: %v", err)
	}
	if err := os.WriteFile(blockedExpiredPath, []byte("retry"), 0o600); err != nil {
		t.Fatalf("replace blocked expired artifact: %v", err)
	}
	count, err = runtime.CleanupOnce(ctx)
	if err != nil || count != 1 {
		t.Fatalf("retried CleanupOnce = %d, %v", count, err)
	}
	assertExportRuntimeArtifactState(t, tx, blockedExpired.ID, dto.ExportStatusExpired, 1, nil)
}

func TestMySQLExportRuntimeLongRangeRetryRefreshesDataSnapshotAndFreezesRates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 4, MaxOpen: 12, MaxLifetime: time.Minute})
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
	lockConnection := acquireExportRuntimeIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", exportRuntimeIntegrationLock)
		_ = lockConnection.Close()
	}()
	if err := database.GORM.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	if err := database.GORM.Where("base_url LIKE ?", "https://long-range-runtime-%").
		Delete(&model.Site{}).Error; err != nil {
		t.Fatalf("clear stale long-range export sites: %v", err)
	}
	if err := database.GORM.Where("username LIKE ?", "export-long-range-%").
		Delete(&model.PlatformUser{}).Error; err != nil {
		t.Fatalf("clear stale long-range export owners: %v", err)
	}

	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 32)
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, location)
	owner := model.PlatformUser{
		Username:     "export-long-range-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		PasswordHash: "integration-only", DisplayName: "Export Long Range", Role: "viewer", Status: 1,
		SessionVersion: 1, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := database.GORM.Create(&owner).Error; err != nil {
		t.Fatalf("create long-range export owner: %v", err)
	}
	quotaPerUnit := "500000.0000000000"
	exchangeRate := "7.1000000000"
	rateAt := now.Add(-time.Hour).Unix()
	statisticsStart, statisticsEnd := start.Unix(), end.Unix()
	site := model.Site{
		Name:          "Long Range Export Site",
		BaseURL:       "https://long-range-runtime-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".example.com",
		ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive,
		OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized,
		StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK,
		DataExportEnabled: true, StatisticsStartAt: &statisticsStart, StatisticsEndAt: &statisticsEnd,
		QuotaPerUnit: &quotaPerUnit, USDExchangeRate: &exchangeRate, LastRateAt: &rateAt,
		CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := database.GORM.Create(&site).Error; err != nil {
		t.Fatalf("create long-range export site: %v", err)
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		_ = database.GORM.WithContext(cleanup).Where("user_id = ?", owner.ID).Delete(&model.ExportJob{}).Error
		_ = database.GORM.WithContext(cleanup).Delete(&model.Site{}, site.ID).Error
		_ = database.GORM.WithContext(cleanup).Delete(&model.PlatformUser{}, owner.ID).Error
	}()

	filters := dto.ExportFilters{
		StartTimestamp: start.Unix(), EndTimestamp: end.Unix(),
		Granularity: dto.StatisticsGranularityHour,
		SiteIDs:     []string{strconv.FormatInt(site.ID, 10)}, CustomerIDs: []string{},
		AccountIDs: []string{}, ModelNames: []string{}, ChannelKeys: []string{},
		SortBy: "bucket_start", SortOrder: "asc",
	}
	rawFilters, err := json.Marshal(filters)
	if err != nil {
		t.Fatalf("marshal long-range filters: %v", err)
	}
	filterHash := strings.Repeat("b", 64)
	activeKey := strconv.FormatInt(owner.ID, 10) + ":csv:global:" + filterHash
	job := model.ExportJob{
		UserID: owner.ID, Format: dto.ExportFormatCSV, StatisticsType: dto.StatisticsScopeGlobal,
		Filters: rawFilters, FilterHash: filterHash, ActiveKey: &activeKey,
		Status: dto.ExportStatusPending, NextAttemptAt: now.Unix(), CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := model.NewExportRepository(database.GORM).Create(ctx, &job); err != nil {
		t.Fatalf("create long-range export job: %v", err)
	}

	clock := testsupport.NewFakeClock(now)
	directory := t.TempDir()
	diskChecks := 0
	runtime, err := NewExportRuntime(ExportRuntimeOptions{
		Database: database.GORM, Clock: clock, ExportDir: directory,
		DiskFree: func(string) (uint64, error) {
			diskChecks++
			if diskChecks == 1 {
				return 0, errors.New("injected disk probe failure")
			}
			return ^uint64(0), nil
		},
	})
	if err != nil {
		t.Fatalf("NewExportRuntime: %v", err)
	}
	processed, err := runtime.RunOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("first long-range attempt = %t, %v", processed, err)
	}
	var firstAttempt model.ExportJob
	if err := database.GORM.First(&firstAttempt, job.ID).Error; err != nil {
		t.Fatalf("read first long-range attempt: %v", err)
	}
	if firstAttempt.Status != dto.ExportStatusPending || firstAttempt.AttemptCount != 1 ||
		len(firstAttempt.RateSnapshot) == 0 || firstAttempt.DataSnapshotAt == nil ||
		*firstAttempt.DataSnapshotAt != now.Unix() {
		t.Fatalf("first long-range attempt state = %#v", firstAttempt)
	}
	frozenRates := append(json.RawMessage(nil), firstAttempt.RateSnapshot...)
	firstDataSnapshot := *firstAttempt.DataSnapshotAt

	updatedQuota := "999999.0000000000"
	updatedExchange := "9.9000000000"
	updatedRateAt := now.Unix()
	if err := database.GORM.Model(&model.Site{}).Where("id = ?", site.ID).Updates(map[string]any{
		"quota_per_unit": updatedQuota, "usd_exchange_rate": updatedExchange,
		"last_rate_at": updatedRateAt, "updated_at": updatedRateAt,
	}).Error; err != nil {
		t.Fatalf("update rate between export attempts: %v", err)
	}
	clock.Advance(61 * time.Second)
	processed, err = runtime.RunOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("second long-range attempt = %t, %v", processed, err)
	}
	var completed model.ExportJob
	if err := database.GORM.First(&completed, job.ID).Error; err != nil {
		t.Fatalf("read completed long-range attempt: %v", err)
	}
	if completed.Status != dto.ExportStatusSuccess || completed.AttemptCount != 2 ||
		completed.RowCount != int64(32*24) || completed.DataSnapshotAt == nil ||
		*completed.DataSnapshotAt == firstDataSnapshot || string(completed.RateSnapshot) != string(frozenRates) ||
		completed.FilePath == nil {
		t.Fatalf("completed long-range state = %#v", completed)
	}

	artifact, err := os.ReadFile(filepath.Join(directory, *completed.FilePath))
	if err != nil {
		t.Fatalf("read long-range export artifact: %v", err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(artifact), string([]byte{0xef, 0xbb, 0xbf}))))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse long-range export CSV: %v", err)
	}
	if len(records) != 32*24+1 || len(records[0]) != 21 {
		t.Fatalf("long-range CSV shape = %d x %d", len(records), len(records[0]))
	}
	var frozen service.ExportRateSnapshot
	if err := json.Unmarshal(frozenRates, &frozen); err != nil || len(frozen.Sites) != 1 {
		t.Fatalf("decode frozen rate snapshot = %#v, %v", frozen, err)
	}
	wantQuotaPerUnit := ""
	wantExchangeRate := ""
	if frozen.Sites[0].QuotaPerUnit != nil {
		wantQuotaPerUnit = *frozen.Sites[0].QuotaPerUnit
	}
	if frozen.Sites[0].USDExchangeRate != nil {
		wantExchangeRate = *frozen.Sites[0].USDExchangeRate
	}
	wantSnapshotTime := time.Unix(*completed.DataSnapshotAt, 0).In(location).Format("2006-01-02 15:04:05")
	if records[1][11] != wantQuotaPerUnit || records[1][12] != wantExchangeRate ||
		records[1][11] == updatedQuota || records[1][12] == updatedExchange || records[1][19] != wantSnapshotTime {
		t.Fatalf("long-range CSV frozen snapshot row = %#v", records[1])
	}
}

func TestMySQLExportRuntimeHardStopPreservesClaimAndCleansTemporaryArtifact(t *testing.T) {
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
	if err := model.NewSeeder(database.SQL).Run(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	lockConnection := acquireExportRuntimeIntegrationLock(t, ctx, database.SQL)
	defer func() {
		_, _ = lockConnection.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", exportRuntimeIntegrationLock)
		_ = lockConnection.Close()
	}()
	if err := database.GORM.Exec("DELETE FROM export_job").Error; err != nil {
		t.Fatalf("clear export jobs: %v", err)
	}
	now := time.Unix(1_752_400_800, 0)
	owner := model.PlatformUser{
		Username:     "export-hard-stop-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		PasswordHash: "integration-only", DisplayName: "Export Hard Stop", Role: "viewer", Status: 1,
		SessionVersion: 1, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}
	if err := database.GORM.Create(&owner).Error; err != nil {
		t.Fatalf("create hard-stop owner: %v", err)
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = database.GORM.WithContext(cleanup).Where("user_id = ?", owner.ID).Delete(&model.ExportJob{}).Error
		_ = database.GORM.WithContext(cleanup).Delete(&model.PlatformUser{}, owner.ID).Error
	}()
	job := exportRuntimePendingJob(owner.ID, "hard-stop", now.Unix())
	if err := model.NewExportRepository(database.GORM).Create(ctx, &job); err != nil {
		t.Fatalf("create hard-stop export: %v", err)
	}

	clock := testsupport.NewFakeClock(now)
	directory := t.TempDir()
	var diskChecks atomic.Int32
	generationBlocked := make(chan struct{})
	releaseGeneration := make(chan struct{})
	runtime, err := NewExportRuntime(ExportRuntimeOptions{
		Database: database.GORM, Clock: clock, ExportDir: directory,
		DiskFree: func(string) (uint64, error) {
			if diskChecks.Add(1) == 2 {
				close(generationBlocked)
				<-releaseGeneration
			}
			return ^uint64(0), nil
		},
	})
	if err != nil {
		t.Fatalf("NewExportRuntime: %v", err)
	}
	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	if err := runtime.Start(runtimeContext); err != nil || !runtime.Ready() {
		t.Fatalf("start export runtime ready=%t err=%v", runtime.Ready(), err)
	}
	var beforeClaim model.ExportJob
	if err := database.GORM.First(&beforeClaim, job.ID).Error; err != nil || beforeClaim.Status != dto.ExportStatusPending {
		t.Fatalf("runtime claimed before ready tick: %#v, %v", beforeClaim, err)
	}
	clock.Advance(time.Second)
	select {
	case <-generationBlocked:
	case <-time.After(10 * time.Second):
		t.Fatal("export generation did not reach the blocking disk check")
	}
	var running model.ExportJob
	if err := database.GORM.First(&running, job.ID).Error; err != nil || running.Status != dto.ExportStatusRunning ||
		running.ClaimToken == nil || running.FilePath == nil {
		t.Fatalf("in-flight export state = %#v, %v", running, err)
	}
	temporaryPath := filepath.Join(directory, *running.FilePath)
	if _, err := os.Stat(temporaryPath); err != nil {
		t.Fatalf("in-flight temporary artifact: %v", err)
	}
	if err := runtime.Quiesce(); err != nil || runtime.Ready() {
		t.Fatalf("quiesce export runtime ready=%t err=%v", runtime.Ready(), err)
	}
	stopContext, cancelStop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	stopErr := runtime.Stop(stopContext)
	cancelStop()
	if !errors.Is(stopErr, ErrRuntimeStopTimeout) || !errors.Is(stopErr, context.DeadlineExceeded) {
		t.Fatalf("hard-stop error = %v", stopErr)
	}
	close(releaseGeneration)
	runtime.mu.Lock()
	done := runtime.done
	runtime.mu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("canceled export runtime goroutine did not exit")
	}
	if _, err := os.Stat(temporaryPath); !os.IsNotExist(err) {
		t.Fatalf("hard stop retained temporary artifact: %v", err)
	}
	var abandoned model.ExportJob
	if err := database.GORM.First(&abandoned, job.ID).Error; err != nil ||
		abandoned.Status != dto.ExportStatusRunning || abandoned.ClaimToken == nil || abandoned.FilePath == nil {
		t.Fatalf("hard stop destroyed takeover claim semantics: %#v, %v", abandoned, err)
	}

	recovery, err := NewExportRuntime(ExportRuntimeOptions{
		Database: database.GORM, Clock: clock, ExportDir: directory,
		DiskFree: func(string) (uint64, error) { return ^uint64(0), nil },
	})
	if err != nil {
		t.Fatalf("create recovery runtime: %v", err)
	}
	recoveryContext, cancelRecovery := context.WithCancel(context.Background())
	defer cancelRecovery()
	if err := recovery.Start(recoveryContext); err != nil {
		t.Fatalf("start recovery runtime: %v", err)
	}
	var recovered model.ExportJob
	if err := database.GORM.First(&recovered, job.ID).Error; err != nil || recovered.Status != dto.ExportStatusPending ||
		recovered.AttemptCount != 1 || recovered.ClaimToken != nil || recovered.FilePath != nil {
		t.Fatalf("takeover recovery state = %#v, %v", recovered, err)
	}
	if err := recovery.Quiesce(); err != nil {
		t.Fatalf("quiesce recovery runtime: %v", err)
	}
	recoveryStop, cancelRecoveryStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRecoveryStop()
	if err := recovery.Stop(recoveryStop); err != nil {
		t.Fatalf("stop recovery runtime: %v", err)
	}
}

func acquireExportRuntimeIntegrationLock(t *testing.T, ctx context.Context, database *sql.DB) *sql.Conn {
	t.Helper()
	connection, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("reserve export runtime integration lock: %v", err)
	}
	var acquired sql.NullInt64
	if err := connection.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", exportRuntimeIntegrationLock).Scan(&acquired); err != nil ||
		!acquired.Valid || acquired.Int64 != 1 {
		_ = connection.Close()
		t.Fatalf("acquire export runtime integration lock = %v, %v", acquired, err)
	}
	return connection
}

func exportRuntimePendingJob(ownerID int64, suffix string, now int64) model.ExportJob {
	activeKey := "runtime:" + suffix
	filters, _ := json.Marshal(dto.ExportFilters{
		StartTimestamp: 1_752_397_200, EndTimestamp: 1_752_400_800,
		Granularity: dto.StatisticsGranularityHour,
		SiteIDs:     []string{}, CustomerIDs: []string{}, AccountIDs: []string{}, ModelNames: []string{}, ChannelKeys: []string{},
		SortBy: "bucket_start", SortOrder: "asc",
	})
	return model.ExportJob{
		UserID: ownerID, Format: dto.ExportFormatCSV, StatisticsType: dto.StatisticsScopeGlobal,
		Filters: filters, FilterHash: strings.Repeat("a", 64), ActiveKey: &activeKey,
		Status: dto.ExportStatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func exportRuntimeRunningJob(ownerID int64, suffix string, path string, now int64) model.ExportJob {
	job := exportRuntimePendingJob(ownerID, suffix, now)
	token := "abandoned-token"
	lease := now + 300
	job.Status = dto.ExportStatusRunning
	job.AttemptCount = 1
	job.ClaimToken = &token
	job.LeaseExpiresAt = &lease
	job.FilePath = &path
	job.StartedAt = &now
	return job
}

func exportRuntimeSuccessfulJob(ownerID int64, path string, expiresAt int64, now int64) model.ExportJob {
	job := exportRuntimePendingJob(ownerID, "success-"+path, now)
	job.Status = dto.ExportStatusSuccess
	job.Progress = 100
	job.AttemptCount = 1
	job.ActiveKey = nil
	job.FilePath = &path
	job.FileName = &path
	job.FileSize = 1
	job.RowCount = 1
	job.ExpiresAt = &expiresAt
	job.FinishedAt = &now
	return job
}

func assertExportRuntimeArtifactState(
	t *testing.T,
	tx *gorm.DB,
	id int64,
	status string,
	attempts int,
	path *string,
) {
	t.Helper()
	var job model.ExportJob
	if err := tx.First(&job, id).Error; err != nil {
		t.Fatalf("read export artifact state: %v", err)
	}
	if job.Status != status || job.AttemptCount != attempts {
		t.Fatalf("artifact state = %#v", job)
	}
	if path == nil && job.FilePath != nil {
		t.Fatalf("artifact path = %q, want nil", *job.FilePath)
	}
	if path != nil && (job.FilePath == nil || *job.FilePath != *path) {
		t.Fatalf("artifact path = %v, want %q", job.FilePath, *path)
	}
}
