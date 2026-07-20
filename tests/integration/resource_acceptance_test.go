package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

const (
	a62AcceptanceID             = "A62"
	a62BulkPerSite              = 2600
	a62BatchSize                = 257
	a62StarvationBatchSize      = 17
	a62StarvationMaximumBatches = 3
	a62StarvationBlockedRows    = a62StarvationBatchSize*a62StarvationMaximumBatches + 1
)

var a62Location = time.FixedZone("Asia/Shanghai", 8*60*60)

type a62Fixture struct {
	SchemaVersion int    `yaml:"schema_version"`
	FixtureID     string `yaml:"fixture_id"`
	Clock         struct {
		NowUnix int64 `yaml:"now_unix"`
	} `yaml:"clock"`
	Retention struct {
		ResourceMinutelyDays int `yaml:"resource_minutely_days"`
	} `yaml:"retention"`
}

func TestA19A63ResourceSnapshotAcceptance(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCoreSiteClient(now)
	factory := &coreSiteClientFactory{client: client}
	sites, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create A19/A63 site service: %v", err)
	}
	seedA19ResourceRules(t, database, now)
	site := createCoreAuthorizedSite(t, database, cipher, now)
	cpu, memory, disk := 25.0, 40.0, 55.0
	client.instances = []dto.UpstreamInstance{
		{
			NodeName: "node-a", Hostname: "shared-resource-host", Status: "online", StaleAfterSeconds: 90,
			StartedAt: now - 3600, LastSeenAt: now, CPUPercent: &cpu, MemoryPercent: &memory, StorageUsedPercent: &disk,
		},
		{
			NodeName: "node-b", Hostname: "shared-resource-host", Status: "online", StaleAfterSeconds: 90,
			StartedAt: now - 3600, LastSeenAt: now, CPUPercent: &cpu, MemoryPercent: &memory, StorageUsedPercent: &disk,
		},
	}
	if fetched, written, executeErr := sites.ExecutePeriodicSiteTask(
		context.Background(), constant.TaskTypeResourceSnapshot, site.ID, site.ConfigVersion, "a19-a63-initial",
	); executeErr != nil || fetched != 2 || written != 3 {
		t.Fatalf("A63 initial resource snapshot fetched=%d written=%d err=%v", fetched, written, executeErr)
	}
	items, err := sites.ListInstances(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("A63 list current instances: %v", err)
	}
	byNode := a19ItemsByNode(items)
	if len(byNode) != 2 || byNode["node-a"].Hostname != "shared-resource-host" ||
		byNode["node-b"].Hostname != "shared-resource-host" ||
		byNode["node-a"].CurrentStatus != "online" || byNode["node-b"].CurrentStatus != "online" ||
		byNode["node-a"].DataStatus != "complete" || byNode["node-b"].DataStatus != "complete" {
		t.Fatalf("A63 node-name resource identity was merged or incomplete: %#v", items)
	}

	// A failed upstream round must not invent a current sample or an all-instance failure.
	clock.Advance(time.Minute)
	failedMinute := clock.Now().Unix() - clock.Now().Unix()%60
	upstreamFailure := errors.New("instances endpoint unavailable")
	client.instancesErr = upstreamFailure
	if fetched, written, executeErr := sites.ExecutePeriodicSiteTask(
		context.Background(), constant.TaskTypeResourceSnapshot, site.ID, site.ConfigVersion, "a19-resource-failure",
	); !errors.Is(executeErr, upstreamFailure) || fetched != 0 || written != 0 {
		t.Fatalf("A19 failed resource snapshot fetched=%d written=%d err=%v", fetched, written, executeErr)
	}
	for _, table := range []any{&model.SiteInstanceStatusMinutely{}, &model.SiteStatusMinutely{}} {
		var count int64
		if err := database.Model(table).Where("site_id = ? AND minute_ts = ?", site.ID, failedMinute).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("A19 failure wrote a current %T row count=%d err=%v", table, count, err)
		}
	}
	items, err = sites.ListInstances(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("A19 list instances after upstream failure: %v", err)
	}
	for node, item := range a19ItemsByNode(items) {
		if item.CurrentStatus != "unknown" || item.DataStatus != "missing" {
			t.Fatalf("A19 failed resource status for %s = %#v, want unknown/missing", node, item)
		}
	}
	alertService, err := service.NewAlertService(service.AlertServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create A19 alert service: %v", err)
	}
	unknown, err := alertService.Evaluate(context.Background(), service.AlertEvaluation{
		RuleKey: "site_no_instance", SiteID: &site.ID, TargetType: "site", TargetKey: fmt.Sprintf("%d", site.ID),
		TargetName: site.Name, State: service.AlertSampleUnknown, ObservedAt: failedMinute, SampleKey: "a19-resource-failure",
	})
	if err != nil || unknown.Transition != "unknown" {
		t.Fatalf("A19 unknown resource alert evaluation = %#v, %v", unknown, err)
	}
	var falseNoInstanceEvents int64
	if err := database.Model(&model.AlertEvent{}).Where("site_id = ? AND rule_key = ?", site.ID, "site_no_instance").
		Count(&falseNoInstanceEvents).Error; err != nil || falseNoInstanceEvents != 0 {
		t.Fatalf("A19 failed resource sample created all-instances event count=%d err=%v", falseNoInstanceEvents, err)
	}

	// A successful empty upstream snapshot retires nodes; a failed round above did not.
	client.instancesErr = nil
	client.instances = []dto.UpstreamInstance{}
	clock.Advance(1439 * time.Minute)
	if fetched, written, executeErr := sites.ExecutePeriodicSiteTask(
		context.Background(), constant.TaskTypeResourceSnapshot, site.ID, site.ConfigVersion, "a63-long-absence",
	); executeErr != nil || fetched != 0 || written != 1 {
		t.Fatalf("A63 long missing snapshot fetched=%d written=%d err=%v", fetched, written, executeErr)
	}
	var retained []model.SiteInstance
	if err := database.Where("site_id = ?", site.ID).Order("node_name ASC").Find(&retained).Error; err != nil || len(retained) != 2 ||
		retained[0].RetiredAt == nil || retained[1].RetiredAt == nil {
		t.Fatalf("A63 retired nodes after successful empty snapshot = %#v, %v", retained, err)
	}

	// The production decoder reports malformed upstream instance payloads as a failed whole round.
	clock.Advance(time.Minute)
	invalidMinute := clock.Now().Unix() - clock.Now().Unix()%60
	client.instancesErr = service.ErrUpstreamResponseInvalid
	if _, _, executeErr := sites.ExecutePeriodicSiteTask(
		context.Background(), constant.TaskTypeResourceSnapshot, site.ID, site.ConfigVersion, "a63-invalid-instance",
	); !errors.Is(executeErr, service.ErrUpstreamResponseInvalid) {
		t.Fatalf("A63 invalid upstream instance result = %v", executeErr)
	}
	for _, table := range []any{&model.SiteInstanceStatusMinutely{}, &model.SiteStatusMinutely{}} {
		var count int64
		if err := database.Model(table).Where("site_id = ? AND minute_ts = ?", site.ID, invalidMinute).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("A63 invalid upstream response partially wrote %T rows=%d err=%v", table, count, err)
		}
	}
}

func seedA19ResourceRules(t *testing.T, database *gorm.DB, now int64) {
	t.Helper()
	staleThreshold, noInstanceThreshold := "90", "0"
	rules := []model.AlertRule{
		{RuleKey: "instance_stale", Name: "A19 Instance Stale", Enabled: true, Level: "warning", Metric: "instance.stale_seconds", CompareOperator: ">=", ThresholdValue: &staleThreshold, ForTimes: 1, ScopeType: "global", ScopeID: 0, CreatedAt: now, UpdatedAt: now},
		{RuleKey: "site_no_instance", Name: "A19 No Instance", Enabled: true, Level: "critical", Metric: "site.online_instances", CompareOperator: "<=", ThresholdValue: &noInstanceThreshold, ForTimes: 1, ScopeType: "global", ScopeID: 0, CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&rules).Error; err != nil {
		t.Fatalf("seed A19 resource alert rules: %v", err)
	}
}

func a19ItemsByNode(items []dto.SiteInstanceItem) map[string]dto.SiteInstanceItem {
	result := make(map[string]dto.SiteInstanceItem, len(items))
	for _, item := range items {
		result[item.NodeName] = item
	}
	return result
}

type a62Report struct {
	SchemaVersion              int                             `json:"schema_version"`
	AcceptanceID               string                          `json:"acceptance_id"`
	Status                     string                          `json:"status"`
	FixturePath                string                          `json:"fixture_path"`
	FixtureSHA256              string                          `json:"fixture_sha256"`
	FixedNowUnix               int64                           `json:"fixed_now_unix"`
	RetentionDays              int                             `json:"retention_days"`
	Cutoff                     int64                           `json:"cutoff"`
	BatchSize                  int                             `json:"batch_size"`
	InitialRowsPerTable        int64                           `json:"initial_rows_per_table"`
	FirstRun                   service.ResourceRetentionResult `json:"first_run"`
	SecondRun                  service.ResourceRetentionResult `json:"second_run"`
	IdempotentRun              service.ResourceRetentionResult `json:"idempotent_run"`
	RowsAfterFirstRun          int64                           `json:"rows_after_first_run"`
	RowsAfterFinalRun          int64                           `json:"rows_after_final_run"`
	ProtectedAggregateSHA256   string                          `json:"protected_aggregate_sha256"`
	BusinessFactsPreserved     bool                            `json:"business_facts_preserved"`
	ExactBoundaryPreserved     bool                            `json:"exact_boundary_preserved"`
	MissingHourlyBlocked       bool                            `json:"missing_hourly_blocked"`
	DailyNotFinalBlocked       bool                            `json:"daily_not_final_blocked"`
	InvalidRetentionRejected   bool                            `json:"invalid_retention_rejected"`
	HourlyDailyValuesUnchanged bool                            `json:"hourly_daily_values_unchanged"`
	StarvationProof            a62StarvationProof              `json:"starvation_proof"`
}

type a62StarvationProof struct {
	BlockedPrefixRows                  int                             `json:"blocked_prefix_rows"`
	BatchSize                          int                             `json:"batch_size"`
	MaximumBatches                     int                             `json:"maximum_batches"`
	FirstRun                           service.ResourceRetentionResult `json:"first_run"`
	RestartRun                         service.ResourceRetentionResult `json:"restart_run"`
	FinalRun                           service.ResourceRetentionResult `json:"final_run"`
	EligibleDeletedBehindBlockedPrefix bool                            `json:"eligible_deleted_behind_blocked_prefix"`
	RestartContinuationProved          bool                            `json:"restart_continuation_proved"`
}

type a62SiteStatusHourly struct {
	SiteID                 int64    `gorm:"column:site_id"`
	HourTS                 int64    `gorm:"column:hour_ts"`
	InstanceCountMax       int      `gorm:"column:instance_count_max"`
	OnlineInstanceCountMin int      `gorm:"column:online_instance_count_min"`
	CPUMaxPercent          *float64 `gorm:"column:cpu_max_percent"`
	CPUAvgPercent          *float64 `gorm:"column:cpu_avg_percent"`
	MemoryMaxPercent       *float64 `gorm:"column:memory_max_percent"`
	MemoryAvgPercent       *float64 `gorm:"column:memory_avg_percent"`
	DiskMaxUsedPercent     *float64 `gorm:"column:disk_max_used_percent"`
	AbnormalSamples        int      `gorm:"column:abnormal_samples"`
	SampleCount            int      `gorm:"column:sample_count"`
	ExpectedSampleCount    int      `gorm:"column:expected_sample_count"`
	DataStatus             string   `gorm:"column:data_status"`
	HealthStatus           string   `gorm:"column:health_status"`
	LastCalculatedAt       int64    `gorm:"column:last_calculated_at"`
}

func (a62SiteStatusHourly) TableName() string { return "site_status_hourly" }

type a62SiteStatusDaily struct {
	SiteID                 int64    `gorm:"column:site_id"`
	DateKey                int      `gorm:"column:date_key"`
	InstanceCountMax       int      `gorm:"column:instance_count_max"`
	OnlineInstanceCountMin int      `gorm:"column:online_instance_count_min"`
	CPUMaxPercent          *float64 `gorm:"column:cpu_max_percent"`
	CPUAvgPercent          *float64 `gorm:"column:cpu_avg_percent"`
	MemoryMaxPercent       *float64 `gorm:"column:memory_max_percent"`
	MemoryAvgPercent       *float64 `gorm:"column:memory_avg_percent"`
	DiskMaxUsedPercent     *float64 `gorm:"column:disk_max_used_percent"`
	AbnormalSamples        int      `gorm:"column:abnormal_samples"`
	SampleCount            int      `gorm:"column:sample_count"`
	ExpectedSampleCount    int      `gorm:"column:expected_sample_count"`
	DataStatus             string   `gorm:"column:data_status"`
	HealthStatus           string   `gorm:"column:health_status"`
	IsFinal                bool     `gorm:"column:is_final"`
	LastCalculatedAt       int64    `gorm:"column:last_calculated_at"`
}

func (a62SiteStatusDaily) TableName() string { return "site_status_daily" }

type a62InstanceStatusHourly struct {
	SiteID              int64    `gorm:"column:site_id"`
	NodeName            string   `gorm:"column:node_name"`
	HourTS              int64    `gorm:"column:hour_ts"`
	CPUMaxPercent       *float64 `gorm:"column:cpu_max_percent"`
	CPUAvgPercent       *float64 `gorm:"column:cpu_avg_percent"`
	MemoryMaxPercent    *float64 `gorm:"column:memory_max_percent"`
	MemoryAvgPercent    *float64 `gorm:"column:memory_avg_percent"`
	DiskMaxUsedPercent  *float64 `gorm:"column:disk_max_used_percent"`
	DiskLastUsedPercent *float64 `gorm:"column:disk_last_used_percent"`
	OnlineSamples       int      `gorm:"column:online_samples"`
	AbnormalSamples     int      `gorm:"column:abnormal_samples"`
	SampleCount         int      `gorm:"column:sample_count"`
	ExpectedSampleCount int      `gorm:"column:expected_sample_count"`
	DataStatus          string   `gorm:"column:data_status"`
	LastCalculatedAt    int64    `gorm:"column:last_calculated_at"`
}

func (a62InstanceStatusHourly) TableName() string { return "site_instance_status_hourly" }

type a62InstanceStatusDaily struct {
	SiteID              int64    `gorm:"column:site_id"`
	NodeName            string   `gorm:"column:node_name"`
	DateKey             int      `gorm:"column:date_key"`
	CPUMaxPercent       *float64 `gorm:"column:cpu_max_percent"`
	CPUAvgPercent       *float64 `gorm:"column:cpu_avg_percent"`
	MemoryMaxPercent    *float64 `gorm:"column:memory_max_percent"`
	MemoryAvgPercent    *float64 `gorm:"column:memory_avg_percent"`
	DiskMaxUsedPercent  *float64 `gorm:"column:disk_max_used_percent"`
	DiskLastUsedPercent *float64 `gorm:"column:disk_last_used_percent"`
	OnlineSamples       int      `gorm:"column:online_samples"`
	AbnormalSamples     int      `gorm:"column:abnormal_samples"`
	SampleCount         int      `gorm:"column:sample_count"`
	ExpectedSampleCount int      `gorm:"column:expected_sample_count"`
	DataStatus          string   `gorm:"column:data_status"`
	IsFinal             bool     `gorm:"column:is_final"`
	LastCalculatedAt    int64    `gorm:"column:last_calculated_at"`
}

func (a62InstanceStatusDaily) TableName() string { return "site_instance_status_daily" }

type a62SeedState struct {
	sites               []model.Site
	nodes               []string
	cutoffMinusMinute   int64
	cutoff              int64
	futureMinute        int64
	missingHourlyMinute int64
	dailyNotFinalMinute int64
	protectedSiteIDs    []int64
}

func TestA62ResourceMinuteRetention(t *testing.T) {
	acceptance := strings.TrimSpace(os.Getenv("ACCEPTANCE_ID")) == a62AcceptanceID
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		if acceptance {
			t.Fatal("A62 requires an isolated MySQL database")
		}
		t.Skip("TEST_DATABASE_DSN is not configured")
	}
	if os.Getenv("A62_ISOLATED_MYSQL") != "true" {
		if acceptance {
			t.Fatal("A62_ISOLATED_MYSQL=true is required")
		}
		t.Skip("A62 requires A62_ISOLATED_MYSQL=true")
	}
	evidenceDirectory := strings.TrimSpace(os.Getenv("ACCEPTANCE_EVIDENCE_DIR"))
	if acceptance {
		assertA62EvidenceDirectory(t, evidenceDirectory)
	}
	fixture, fixturePath, fixtureSHA := loadA62Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	database, err := model.Open(ctx, model.Options{DSN: dsn, MaxIdle: 2, MaxOpen: 12, MaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open isolated A62 MySQL: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := model.NewMigrationRunner(database.SQL).Run(ctx); err != nil {
		t.Fatalf("run A62 migrations: %v", err)
	}
	seeder := model.NewSeeder(database.SQL)
	seeder.Now = func() time.Time { return time.Unix(fixture.Clock.NowUnix, 0) }
	if err := seeder.Run(ctx); err != nil {
		t.Fatalf("run A62 seeds: %v", err)
	}
	clock := testsupport.NewFakeClock(time.Unix(fixture.Clock.NowUnix, 0))
	cleaner, err := service.NewResourceRetentionService(service.ResourceRetentionServiceOptions{
		Repository: model.NewResourceRetentionRepository(database.GORM), Clock: clock,
		BatchSize: a62BatchSize, MaximumBatches: 100,
	})
	if err != nil {
		t.Fatalf("create A62 retention service: %v", err)
	}
	if _, err := cleaner.Clean(ctx, 0); !errors.Is(err, service.ErrResourceRetentionInvalid) {
		t.Fatalf("invalid retention error = %v", err)
	}
	state := seedA62ResourceData(t, database.GORM, fixture.Clock.NowUnix, fixture.Retention.ResourceMinutelyDays)
	initialInstance := countA62Rows(t, database.GORM, model.ResourceMinuteTableInstance)
	initialSite := countA62Rows(t, database.GORM, model.ResourceMinuteTableSite)
	if initialInstance <= 5000 || initialSite != initialInstance {
		t.Fatalf("A62 initial minute rows instance=%d site=%d", initialInstance, initialSite)
	}
	protectedHash := hashA62ProtectedAggregates(t, database.GORM, state.protectedSiteIDs)
	assertA62BusinessFacts(t, database.GORM, state.sites[0].ID)

	first, err := cleaner.Clean(ctx, fixture.Retention.ResourceMinutelyDays)
	if err != nil || first.Complete || first.Cutoff != state.cutoff {
		t.Fatalf("first A62 cleanup report=%#v err=%v", first, err)
	}
	wantDeleted := int64(a62BulkPerSite*2 + 1)
	assertA62FirstTableReport(t, "instance", first.Instance, wantDeleted)
	assertA62FirstTableReport(t, "site", first.Site, wantDeleted)
	rowsAfterFirst := countA62Rows(t, database.GORM, model.ResourceMinuteTableInstance)
	if rowsAfterFirst != 4 || countA62Rows(t, database.GORM, model.ResourceMinuteTableSite) != rowsAfterFirst {
		t.Fatalf("A62 rows after first cleanup = %d", rowsAfterFirst)
	}
	assertA62MinutePresence(t, database.GORM, state, true)
	if after := hashA62ProtectedAggregates(t, database.GORM, state.protectedSiteIDs); after != protectedHash {
		t.Fatalf("protected aggregate hash changed: before=%s after=%s", protectedHash, after)
	}
	assertA62BusinessFacts(t, database.GORM, state.sites[0].ID)

	finalizeA62BlockedAggregates(t, database.GORM, state, fixture.Clock.NowUnix)
	finalizedHash := hashA62AllAggregates(t, database.GORM)
	second, err := cleaner.Clean(ctx, fixture.Retention.ResourceMinutelyDays)
	if err != nil || !second.Complete || second.Instance.Deleted != 2 || second.Site.Deleted != 2 ||
		second.Instance.SkippedUnfinalized != 0 || second.Site.SkippedUnfinalized != 0 {
		t.Fatalf("second A62 cleanup report=%#v err=%v", second, err)
	}
	rowsAfterFinal := countA62Rows(t, database.GORM, model.ResourceMinuteTableInstance)
	if rowsAfterFinal != 2 || countA62Rows(t, database.GORM, model.ResourceMinuteTableSite) != rowsAfterFinal {
		t.Fatalf("A62 rows after final cleanup = %d", rowsAfterFinal)
	}
	assertA62MinutePresence(t, database.GORM, state, false)
	if after := hashA62AllAggregates(t, database.GORM); after != finalizedHash {
		t.Fatalf("aggregate hash changed during resumed cleanup: before=%s after=%s", finalizedHash, after)
	}
	assertA62BusinessFacts(t, database.GORM, state.sites[0].ID)
	idempotent, err := cleaner.Clean(ctx, fixture.Retention.ResourceMinutelyDays)
	if err != nil || !idempotent.Complete || idempotent.Instance.Deleted != 0 || idempotent.Site.Deleted != 0 {
		t.Fatalf("idempotent A62 cleanup report=%#v err=%v", idempotent, err)
	}
	starvationProof := proveA62NoStarvationAcrossRestart(t, database.GORM, clock, fixture.Clock.NowUnix,
		fixture.Retention.ResourceMinutelyDays)
	report := a62Report{
		SchemaVersion: 1, AcceptanceID: a62AcceptanceID, Status: "passed",
		FixturePath: fixturePath, FixtureSHA256: fixtureSHA, FixedNowUnix: fixture.Clock.NowUnix,
		RetentionDays: fixture.Retention.ResourceMinutelyDays, Cutoff: state.cutoff, BatchSize: a62BatchSize,
		InitialRowsPerTable: initialInstance, FirstRun: first, SecondRun: second, IdempotentRun: idempotent,
		RowsAfterFirstRun: rowsAfterFirst, RowsAfterFinalRun: rowsAfterFinal,
		ProtectedAggregateSHA256: protectedHash, BusinessFactsPreserved: true,
		ExactBoundaryPreserved: true, MissingHourlyBlocked: first.Instance.SkippedMissingHourly > 0 && first.Site.SkippedMissingHourly > 0,
		DailyNotFinalBlocked:     first.Instance.SkippedDailyNotFinal > 0 && first.Site.SkippedDailyNotFinal > 0,
		InvalidRetentionRejected: true, HourlyDailyValuesUnchanged: true,
		StarvationProof: starvationProof,
	}
	if acceptance {
		writeA62Report(t, evidenceDirectory, report)
	}
}

func assertA62FirstTableReport(t *testing.T, name string, report service.ResourceRetentionTableResult, wantDeleted int64) {
	t.Helper()
	if report.Deleted != wantDeleted || report.Scanned != int(wantDeleted)+2 || report.SkippedUnfinalized != 2 ||
		report.SkippedMissingHourly != 1 || report.SkippedDailyNotFinal != 1 || report.Batches < 2 || report.Complete ||
		!report.PendingRows || report.BlockedDiagnosticsTruncated {
		t.Fatalf("first %s cleanup report = %#v", name, report)
	}
}

func proveA62NoStarvationAcrossRestart(
	t *testing.T,
	db *gorm.DB,
	clock *testsupport.FakeClock,
	now int64,
	retentionDays int,
) a62StarvationProof {
	t.Helper()
	cutoff := now - now%60 - int64(retentionDays)*24*60*60
	sites := []model.Site{
		{
			Name: "A62 blocked prefix", BaseURL: "https://a62-blocked-prefix.example.test", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now,
		},
		{
			Name: "A62 eligible suffix", BaseURL: "https://a62-eligible-suffix.example.test", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now,
		},
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create A62 starvation sites: %v", err)
	}
	nodes := []string{"blocked-prefix-node", "eligible-suffix-node"}
	instances := []model.SiteInstance{
		{SiteID: sites[0].ID, NodeName: nodes[0], Hostname: "a62-starvation-host", CurrentStatus: "online",
			FirstSeenAt: cutoff - 60*24*60*60, LastSyncedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: sites[1].ID, NodeName: nodes[1], Hostname: "a62-starvation-host", CurrentStatus: "online",
			FirstSeenAt: cutoff - 60*24*60*60, LastSyncedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&instances).Error; err != nil {
		t.Fatalf("create A62 starvation instances: %v", err)
	}
	instanceMinutes := make([]model.SiteInstanceStatusMinutely, 0, a62StarvationBlockedRows+1)
	siteMinutes := make([]model.SiteStatusMinutely, 0, a62StarvationBlockedRows+1)
	blockedStart := cutoff - 50*24*60*60
	for offset := 0; offset < a62StarvationBlockedRows; offset++ {
		appendA62Minute(&instanceMinutes, &siteMinutes, sites[0].ID, nodes[0], blockedStart+int64(offset)*60, now)
	}
	eligibleMinute := cutoff - 45*24*60*60
	appendA62Minute(&instanceMinutes, &siteMinutes, sites[1].ID, nodes[1], eligibleMinute, now)
	if err := db.CreateInBatches(&instanceMinutes, 100).Error; err != nil {
		t.Fatalf("create A62 starvation instance minutes: %v", err)
	}
	if err := db.CreateInBatches(&siteMinutes, 100).Error; err != nil {
		t.Fatalf("create A62 starvation site minutes: %v", err)
	}
	readySiteHours := make(map[resourceSiteHourKey]a62SiteStatusHourly)
	readySiteDays := make(map[resourceSiteDayKey]a62SiteStatusDaily)
	readyInstanceHours := make(map[resourceInstanceHourKey]a62InstanceStatusHourly)
	readyInstanceDays := make(map[resourceInstanceDayKey]a62InstanceStatusDaily)
	ensureA62Aggregates(readySiteHours, readySiteDays, readyInstanceHours, readyInstanceDays,
		sites[1].ID, nodes[1], eligibleMinute, "complete", true, now)
	createA62AggregateMaps(t, db, readySiteHours, readySiteDays, readyInstanceHours, readyInstanceDays)

	newCleaner := func() *service.ResourceRetentionService {
		cleaner, err := service.NewResourceRetentionService(service.ResourceRetentionServiceOptions{
			Repository: model.NewResourceRetentionRepository(db), Clock: clock,
			BatchSize: a62StarvationBatchSize, MaximumBatches: a62StarvationMaximumBatches,
		})
		if err != nil {
			t.Fatalf("create A62 starvation cleaner: %v", err)
		}
		return cleaner
	}
	first, err := newCleaner().Clean(context.Background(), retentionDays)
	if err != nil || first.Complete || first.Instance.Deleted != 1 || first.Site.Deleted != 1 ||
		!first.Instance.PendingRows || !first.Site.PendingRows ||
		!first.Instance.BlockedDiagnosticsTruncated || !first.Site.BlockedDiagnosticsTruncated {
		t.Fatalf("A62 starvation first report=%#v err=%v", first, err)
	}
	assertA62TimestampCount(t, db, model.ResourceMinuteTableInstance, eligibleMinute, 0)
	assertA62TimestampCount(t, db, model.ResourceMinuteTableSite, eligibleMinute, 0)

	blockedSiteHours := make(map[resourceSiteHourKey]a62SiteStatusHourly)
	blockedSiteDays := make(map[resourceSiteDayKey]a62SiteStatusDaily)
	blockedInstanceHours := make(map[resourceInstanceHourKey]a62InstanceStatusHourly)
	blockedInstanceDays := make(map[resourceInstanceDayKey]a62InstanceStatusDaily)
	for offset := 0; offset < a62StarvationBlockedRows; offset++ {
		ensureA62Aggregates(blockedSiteHours, blockedSiteDays, blockedInstanceHours, blockedInstanceDays,
			sites[0].ID, nodes[0], blockedStart+int64(offset)*60, "partial", true, now)
	}
	createA62AggregateMaps(t, db, blockedSiteHours, blockedSiteDays, blockedInstanceHours, blockedInstanceDays)
	aggregateHash := hashA62Aggregates(t, db, []int64{sites[0].ID, sites[1].ID})
	restart, err := newCleaner().Clean(context.Background(), retentionDays)
	wantRestartDeleted := int64(a62StarvationBatchSize * a62StarvationMaximumBatches)
	if err != nil || restart.Complete || restart.Instance.Deleted != wantRestartDeleted ||
		restart.Site.Deleted != wantRestartDeleted || !restart.Instance.PendingRows || !restart.Site.PendingRows {
		t.Fatalf("A62 starvation restart report=%#v err=%v", restart, err)
	}
	final, err := newCleaner().Clean(context.Background(), retentionDays)
	if err != nil || !final.Complete || final.Instance.Deleted != 1 || final.Site.Deleted != 1 ||
		final.Instance.PendingRows || final.Site.PendingRows {
		t.Fatalf("A62 starvation final report=%#v err=%v", final, err)
	}
	if countA62SiteRows(t, db, model.ResourceMinuteTableInstance, sites[0].ID)+
		countA62SiteRows(t, db, model.ResourceMinuteTableInstance, sites[1].ID) != 0 ||
		countA62SiteRows(t, db, model.ResourceMinuteTableSite, sites[0].ID)+
			countA62SiteRows(t, db, model.ResourceMinuteTableSite, sites[1].ID) != 0 {
		t.Fatal("A62 starvation rows remain after restart continuation")
	}
	if after := hashA62Aggregates(t, db, []int64{sites[0].ID, sites[1].ID}); after != aggregateHash {
		t.Fatalf("A62 starvation aggregate hash changed: before=%s after=%s", aggregateHash, after)
	}
	return a62StarvationProof{
		BlockedPrefixRows: a62StarvationBlockedRows, BatchSize: a62StarvationBatchSize,
		MaximumBatches: a62StarvationMaximumBatches, FirstRun: first, RestartRun: restart, FinalRun: final,
		EligibleDeletedBehindBlockedPrefix: true, RestartContinuationProved: true,
	}
}

func loadA62Fixture(t *testing.T) (a62Fixture, string, string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "design", "f05-ops-capacity.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read A62 fixture: %v", err)
	}
	var fixture a62Fixture
	if err := yaml.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode A62 fixture: %v", err)
	}
	if fixture.SchemaVersion != 2 || fixture.FixtureID != "F05" || fixture.Clock.NowUnix <= 0 ||
		fixture.Retention.ResourceMinutelyDays != 90 {
		t.Fatalf("unexpected A62 fixture contract: %#v", fixture)
	}
	digest := sha256.Sum256(payload)
	return fixture, "testdata/design/f05-ops-capacity.yaml", hex.EncodeToString(digest[:])
}

func seedA62ResourceData(t *testing.T, db *gorm.DB, now int64, retentionDays int) a62SeedState {
	t.Helper()
	cutoff := now - now%60 - int64(retentionDays)*24*60*60
	sites := make([]model.Site, 5)
	for index := range sites {
		sites[index] = model.Site{
			Name: fmt.Sprintf("A62 Site %d", index+1), BaseURL: fmt.Sprintf("https://a62-%d.example.test", index+1),
			ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, DataExportEnabled: true, CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := db.Create(&sites).Error; err != nil {
		t.Fatalf("create A62 sites: %v", err)
	}
	nodes := []string{"complete-node", "partial-node", "missing-node", "missing-hour-node", "daily-not-final-node"}
	instances := make([]model.SiteInstance, len(sites))
	for index := range sites {
		instances[index] = model.SiteInstance{
			SiteID: sites[index].ID, NodeName: nodes[index], Hostname: "shared-a62-host",
			CurrentStatus: "online", FirstSeenAt: cutoff - 40*24*60*60, LastSyncedAt: now,
			CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := db.Create(&instances).Error; err != nil {
		t.Fatalf("create A62 instances: %v", err)
	}
	state := a62SeedState{
		sites: sites, nodes: nodes, cutoffMinusMinute: cutoff - 60, cutoff: cutoff, futureMinute: cutoff + 60,
		missingHourlyMinute: cutoff - 25*24*60*60,
		dailyNotFinalMinute: cutoff - 26*24*60*60,
		protectedSiteIDs:    []int64{sites[0].ID, sites[1].ID, sites[2].ID},
	}
	instanceMinutes := make([]model.SiteInstanceStatusMinutely, 0, a62BulkPerSite*2+5)
	siteMinutes := make([]model.SiteStatusMinutely, 0, a62BulkPerSite*2+5)
	hoursSite := make(map[resourceSiteHourKey]a62SiteStatusHourly)
	daysSite := make(map[resourceSiteDayKey]a62SiteStatusDaily)
	hoursInstance := make(map[resourceInstanceHourKey]a62InstanceStatusHourly)
	daysInstance := make(map[resourceInstanceDayKey]a62InstanceStatusDaily)
	bulkStart := cutoff - 15*24*60*60
	for siteIndex := 0; siteIndex < 2; siteIndex++ {
		profile := "complete"
		if siteIndex == 1 {
			profile = "partial"
		}
		for offset := 0; offset < a62BulkPerSite; offset++ {
			minute := bulkStart + int64(offset)*60
			appendA62Minute(&instanceMinutes, &siteMinutes, sites[siteIndex].ID, nodes[siteIndex], minute, now)
			ensureA62Aggregates(hoursSite, daysSite, hoursInstance, daysInstance,
				sites[siteIndex].ID, nodes[siteIndex], minute, profile, true, now)
		}
	}
	for _, minute := range []int64{state.cutoffMinusMinute, state.cutoff, state.futureMinute} {
		appendA62Minute(&instanceMinutes, &siteMinutes, sites[0].ID, nodes[0], minute, now)
		ensureA62Aggregates(hoursSite, daysSite, hoursInstance, daysInstance,
			sites[0].ID, nodes[0], minute, "complete", true, now)
	}
	appendA62Minute(&instanceMinutes, &siteMinutes, sites[3].ID, nodes[3], state.missingHourlyMinute, now)
	ensureA62Aggregates(hoursSite, daysSite, hoursInstance, daysInstance,
		sites[3].ID, nodes[3], state.missingHourlyMinute, "partial", true, now)
	delete(hoursSite, resourceSiteHourKey{SiteID: sites[3].ID, HourTS: floorA62Hour(state.missingHourlyMinute)})
	delete(hoursInstance, resourceInstanceHourKey{SiteID: sites[3].ID, NodeName: nodes[3], HourTS: floorA62Hour(state.missingHourlyMinute)})
	appendA62Minute(&instanceMinutes, &siteMinutes, sites[4].ID, nodes[4], state.dailyNotFinalMinute, now)
	ensureA62Aggregates(hoursSite, daysSite, hoursInstance, daysInstance,
		sites[4].ID, nodes[4], state.dailyNotFinalMinute, "partial", false, now)
	missingMinute := cutoff - 5*24*60*60
	ensureA62Aggregates(hoursSite, daysSite, hoursInstance, daysInstance,
		sites[2].ID, nodes[2], missingMinute, "missing", true, now)
	if err := db.CreateInBatches(&instanceMinutes, 500).Error; err != nil {
		t.Fatalf("create A62 instance minutes: %v", err)
	}
	if err := db.CreateInBatches(&siteMinutes, 500).Error; err != nil {
		t.Fatalf("create A62 site minutes: %v", err)
	}
	createA62AggregateMaps(t, db, hoursSite, daysSite, hoursInstance, daysInstance)
	if err := db.Create(&model.UsageFactHourly{
		SiteID: sites[0].ID, RemoteUserID: 1, UsernameSnapshot: "a62-user", ModelName: "a62-model",
		ChannelID: 1, HourTS: floorA62Hour(bulkStart), RequestCount: 7, Quota: 70, TokenUsed: 700, CollectedAt: now,
	}).Error; err != nil {
		t.Fatalf("create A62 hourly business fact: %v", err)
	}
	if err := db.Create(&model.UsageFactDaily{
		SiteID: sites[0].ID, RemoteUserID: 1, UsernameSnapshot: "a62-user", ModelName: "a62-model",
		ChannelID: 1, DateKey: a62DateKey(bulkStart), RequestCount: 7, Quota: 70, TokenUsed: 700,
		IsFinal: true, LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create A62 daily business fact: %v", err)
	}
	return state
}

type resourceSiteHourKey struct{ SiteID, HourTS int64 }
type resourceSiteDayKey struct {
	SiteID  int64
	DateKey int
}
type resourceInstanceHourKey struct {
	SiteID   int64
	NodeName string
	HourTS   int64
}
type resourceInstanceDayKey struct {
	SiteID   int64
	NodeName string
	DateKey  int
}

func appendA62Minute(
	instances *[]model.SiteInstanceStatusMinutely,
	sites *[]model.SiteStatusMinutely,
	siteID int64,
	node string,
	minute int64,
	now int64,
) {
	cpu, memory, disk := 50.0, 60.0, 70.0
	*instances = append(*instances, model.SiteInstanceStatusMinutely{
		SiteID: siteID, NodeName: node, MinuteTS: minute, Status: "online",
		CPUPercent: &cpu, MemoryPercent: &memory, DiskUsedPercent: &disk, CreatedAt: now,
	})
	*sites = append(*sites, model.SiteStatusMinutely{
		SiteID: siteID, MinuteTS: minute, InstanceCount: 1, OnlineInstanceCount: 1,
		CPUMaxPercent: &cpu, CPUAvgPercent: &cpu, MemoryMaxPercent: &memory,
		MemoryAvgPercent: &memory, DiskMaxUsedPercent: &disk, HealthStatus: "ok", CreatedAt: now,
	})
}

func ensureA62Aggregates(
	siteHours map[resourceSiteHourKey]a62SiteStatusHourly,
	siteDays map[resourceSiteDayKey]a62SiteStatusDaily,
	instanceHours map[resourceInstanceHourKey]a62InstanceStatusHourly,
	instanceDays map[resourceInstanceDayKey]a62InstanceStatusDaily,
	siteID int64,
	node string,
	minute int64,
	status string,
	final bool,
	now int64,
) {
	hour, dateKey := floorA62Hour(minute), a62DateKey(minute)
	sample, expected, abnormal := 60, 60, 0
	health := "ok"
	if status == "partial" {
		sample, abnormal, health = 30, 10, "warning"
	}
	if status == "missing" {
		sample, abnormal, health = 0, 0, "unavailable"
	}
	cpuMax, cpuAvg, memoryMax, memoryAvg, diskMax, diskLast := 95.0, 50.0, 96.0, 60.0, 97.0, 70.0
	if sample == 0 {
		cpuMax, cpuAvg, memoryMax, memoryAvg, diskMax, diskLast = 0, 0, 0, 0, 0, 0
	}
	pointer := func(value float64) *float64 {
		if sample == 0 {
			return nil
		}
		copy := value
		return &copy
	}
	siteHours[resourceSiteHourKey{SiteID: siteID, HourTS: hour}] = a62SiteStatusHourly{
		SiteID: siteID, HourTS: hour, InstanceCountMax: 1, OnlineInstanceCountMin: 1,
		CPUMaxPercent: pointer(cpuMax), CPUAvgPercent: pointer(cpuAvg), MemoryMaxPercent: pointer(memoryMax),
		MemoryAvgPercent: pointer(memoryAvg), DiskMaxUsedPercent: pointer(diskMax), AbnormalSamples: abnormal,
		SampleCount: sample, ExpectedSampleCount: expected, DataStatus: status, HealthStatus: health, LastCalculatedAt: now,
	}
	siteDays[resourceSiteDayKey{SiteID: siteID, DateKey: dateKey}] = a62SiteStatusDaily{
		SiteID: siteID, DateKey: dateKey, InstanceCountMax: 1, OnlineInstanceCountMin: 1,
		CPUMaxPercent: pointer(cpuMax), CPUAvgPercent: pointer(cpuAvg), MemoryMaxPercent: pointer(memoryMax),
		MemoryAvgPercent: pointer(memoryAvg), DiskMaxUsedPercent: pointer(diskMax), AbnormalSamples: abnormal,
		SampleCount: sample, ExpectedSampleCount: expected, DataStatus: status, HealthStatus: health,
		IsFinal: final, LastCalculatedAt: now,
	}
	instanceHours[resourceInstanceHourKey{SiteID: siteID, NodeName: node, HourTS: hour}] = a62InstanceStatusHourly{
		SiteID: siteID, NodeName: node, HourTS: hour, CPUMaxPercent: pointer(cpuMax), CPUAvgPercent: pointer(cpuAvg),
		MemoryMaxPercent: pointer(memoryMax), MemoryAvgPercent: pointer(memoryAvg),
		DiskMaxUsedPercent: pointer(diskMax), DiskLastUsedPercent: pointer(diskLast),
		OnlineSamples: sample - abnormal, AbnormalSamples: abnormal, SampleCount: sample,
		ExpectedSampleCount: expected, DataStatus: status, LastCalculatedAt: now,
	}
	instanceDays[resourceInstanceDayKey{SiteID: siteID, NodeName: node, DateKey: dateKey}] = a62InstanceStatusDaily{
		SiteID: siteID, NodeName: node, DateKey: dateKey, CPUMaxPercent: pointer(cpuMax), CPUAvgPercent: pointer(cpuAvg),
		MemoryMaxPercent: pointer(memoryMax), MemoryAvgPercent: pointer(memoryAvg),
		DiskMaxUsedPercent: pointer(diskMax), DiskLastUsedPercent: pointer(diskLast),
		OnlineSamples: sample - abnormal, AbnormalSamples: abnormal, SampleCount: sample,
		ExpectedSampleCount: expected, DataStatus: status, IsFinal: final, LastCalculatedAt: now,
	}
}

func createA62AggregateMaps(
	t *testing.T,
	db *gorm.DB,
	siteHours map[resourceSiteHourKey]a62SiteStatusHourly,
	siteDays map[resourceSiteDayKey]a62SiteStatusDaily,
	instanceHours map[resourceInstanceHourKey]a62InstanceStatusHourly,
	instanceDays map[resourceInstanceDayKey]a62InstanceStatusDaily,
) {
	t.Helper()
	toSlice := func(values any) any { return values }
	sh := make([]a62SiteStatusHourly, 0, len(siteHours))
	for _, row := range siteHours {
		sh = append(sh, row)
	}
	sd := make([]a62SiteStatusDaily, 0, len(siteDays))
	for _, row := range siteDays {
		sd = append(sd, row)
	}
	ih := make([]a62InstanceStatusHourly, 0, len(instanceHours))
	for _, row := range instanceHours {
		ih = append(ih, row)
	}
	id := make([]a62InstanceStatusDaily, 0, len(instanceDays))
	for _, row := range instanceDays {
		id = append(id, row)
	}
	for name, rows := range map[string]any{
		"site hours": toSlice(sh), "site days": toSlice(sd), "instance hours": toSlice(ih), "instance days": toSlice(id),
	} {
		if err := db.CreateInBatches(rows, 500).Error; err != nil {
			t.Fatalf("create A62 %s: %v", name, err)
		}
	}
}

func finalizeA62BlockedAggregates(t *testing.T, db *gorm.DB, state a62SeedState, now int64) {
	t.Helper()
	missingHourSite := make(map[resourceSiteHourKey]a62SiteStatusHourly)
	missingHourDay := make(map[resourceSiteDayKey]a62SiteStatusDaily)
	missingHourInstance := make(map[resourceInstanceHourKey]a62InstanceStatusHourly)
	missingHourInstanceDay := make(map[resourceInstanceDayKey]a62InstanceStatusDaily)
	ensureA62Aggregates(missingHourSite, missingHourDay, missingHourInstance, missingHourInstanceDay,
		state.sites[3].ID, state.nodes[3], state.missingHourlyMinute, "partial", true, now)
	for _, row := range missingHourSite {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create missing A62 site hour: %v", err)
		}
	}
	for _, row := range missingHourInstance {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create missing A62 instance hour: %v", err)
		}
	}
	dateKey := a62DateKey(state.dailyNotFinalMinute)
	if result := db.Model(&a62SiteStatusDaily{}).
		Where("site_id = ? AND date_key = ?", state.sites[4].ID, dateKey).Update("is_final", true); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("finalize A62 site day rows=%d err=%v", result.RowsAffected, result.Error)
	}
	if result := db.Model(&a62InstanceStatusDaily{}).
		Where("site_id = ? AND node_name = ? AND date_key = ?", state.sites[4].ID, state.nodes[4], dateKey).
		Update("is_final", true); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("finalize A62 instance day rows=%d err=%v", result.RowsAffected, result.Error)
	}
}

func assertA62MinutePresence(t *testing.T, db *gorm.DB, state a62SeedState, blockersExpected bool) {
	t.Helper()
	for _, table := range []string{model.ResourceMinuteTableInstance, model.ResourceMinuteTableSite} {
		assertA62TimestampCount(t, db, table, state.cutoffMinusMinute, 0)
		assertA62TimestampCount(t, db, table, state.cutoff, 1)
		assertA62TimestampCount(t, db, table, state.futureMinute, 1)
		wantBlocker := int64(0)
		if blockersExpected {
			wantBlocker = 1
		}
		assertA62TimestampCount(t, db, table, state.missingHourlyMinute, wantBlocker)
		assertA62TimestampCount(t, db, table, state.dailyNotFinalMinute, wantBlocker)
	}
}

func assertA62TimestampCount(t *testing.T, db *gorm.DB, table string, minute, want int64) {
	t.Helper()
	var count int64
	if err := db.Table(table).Where("minute_ts = ?", minute).Count(&count).Error; err != nil || count != want {
		t.Fatalf("%s minute %d count=%d want=%d err=%v", table, minute, count, want, err)
	}
}

func countA62Rows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatalf("count A62 %s: %v", table, err)
	}
	return count
}

func countA62SiteRows(t *testing.T, db *gorm.DB, table string, siteID int64) int64 {
	t.Helper()
	var count int64
	if err := db.Table(table).Where("site_id = ?", siteID).Count(&count).Error; err != nil {
		t.Fatalf("count A62 %s site %d: %v", table, siteID, err)
	}
	return count
}

func assertA62BusinessFacts(t *testing.T, db *gorm.DB, siteID int64) {
	t.Helper()
	var hourly model.UsageFactHourly
	if err := db.Where("site_id = ?", siteID).Take(&hourly).Error; err != nil || hourly.RequestCount != 7 || hourly.Quota != 70 || hourly.TokenUsed != 700 {
		t.Fatalf("A62 hourly business fact=%#v err=%v", hourly, err)
	}
	var daily model.UsageFactDaily
	if err := db.Where("site_id = ?", siteID).Take(&daily).Error; err != nil || daily.RequestCount != 7 || daily.Quota != 70 || daily.TokenUsed != 700 || !daily.IsFinal {
		t.Fatalf("A62 daily business fact=%#v err=%v", daily, err)
	}
}

func hashA62ProtectedAggregates(t *testing.T, db *gorm.DB, siteIDs []int64) string {
	t.Helper()
	return hashA62Aggregates(t, db, siteIDs)
}

func hashA62AllAggregates(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var ids []int64
	if err := db.Model(&model.Site{}).Order("id ASC").Pluck("id", &ids).Error; err != nil {
		t.Fatalf("load A62 aggregate site IDs: %v", err)
	}
	return hashA62Aggregates(t, db, ids)
}

func hashA62Aggregates(t *testing.T, db *gorm.DB, siteIDs []int64) string {
	t.Helper()
	queries := []string{
		`SELECT CONCAT_WS('#', site_id, hour_ts, instance_count_max, online_instance_count_min,
COALESCE(CAST(cpu_max_percent AS CHAR),'NULL'), COALESCE(CAST(cpu_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(memory_max_percent AS CHAR),'NULL'), COALESCE(CAST(memory_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(disk_max_used_percent AS CHAR),'NULL'), abnormal_samples, sample_count, expected_sample_count,
data_status, health_status, last_calculated_at) AS row_value
FROM site_status_hourly WHERE site_id IN ? ORDER BY site_id, hour_ts`,
		`SELECT CONCAT_WS('#', site_id, date_key, instance_count_max, online_instance_count_min,
COALESCE(CAST(cpu_max_percent AS CHAR),'NULL'), COALESCE(CAST(cpu_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(memory_max_percent AS CHAR),'NULL'), COALESCE(CAST(memory_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(disk_max_used_percent AS CHAR),'NULL'), abnormal_samples, sample_count, expected_sample_count,
data_status, health_status, is_final, last_calculated_at) AS row_value
FROM site_status_daily WHERE site_id IN ? ORDER BY site_id, date_key`,
		`SELECT CONCAT_WS('#', site_id, node_name, hour_ts,
COALESCE(CAST(cpu_max_percent AS CHAR),'NULL'), COALESCE(CAST(cpu_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(memory_max_percent AS CHAR),'NULL'), COALESCE(CAST(memory_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(disk_max_used_percent AS CHAR),'NULL'), COALESCE(CAST(disk_last_used_percent AS CHAR),'NULL'),
online_samples, abnormal_samples, sample_count, expected_sample_count, data_status, last_calculated_at) AS row_value
FROM site_instance_status_hourly WHERE site_id IN ? ORDER BY site_id, node_name, hour_ts`,
		`SELECT CONCAT_WS('#', site_id, node_name, date_key,
COALESCE(CAST(cpu_max_percent AS CHAR),'NULL'), COALESCE(CAST(cpu_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(memory_max_percent AS CHAR),'NULL'), COALESCE(CAST(memory_avg_percent AS CHAR),'NULL'),
COALESCE(CAST(disk_max_used_percent AS CHAR),'NULL'), COALESCE(CAST(disk_last_used_percent AS CHAR),'NULL'),
online_samples, abnormal_samples, sample_count, expected_sample_count, data_status, is_final, last_calculated_at) AS row_value
FROM site_instance_status_daily WHERE site_id IN ? ORDER BY site_id, node_name, date_key`,
	}
	rows := make([]string, 0)
	for index, query := range queries {
		var values []string
		if err := db.Raw(query, siteIDs).Scan(&values).Error; err != nil {
			t.Fatalf("hash A62 aggregate query %d: %v", index, err)
		}
		rows = append(rows, fmt.Sprintf("table:%d", index))
		rows = append(rows, values...)
	}
	digest := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return hex.EncodeToString(digest[:])
}

func floorA62Hour(value int64) int64 { return value - value%3600 }

func a62DateKey(value int64) int {
	local := time.Unix(value, 0).In(a62Location)
	return local.Year()*10000 + int(local.Month())*100 + local.Day()
}

func assertA62EvidenceDirectory(t *testing.T, directory string) {
	t.Helper()
	if directory == "" || !filepath.IsAbs(directory) {
		t.Fatal("A62 evidence directory must be an existing absolute path")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		t.Fatalf("A62 evidence directory is invalid: %v", err)
	}
}

func writeA62Report(t *testing.T, directory string, report a62Report) {
	t.Helper()
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal A62 report: %v", err)
	}
	payload = append(payload, '\n')
	temporary := filepath.Join(directory, "a62-report.json.tmp")
	final := filepath.Join(directory, "a62-report.json")
	if err := os.WriteFile(temporary, payload, 0o640); err != nil {
		t.Fatalf("write A62 report: %v", err)
	}
	if err := os.Rename(temporary, final); err != nil {
		t.Fatalf("publish A62 report: %v", err)
	}
}

func sortedA62Keys[K ~int64 | ~int](values map[K]struct{}) []K {
	result := make([]K, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
