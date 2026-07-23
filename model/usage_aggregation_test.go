package model

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"new-api-pilot/constant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestUsageAggregationRebuildsSixLevelsAndRollingDaily(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 7, 13, 12, 5, 0, 0, location).Unix()
	start := now - now%3600 - 2*3600
	fixture, accounts := createUsageAggregationFixture(t, database, start, 2, now, "six-level")

	hour0Facts := []UsageFactInput{
		{RemoteUserID: 1, UsernameSnapshot: "root", ModelName: "Model-A", ChannelID: 1, RequestCount: 1, Quota: 10, TokenUsed: 100},
		{RemoteUserID: 1, UsernameSnapshot: "root", ModelName: "Model-B", ChannelID: 2, RequestCount: 2, Quota: 20, TokenUsed: 200},
		{RemoteUserID: 2, UsernameSnapshot: "second", ModelName: "Model-A", ChannelID: 1, RequestCount: 3, Quota: 30, TokenUsed: 300},
		{RemoteUserID: 3, UsernameSnapshot: "third", ModelName: "Model-A", ChannelID: 1, RequestCount: 4, Quota: 40, TokenUsed: 400},
	}
	hour0 := applyCompleteUsageAggregation(t, database, fixture, 0, now+1, hour0Facts)
	if hour0.HourlyRows != 9 || hour0.DailyRows == 0 {
		t.Fatalf("hour zero aggregation result = %#v", hour0)
	}
	assertUsageAggregationMetric(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", []any{fixture.site.ID, fixture.hours[0]}, 10, 100, 1000, 3)
	assertUsageAggregationMetric(t, database.GORM, &CustomerStatHourly{},
		"customer_id = ? AND site_id = ? AND hour_ts = ?",
		[]any{accounts[0].CustomerID, fixture.site.ID, fixture.hours[0]}, 6, 60, 600, 2)
	assertUsageAggregationMetric(t, database.GORM, &AccountStatHourly{},
		"account_id = ? AND hour_ts = ?", []any{accounts[0].ID, fixture.hours[0]}, 3, 30, 300, -1)
	assertUsageAggregationMetric(t, database.GORM, &ModelStatHourly{},
		"site_id = ? AND model_name = ? AND hour_ts = ?",
		[]any{fixture.site.ID, "Model-A", fixture.hours[0]}, 8, 80, 800, 3)
	assertUsageAggregationMetric(t, database.GORM, &ChannelStatHourly{},
		"site_id = ? AND channel_id = ? AND hour_ts = ?",
		[]any{fixture.site.ID, int64(1), fixture.hours[0]}, 8, 80, 800, 3)
	assertUsageAggregationMetric(t, database.GORM, &GlobalStatHourly{},
		"hour_ts = ?", []any{fixture.hours[0]}, 10, 100, 1000, 3)
	assertAggregationRowCount(t, database.GORM, &AccountStatHourly{},
		"account_id = ? AND hour_ts = ?", accounts[2].ID, fixture.hours[0], 0)

	repeated := applyCompleteUsageAggregation(t, database, fixture, 0, now+2, hour0Facts)
	if repeated.HourlyRows != hour0.HourlyRows {
		t.Fatalf("repeated aggregation rows = %#v, want %#v", repeated, hour0)
	}
	assertAggregationRowCount(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", fixture.site.ID, fixture.hours[0], 1)

	hour1Facts := []UsageFactInput{
		{RemoteUserID: 1, UsernameSnapshot: "root", ModelName: "Model-A", ChannelID: 1, RequestCount: 5, Quota: 50, TokenUsed: 500},
		{RemoteUserID: 3, UsernameSnapshot: "third", ModelName: "Model-A", ChannelID: 2, RequestCount: 6, Quota: 60, TokenUsed: 600},
	}
	applyCompleteUsageAggregation(t, database, fixture, 1, now+3, hour1Facts)
	dateKey, _, _, err := UsageDateBucket(fixture.hours[0])
	if err != nil {
		t.Fatalf("usage date bucket: %v", err)
	}
	assertUsageAggregationMetric(t, database.GORM, &SiteStatDaily{},
		"site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}, 21, 210, 2100, 3)
	assertUsageAggregationMetric(t, database.GORM, &CustomerStatDaily{},
		"customer_id = ? AND site_id = ? AND date_key = ?",
		[]any{accounts[0].CustomerID, fixture.site.ID, dateKey}, 17, 170, 1700, 3)
	assertUsageAggregationMetric(t, database.GORM, &AccountStatDaily{},
		"account_id = ? AND date_key = ?", []any{accounts[2].ID, dateKey}, 6, 60, 600, -1)
	assertUsageAggregationMetric(t, database.GORM, &ModelStatDaily{},
		"site_id = ? AND model_name = ? AND date_key = ?",
		[]any{fixture.site.ID, "Model-A", dateKey}, 19, 190, 1900, 3)
	var rolling SiteStatDaily
	if err := database.GORM.Where("site_id = ? AND date_key = ?", fixture.site.ID, dateKey).First(&rolling).Error; err != nil {
		t.Fatalf("read rolling site daily: %v", err)
	}
	if rolling.DataStatus != UsageAggregationStatusPartial || rolling.IsFinal {
		t.Fatalf("rolling site daily status = %#v", rolling)
	}

	applyCompleteUsageAggregation(t, database, fixture, 0, now+4, nil)
	assertAggregationRowCount(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", fixture.site.ID, fixture.hours[0], 0)
	assertUsageAggregationMetric(t, database.GORM, &SiteStatDaily{},
		"site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}, 11, 110, 1100, 2)
	assertUsageWindowStatus(t, database, fixture.site.ID, fixture.hours[0], CollectionWindowStatusComplete)

	failedRequest := fixture.failedRequest(1, now+5, true)
	failedRequest.ReasonCode = string(constant.MessageDataValidationMismatch)
	factFailure, err := NewFailedUsageWindowMutation(failedRequest)
	if err != nil {
		t.Fatalf("plan aggregation mismatch: %v", err)
	}
	aggregation, err := NewUsageAggregationCommit(aggregationRequest(fixture, 1, now+5, nil), factFailure)
	if err != nil {
		t.Fatalf("bind aggregation mismatch: %v", err)
	}
	applyUsageAggregationMutation(t, database, fixture, 1, aggregation)
	assertAggregationRowCount(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", fixture.site.ID, fixture.hours[1], 0)
	assertUsageAggregationMetric(t, database.GORM, &SiteStatDaily{},
		"site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}, 0, 0, 0, 0)
	assertUsageAggregationStatus(t, database.GORM, &SiteStatDaily{},
		"site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}, UsageAggregationStatusPartial, false)
	assertUsageFactCount(t, database, fixture.site.ID, fixture.hours[1], 2)
	assertUsageWindowStatus(t, database, fixture.site.ID, fixture.hours[1], CollectionWindowStatusMissing)
}

func TestUsageAggregationIncludesLegacyResidualWithoutCountingSyntheticUser(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2026, 7, 14, 12, 5, 0, 0, location).Unix()
	hour := now - now%3600 - 3600
	fixture, accounts := createUsageAggregationFixture(t, database, hour, 1, now, "legacy-unattributed")
	facts := []UsageFactInput{
		{RemoteUserID: 1, UsernameSnapshot: "root", ModelName: "Model-A", ChannelID: 1,
			RequestCount: 2, Quota: 20, TokenUsed: 200},
		{RemoteUserID: UsageLegacyUnattributedRemoteUserID,
			UsernameSnapshot: UsageLegacyUnattributedUsername, ModelName: "Model-A",
			UseGroup: UsageLegacyUnattributedGroup, RequestCount: 3, Quota: 30, TokenUsed: 300},
	}
	applyCompleteUsageAggregation(t, database, fixture, 0, now+1, facts)
	assertUsageAggregationMetric(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 5, 50, 500, 1)
	assertUsageAggregationMetric(t, database.GORM, &ModelStatHourly{},
		"site_id = ? AND model_name = ? AND hour_ts = ?",
		[]any{fixture.site.ID, "Model-A", hour}, 5, 50, 500, 1)
	assertUsageAggregationMetric(t, database.GORM, &AccountStatHourly{},
		"account_id = ? AND hour_ts = ?", []any{accounts[0].ID, hour}, 2, 20, 200, -1)
	var window CollectionWindow
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).First(&window).Error; err != nil {
		t.Fatalf("read legacy unattributed window: %v", err)
	}
	if window.AttributionStatus != UsageAttributionLegacyUnattributed {
		t.Fatalf("legacy unattributed window quality = %q", window.AttributionStatus)
	}
}

func TestUsageAggregationPersistsKnownPartialZeroButKeepsCompleteZeroSparse(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2031, 6, 7, 0, 5, 0, 0, location).Unix()
	start := now - now%3600 - 2*3600
	fixture, accounts := createUsageAggregationFixture(t, database, start, 2, now, "partial-zero")
	dateKey, _, _, err := UsageDateBucket(start)
	if err != nil {
		t.Fatalf("partial-zero date bucket: %v", err)
	}

	applyCompleteUsageAggregation(t, database, fixture, 0, now+1, nil)
	checks := []struct {
		modelValue any
		where      string
		args       []any
		active     int64
	}{
		{&SiteStatDaily{}, "site_id = ? AND date_key = ?", []any{fixture.site.ID, dateKey}, 0},
		{&CustomerStatDaily{}, "customer_id = ? AND site_id = ? AND date_key = ?",
			[]any{accounts[0].CustomerID, fixture.site.ID, dateKey}, 0},
		{&AccountStatDaily{}, "account_id = ? AND date_key = ?", []any{accounts[0].ID, dateKey}, -1},
	}
	for _, check := range checks {
		assertUsageAggregationMetric(t, database.GORM, check.modelValue, check.where, check.args, 0, 0, 0, check.active)
		assertUsageAggregationStatus(t, database.GORM, check.modelValue, check.where, check.args,
			UsageAggregationStatusPartial, false)
	}
	assertAggregationRowCount(t, database.GORM, &AccountStatDaily{},
		"account_id = ? AND date_key = ?", accounts[2].ID, dateKey, 0)

	applyCompleteUsageAggregation(t, database, fixture, 1, now+2, nil)
	assertAggregationRowCount(t, database.GORM, &SiteStatDaily{},
		"site_id = ? AND date_key = ?", fixture.site.ID, dateKey, 0)
	assertAggregationRowCount(t, database.GORM, &CustomerStatDaily{},
		"customer_id = ? AND site_id = ? AND date_key = ?", accounts[0].CustomerID, fixture.site.ID, dateKey, 0)
	for _, account := range accounts {
		assertAggregationRowCount(t, database.GORM, &AccountStatDaily{},
			"account_id = ? AND date_key = ?", account.ID, dateKey, 0)
	}
}

func TestUsageAggregationConcurrentSitesSerializeGlobalRebuild(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2027, 2, 3, 12, 5, 0, 0, location).Unix()
	hour := now - now%3600 - 3600
	first, _ := createUsageAggregationFixture(t, database, hour, 1, now, "concurrent-first")
	second, _ := createUsageAggregationFixture(t, database, hour, 1, now, "concurrent-second")
	firstFacts := []UsageFactInput{{
		RemoteUserID: 1, ModelName: "shared", ChannelID: 1, RequestCount: 10, Quota: 100, TokenUsed: 1000,
	}}
	secondFacts := []UsageFactInput{{
		RemoteUserID: 1, ModelName: "shared", ChannelID: 1, RequestCount: 20, Quota: 200, TokenUsed: 2000,
	}}
	firstMutation := buildCompleteUsageAggregation(t, first, 0, now+1, firstFacts)
	secondMutation := buildCompleteUsageAggregation(t, second, 0, now+1, secondFacts)

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for _, item := range []struct {
		fixture  usageMutationFixture
		mutation UsageAggregationCommit
	}{{first, firstMutation}, {second, secondMutation}} {
		wait.Add(1)
		go func(item struct {
			fixture  usageMutationFixture
			mutation UsageAggregationCommit
		}) {
			defer wait.Done()
			errorsChannel <- database.GORM.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
				scope, err := lockUsageAggregationTestScope(tx, item.fixture, 0)
				if err != nil {
					return err
				}
				ready <- struct{}{}
				<-release
				_, err = item.mutation.Apply(context.Background(), tx, scope)
				return err
			})
		}(item)
	}
	<-ready
	<-ready
	close(release)
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent usage aggregation: %v", err)
		}
	}
	assertUsageAggregationMetric(t, database.GORM, &GlobalStatHourly{},
		"hour_ts = ?", []any{hour}, 30, 300, 3000, 2)
	dateKey, _, _, err := UsageDateBucket(hour)
	if err != nil {
		t.Fatalf("concurrent usage date: %v", err)
	}
	assertUsageAggregationMetric(t, database.GORM, &GlobalStatDaily{},
		"date_key = ?", []any{dateKey}, 30, 300, 3000, 2)
}

func TestUsageAggregationFailureRollsBackFactsAndSummaries(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2028, 3, 4, 12, 5, 0, 0, location).Unix()
	hour := now - now%3600 - 3600
	fixture, _ := createUsageAggregationFixture(t, database, hour, 1, now, "rollback")
	initialFacts := []UsageFactInput{{
		RemoteUserID: 1, ModelName: "initial", ChannelID: 1, RequestCount: 1, Quota: 2, TokenUsed: 3,
	}}
	applyCompleteUsageAggregation(t, database, fixture, 0, now+1, initialFacts)
	replacement := []UsageFactInput{{
		RemoteUserID: 1, ModelName: "replacement", ChannelID: 2, RequestCount: 9, Quota: 8, TokenUsed: 7,
	}}
	mutation := buildCompleteUsageAggregation(t, fixture, 0, now+2, replacement)
	rollback := errors.New("force usage aggregation rollback")
	err := database.GORM.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		scope, err := lockUsageAggregationTestScope(tx, fixture, 0)
		if err != nil {
			return err
		}
		if _, err := mutation.Apply(context.Background(), tx, scope); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("forced rollback error = %v", err)
	}
	assertUsageAggregationMetric(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", []any{fixture.site.ID, hour}, 1, 2, 3, 1)
	var facts []UsageFactHourly
	if err := database.GORM.Where("site_id = ? AND hour_ts = ?", fixture.site.ID, hour).Find(&facts).Error; err != nil {
		t.Fatalf("read rollback usage facts: %v", err)
	}
	if len(facts) != 1 || facts[0].ModelName != "initial" || facts[0].ChannelID != 1 {
		t.Fatalf("rollback usage facts = %#v", facts)
	}
}

func TestUsageAggregationRejectsCrossKeyMetricOverflow(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2029, 4, 5, 12, 5, 0, 0, location).Unix()
	hour := now - now%3600 - 3600
	fixture, _ := createUsageAggregationFixture(t, database, hour, 1, now, "overflow")
	facts := []UsageFactInput{
		{RemoteUserID: 1, ModelName: "first", ChannelID: 1, RequestCount: math.MaxInt64},
		{RemoteUserID: 2, ModelName: "second", ChannelID: 2, RequestCount: 1},
	}
	mutation := buildCompleteUsageAggregation(t, fixture, 0, now+1, facts)
	var applyErr error
	_ = database.GORM.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		scope, err := lockUsageAggregationTestScope(tx, fixture, 0)
		if err != nil {
			return err
		}
		_, applyErr = mutation.Apply(context.Background(), tx, scope)
		return applyErr
	})
	if !errors.Is(applyErr, ErrCollectionRunContract) {
		t.Fatalf("usage aggregation overflow error = %v", applyErr)
	}
	assertUsageFactCount(t, database, fixture.site.ID, hour, 0)
	assertAggregationRowCount(t, database.GORM, &SiteStatHourly{},
		"site_id = ? AND hour_ts = ?", fixture.site.ID, hour, 0)
	if _, err := FindUsageCursor(context.Background(), database.GORM, fixture.site.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("overflow usage cursor error = %v", err)
	}
}

func TestUsageAggregationBucketKeysCoverEveryAffectedDimensionAndGranularity(t *testing.T) {
	database := openLockedSiteRunDatabase(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2030, 5, 6, 12, 5, 0, 0, location).Unix()
	hour := now - now%3600 - 3600
	fixture, accounts := createUsageAggregationFixture(t, database, hour, 1, now, "bucket-keys")
	inputs := []UsageFactInput{
		{RemoteUserID: 1, ModelName: "Model-A", ChannelID: 7, RequestCount: 1},
		{RemoteUserID: 2, ModelName: "Model-B", ChannelID: 8, RequestCount: 1},
	}
	facts, _, err := canonicalUsageFacts(fixture.site.ID, hour, now, inputs)
	if err != nil {
		t.Fatalf("canonical bucket-key facts: %v", err)
	}
	dateKey, dateStart, dateEnd, err := UsageDateBucket(hour)
	if err != nil {
		t.Fatalf("bucket-key date: %v", err)
	}
	keys, err := usageAggregationBucketKeys(
		context.Background(), database.GORM, fixture.site.ID, hour, dateKey, dateStart, dateEnd, facts,
	)
	if err != nil {
		t.Fatalf("build aggregation bucket keys: %v", err)
	}
	want := []string{
		"stats:site:" + strconv.FormatInt(fixture.site.ID, 10) + ":hour:" + strconv.FormatInt(hour, 10),
		"stats:site:" + strconv.FormatInt(fixture.site.ID, 10) + ":date:" + strconv.Itoa(dateKey),
		"stats:global:hour:" + strconv.FormatInt(hour, 10),
		"stats:global:date:" + strconv.Itoa(dateKey),
	}
	for _, account := range accounts {
		want = append(want,
			usageHashedBucketKey("account", "hour", account.ID, fixture.site.ID, hour),
			usageHashedBucketKey("account", "date", account.ID, fixture.site.ID, int64(dateKey)),
		)
	}
	want = append(want,
		usageHashedBucketKey("customer", "hour", accounts[0].CustomerID, fixture.site.ID, hour),
		usageHashedBucketKey("customer", "date", accounts[0].CustomerID, fixture.site.ID, int64(dateKey)),
	)
	for _, modelName := range []string{"Model-A", "Model-B"} {
		want = append(want,
			usageHashedStringBucketKey("model", "hour", fixture.site.ID, hour, modelName),
			usageHashedStringBucketKey("model", "date", fixture.site.ID, int64(dateKey), modelName),
		)
	}
	for _, channelID := range []int64{7, 8} {
		want = append(want,
			usageHashedBucketKey("channel", "hour", fixture.site.ID, channelID, hour),
			usageHashedBucketKey("channel", "date", fixture.site.ID, channelID, int64(dateKey)),
		)
	}
	sort.Strings(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("aggregation bucket keys = %#v, want %#v", keys, want)
	}
}

func createUsageAggregationFixture(
	t *testing.T,
	database *Database,
	start int64,
	windowCount int,
	now int64,
	name string,
) (usageMutationFixture, []Account) {
	t.Helper()
	fixture := createUsageMutationFixture(t, database, start, windowCount, constant.TaskTypeUsageBackfill, now)
	customer := Customer{
		Name: "Aggregation " + name, Status: "using", StatisticsBackfillStatus: "none",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := database.GORM.Create(&customer).Error; err != nil {
		t.Fatalf("create aggregation customer: %v", err)
	}
	accounts := []Account{
		{SiteID: fixture.site.ID, CustomerID: customer.ID, RemoteUserID: 1, RemoteCreatedAt: start - 3600,
			Username: "root", RemoteState: AccountRemoteStateNormal, ManagedStatus: AccountManagedStatusActive,
			StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
		{SiteID: fixture.site.ID, CustomerID: customer.ID, RemoteUserID: 2, RemoteCreatedAt: start - 3600,
			Username: "second", RemoteState: AccountRemoteStateNormal, ManagedStatus: AccountManagedStatusActive,
			StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
		{SiteID: fixture.site.ID, CustomerID: customer.ID, RemoteUserID: 3, RemoteCreatedAt: start + 3600,
			Username: "third", RemoteState: AccountRemoteStateNormal, ManagedStatus: AccountManagedStatusActive,
			StatisticsBackfillStatus: "none", CreatedAt: now, UpdatedAt: now},
	}
	if err := database.GORM.Create(&accounts).Error; err != nil {
		t.Fatalf("create aggregation accounts: %v", err)
	}
	t.Cleanup(func() {
		cleanupUsageAggregationFixture(database.GORM, fixture, customer.ID, accounts)
	})
	return fixture, accounts
}

func cleanupUsageAggregationFixture(db *gorm.DB, fixture usageMutationFixture, customerID int64, accounts []Account) {
	ctx := context.Background()
	accountIDs := make([]int64, len(accounts))
	for index := range accounts {
		accountIDs[index] = accounts[index].ID
	}
	keySet := make(map[string]struct{})
	dateSet := make(map[int]struct{})
	for _, hour := range fixture.hours {
		dateKey, dateStart, dateEnd, _ := UsageDateBucket(hour)
		dateSet[dateKey] = struct{}{}
		keys, _ := usageAggregationBucketKeys(ctx, db, fixture.site.ID, hour, dateKey, dateStart, dateEnd, nil)
		for _, key := range keys {
			keySet[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	dateKeys := make([]int, 0, len(dateSet))
	for dateKey := range dateSet {
		dateKeys = append(dateKeys, dateKey)
	}
	_ = db.WithContext(ctx).Where("account_id IN ?", accountIDs).Delete(&AccountStatHourly{}).Error
	_ = db.WithContext(ctx).Where("account_id IN ?", accountIDs).Delete(&AccountStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&CustomerStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&CustomerStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&SiteStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&SiteStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&ModelStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&ModelStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&ChannelStatHourly{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&ChannelStatDaily{}).Error
	_ = db.WithContext(ctx).Where("site_id = ?", fixture.site.ID).Delete(&UsageFactDaily{}).Error
	_ = db.WithContext(ctx).Where("hour_ts IN ?", fixture.hours).Delete(&GlobalStatHourly{}).Error
	_ = db.WithContext(ctx).Where("date_key IN ?", dateKeys).Delete(&GlobalStatDaily{}).Error
	if len(keys) > 0 {
		_ = db.WithContext(ctx).Where("lock_key IN ?", keys).Delete(&AggregationBucketLock{}).Error
	}
	_ = db.WithContext(ctx).Where("id IN ?", accountIDs).Delete(&Account{}).Error
	_ = db.WithContext(ctx).Delete(&Customer{}, customerID).Error
}

func aggregationRequest(
	fixture usageMutationFixture,
	index int,
	now int64,
	facts []UsageFactInput,
) UsageAggregationMutationRequest {
	return UsageAggregationMutationRequest{
		RunID: fixture.run.ID, WindowID: fixture.windows[index].ID, SiteID: fixture.site.ID,
		ExpectedConfigVersion: fixture.run.SiteConfigVersion, HourTS: fixture.hours[index],
		AttemptCount: fixture.windows[index].AttemptCount, RequestID: fixture.run.LastRequestID,
		Now: now, NewFacts: facts,
	}
}

func applyCompleteUsageAggregation(
	t *testing.T,
	database *Database,
	fixture usageMutationFixture,
	index int,
	now int64,
	facts []UsageFactInput,
) UsageAggregationMutationResult {
	t.Helper()
	mutation := buildCompleteUsageAggregation(t, fixture, index, now, facts)
	return applyUsageAggregationMutation(t, database, fixture, index, mutation)
}

func buildCompleteUsageAggregation(
	t *testing.T,
	fixture usageMutationFixture,
	index int,
	now int64,
	facts []UsageFactInput,
) UsageAggregationCommit {
	t.Helper()
	factMutation, _, err := NewCompleteUsageWindowMutation(fixture.completeRequest(index, now, facts))
	if err != nil {
		t.Fatalf("plan complete usage aggregation: %v", err)
	}
	aggregation, err := NewUsageAggregationCommit(aggregationRequest(fixture, index, now, facts), factMutation)
	if err != nil {
		t.Fatalf("bind complete usage aggregation: %v", err)
	}
	return aggregation
}

func applyUsageAggregationMutation(
	t *testing.T,
	database *Database,
	fixture usageMutationFixture,
	index int,
	mutation UsageAggregationCommit,
) UsageAggregationMutationResult {
	t.Helper()
	var result UsageAggregationMutationResult
	err := database.GORM.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		scope, err := lockUsageAggregationTestScope(tx, fixture, index)
		if err != nil {
			return err
		}
		result, err = mutation.Apply(context.Background(), tx, scope)
		return err
	})
	if err != nil {
		t.Fatalf("apply usage aggregation hour %d: %v", index, err)
	}
	return result
}

func lockUsageAggregationTestScope(
	tx *gorm.DB,
	fixture usageMutationFixture,
	index int,
) (UsageWindowMutationScope, error) {
	var scope UsageWindowMutationScope
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&scope.Site, fixture.site.ID).Error; err != nil {
		return UsageWindowMutationScope{}, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&scope.Run, fixture.run.ID).Error; err != nil {
		return UsageWindowMutationScope{}, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&scope.Window, fixture.windows[index].ID).Error; err != nil {
		return UsageWindowMutationScope{}, err
	}
	return scope, nil
}

func assertUsageAggregationMetric(
	t *testing.T,
	db *gorm.DB,
	modelValue any,
	where string,
	args []any,
	requestCount, quota, tokenUsed, activeUsers int64,
) {
	t.Helper()
	var row struct {
		RequestCount int64 `gorm:"column:request_count"`
		Quota        int64 `gorm:"column:quota"`
		TokenUsed    int64 `gorm:"column:token_used"`
		ActiveUsers  int64 `gorm:"column:active_users"`
	}
	selectColumns := "request_count, quota, token_used, 0 AS active_users"
	if activeUsers >= 0 {
		selectColumns = "request_count, quota, token_used, active_users"
	}
	if err := db.Model(modelValue).Select(selectColumns).Where(where, args...).Take(&row).Error; err != nil {
		t.Fatalf("read usage aggregation metric: %v", err)
	}
	if row.RequestCount != requestCount || row.Quota != quota || row.TokenUsed != tokenUsed ||
		(activeUsers >= 0 && row.ActiveUsers != activeUsers) {
		t.Fatalf("usage aggregation metric = %#v, want request=%d quota=%d token=%d active=%d",
			row, requestCount, quota, tokenUsed, activeUsers)
	}
}

func assertUsageAggregationStatus(
	t *testing.T,
	db *gorm.DB,
	modelValue any,
	where string,
	args []any,
	status string,
	final bool,
) {
	t.Helper()
	var row struct {
		DataStatus string `gorm:"column:data_status"`
		IsFinal    bool   `gorm:"column:is_final"`
	}
	if err := db.Model(modelValue).Select("data_status, is_final").Where(where, args...).Take(&row).Error; err != nil {
		t.Fatalf("read usage aggregation status: %v", err)
	}
	if row.DataStatus != status || row.IsFinal != final {
		t.Fatalf("usage aggregation status = %#v, want status=%s final=%t", row, status, final)
	}
}

func assertAggregationRowCount(t *testing.T, db *gorm.DB, modelValue any, where string, args ...any) {
	t.Helper()
	expected := args[len(args)-1].(int)
	queryArgs := args[:len(args)-1]
	var count int64
	if err := db.Model(modelValue).Where(where, queryArgs...).Count(&count).Error; err != nil {
		t.Fatalf("count aggregation rows: %v", err)
	}
	if count != int64(expected) {
		t.Fatalf("aggregation row count = %d, want %d", count, expected)
	}
}
