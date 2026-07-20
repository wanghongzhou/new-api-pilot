package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

type a15ExportFixture struct {
	SchemaVersion int    `yaml:"schema_version"`
	FixtureID     string `yaml:"fixture_id"`
	Clock         struct {
		NowUnix int64 `yaml:"now_unix"`
	} `yaml:"clock"`
	Export struct {
		PerUserActiveLimit int   `yaml:"per_user_active_limit"`
		GlobalActiveLimit  int   `yaml:"global_active_limit"`
		MinFreeBytes       int64 `yaml:"min_free_bytes"`
	} `yaml:"export"`
}

func TestA15A16A17ExportCreationAcceptance(t *testing.T) {
	requireA15Database(t)
	ctx := context.Background()
	fixture, fixtureSHA := loadA15ExportFixture(t)
	tx := openCoreAcceptanceTransaction(t)
	if err := testsupport.ResetPlatformSettings(ctx, tx, fixture.Clock.NowUnix); err != nil {
		t.Fatalf("reset deterministic export settings: %v", err)
	}
	configureA15ExportLimits(t, tx, fixture)

	clock := testsupport.NewFakeClock(time.Unix(fixture.Clock.NowUnix, 0))
	exports, err := service.NewExportService(service.ExportServiceOptions{
		Database: tx, Clock: clock, ExportDir: t.TempDir(),
		DiskFree: func(string) (uint64, error) { return math.MaxUint64, nil },
	})
	if err != nil {
		t.Fatalf("create export service: %v", err)
	}
	owner := createA15ExportUser(t, tx, "owner", fixture.Clock.NowUnix)
	ownerID := strconv.FormatInt(owner.ID, 10)

	firstRequest := a15ExportRequest(clock.Now(), 0)
	first, err := exports.Create(ctx, ownerID, firstRequest)
	if err != nil || first.Deduplicated || first.Status != dto.ExportStatusPending || first.ID == "" {
		t.Fatalf("A15 first export = %#v, %v", first, err)
	}
	second, err := exports.Create(ctx, ownerID, firstRequest)
	if err != nil || !second.Deduplicated || second.ID != first.ID || second.Status != dto.ExportStatusPending {
		t.Fatalf("A15 deduplicated export = %#v, %v", second, err)
	}
	if count := countA15ExportJobs(t, tx, owner.ID); count != 1 {
		t.Fatalf("A15 active jobs after duplicate = %d, want 1", count)
	}

	for index := 1; index < fixture.Export.PerUserActiveLimit; index++ {
		created, createErr := exports.Create(ctx, ownerID, a15ExportRequest(clock.Now(), index))
		if createErr != nil || created.Deduplicated || created.Status != dto.ExportStatusPending {
			t.Fatalf("A16 create %d = %#v, %v", index+1, created, createErr)
		}
	}
	_, err = exports.Create(ctx, ownerID, a15ExportRequest(clock.Now(), fixture.Export.PerUserActiveLimit))
	if !errors.Is(err, service.ErrExportLimitReached) {
		t.Fatalf("A16 fourth active export error = %v, want ErrExportLimitReached", err)
	}
	if count := countA15ExportJobs(t, tx, owner.ID); count != int64(fixture.Export.PerUserActiveLimit) {
		t.Fatalf("A16 active jobs = %d, want %d", count, fixture.Export.PerUserActiveLimit)
	}

	lowDisk, err := service.NewExportService(service.ExportServiceOptions{
		Database: tx, Clock: clock, ExportDir: t.TempDir(),
		DiskFree: func(string) (uint64, error) { return uint64(fixture.Export.MinFreeBytes - 1), nil },
	})
	if err != nil {
		t.Fatalf("create low-disk export service: %v", err)
	}
	lowDiskOwner := createA15ExportUser(t, tx, "low-disk", fixture.Clock.NowUnix)
	_, err = lowDisk.Create(ctx, strconv.FormatInt(lowDiskOwner.ID, 10), a15ExportRequest(clock.Now(), 50))
	var diskError *service.ExportDiskLowError
	if !errors.As(err, &diskError) || diskError.FreeBytes != uint64(fixture.Export.MinFreeBytes-1) ||
		diskError.ThresholdBytes != fixture.Export.MinFreeBytes {
		t.Fatalf("A17 low-disk export error = %#v (%v)", diskError, err)
	}
	if count := countA15ExportJobs(t, tx, lowDiskOwner.ID); count != 0 {
		t.Fatalf("A17 low-disk request created %d jobs", count)
	}

	if fixtureSHA == "" {
		t.Fatal("A15 fixture checksum is empty")
	}
}

func TestA41ExportFormulaInjectionAcceptance(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	if err := testsupport.ResetPlatformSettings(context.Background(), database, now); err != nil {
		t.Fatalf("reset A41 platform settings: %v", err)
	}
	hour := coreFloorHour(now - 3600)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCollectionSiteClient(now)
	collector, err := service.NewUsageCollectionService(service.UsageCollectionServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: &collectionSiteClientFactory{client: client}, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create A41 collection service: %v", err)
	}
	site := createCoreAuthorizedSite(t, database, cipher, now)
	formulaValues := []string{"=SUM(A1:A2)", "+cmd", "-1", "@HYPERLINK(\"https://example.test\")"}
	client.flow = make([]dto.UpstreamFlowRow, 0, len(formulaValues))
	client.data = make([]dto.UpstreamDataRow, 0, len(formulaValues))
	for index, name := range formulaValues {
		client.flow = append(client.flow, dto.UpstreamFlowRow{
			UserID: int64(index + 1), Username: fmt.Sprintf("a41-user-%d", index), ModelName: name, ChannelID: 1,
			RequestCount: 1, Quota: 1, TokenUsed: 1,
		})
		client.data = append(client.data, dto.UpstreamDataRow{
			ModelName: name, CreatedAt: hour, RequestCount: 1, Quota: 1, TokenUsed: 1,
		})
	}
	repository := model.NewCollectionTaskRepository(database)
	claim := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now, "a41-formula")
	coreCollectAndCommitUsageWindow(t, repository, collector, claim, now+1)

	query := dto.StatisticsQuery{
		StartTimestamp: hour, EndTimestamp: hour + 3600, Granularity: dto.StatisticsGranularityHour,
		Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	}
	for _, format := range []string{dto.ExportFormatCSV, dto.ExportFormatXLSX} {
		directory := t.TempDir()
		path := filepath.Join(directory, "a41."+format)
		iterator, iteratorErr := service.NewStatisticsExportIterator(service.StatisticsExportIteratorOptions{
			Database: database, Clock: clock, Scope: dto.StatisticsScopeModel, Query: query,
		})
		if iteratorErr != nil {
			t.Fatalf("create A41 %s iterator: %v", format, iteratorErr)
		}
		generated, generateErr := service.GenerateExportFile(context.Background(), service.ExportGenerateOptions{
			Iterator: iterator, Format: format, TemporaryPath: path,
			DataSnapshotAt: now + 1, ExportedAt: now + 2, MaxFileBytes: 1 << 20, MinFreeBytes: 1,
			DiskFree: func(string) (uint64, error) { return math.MaxUint64, nil },
		})
		if generateErr != nil || generated.RowCount != int64(len(formulaValues)) {
			t.Fatalf("A41 generate %s result=%#v err=%v", format, generated, generateErr)
		}
		if format == dto.ExportFormatCSV {
			assertA41CSVFormulaSafety(t, path, formulaValues)
		} else {
			assertA41XLSXFormulaSafety(t, path, formulaValues)
		}
	}
}

func assertA41CSVFormulaSafety(t *testing.T, path string, values []string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(payload, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("A41 read CSV payload err=%v bytes=%x", err, payload[:minA41(3, len(payload))])
	}
	records, err := csv.NewReader(bytes.NewReader(payload[3:])).ReadAll()
	if err != nil || len(records) != len(values)+1 {
		t.Fatalf("A41 parse CSV records=%#v err=%v", records, err)
	}
	seen := make(map[string]bool, len(values))
	for _, record := range records[1:] {
		if len(record) < 3 {
			t.Fatalf("A41 CSV row is truncated: %#v", record)
		}
		seen[record[2]] = true
	}
	for _, value := range values {
		if !seen["'"+value] {
			t.Fatalf("A41 CSV did not quote formula text %q: %#v", value, seen)
		}
	}
}

func assertA41XLSXFormulaSafety(t *testing.T, path string, values []string) {
	t.Helper()
	book, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("A41 open XLSX: %v", err)
	}
	defer func() { _ = book.Close() }()
	rows, err := book.GetRows("Data")
	if err != nil || len(rows) != len(values)+1 {
		t.Fatalf("A41 XLSX rows=%#v err=%v", rows, err)
	}
	seen := make(map[string]bool, len(values))
	for rowIndex, row := range rows[1:] {
		if len(row) < 3 {
			t.Fatalf("A41 XLSX row is truncated: %#v", row)
		}
		axis, axisErr := excelize.CoordinatesToCellName(3, rowIndex+2)
		formula, formulaErr := book.GetCellFormula("Data", axis)
		if axisErr != nil || formulaErr != nil || formula != "" {
			t.Fatalf("A41 XLSX %s formula=%q errors=%v/%v", axis, formula, axisErr, formulaErr)
		}
		seen[row[2]] = true
	}
	for _, value := range values {
		if !seen[value] {
			t.Fatalf("A41 XLSX did not preserve text formula value %q: %#v", value, seen)
		}
	}
}

func minA41(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func requireA15Database(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN")) != "" {
		return
	}
	switch strings.TrimSpace(os.Getenv("ACCEPTANCE_ID")) {
	case "A15", "A16", "A17":
		t.Fatalf("%s requires TEST_DATABASE_DSN", os.Getenv("ACCEPTANCE_ID"))
	}
}

func loadA15ExportFixture(t *testing.T) (a15ExportFixture, string) {
	t.Helper()
	contents, err := os.ReadFile(testsupport.DesignFixturePath("f05-ops-capacity.yaml"))
	if err != nil {
		t.Fatalf("read F05 export fixture: %v", err)
	}
	var fixture a15ExportFixture
	if err := yaml.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode F05 export fixture: %v", err)
	}
	if fixture.SchemaVersion != 2 || fixture.FixtureID != "F05" || fixture.Clock.NowUnix <= 0 ||
		fixture.Export.PerUserActiveLimit != 3 || fixture.Export.GlobalActiveLimit != 10 || fixture.Export.MinFreeBytes <= 1 {
		t.Fatalf("unexpected F05 export fixture: %#v", fixture)
	}
	digest := sha256.Sum256(contents)
	return fixture, hex.EncodeToString(digest[:])
}

func configureA15ExportLimits(t *testing.T, tx *gorm.DB, fixture a15ExportFixture) {
	t.Helper()
	values := map[string]string{
		"export.max_active_per_user": strconv.Itoa(fixture.Export.PerUserActiveLimit),
		"export.max_active_global":   strconv.Itoa(fixture.Export.GlobalActiveLimit),
		"export.min_free_disk_bytes": strconv.FormatInt(fixture.Export.MinFreeBytes, 10),
	}
	for key, value := range values {
		result := tx.Model(&model.PlatformSetting{}).Where("setting_key = ?", key).
			Updates(map[string]any{"setting_value": value, "updated_at": fixture.Clock.NowUnix})
		if result.Error != nil {
			t.Fatalf("configure F05 export setting %s: %v", key, result.Error)
		}
		var setting model.PlatformSetting
		if err := tx.Where("setting_key = ?", key).Take(&setting).Error; err != nil || setting.Value != value {
			t.Fatalf("configure F05 export setting %s = %#v, %v", key, setting, err)
		}
	}
}

func createA15ExportUser(t *testing.T, tx *gorm.DB, suffix string, now int64) model.PlatformUser {
	t.Helper()
	user := model.PlatformUser{
		Username:     fmt.Sprintf("a15-%s-%s", suffix, coreAcceptanceName("export")),
		PasswordHash: "acceptance-only", DisplayName: "A15 Export", Role: "viewer", Status: 1,
		SessionVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&user).Error; err != nil || user.ID <= 0 {
		t.Fatalf("create A15 export user = %#v, %v", user, err)
	}
	return user
}

func a15ExportRequest(now time.Time, discriminator int) dto.ExportCreateRequest {
	end := now.UTC().Truncate(time.Hour).Unix() - int64(discriminator)*2*3600
	start := end - 3600
	return dto.ExportCreateRequest{
		Format: dto.ExportFormatCSV, StatisticsType: dto.StatisticsScopeGlobal,
		Filters: dto.ExportFilters{
			StartTimestamp: start, EndTimestamp: end, Granularity: dto.StatisticsGranularityHour,
			SiteIDs: []string{}, CustomerIDs: []string{}, AccountIDs: []string{}, ModelNames: []string{}, ChannelKeys: []string{},
			SortBy: "bucket_start", SortOrder: "asc",
		},
	}
}

func countA15ExportJobs(t *testing.T, tx *gorm.DB, ownerID int64) int64 {
	t.Helper()
	var count int64
	if err := tx.Model(&model.ExportJob{}).Where("user_id = ?", ownerID).Count(&count).Error; err != nil {
		t.Fatalf("count A15 export jobs: %v", err)
	}
	return count
}
