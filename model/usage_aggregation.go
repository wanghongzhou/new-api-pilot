package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	UsageAggregationStatusComplete = "complete"
	UsageAggregationStatusPartial  = "partial"
)

var usageAggregationLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type AggregationBucketLock struct {
	LockKey   string `gorm:"column:lock_key;primaryKey"`
	UpdatedAt int64  `gorm:"column:updated_at"`
}

func (AggregationBucketLock) TableName() string { return "aggregation_bucket_lock" }

type UsageFactDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	RemoteUserID     int64  `gorm:"column:remote_user_id"`
	UsernameSnapshot string `gorm:"column:username_snapshot"`
	ModelName        string `gorm:"column:model_name"`
	ChannelID        int64  `gorm:"column:channel_id"`
	UseGroup         string `gorm:"column:use_group"`
	TokenID          int64  `gorm:"column:token_id"`
	TokenName        string `gorm:"column:token_name"`
	NodeName         string `gorm:"column:node_name"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (UsageFactDaily) TableName() string { return "usage_fact_daily" }

type AccountStatHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	AccountID        int64  `gorm:"column:account_id"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	DataStatus       string `gorm:"column:data_status"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (AccountStatHourly) TableName() string { return "account_stat_hourly" }

type AccountStatDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	AccountID        int64  `gorm:"column:account_id"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	DataStatus       string `gorm:"column:data_status"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (AccountStatDaily) TableName() string { return "account_stat_daily" }

type CustomerStatHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CustomerID       int64  `gorm:"column:customer_id"`
	SiteID           int64  `gorm:"column:site_id"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (CustomerStatHourly) TableName() string { return "customer_stat_hourly" }

type CustomerStatDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CustomerID       int64  `gorm:"column:customer_id"`
	SiteID           int64  `gorm:"column:site_id"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (CustomerStatDaily) TableName() string { return "customer_stat_daily" }

type SiteStatHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (SiteStatHourly) TableName() string { return "site_stat_hourly" }

type SiteStatDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (SiteStatDaily) TableName() string { return "site_stat_daily" }

type GlobalStatHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (GlobalStatHourly) TableName() string { return "global_stat_hourly" }

type GlobalStatDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (GlobalStatDaily) TableName() string { return "global_stat_daily" }

type ModelStatHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	ModelName        string `gorm:"column:model_name"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (ModelStatHourly) TableName() string { return "model_stat_hourly" }

type ModelStatDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	ModelName        string `gorm:"column:model_name"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (ModelStatDaily) TableName() string { return "model_stat_daily" }

type ChannelStatHourly struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	ChannelID        int64  `gorm:"column:channel_id"`
	HourTS           int64  `gorm:"column:hour_ts"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (ChannelStatHourly) TableName() string { return "channel_stat_hourly" }

type ChannelStatDaily struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SiteID           int64  `gorm:"column:site_id"`
	ChannelID        int64  `gorm:"column:channel_id"`
	DateKey          int    `gorm:"column:date_key"`
	RequestCount     int64  `gorm:"column:request_count"`
	Quota            int64  `gorm:"column:quota"`
	TokenUsed        int64  `gorm:"column:token_used"`
	ActiveUsers      int64  `gorm:"column:active_users"`
	DataStatus       string `gorm:"column:data_status"`
	IsFinal          bool   `gorm:"column:is_final"`
	LastCalculatedAt int64  `gorm:"column:last_calculated_at"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

func (ChannelStatDaily) TableName() string { return "channel_stat_daily" }

type UsageAggregationMutationRequest struct {
	RunID                 int64
	WindowID              int64
	SiteID                int64
	ExpectedConfigVersion int
	HourTS                int64
	AttemptCount          int
	RequestID             string
	Now                   int64
	NewFacts              []UsageFactInput
}

type UsageAggregationMutationResult struct {
	Window     UsageWindowMutationResult
	HourTS     int64
	DateKey    int
	HourlyRows int64
	DailyRows  int64
}

type UsageAggregationCommit struct {
	applyFn func(context.Context, *gorm.DB, UsageWindowMutationScope) (UsageAggregationMutationResult, error)
}

func (commit UsageAggregationCommit) Apply(
	ctx context.Context,
	tx *gorm.DB,
	scope UsageWindowMutationScope,
) (UsageAggregationMutationResult, error) {
	if commit.applyFn == nil {
		return UsageAggregationMutationResult{}, ErrCollectionRunContract
	}
	return commit.applyFn(ctx, tx, scope)
}

func (commit UsageAggregationCommit) ApplyCollectionTaskWindow(
	ctx context.Context,
	tx *gorm.DB,
	scope CollectionTaskWindowMutationScope,
) (CollectionTaskWindowMutationResult, error) {
	result, err := commit.Apply(ctx, tx, UsageWindowMutationScope{
		Site: scope.Site, Run: scope.Run, Window: scope.Window,
	})
	if err != nil {
		return CollectionTaskWindowMutationResult{}, err
	}
	return CollectionTaskWindowMutationResult{
		FetchedRows: result.Window.FetchedRows,
		WrittenRows: result.Window.WrittenRows,
	}, nil
}

func (commit UsageAggregationCommit) Valid() bool { return commit.applyFn != nil }

func NewUsageAggregationCommit(
	request UsageAggregationMutationRequest,
	factMutation UsageFactMutation,
) (UsageAggregationCommit, error) {
	canonicalFacts, _, err := canonicalUsageFacts(request.SiteID, request.HourTS, request.Now, request.NewFacts)
	if err != nil || !factMutation.valid() ||
		!validUsageMutationRequest(request.RunID, request.WindowID, request.SiteID, request.ExpectedConfigVersion,
			request.HourTS, request.AttemptCount, request.RequestID, request.Now, 0) {
		return UsageAggregationCommit{}, ErrCollectionRunContract
	}
	return UsageAggregationCommit{applyFn: func(ctx context.Context, tx *gorm.DB, scope UsageWindowMutationScope) (UsageAggregationMutationResult, error) {
		lockedScope, err := lockUsageMutationScope(ctx, tx, scope, request.RunID, request.WindowID, request.SiteID,
			request.ExpectedConfigVersion, request.HourTS, request.AttemptCount, request.RequestID, request.Now)
		if err != nil {
			return UsageAggregationMutationResult{}, err
		}
		if _, _, err := lockCollectionWindow(ctx, tx, request.SiteID, request.HourTS); err != nil {
			return UsageAggregationMutationResult{}, err
		}
		dateKey, dateStart, dateEnd, err := UsageDateBucket(request.HourTS)
		if err != nil {
			return UsageAggregationMutationResult{}, err
		}
		keys, err := usageAggregationBucketKeys(ctx, tx, request.SiteID, request.HourTS, dateKey, dateStart, dateEnd, canonicalFacts)
		if err != nil {
			return UsageAggregationMutationResult{}, err
		}
		if err := lockUsageAggregationBuckets(ctx, tx, keys, request.Now); err != nil {
			return UsageAggregationMutationResult{}, err
		}
		windowResult, err := factMutation.apply(ctx, tx, lockedScope)
		if err != nil {
			return UsageAggregationMutationResult{}, err
		}
		rebuilt, err := rebuildUsageAggregationBuckets(
			ctx, tx, request.SiteID, request.HourTS, dateKey, dateStart, dateEnd, request.Now,
			usageAggregationRebuildOptions{},
		)
		if err != nil {
			return UsageAggregationMutationResult{}, err
		}
		rebuilt.Window = windowResult
		return rebuilt, nil
	}}, nil
}

func UsageDateBucket(hourTS int64) (int, int64, int64, error) {
	if hourTS <= 0 || hourTS%3600 != 0 {
		return 0, 0, 0, ErrCollectionRunContract
	}
	local := time.Unix(hourTS, 0).In(usageAggregationLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, usageAggregationLocation)
	dateKey := local.Year()*10000 + int(local.Month())*100 + local.Day()
	return dateKey, start.Unix(), start.Add(24 * time.Hour).Unix(), nil
}

type usageAggregationAccountRef struct {
	ID         int64 `gorm:"column:id"`
	CustomerID int64 `gorm:"column:customer_id"`
}

type usageAggregationModelRef struct {
	ModelName string `gorm:"column:model_name"`
}

type usageAggregationChannelRef struct {
	ChannelID int64 `gorm:"column:channel_id"`
}

func usageAggregationBucketKeys(
	ctx context.Context,
	tx *gorm.DB,
	siteID, hourTS int64,
	dateKey int,
	dateStart, dateEnd int64,
	newFacts []UsageFactHourly,
) ([]string, error) {
	if tx == nil || siteID <= 0 {
		return nil, ErrCollectionRunContract
	}
	keys := []string{
		fmt.Sprintf("stats:site:%d:hour:%d", siteID, hourTS),
		fmt.Sprintf("stats:site:%d:date:%d", siteID, dateKey),
		fmt.Sprintf("stats:global:hour:%d", hourTS),
		fmt.Sprintf("stats:global:date:%d", dateKey),
	}
	var accounts []usageAggregationAccountRef
	if err := tx.WithContext(ctx).Table("account").Select("id, customer_id").Where("site_id = ?", siteID).
		Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	customers := make(map[int64]struct{})
	for _, account := range accounts {
		keys = append(keys,
			usageHashedBucketKey("account", "hour", account.ID, siteID, hourTS),
			usageHashedBucketKey("account", "date", account.ID, siteID, int64(dateKey)),
		)
		customers[account.CustomerID] = struct{}{}
	}
	for customerID := range customers {
		keys = append(keys,
			usageHashedBucketKey("customer", "hour", customerID, siteID, hourTS),
			usageHashedBucketKey("customer", "date", customerID, siteID, int64(dateKey)),
		)
	}
	var models []usageAggregationModelRef
	modelQuery := `SELECT model_name FROM usage_fact_hourly
WHERE site_id = ? AND hour_ts >= ? AND hour_ts < ?
UNION SELECT model_name FROM model_stat_hourly WHERE site_id = ? AND hour_ts = ?
UNION SELECT model_name FROM model_stat_daily WHERE site_id = ? AND date_key = ?`
	if err := tx.WithContext(ctx).Raw(modelQuery, siteID, dateStart, dateEnd, siteID, hourTS, siteID, dateKey).
		Scan(&models).Error; err != nil {
		return nil, err
	}
	modelSet := make(map[string]struct{}, len(models)+len(newFacts))
	for _, model := range models {
		modelSet[model.ModelName] = struct{}{}
	}
	for _, fact := range newFacts {
		modelSet[fact.ModelName] = struct{}{}
	}
	for modelName := range modelSet {
		keys = append(keys,
			usageHashedStringBucketKey("model", "hour", siteID, hourTS, modelName),
			usageHashedStringBucketKey("model", "date", siteID, int64(dateKey), modelName),
		)
	}
	var channels []usageAggregationChannelRef
	channelQuery := `SELECT channel_id FROM usage_fact_hourly
WHERE site_id = ? AND hour_ts >= ? AND hour_ts < ?
UNION SELECT channel_id FROM channel_stat_hourly WHERE site_id = ? AND hour_ts = ?
UNION SELECT channel_id FROM channel_stat_daily WHERE site_id = ? AND date_key = ?`
	if err := tx.WithContext(ctx).Raw(channelQuery, siteID, dateStart, dateEnd, siteID, hourTS, siteID, dateKey).
		Scan(&channels).Error; err != nil {
		return nil, err
	}
	channelSet := make(map[int64]struct{}, len(channels)+len(newFacts))
	for _, channel := range channels {
		channelSet[channel.ChannelID] = struct{}{}
	}
	for _, fact := range newFacts {
		channelSet[fact.ChannelID] = struct{}{}
	}
	for channelID := range channelSet {
		keys = append(keys,
			usageHashedBucketKey("channel", "hour", siteID, channelID, hourTS),
			usageHashedBucketKey("channel", "date", siteID, channelID, int64(dateKey)),
		)
	}
	sort.Strings(keys)
	result := keys[:0]
	for _, key := range keys {
		if len(key) > 64 {
			return nil, ErrCollectionRunContract
		}
		if len(result) == 0 || result[len(result)-1] != key {
			result = append(result, key)
		}
	}
	return result, nil
}

func usageHashedBucketKey(kind, granularity string, values ...int64) string {
	input := kind + "\x00" + granularity
	for _, value := range values {
		input += fmt.Sprintf("\x00%d", value)
	}
	return usageHashedBucketKeyValue(kind, input)
}

func usageHashedStringBucketKey(kind, granularity string, first, second int64, value string) string {
	return usageHashedBucketKeyValue(kind, fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", kind, granularity, first, second, value))
}

func usageHashedBucketKeyValue(kind, input string) string {
	digest := sha256.Sum256([]byte(input))
	return "stats:" + kind + ":" + hex.EncodeToString(digest[:24])
}

func lockUsageAggregationBuckets(ctx context.Context, tx *gorm.DB, keys []string, now int64) error {
	if tx == nil || len(keys) == 0 || now <= 0 || !sort.StringsAreSorted(keys) {
		return ErrCollectionRunContract
	}
	for _, key := range keys {
		row := AggregationBucketLock{LockKey: key, UpdatedAt: now}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("lock_key = ?", key).Take(&row).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&AggregationBucketLock{}).Where("lock_key = ?", key).
			Update("updated_at", now).Error; err != nil {
			return err
		}
	}
	return nil
}

func rebuildUsageAggregationBuckets(
	ctx context.Context,
	tx *gorm.DB,
	siteID int64,
	hourTS int64,
	dateKey int,
	dateStart int64,
	dateEnd int64,
	now int64,
	options usageAggregationRebuildOptions,
) (UsageAggregationMutationResult, error) {
	if tx == nil || siteID <= 0 || hourTS <= 0 || hourTS%3600 != 0 || dateKey <= 0 ||
		dateStart <= 0 || dateEnd-dateStart != 24*3600 || hourTS < dateStart || hourTS >= dateEnd || now <= 0 {
		return UsageAggregationMutationResult{}, ErrCollectionRunContract
	}
	hourMetrics, err := loadUsageAggregateMetrics(ctx, tx, usageAggregateHour, hourTS, 0)
	if err != nil {
		return UsageAggregationMutationResult{}, err
	}
	dateMetrics, err := loadUsageAggregateMetrics(ctx, tx, usageAggregateDate, dateStart, dateEnd)
	if err != nil {
		return UsageAggregationMutationResult{}, err
	}
	coverage, err := loadUsageDailyCoverage(ctx, tx, dateStart, dateEnd)
	if err != nil {
		return UsageAggregationMutationResult{}, err
	}
	if err := options.validate(); err != nil {
		return UsageAggregationMutationResult{}, err
	}
	hourlyRows, err := rebuildUsageHourly(ctx, tx, siteID, hourTS, now, hourMetrics, coverage.hour(hourTS), options)
	if err != nil {
		return UsageAggregationMutationResult{}, err
	}
	dailyRows, err := rebuildUsageDaily(ctx, tx, siteID, dateKey, dateStart, dateEnd, now, dateMetrics, coverage, options)
	if err != nil {
		return UsageAggregationMutationResult{}, err
	}
	return UsageAggregationMutationResult{
		HourTS: hourTS, DateKey: dateKey, HourlyRows: hourlyRows, DailyRows: dailyRows,
	}, nil
}

type usageAggregationRebuildOptions struct {
	includePausedAccountIDs  map[int64]struct{}
	includePausedCustomerIDs map[int64]struct{}
}

func (options usageAggregationRebuildOptions) validate() error {
	for id := range options.includePausedAccountIDs {
		if id <= 0 {
			return ErrCollectionRunContract
		}
	}
	for id := range options.includePausedCustomerIDs {
		if id <= 0 {
			return ErrCollectionRunContract
		}
	}
	return nil
}

func (options usageAggregationRebuildOptions) includesPausedAccount(id int64) bool {
	_, included := options.includePausedAccountIDs[id]
	return included
}

func (options usageAggregationRebuildOptions) includesPausedCustomer(id int64) bool {
	_, included := options.includePausedCustomerIDs[id]
	return included
}

func usageAggregationOverrideIDs(values map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	if len(result) == 0 {
		return []int64{-1}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

type usageAggregateScope int

const (
	usageAggregateHour usageAggregateScope = iota + 1
	usageAggregateDate
)

type usageMetricAggregate struct {
	RequestCount int64
	Quota        int64
	TokenUsed    int64
	ActiveUsers  int64
}

func (aggregate usageMetricAggregate) nonzero() bool {
	return aggregate.RequestCount != 0 || aggregate.Quota != 0 || aggregate.TokenUsed != 0 || aggregate.ActiveUsers != 0
}

type usageMetricAggregateRow struct {
	RequestCount string `gorm:"column:request_count"`
	Quota        string `gorm:"column:quota"`
	TokenUsed    string `gorm:"column:token_used"`
	ActiveUsers  int64  `gorm:"column:active_users"`
	InvalidRows  int64  `gorm:"column:invalid_rows"`
}

func loadUsageAggregateMetrics(
	ctx context.Context,
	tx *gorm.DB,
	scope usageAggregateScope,
	start int64,
	end int64,
) (usageMetricAggregate, error) {
	where := "f.hour_ts = ?"
	arguments := []any{start}
	if scope == usageAggregateDate {
		if end <= start {
			return usageMetricAggregate{}, ErrCollectionRunContract
		}
		where = "f.hour_ts >= ? AND f.hour_ts < ?"
		arguments = []any{start, end}
	} else if scope != usageAggregateHour {
		return usageMetricAggregate{}, ErrCollectionRunContract
	}
	query := `SELECT
  CAST(COALESCE(SUM(f.request_count), 0) AS CHAR) AS request_count,
  CAST(COALESCE(SUM(f.quota), 0) AS CHAR) AS quota,
  CAST(COALESCE(SUM(f.token_used), 0) AS CHAR) AS token_used,
  COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN CONCAT(f.site_id, ':', f.remote_user_id) END) AS active_users,
  COALESCE(SUM(CASE WHEN f.request_count < 0 OR f.quota < 0 OR f.token_used < 0 THEN 1 ELSE 0 END), 0) AS invalid_rows
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
JOIN site AS s ON s.id = f.site_id
WHERE ` + where + `
  AND s.statistics_start_at IS NOT NULL
  AND s.statistics_start_at < f.hour_ts + 3600
  AND (s.statistics_end_at IS NULL OR s.statistics_end_at > f.hour_ts)
FOR SHARE OF f, w`
	var row usageMetricAggregateRow
	if err := tx.WithContext(ctx).Raw(query, arguments...).Scan(&row).Error; err != nil {
		return usageMetricAggregate{}, err
	}
	if row.InvalidRows != 0 || row.ActiveUsers < 0 {
		return usageMetricAggregate{}, ErrCollectionRunContract
	}
	requestCount, err := parseUsageAggregateMetric(row.RequestCount)
	if err != nil {
		return usageMetricAggregate{}, err
	}
	quota, err := parseUsageAggregateMetric(row.Quota)
	if err != nil {
		return usageMetricAggregate{}, err
	}
	tokenUsed, err := parseUsageAggregateMetric(row.TokenUsed)
	if err != nil {
		return usageMetricAggregate{}, err
	}
	return usageMetricAggregate{
		RequestCount: requestCount, Quota: quota, TokenUsed: tokenUsed, ActiveUsers: row.ActiveUsers,
	}, nil
}

func parseUsageAggregateMetric(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, ErrCollectionRunContract
	}
	return parsed, nil
}

type usageCoverage struct {
	Expected int64
	Complete int64
	Verified int64
}

func (coverage usageCoverage) status() string {
	if coverage.Expected > 0 && coverage.Complete == coverage.Expected {
		return UsageAggregationStatusComplete
	}
	return UsageAggregationStatusPartial
}

func (coverage usageCoverage) valid() bool {
	return coverage.Expected >= 0 && coverage.Complete >= 0 && coverage.Complete <= coverage.Expected &&
		coverage.Verified >= 0 && coverage.Verified <= coverage.Complete
}

func (coverage usageCoverage) knownPartial() bool {
	return coverage.valid() && coverage.Complete > 0 && coverage.Complete < coverage.Expected
}

func (coverage usageCoverage) final(now, dateEnd int64) bool {
	return now >= dateEnd && coverage.Expected > 0 && coverage.Complete == coverage.Expected &&
		coverage.Verified == coverage.Expected
}

type usageCoverageSite struct {
	ID                int64  `gorm:"column:id"`
	StatisticsStartAt *int64 `gorm:"column:statistics_start_at"`
	StatisticsEndAt   *int64 `gorm:"column:statistics_end_at"`
}

type usageCoverageWindow struct {
	SiteID     int64  `gorm:"column:site_id"`
	HourTS     int64  `gorm:"column:hour_ts"`
	Status     string `gorm:"column:status"`
	VerifiedAt *int64 `gorm:"column:verified_at"`
}

type usageCoverageKey struct {
	SiteID int64
	HourTS int64
}

type usageDailyCoverage struct {
	dateStart int64
	dateEnd   int64
	sites     map[int64]usageCoverageSite
	windows   map[usageCoverageKey]usageCoverageWindow
	bySite    map[int64]usageCoverage
	global    usageCoverage
}

func loadUsageDailyCoverage(ctx context.Context, tx *gorm.DB, dateStart, dateEnd int64) (usageDailyCoverage, error) {
	state := usageDailyCoverage{
		dateStart: dateStart, dateEnd: dateEnd,
		sites: make(map[int64]usageCoverageSite), windows: make(map[usageCoverageKey]usageCoverageWindow),
		bySite: make(map[int64]usageCoverage),
	}
	var sites []usageCoverageSite
	if err := tx.WithContext(ctx).Table("site").Select("id, statistics_start_at, statistics_end_at").
		Where("statistics_start_at IS NOT NULL AND statistics_start_at < ? AND (statistics_end_at IS NULL OR statistics_end_at > ?)",
			dateEnd, dateStart).Order("id ASC").Find(&sites).Error; err != nil {
		return usageDailyCoverage{}, err
	}
	var windows []usageCoverageWindow
	if err := tx.WithContext(ctx).Raw(`SELECT site_id, hour_ts, status, verified_at
FROM collection_window AS w
WHERE hour_ts >= ? AND hour_ts < ?
FOR SHARE OF w`, dateStart, dateEnd).Scan(&windows).Error; err != nil {
		return usageDailyCoverage{}, err
	}
	for _, window := range windows {
		state.windows[usageCoverageKey{SiteID: window.SiteID, HourTS: window.HourTS}] = window
	}
	for _, site := range sites {
		state.sites[site.ID] = site
		coverage := usageCoverage{}
		for hour := dateStart; hour < dateEnd; hour += 3600 {
			if !usageSiteExpectedAt(site, hour) {
				continue
			}
			coverage.Expected++
			window := state.windows[usageCoverageKey{SiteID: site.ID, HourTS: hour}]
			if window.Status == CollectionWindowStatusComplete {
				coverage.Complete++
				if window.VerifiedAt != nil && *window.VerifiedAt >= dateEnd {
					coverage.Verified++
				}
			}
		}
		state.bySite[site.ID] = coverage
		state.global.Expected += coverage.Expected
		state.global.Complete += coverage.Complete
		state.global.Verified += coverage.Verified
	}
	return state, nil
}

func usageSiteExpectedAt(site usageCoverageSite, hourTS int64) bool {
	return site.StatisticsStartAt != nil && *site.StatisticsStartAt < hourTS+3600 &&
		(site.StatisticsEndAt == nil || *site.StatisticsEndAt > hourTS)
}

func (state usageDailyCoverage) hour(hourTS int64) usageCoverage {
	coverage := usageCoverage{}
	for _, site := range state.sites {
		if !usageSiteExpectedAt(site, hourTS) {
			continue
		}
		coverage.Expected++
		window := state.windows[usageCoverageKey{SiteID: site.ID, HourTS: hourTS}]
		if window.Status == CollectionWindowStatusComplete {
			coverage.Complete++
			if window.VerifiedAt != nil && *window.VerifiedAt >= state.dateEnd {
				coverage.Verified++
			}
		}
	}
	return coverage
}

func rebuildUsageHourly(
	ctx context.Context,
	tx *gorm.DB,
	siteID, hourTS, now int64,
	globalMetrics usageMetricAggregate,
	globalCoverage usageCoverage,
	options usageAggregationRebuildOptions,
) (int64, error) {
	includePausedAccounts := usageAggregationOverrideIDs(options.includePausedAccountIDs)
	includePausedCustomers := usageAggregationOverrideIDs(options.includePausedCustomerIDs)
	deleteQueries := []struct {
		query string
		args  []any
	}{
		{query: `DELETE FROM account_stat_hourly
WHERE hour_ts = ? AND account_id IN (SELECT id FROM account WHERE site_id = ?)`, args: []any{hourTS, siteID}},
		{query: "DELETE FROM customer_stat_hourly WHERE site_id = ? AND hour_ts = ?", args: []any{siteID, hourTS}},
		{query: "DELETE FROM site_stat_hourly WHERE site_id = ? AND hour_ts = ?", args: []any{siteID, hourTS}},
		{query: "DELETE FROM model_stat_hourly WHERE site_id = ? AND hour_ts = ?", args: []any{siteID, hourTS}},
		{query: "DELETE FROM channel_stat_hourly WHERE site_id = ? AND hour_ts = ?", args: []any{siteID, hourTS}},
		{query: "DELETE FROM global_stat_hourly WHERE hour_ts = ?", args: []any{hourTS}},
	}
	for _, statement := range deleteQueries {
		if err := tx.WithContext(ctx).Exec(statement.query, statement.args...).Error; err != nil {
			return 0, err
		}
	}
	insertQueries := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO account_stat_hourly
  (account_id, hour_ts, request_count, quota, token_used, data_status,
   last_calculated_at, created_at, updated_at)
SELECT a.id, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used), 'complete', ?, ?, ?
FROM account AS a
JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE a.site_id = ? AND f.hour_ts = ?
  AND a.remote_created_at < ?
  AND (a.statistics_paused_at IS NULL OR ? < a.statistics_paused_at OR a.id IN ?)
GROUP BY a.id`,
			args: []any{hourTS, now, now, now, siteID, hourTS, hourTS + 3600, hourTS, includePausedAccounts},
		},
		{
			query: `INSERT INTO customer_stat_hourly
  (customer_id, site_id, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
SELECT c.id, a.site_id, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT a.id), 'complete', ?, ?, ?
FROM account AS a
JOIN customer AS c ON c.id = a.customer_id
JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE a.site_id = ? AND f.hour_ts = ?
  AND a.remote_created_at < ?
  AND (a.statistics_paused_at IS NULL OR ? < a.statistics_paused_at OR a.id IN ?)
  AND (c.statistics_paused_at IS NULL OR ? < c.statistics_paused_at OR c.id IN ?)
GROUP BY c.id, a.site_id`,
			args: []any{
				hourTS, now, now, now, siteID, hourTS, hourTS + 3600,
				hourTS, includePausedAccounts, hourTS, includePausedCustomers,
			},
		},
		{
			query: `INSERT INTO site_stat_hourly
  (site_id, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
SELECT f.site_id, f.hour_ts, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END), 'complete', ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE f.site_id = ? AND f.hour_ts = ?
GROUP BY f.site_id, f.hour_ts`,
			args: []any{now, now, now, siteID, hourTS},
		},
		{
			query: `INSERT INTO model_stat_hourly
  (site_id, model_name, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
SELECT f.site_id, f.model_name, f.hour_ts, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END), 'complete', ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE f.site_id = ? AND f.hour_ts = ?
GROUP BY f.site_id, f.model_name, f.hour_ts`,
			args: []any{now, now, now, siteID, hourTS},
		},
		{
			query: `INSERT INTO channel_stat_hourly
  (site_id, channel_id, hour_ts, request_count, quota, token_used, active_users,
   data_status, last_calculated_at, created_at, updated_at)
SELECT f.site_id, f.channel_id, f.hour_ts, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END), 'complete', ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE f.site_id = ? AND f.hour_ts = ?
GROUP BY f.site_id, f.channel_id, f.hour_ts`,
			args: []any{now, now, now, siteID, hourTS},
		},
	}
	var rows int64
	for _, statement := range insertQueries {
		result := tx.WithContext(ctx).Exec(statement.query, statement.args...)
		if result.Error != nil {
			return 0, result.Error
		}
		rows += result.RowsAffected
	}
	if globalCoverage.Complete > globalCoverage.Expected ||
		(globalMetrics.nonzero() && globalCoverage.Complete == 0) {
		return 0, ErrCollectionRunContract
	}
	if globalCoverage.Complete > 0 &&
		(globalMetrics.nonzero() || globalCoverage.Complete < globalCoverage.Expected) {
		global := GlobalStatHourly{
			HourTS: hourTS, RequestCount: globalMetrics.RequestCount, Quota: globalMetrics.Quota,
			TokenUsed: globalMetrics.TokenUsed, ActiveUsers: globalMetrics.ActiveUsers,
			DataStatus: globalCoverage.status(), LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.WithContext(ctx).Create(&global).Error; err != nil {
			return 0, err
		}
		rows++
	}
	return rows, nil
}

type usageDailyAccount struct {
	ID                 int64  `gorm:"column:id"`
	CustomerID         int64  `gorm:"column:customer_id"`
	RemoteCreatedAt    int64  `gorm:"column:remote_created_at"`
	StatisticsPausedAt *int64 `gorm:"column:statistics_paused_at"`
}

type usageDailyCustomer struct {
	ID                 int64  `gorm:"column:id"`
	StatisticsPausedAt *int64 `gorm:"column:statistics_paused_at"`
}

func loadUsageEntityCoverage(
	ctx context.Context,
	tx *gorm.DB,
	state usageDailyCoverage,
	siteID int64,
	options usageAggregationRebuildOptions,
) (map[int64]usageCoverage, map[int64]usageCoverage, error) {
	var accounts []usageDailyAccount
	if err := tx.WithContext(ctx).Table("account").
		Select("id, customer_id, remote_created_at, statistics_paused_at").Where("site_id = ?", siteID).
		Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, nil, err
	}
	customerIDs := make([]int64, 0)
	seenCustomers := make(map[int64]struct{})
	for _, account := range accounts {
		if _, exists := seenCustomers[account.CustomerID]; !exists {
			seenCustomers[account.CustomerID] = struct{}{}
			customerIDs = append(customerIDs, account.CustomerID)
		}
	}
	var customers []usageDailyCustomer
	if len(customerIDs) > 0 {
		sort.Slice(customerIDs, func(left, right int) bool { return customerIDs[left] < customerIDs[right] })
		if err := tx.WithContext(ctx).Table("customer").Select("id, statistics_paused_at").
			Where("id IN ?", customerIDs).Order("id ASC").Find(&customers).Error; err != nil {
			return nil, nil, err
		}
		if len(customers) != len(customerIDs) {
			return nil, nil, ErrCollectionRunContract
		}
	}
	customersByID := make(map[int64]usageDailyCustomer, len(customers))
	for _, customer := range customers {
		customersByID[customer.ID] = customer
	}
	accountCoverage := make(map[int64]usageCoverage, len(accounts))
	customerCoverage := make(map[int64]usageCoverage, len(customers))
	site, exists := state.sites[siteID]
	if !exists {
		return accountCoverage, customerCoverage, nil
	}
	for hour := state.dateStart; hour < state.dateEnd; hour += 3600 {
		if !usageSiteExpectedAt(site, hour) {
			continue
		}
		window := state.windows[usageCoverageKey{SiteID: siteID, HourTS: hour}]
		customerExpected := make(map[int64]bool)
		for _, account := range accounts {
			if account.RemoteCreatedAt >= hour+3600 ||
				(account.StatisticsPausedAt != nil && hour >= *account.StatisticsPausedAt &&
					!options.includesPausedAccount(account.ID)) {
				continue
			}
			coverage := accountCoverage[account.ID]
			addUsageCoverageWindow(&coverage, window, state.dateEnd)
			accountCoverage[account.ID] = coverage
			customer := customersByID[account.CustomerID]
			if customer.StatisticsPausedAt == nil || hour < *customer.StatisticsPausedAt ||
				options.includesPausedCustomer(customer.ID) {
				customerExpected[customer.ID] = true
			}
		}
		for customerID := range customerExpected {
			coverage := customerCoverage[customerID]
			addUsageCoverageWindow(&coverage, window, state.dateEnd)
			customerCoverage[customerID] = coverage
		}
	}
	return accountCoverage, customerCoverage, nil
}

func addUsageCoverageWindow(coverage *usageCoverage, window usageCoverageWindow, dateEnd int64) {
	coverage.Expected++
	if window.Status == CollectionWindowStatusComplete {
		coverage.Complete++
		if window.VerifiedAt != nil && *window.VerifiedAt >= dateEnd {
			coverage.Verified++
		}
	}
}

func rebuildUsageDaily(
	ctx context.Context,
	tx *gorm.DB,
	siteID int64,
	dateKey int,
	dateStart, dateEnd, now int64,
	globalMetrics usageMetricAggregate,
	coverage usageDailyCoverage,
	options usageAggregationRebuildOptions,
) (int64, error) {
	siteCoverage, exists := coverage.bySite[siteID]
	if !exists || siteCoverage.Expected <= 0 {
		return 0, ErrCollectionRunContract
	}
	accountCoverage, customerCoverage, err := loadUsageEntityCoverage(ctx, tx, coverage, siteID, options)
	if err != nil {
		return 0, err
	}
	deleteQueries := []struct {
		query string
		args  []any
	}{
		{query: "DELETE FROM usage_fact_daily WHERE site_id = ? AND date_key = ?", args: []any{siteID, dateKey}},
		{query: `DELETE FROM account_stat_daily
WHERE date_key = ? AND account_id IN (SELECT id FROM account WHERE site_id = ?)`, args: []any{dateKey, siteID}},
		{query: "DELETE FROM customer_stat_daily WHERE site_id = ? AND date_key = ?", args: []any{siteID, dateKey}},
		{query: "DELETE FROM site_stat_daily WHERE site_id = ? AND date_key = ?", args: []any{siteID, dateKey}},
		{query: "DELETE FROM model_stat_daily WHERE site_id = ? AND date_key = ?", args: []any{siteID, dateKey}},
		{query: "DELETE FROM channel_stat_daily WHERE site_id = ? AND date_key = ?", args: []any{siteID, dateKey}},
		{query: "DELETE FROM global_stat_daily WHERE date_key = ?", args: []any{dateKey}},
	}
	for _, statement := range deleteQueries {
		if err := tx.WithContext(ctx).Exec(statement.query, statement.args...).Error; err != nil {
			return 0, err
		}
	}
	siteStatus := siteCoverage.status()
	siteFinal := siteCoverage.final(now, dateEnd)
	includePausedAccounts := usageAggregationOverrideIDs(options.includePausedAccountIDs)
	includePausedCustomers := usageAggregationOverrideIDs(options.includePausedCustomerIDs)
	insertQueries := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO usage_fact_daily
  (site_id, remote_user_id, username_snapshot, model_name, channel_id, use_group, token_id, token_name, node_name, date_key,
   request_count, quota, token_used, is_final, last_calculated_at, created_at, updated_at)
SELECT f.site_id, f.remote_user_id,
       COALESCE(MIN(NULLIF(f.username_snapshot, '') COLLATE utf8mb4_bin), ''),
       f.model_name, f.channel_id, f.use_group, f.token_id,
       COALESCE(MIN(NULLIF(f.token_name, '') COLLATE utf8mb4_bin), ''), f.node_name,
       ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       ?, ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
JOIN site AS s ON s.id = f.site_id
WHERE f.site_id = ? AND f.hour_ts >= ? AND f.hour_ts < ?
  AND s.statistics_start_at IS NOT NULL AND s.statistics_start_at < f.hour_ts + 3600
  AND (s.statistics_end_at IS NULL OR s.statistics_end_at > f.hour_ts)
GROUP BY f.site_id, f.remote_user_id, f.model_name, f.channel_id, f.use_group, f.token_id, f.node_name`,
			args: []any{dateKey, siteFinal, now, now, now, siteID, dateStart, dateEnd},
		},
		{
			query: `INSERT INTO account_stat_daily
  (account_id, date_key, request_count, quota, token_used, data_status, is_final,
   last_calculated_at, created_at, updated_at)
SELECT a.id, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used), 'complete', 0, ?, ?, ?
FROM account AS a
JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE a.site_id = ? AND f.hour_ts >= ? AND f.hour_ts < ?
  AND a.remote_created_at < f.hour_ts + 3600
  AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at OR a.id IN ?)
GROUP BY a.id`,
			args: []any{dateKey, now, now, now, siteID, dateStart, dateEnd, includePausedAccounts},
		},
		{
			query: `INSERT INTO customer_stat_daily
  (customer_id, site_id, date_key, request_count, quota, token_used, active_users,
   data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT c.id, a.site_id, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT a.id), 'complete', 0, ?, ?, ?
FROM account AS a
JOIN customer AS c ON c.id = a.customer_id
JOIN usage_fact_hourly AS f ON f.site_id = a.site_id AND f.remote_user_id = a.remote_user_id
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE a.site_id = ? AND f.hour_ts >= ? AND f.hour_ts < ?
  AND a.remote_created_at < f.hour_ts + 3600
  AND (a.statistics_paused_at IS NULL OR f.hour_ts < a.statistics_paused_at OR a.id IN ?)
  AND (c.statistics_paused_at IS NULL OR f.hour_ts < c.statistics_paused_at OR c.id IN ?)
GROUP BY c.id, a.site_id`,
			args: []any{
				dateKey, now, now, now, siteID, dateStart, dateEnd,
				includePausedAccounts, includePausedCustomers,
			},
		},
		{
			query: `INSERT INTO site_stat_daily
  (site_id, date_key, request_count, quota, token_used, active_users, data_status,
   is_final, last_calculated_at, created_at, updated_at)
SELECT f.site_id, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END), ?, ?, ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE f.site_id = ? AND f.hour_ts >= ? AND f.hour_ts < ?
GROUP BY f.site_id`,
			args: []any{dateKey, siteStatus, siteFinal, now, now, now, siteID, dateStart, dateEnd},
		},
		{
			query: `INSERT INTO model_stat_daily
  (site_id, model_name, date_key, request_count, quota, token_used, active_users,
   data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT f.site_id, f.model_name, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END), ?, ?, ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE f.site_id = ? AND f.hour_ts >= ? AND f.hour_ts < ?
GROUP BY f.site_id, f.model_name`,
			args: []any{dateKey, siteStatus, siteFinal, now, now, now, siteID, dateStart, dateEnd},
		},
		{
			query: `INSERT INTO channel_stat_daily
  (site_id, channel_id, date_key, request_count, quota, token_used, active_users,
   data_status, is_final, last_calculated_at, created_at, updated_at)
SELECT f.site_id, f.channel_id, ?, SUM(f.request_count), SUM(f.quota), SUM(f.token_used),
       COUNT(DISTINCT CASE WHEN f.remote_user_id > 0 THEN f.remote_user_id END), ?, ?, ?, ?, ?
FROM usage_fact_hourly AS f
JOIN collection_window AS w
  ON w.site_id = f.site_id AND w.hour_ts = f.hour_ts AND w.status = 'complete'
WHERE f.site_id = ? AND f.hour_ts >= ? AND f.hour_ts < ?
GROUP BY f.site_id, f.channel_id`,
			args: []any{dateKey, siteStatus, siteFinal, now, now, now, siteID, dateStart, dateEnd},
		},
	}
	var rows int64
	for _, statement := range insertQueries {
		result := tx.WithContext(ctx).Exec(statement.query, statement.args...)
		if result.Error != nil {
			return 0, result.Error
		}
		rows += result.RowsAffected
	}
	if !siteCoverage.valid() {
		return 0, ErrCollectionRunContract
	}
	if siteCoverage.knownPartial() {
		zero := SiteStatDaily{
			SiteID: siteID, DateKey: dateKey, DataStatus: UsageAggregationStatusPartial,
			LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&zero)
		if result.Error != nil {
			return 0, result.Error
		}
		rows += result.RowsAffected
	}
	accountZeros := make([]AccountStatDaily, 0)
	for accountID, itemCoverage := range accountCoverage {
		if !itemCoverage.valid() {
			return 0, ErrCollectionRunContract
		}
		if itemCoverage.knownPartial() {
			accountZeros = append(accountZeros, AccountStatDaily{
				AccountID: accountID, DateKey: dateKey, DataStatus: UsageAggregationStatusPartial,
				LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	if len(accountZeros) > 0 {
		result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(accountZeros, 500)
		if result.Error != nil {
			return 0, result.Error
		}
		rows += result.RowsAffected
	}
	customerZeros := make([]CustomerStatDaily, 0)
	for customerID, itemCoverage := range customerCoverage {
		if !itemCoverage.valid() {
			return 0, ErrCollectionRunContract
		}
		if itemCoverage.knownPartial() {
			customerZeros = append(customerZeros, CustomerStatDaily{
				CustomerID: customerID, SiteID: siteID, DateKey: dateKey,
				DataStatus:       UsageAggregationStatusPartial,
				LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	if len(customerZeros) > 0 {
		result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(customerZeros, 500)
		if result.Error != nil {
			return 0, result.Error
		}
		rows += result.RowsAffected
	}
	for accountID, itemCoverage := range accountCoverage {
		if err := tx.WithContext(ctx).Model(&AccountStatDaily{}).
			Where("account_id = ? AND date_key = ?", accountID, dateKey).
			Updates(map[string]any{
				"data_status": itemCoverage.status(), "is_final": itemCoverage.final(now, dateEnd),
			}).Error; err != nil {
			return 0, err
		}
	}
	for customerID, itemCoverage := range customerCoverage {
		if err := tx.WithContext(ctx).Model(&CustomerStatDaily{}).
			Where("customer_id = ? AND site_id = ? AND date_key = ?", customerID, siteID, dateKey).
			Updates(map[string]any{
				"data_status": itemCoverage.status(), "is_final": itemCoverage.final(now, dateEnd),
			}).Error; err != nil {
			return 0, err
		}
	}
	if coverage.global.Complete > coverage.global.Expected ||
		(globalMetrics.nonzero() && coverage.global.Complete == 0) {
		return 0, ErrCollectionRunContract
	}
	if coverage.global.Complete > 0 &&
		(globalMetrics.nonzero() || coverage.global.Complete < coverage.global.Expected) {
		global := GlobalStatDaily{
			DateKey: dateKey, RequestCount: globalMetrics.RequestCount, Quota: globalMetrics.Quota,
			TokenUsed: globalMetrics.TokenUsed, ActiveUsers: globalMetrics.ActiveUsers,
			DataStatus: coverage.global.status(), IsFinal: coverage.global.final(now, dateEnd),
			LastCalculatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.WithContext(ctx).Create(&global).Error; err != nil {
			return 0, err
		}
		rows++
	}
	return rows, nil
}
