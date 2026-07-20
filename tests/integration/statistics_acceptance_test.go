package integration_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"new-api-pilot/service"
	testsupport "new-api-pilot/tests/support"
)

func TestA27A38A39A40A65StatisticsMaterializationAndChannelIdentity(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	hour := coreFloorHour(now - 3600)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCollectionSiteClient(now)
	collector, err := service.NewUsageCollectionService(service.UsageCollectionServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: &collectionSiteClientFactory{client: client}, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create statistics collection service: %v", err)
	}
	first := createCoreAuthorizedSite(t, database, cipher, now)
	customer := createCoreCustomer(t, database, now, dto.CustomerStatusUsing)
	account := createCoreAccount(t, database, first.ID, customer.ID, 7, now)
	client.flow = []dto.UpstreamFlowRow{
		{UserID: 1, Username: "root", ModelName: "Model-A", ChannelID: 1, RequestCount: 2, Quota: 20, TokenUsed: 200},
		{UserID: 1, Username: "root", ModelName: "model-a", ChannelID: 0, RequestCount: 3, Quota: 30, TokenUsed: 300},
		{UserID: 7, Username: "managed", ModelName: "Model-A", ChannelID: 1, RequestCount: 1, Quota: 10, TokenUsed: 100},
	}
	client.data = []dto.UpstreamDataRow{
		{ModelName: "Model-A", CreatedAt: hour, RequestCount: 3, Quota: 30, TokenUsed: 300},
		{ModelName: "model-a", CreatedAt: hour, RequestCount: 3, Quota: 30, TokenUsed: 300},
	}
	repository := model.NewCollectionTaskRepository(database)
	claim := coreClaimUsageWindow(t, repository, first, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now, "a38-first")
	coreCollectAndCommitUsageWindow(t, repository, collector, claim, now+1)

	// The same complete fact set materializes every retained hourly/daily level.
	for _, table := range []any{
		&model.UsageFactDaily{}, &model.AccountStatHourly{}, &model.AccountStatDaily{},
		&model.CustomerStatHourly{}, &model.CustomerStatDaily{}, &model.SiteStatHourly{}, &model.SiteStatDaily{},
		&model.GlobalStatHourly{}, &model.GlobalStatDaily{}, &model.ModelStatHourly{}, &model.ModelStatDaily{},
		&model.ChannelStatHourly{}, &model.ChannelStatDaily{},
	} {
		var count int64
		if err := database.Model(table).Count(&count).Error; err != nil || count == 0 {
			t.Fatalf("A38 materialized %T count=%d err=%v", table, count, err)
		}
	}
	var models []string
	if err := database.Model(&model.ModelStatHourly{}).Where("site_id = ? AND hour_ts = ?", first.ID, hour).
		Order("model_name ASC").Pluck("model_name", &models).Error; err != nil || len(models) != 2 || models[0] != "Model-A" || models[1] != "model-a" {
		t.Fatalf("A27 case-sensitive model aggregation = %#v, %v", models, err)
	}
	var accountRows int64
	if err := database.Model(&model.AccountStatHourly{}).Where("account_id = ? AND hour_ts = ?", account.ID, hour).Count(&accountRows).Error; err != nil || accountRows != 1 {
		t.Fatalf("A38 managed account aggregation count=%d err=%v", accountRows, err)
	}

	second := createCoreAuthorizedSite(t, database, cipher, now)
	client.flow = []dto.UpstreamFlowRow{{
		UserID: 1, Username: "root", ModelName: "Model-A", ChannelID: 1, RequestCount: 9, Quota: 900, TokenUsed: 9000,
	}}
	client.data = []dto.UpstreamDataRow{{
		ModelName: "Model-A", CreatedAt: hour, RequestCount: 9, Quota: 900, TokenUsed: 9000,
	}}
	secondClaim := coreClaimUsageWindow(t, repository, second, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now+2, "a65-second")
	coreCollectAndCommitUsageWindow(t, repository, collector, secondClaim, now+3)
	channels := []model.SiteChannel{
		{SiteID: first.ID, RemoteChannelID: 1, Name: "first-channel", LastSyncedAt: now, CreatedAt: now, UpdatedAt: now},
		{SiteID: second.ID, RemoteChannelID: 1, Name: "second-channel", LastSyncedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Create(&channels).Error; err != nil {
		t.Fatalf("create channel options: %v", err)
	}

	statistics, err := service.NewStatisticsService(service.StatisticsServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create statistics service: %v", err)
	}
	channelQuery := dto.StatisticsQuery{
		StartTimestamp: hour, EndTimestamp: hour + 3600, Granularity: dto.StatisticsGranularityHour,
		ChannelKeys: []string{strconv.FormatInt(first.ID, 10) + ":1"},
		Page:        1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	}
	filtered, err := statistics.Channels(context.Background(), channelQuery)
	if err != nil || len(filtered.Trend) != 1 || filtered.Trend[0].Quota == nil || *filtered.Trend[0].Quota != "30" {
		t.Fatalf("A65 exact site/channel filter = %#v, %v", filtered, err)
	}
	options, err := statistics.ChannelOptions(context.Background(), dto.StatisticsOptionQuery{
		SiteIDs: []int64{first.ID, second.ID}, Page: 1, PageSize: 20,
	})
	if err != nil || !coreHasChannelOption(options.Items, strconv.FormatInt(first.ID, 10)+":0") ||
		!coreHasChannelOption(options.Items, strconv.FormatInt(second.ID, 10)+":0") ||
		!coreHasChannelOption(options.Items, strconv.FormatInt(first.ID, 10)+":1") ||
		!coreHasChannelOption(options.Items, strconv.FormatInt(second.ID, 10)+":1") {
		t.Fatalf("A65 channel options = %#v, %v", options, err)
	}

	location := time.FixedZone("Asia/Shanghai", 8*3600)
	month := time.Unix(hour, 0).In(location)
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, location).Unix()
	monthEnd := time.Date(month.Year(), month.Month()+1, 1, 0, 0, 0, 0, location).Unix()
	monthly, err := statistics.Global(context.Background(), dto.StatisticsQuery{
		StartTimestamp: monthStart, EndTimestamp: monthEnd, Granularity: dto.StatisticsGranularityMonth,
		Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	})
	if err != nil || len(monthly.Trend) != 1 || monthly.Trend[0].RequestCount == nil || monthly.Trend[0].ActiveUsers == nil {
		t.Fatalf("A39 daily-backed month aggregation = %#v, %v", monthly, err)
	}
	for _, table := range []any{&model.SiteStatHourly{}, &model.SiteStatDaily{}} {
		columns, err := database.Migrator().ColumnTypes(table)
		if err != nil {
			t.Fatalf("A40 inspect %T columns: %v", table, err)
		}
		for _, column := range columns {
			name := strings.ToLower(column.Name())
			if strings.Contains(name, "usd") || strings.Contains(name, "cny") || strings.Contains(name, "amount") {
				t.Fatalf("A40 statistics table %T persists monetary column %q", table, name)
			}
		}
	}
}

func TestA29A68StatisticsMissingDerivationAndPausedRecovery(t *testing.T) {
	database := openCoreAcceptanceTransaction(t)
	const now = int64(1_768_622_400)
	hour := coreFloorHour(now - 3600)
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	cipher := newCoreCipher(t)
	client := newCollectionSiteClient(now)
	factory := &collectionSiteClientFactory{client: client}
	site := createCoreAuthorizedSite(t, database, cipher, now)
	statistics, err := service.NewStatisticsService(service.StatisticsServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create statistics service: %v", err)
	}
	missingHour := hour - 3600
	response, err := statistics.Sites(context.Background(), dto.StatisticsQuery{
		StartTimestamp: missingHour, EndTimestamp: missingHour + 3600, Granularity: dto.StatisticsGranularityHour,
		SiteIDs: []int64{site.ID}, Page: 1, PageSize: 20, SortBy: "bucket_start", SortOrder: "asc",
	})
	if err != nil || len(response.Trend) != 1 || response.Trend[0].DataStatus != model.CollectionWindowStatusMissing ||
		response.Trend[0].RequestCount != nil {
		t.Fatalf("A29 derived missing window = %#v, %v", response, err)
	}

	collector, err := service.NewUsageCollectionService(service.UsageCollectionServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
	})
	if err != nil {
		t.Fatalf("create recovery collector: %v", err)
	}
	client.flow = []dto.UpstreamFlowRow{{UserID: 1, Username: "root", ModelName: "Model-A", ChannelID: 1, RequestCount: 1, Quota: 1, TokenUsed: 1}}
	client.data = []dto.UpstreamDataRow{{ModelName: "Model-A", CreatedAt: hour, RequestCount: 1, Quota: 1, TokenUsed: 1}}
	repository := model.NewCollectionTaskRepository(database)
	claim := coreClaimUsageWindow(t, repository, site, constant.TaskTypeUsageBackfill,
		constant.CollectionTriggerManual, constant.CollectionPriorityManualBackfill, hour, hour+3600, now, "a68-history")
	coreCollectAndCommitUsageWindow(t, repository, collector, claim, now+1)

	sites, err := service.NewSiteService(service.SiteServiceOptions{
		Repository: model.NewSiteRepository(database), ClientFactory: factory, Cipher: cipher, Clock: clock,
		PreflightSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatalf("create site lifecycle service: %v", err)
	}
	if _, err := sites.Disable(context.Background(), site.ID); err != nil {
		t.Fatalf("A68 disable site: %v", err)
	}
	coreUsageFactCount(t, database, site.ID, hour, 1)
	clock.Advance(2 * time.Hour)
	recovery, err := sites.Enable(context.Background(), site.ID, "a68-enable")
	if err != nil || recovery.Status != model.CollectionTaskStatusPending {
		t.Fatalf("A68 enable recovery = %#v, %v", recovery, err)
	}
	persisted, err := model.NewSiteRepository(database).FindByID(context.Background(), site.ID)
	if err != nil || persisted.ManagementStatus != constant.SiteManagementActive ||
		persisted.StatisticsStatus != constant.SiteStatisticsBackfilling || persisted.DisabledAt == nil {
		t.Fatalf("A68 site stays paused during recovery = %#v, %v", persisted, err)
	}
	coreUsageFactCount(t, database, site.ID, hour, 1)
}

func coreHasChannelOption(options []dto.ChannelOption, key string) bool {
	for _, option := range options {
		if option.Key == key {
			return true
		}
	}
	return false
}
