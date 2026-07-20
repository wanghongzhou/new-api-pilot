package service

import (
	"context"
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"

	"gorm.io/gorm"
)

var (
	ErrStatisticsInvalid  = errors.New("statistics query is invalid")
	ErrStatisticsRead     = errors.New("statistics query failed")
	ErrStatisticsNotFound = errors.New("statistics scope was not found")
)

type StatisticsService struct {
	repository *model.StatisticsRepository
	clock      common.Clock
}

type StatisticsServiceOptions struct {
	Database *gorm.DB
	Clock    common.Clock
}

func NewStatisticsService(options StatisticsServiceOptions) (*StatisticsService, error) {
	if options.Database == nil || options.Clock == nil {
		return nil, errors.New("statistics service dependencies are required")
	}
	return &StatisticsService{repository: model.NewStatisticsRepository(options.Database), clock: options.Clock}, nil
}

func (service *StatisticsService) Global(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeGlobal, query)
}

func (service *StatisticsService) Sites(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeSite, query)
}

func (service *StatisticsService) Customers(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeCustomer, query)
}

func (service *StatisticsService) Accounts(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeAccount, query)
}

func (service *StatisticsService) Models(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeModel, query)
}

func (service *StatisticsService) Channels(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeChannel, query)
}

func (service *StatisticsService) Groups(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeGroup, query)
}
func (service *StatisticsService) Tokens(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeToken, query)
}
func (service *StatisticsService) Nodes(ctx context.Context, query dto.StatisticsQuery) (dto.StatisticsResponse, error) {
	return service.query(ctx, dto.StatisticsScopeNode, query)
}

func (service *StatisticsService) SiteStatistics(
	ctx context.Context,
	id int64,
	query dto.StatisticsQuery,
) (dto.StatisticsResponse, error) {
	if service == nil || service.repository == nil || id <= 0 {
		return dto.StatisticsResponse{}, ErrStatisticsInvalid
	}
	sites, err := service.repository.LoadSites(ctx, []int64{id})
	if err != nil {
		return dto.StatisticsResponse{}, errors.Join(ErrStatisticsRead, err)
	}
	if len(sites) != 1 || sites[0].ID != id {
		return dto.StatisticsResponse{}, ErrStatisticsNotFound
	}
	query.SiteIDs = []int64{id}
	return service.Sites(ctx, query)
}

func (service *StatisticsService) ModelOptions(
	ctx context.Context,
	query dto.StatisticsOptionQuery,
) (common.PageData[dto.ModelOption], error) {
	query.Normalize()
	if service == nil || service.repository == nil || query.Validate() != nil {
		return common.PageData[dto.ModelOption]{}, ErrStatisticsInvalid
	}
	rows, total, err := service.repository.LoadModelOptions(ctx, query.Keyword, query.SiteIDs, query.PageSize, query.Offset())
	if err != nil {
		return common.PageData[dto.ModelOption]{}, errors.Join(ErrStatisticsRead, err)
	}
	items := make([]dto.ModelOption, 0, len(rows))
	for _, row := range rows {
		siteID := strconv.FormatInt(row.SiteID, 10)
		items = append(items, dto.ModelOption{
			Key: siteID + ":" + row.ModelName, SiteID: siteID, SiteName: row.SiteName, ModelName: row.ModelName,
		})
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *StatisticsService) ChannelOptions(
	ctx context.Context,
	query dto.StatisticsOptionQuery,
) (common.PageData[dto.ChannelOption], error) {
	query.Normalize()
	if service == nil || service.repository == nil || query.Validate() != nil {
		return common.PageData[dto.ChannelOption]{}, ErrStatisticsInvalid
	}
	rows, total, err := service.repository.LoadChannelOptions(ctx, query.Keyword, query.SiteIDs, query.PageSize, query.Offset())
	if err != nil {
		return common.PageData[dto.ChannelOption]{}, errors.Join(ErrStatisticsRead, err)
	}
	items := make([]dto.ChannelOption, 0, len(rows))
	for _, row := range rows {
		siteID := strconv.FormatInt(row.SiteID, 10)
		channelID := strconv.FormatInt(row.RemoteChannelID, 10)
		items = append(items, dto.ChannelOption{
			Key: siteID + ":" + channelID, SiteID: siteID, SiteName: row.SiteName,
			RemoteChannelID: channelID, Name: row.Name, RemoteMissing: row.RemoteMissing,
		})
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *StatisticsService) GroupOptions(ctx context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.GroupOption], error) {
	query.Normalize()
	if query.Validate() != nil {
		return common.PageData[dto.GroupOption]{}, ErrStatisticsInvalid
	}
	value, total, err := service.repository.LoadFlowOptions(ctx, "group", query.Keyword, query.SiteIDs, query.PageSize, query.Offset())
	if err != nil {
		return common.PageData[dto.GroupOption]{}, errors.Join(ErrStatisticsRead, err)
	}
	rows := value.([]model.StatisticsGroupOption)
	items := make([]dto.GroupOption, 0, len(rows))
	for _, row := range rows {
		id := strconv.FormatInt(row.SiteID, 10)
		items = append(items, dto.GroupOption{Key: statisticsModelDimensionKey(row.SiteID, row.UseGroup), SiteID: id, SiteName: row.SiteName, UseGroup: row.UseGroup})
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *StatisticsService) TokenOptions(ctx context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.TokenOption], error) {
	query.Normalize()
	if query.Validate() != nil {
		return common.PageData[dto.TokenOption]{}, ErrStatisticsInvalid
	}
	value, total, err := service.repository.LoadFlowOptions(ctx, "token", query.Keyword, query.SiteIDs, query.PageSize, query.Offset())
	if err != nil {
		return common.PageData[dto.TokenOption]{}, errors.Join(ErrStatisticsRead, err)
	}
	rows := value.([]model.StatisticsTokenOption)
	items := make([]dto.TokenOption, 0, len(rows))
	for _, row := range rows {
		id := strconv.FormatInt(row.SiteID, 10)
		token := strconv.FormatInt(row.TokenID, 10)
		items = append(items, dto.TokenOption{Key: id + ":" + token, SiteID: id, SiteName: row.SiteName, TokenID: token, TokenName: row.TokenName})
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *StatisticsService) NodeOptions(ctx context.Context, query dto.StatisticsOptionQuery) (common.PageData[dto.NodeOption], error) {
	query.Normalize()
	if query.Validate() != nil {
		return common.PageData[dto.NodeOption]{}, ErrStatisticsInvalid
	}
	value, total, err := service.repository.LoadFlowOptions(ctx, "node", query.Keyword, query.SiteIDs, query.PageSize, query.Offset())
	if err != nil {
		return common.PageData[dto.NodeOption]{}, errors.Join(ErrStatisticsRead, err)
	}
	rows := value.([]model.StatisticsNodeOption)
	items := make([]dto.NodeOption, 0, len(rows))
	for _, row := range rows {
		id := strconv.FormatInt(row.SiteID, 10)
		items = append(items, dto.NodeOption{Key: statisticsModelDimensionKey(row.SiteID, row.NodeName), SiteID: id, SiteName: row.SiteName, NodeName: row.NodeName})
	}
	return common.NewPageData(query.Page, query.PageSize, total, items), nil
}

func (service *StatisticsService) CustomerStatistics(
	ctx context.Context,
	id int64,
	query dto.StatisticsQuery,
) (dto.StatisticsResponse, error) {
	query.CustomerIDs = []int64{id}
	return service.Customers(ctx, query)
}

func (service *StatisticsService) AccountStatistics(
	ctx context.Context,
	id int64,
	query dto.StatisticsQuery,
) (dto.StatisticsResponse, error) {
	query.AccountIDs = []int64{id}
	return service.Accounts(ctx, query)
}

func (service *StatisticsService) query(
	ctx context.Context,
	scope string,
	query dto.StatisticsQuery,
) (dto.StatisticsResponse, error) {
	return service.queryInternal(ctx, scope, query, false)
}

// ExportSnapshot uses the exact statistics read and response builder while
// omitting only the UI page slice. Callers must wrap it in their read-only
// repeatable-read transaction.
func (service *StatisticsService) ExportSnapshot(
	ctx context.Context,
	scope string,
	query dto.StatisticsQuery,
) (dto.StatisticsResponse, error) {
	return service.queryInternal(ctx, scope, query, true)
}

func (service *StatisticsService) queryInternal(
	ctx context.Context,
	scope string,
	query dto.StatisticsQuery,
	unpaginated bool,
) (dto.StatisticsResponse, error) {
	if service == nil || service.repository == nil || service.clock == nil {
		return dto.StatisticsResponse{}, ErrStatisticsRead
	}
	query.Normalize()
	validation := query.Validate(scope)
	if unpaginated {
		validation = query.ValidateForExport(scope)
	}
	if validation != nil {
		return dto.StatisticsResponse{}, ErrStatisticsInvalid
	}
	request, err := statisticsReadRequest(scope, query)
	if err != nil {
		return dto.StatisticsResponse{}, ErrStatisticsInvalid
	}
	buckets, err := newStatisticsBuckets(query)
	if err != nil {
		return dto.StatisticsResponse{}, ErrStatisticsInvalid
	}
	sites, err := service.repository.LoadSites(ctx, query.SiteIDs)
	if err != nil {
		return dto.StatisticsResponse{}, errors.Join(ErrStatisticsRead, err)
	}
	data := statisticsReadData{sites: sites}
	switch scope {
	case dto.StatisticsScopeCustomer:
		data.customers, err = service.repository.LoadCustomers(ctx, query.CustomerIDs)
		if err == nil {
			if len(query.CustomerIDs) > 0 && len(data.customers) == 0 {
				sites = nil
			} else {
				request.CustomerIDs = statisticsCustomerIDs(data.customers)
				request.SiteIDs = statisticsSiteIDs(sites)
				data.accounts, err = service.repository.LoadAccounts(ctx, request)
			}
		}
		sites = statisticsSitesForAccounts(sites, data.accounts)
	case dto.StatisticsScopeAccount:
		request.SiteIDs = statisticsSiteIDs(sites)
		data.accounts, err = service.repository.LoadAccounts(ctx, request)
		if err == nil {
			request.AccountIDs = statisticsAccountIDs(data.accounts)
			data.customers, err = service.repository.LoadCustomers(ctx, statisticsAccountCustomerIDs(data.accounts))
		}
		sites = statisticsSitesForAccounts(sites, data.accounts)
	case dto.StatisticsScopeChannel:
		if len(request.ChannelKeys) > 0 {
			sites = statisticsSitesForChannelKeys(sites, request.ChannelKeys)
		}
		request.SiteIDs = statisticsSiteIDs(sites)
		data.channels, err = service.repository.LoadChannels(ctx, request)
	}
	if err != nil {
		return dto.StatisticsResponse{}, errors.Join(ErrStatisticsRead, err)
	}
	data.sites = sites
	request.SiteIDs = statisticsSiteIDs(sites)
	if len(request.SiteIDs) > 0 {
		data.metrics, err = service.repository.LoadMetricRows(ctx, request)
		if err == nil {
			data.active, err = service.repository.LoadActiveRows(ctx, request)
		}
		windowEnd := minInt64(query.EndTimestamp, floorHour(service.clock.Now().Unix()))
		if err == nil && windowEnd > query.StartTimestamp {
			data.windows, err = service.repository.LoadWindows(ctx, request.SiteIDs, query.StartTimestamp, windowEnd)
		}
		if err == nil {
			data.rates, err = service.repository.LoadFallbackRates(ctx)
		}
	}
	if err != nil {
		return dto.StatisticsResponse{}, errors.Join(ErrStatisticsRead, err)
	}
	response, err := (&statisticsResponseBuilder{
		scope: scope, query: query, request: request, now: service.clock.Now().Unix(), buckets: buckets, data: data,
		unpaginated: unpaginated,
	}).build()
	if err != nil {
		return dto.StatisticsResponse{}, errors.Join(ErrStatisticsRead, err)
	}
	return response, nil
}

type statisticsReadData struct {
	sites     []model.StatisticsSite
	customers []model.StatisticsCustomer
	accounts  []model.StatisticsAccount
	channels  []model.StatisticsChannel
	windows   []model.StatisticsWindow
	metrics   []model.StatisticsMetricRow
	active    []model.StatisticsActiveRow
	rates     model.StatisticsFallbackRates
}

func statisticsReadRequest(scope string, query dto.StatisticsQuery) (model.StatisticsReadRequest, error) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Unix(query.StartTimestamp, 0).In(location)
	end := time.Unix(query.EndTimestamp, 0).In(location)
	request := model.StatisticsReadRequest{
		Scope: scope, Granularity: query.Granularity,
		StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp,
		StartDateKey: start.Year()*10000 + int(start.Month())*100 + start.Day(),
		EndDateKey:   end.Year()*10000 + int(end.Month())*100 + end.Day(),
		SiteIDs:      append([]int64(nil), query.SiteIDs...), CustomerIDs: append([]int64(nil), query.CustomerIDs...),
		AccountIDs: append([]int64(nil), query.AccountIDs...), ModelNames: append([]string(nil), query.ModelNames...),
		UseGroups: append([]string(nil), query.UseGroups...), NodeNames: append([]string(nil), query.NodeNames...),
	}
	for _, value := range query.ChannelKeys {
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return model.StatisticsReadRequest{}, ErrStatisticsInvalid
		}
		siteID, siteErr := strconv.ParseInt(parts[0], 10, 64)
		channelID, channelErr := strconv.ParseInt(parts[1], 10, 64)
		if siteErr != nil || channelErr != nil || siteID <= 0 || channelID < 0 {
			return model.StatisticsReadRequest{}, ErrStatisticsInvalid
		}
		request.ChannelKeys = append(request.ChannelKeys, model.StatisticsChannelKey{SiteID: siteID, ChannelID: channelID})
	}
	for _, value := range query.TokenKeys {
		parts := strings.Split(value, ":")
		if len(parts) != 2 {
			return model.StatisticsReadRequest{}, ErrStatisticsInvalid
		}
		siteID, e1 := strconv.ParseInt(parts[0], 10, 64)
		tokenID, e2 := strconv.ParseInt(parts[1], 10, 64)
		if e1 != nil || e2 != nil || siteID <= 0 || tokenID < 0 {
			return model.StatisticsReadRequest{}, ErrStatisticsInvalid
		}
		request.TokenKeys = append(request.TokenKeys, model.StatisticsChannelKey{SiteID: siteID, ChannelID: tokenID})
	}
	return request, nil
}

func statisticsSiteIDs(sites []model.StatisticsSite) []int64 {
	result := make([]int64, len(sites))
	for index := range sites {
		result[index] = sites[index].ID
	}
	return result
}

func statisticsUnitType(scope string) string {
	switch scope {
	case dto.StatisticsScopeGlobal, dto.StatisticsScopeSite, dto.StatisticsScopeModel, dto.StatisticsScopeChannel,
		dto.StatisticsScopeGroup, dto.StatisticsScopeToken, dto.StatisticsScopeNode:
		return "site_hour"
	case dto.StatisticsScopeCustomer:
		return "customer_site_hour"
	default:
		return "hour"
	}
}

func minInt64(first, second int64) int64 {
	if first < second {
		return first
	}
	return second
}

var statisticsLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type statisticsBucket struct {
	Key   int64
	Start int64
	End   int64
}

type statisticsDimension struct {
	Key             string
	ID              string
	Name            string
	SiteID          int64
	CustomerID      int64
	RemoteUserID    int64
	RemoteChannelID int64
	TokenID         int64
	RemoteMissing   bool
	AccountCount    int
	SiteCount       int
}

type statisticsMetric struct {
	RequestCount     int64
	Quota            int64
	TokenUsed        int64
	LastCalculatedAt int64
}

type statisticsMetricKey struct {
	Dimension string
	SiteID    int64
	Bucket    int64
}

type statisticsSiteBucketKey struct {
	SiteID int64
	Bucket int64
}

type statisticsDimensionBucketKey struct {
	Dimension string
	Bucket    int64
}

type statisticsSiteUnits struct {
	Expected int64
	Complete int64
}

type statisticsCoverage struct {
	Expected     int64
	Complete     int64
	Verified     int64
	AsOf         int64
	LastVerified int64
	Statuses     map[string]int64
	Sites        map[int64]*statisticsSiteUnits
}

func newStatisticsCoverage() *statisticsCoverage {
	return &statisticsCoverage{Statuses: make(map[string]int64), Sites: make(map[int64]*statisticsSiteUnits)}
}

func (coverage *statisticsCoverage) add(siteID int64, status string, hourEnd int64, verified bool, verifiedAt *int64) {
	if coverage == nil {
		return
	}
	coverage.Expected++
	coverage.Statuses[status]++
	units := coverage.Sites[siteID]
	if units == nil {
		units = &statisticsSiteUnits{}
		coverage.Sites[siteID] = units
	}
	units.Expected++
	if status == model.CollectionWindowStatusComplete {
		coverage.Complete++
		units.Complete++
		if hourEnd > coverage.AsOf {
			coverage.AsOf = hourEnd
		}
		if verified {
			coverage.Verified++
		}
		if verifiedAt != nil && *verifiedAt > coverage.LastVerified {
			coverage.LastVerified = *verifiedAt
		}
	}
}

func (coverage *statisticsCoverage) merge(source *statisticsCoverage) {
	if coverage == nil || source == nil {
		return
	}
	coverage.Expected += source.Expected
	coverage.Complete += source.Complete
	coverage.Verified += source.Verified
	if source.AsOf > coverage.AsOf {
		coverage.AsOf = source.AsOf
	}
	if source.LastVerified > coverage.LastVerified {
		coverage.LastVerified = source.LastVerified
	}
	for status, count := range source.Statuses {
		coverage.Statuses[status] += count
	}
	for siteID, sourceUnits := range source.Sites {
		units := coverage.Sites[siteID]
		if units == nil {
			units = &statisticsSiteUnits{}
			coverage.Sites[siteID] = units
		}
		units.Expected += sourceUnits.Expected
		units.Complete += sourceUnits.Complete
	}
}

func (coverage *statisticsCoverage) mark(status string) {
	if coverage != nil && status != "" {
		coverage.Statuses[status]++
	}
}

func (coverage *statisticsCoverage) status() string {
	if coverage == nil {
		return model.CollectionWindowStatusPending
	}
	if coverage.Expected == 0 {
		return model.CollectionWindowStatusComplete
	}
	if coverage.Complete == coverage.Expected {
		return model.CollectionWindowStatusComplete
	}
	if coverage.Complete > 0 {
		return model.UsageAggregationStatusPartial
	}
	for _, status := range []string{
		model.CollectionWindowStatusMissing,
		model.CollectionWindowStatusUnavailable,
		constant.SiteStatisticsPaused,
		constant.SiteStatisticsBackfilling,
		model.CollectionWindowStatusPending,
	} {
		if coverage.Statuses[status] > 0 {
			return status
		}
	}
	return model.CollectionWindowStatusPending
}

func (coverage *statisticsCoverage) completenessRate() float64 {
	if coverage == nil || coverage.Expected == 0 {
		return 1
	}
	return float64(coverage.Complete) / float64(coverage.Expected)
}

func (coverage *statisticsCoverage) siteCounts() (int, int) {
	if coverage == nil {
		return 0, 0
	}
	complete := 0
	for _, units := range coverage.Sites {
		if units.Expected > 0 && units.Complete == units.Expected {
			complete++
		}
	}
	return complete, len(coverage.Sites)
}

func (coverage *statisticsCoverage) final(granularity string, bucketEnd, now int64) bool {
	if coverage == nil || coverage.Expected == 0 || coverage.Complete != coverage.Expected {
		return false
	}
	if granularity == dto.StatisticsGranularityHour {
		return true
	}
	return now >= bucketEnd && coverage.Verified == coverage.Expected
}

type statisticsMissingCandidate struct {
	SiteID int64
	HourTS int64
	Status string
	Reason dto.MessageRef
}

type statisticsResponseBuilder struct {
	scope       string
	query       dto.StatisticsQuery
	request     model.StatisticsReadRequest
	now         int64
	buckets     []statisticsBucket
	data        statisticsReadData
	unpaginated bool

	dimensions    []statisticsDimension
	sitesByID     map[int64]model.StatisticsSite
	customers     map[int64]model.StatisticsCustomer
	accounts      map[int64]model.StatisticsAccount
	windows       map[statisticsSiteBucketKey]model.StatisticsWindow
	activeWindows map[statisticsSiteBucketKey]string

	dimensionMetrics map[statisticsDimensionBucketKey]statisticsMetric
	dimensionSites   map[statisticsMetricKey]statisticsMetric
	trendMetrics     map[int64]statisticsMetric
	siteMetrics      map[statisticsSiteBucketKey]statisticsMetric
	summaryMetrics   statisticsMetric

	dimensionActive map[statisticsDimensionBucketKey]int64
	trendActive     map[int64]int64
	siteActive      map[statisticsSiteBucketKey]int64
	summaryActive   int64

	dimensionCoverage map[statisticsDimensionBucketKey]*statisticsCoverage
	dimensionSitesCov map[statisticsMetricKey]*statisticsCoverage
	trendCoverage     map[int64]*statisticsCoverage
	siteCoverage      map[statisticsSiteBucketKey]*statisticsCoverage
	rangeCoverage     *statisticsCoverage
	siteRangeCoverage map[int64]*statisticsCoverage
	missing           []statisticsMissingCandidate
}

func (builder *statisticsResponseBuilder) build() (dto.StatisticsResponse, error) {
	if err := builder.initialize(); err != nil {
		return dto.StatisticsResponse{}, err
	}
	trend := builder.buildTrend()
	breakdown, err := builder.buildBreakdown()
	if err != nil {
		return dto.StatisticsResponse{}, err
	}
	siteBreakdown, err := builder.buildRangeSiteBreakdown()
	if err != nil {
		return dto.StatisticsResponse{}, err
	}
	completeness := builder.buildCompleteness()
	return dto.StatisticsResponse{
		Scope: builder.scope, Granularity: builder.query.Granularity,
		Range: dto.StatisticsRange{
			StartTimestamp: builder.query.StartTimestamp, EndTimestamp: builder.query.EndTimestamp,
			Timezone: "Asia/Shanghai", AsOf: builder.rangeCoverage.AsOf,
		},
		Summary:       builder.buildSummary(),
		Trend:         trend,
		Breakdown:     breakdown,
		SiteBreakdown: siteBreakdown,
		Completeness:  completeness,
	}, nil
}

func newStatisticsBuckets(query dto.StatisticsQuery) ([]statisticsBucket, error) {
	cursor := time.Unix(query.StartTimestamp, 0).In(statisticsLocation)
	end := time.Unix(query.EndTimestamp, 0).In(statisticsLocation)
	result := make([]statisticsBucket, 0)
	for cursor.Before(end) {
		next := cursor
		switch query.Granularity {
		case dto.StatisticsGranularityHour:
			next = cursor.Add(time.Hour)
		case dto.StatisticsGranularityDay:
			next = cursor.AddDate(0, 0, 1)
		case dto.StatisticsGranularityMonth:
			next = cursor.AddDate(0, 1, 0)
		case dto.StatisticsGranularityYear:
			next = cursor.AddDate(1, 0, 0)
		default:
			return nil, ErrStatisticsInvalid
		}
		key := cursor.Unix()
		switch query.Granularity {
		case dto.StatisticsGranularityDay:
			key = int64(cursor.Year()*10000 + int(cursor.Month())*100 + cursor.Day())
		case dto.StatisticsGranularityMonth:
			key = int64(cursor.Year()*100 + int(cursor.Month()))
		case dto.StatisticsGranularityYear:
			key = int64(cursor.Year())
		}
		result = append(result, statisticsBucket{Key: key, Start: cursor.Unix(), End: next.Unix()})
		if len(result) > 2000 || !next.After(cursor) {
			return nil, ErrStatisticsInvalid
		}
		cursor = next
	}
	if cursor.Unix() != query.EndTimestamp {
		return nil, ErrStatisticsInvalid
	}
	return result, nil
}

func statisticsCustomerIDs(customers []model.StatisticsCustomer) []int64 {
	result := make([]int64, len(customers))
	for index := range customers {
		result[index] = customers[index].ID
	}
	return result
}

func statisticsAccountIDs(accounts []model.StatisticsAccount) []int64 {
	result := make([]int64, len(accounts))
	for index := range accounts {
		result[index] = accounts[index].ID
	}
	return result
}

func statisticsAccountCustomerIDs(accounts []model.StatisticsAccount) []int64 {
	seen := make(map[int64]struct{})
	result := make([]int64, 0)
	for _, account := range accounts {
		if _, exists := seen[account.CustomerID]; exists {
			continue
		}
		seen[account.CustomerID] = struct{}{}
		result = append(result, account.CustomerID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func statisticsSitesForAccounts(sites []model.StatisticsSite, accounts []model.StatisticsAccount) []model.StatisticsSite {
	ids := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		ids[account.SiteID] = struct{}{}
	}
	result := make([]model.StatisticsSite, 0, len(ids))
	for _, site := range sites {
		if _, exists := ids[site.ID]; exists {
			result = append(result, site)
		}
	}
	return result
}

func statisticsSitesForChannelKeys(
	sites []model.StatisticsSite,
	keys []model.StatisticsChannelKey,
) []model.StatisticsSite {
	ids := make(map[int64]struct{}, len(keys))
	for _, key := range keys {
		ids[key.SiteID] = struct{}{}
	}
	result := make([]model.StatisticsSite, 0, len(ids))
	for _, site := range sites {
		if _, exists := ids[site.ID]; exists {
			result = append(result, site)
		}
	}
	return result
}

func (builder *statisticsResponseBuilder) initialize() error {
	builder.sitesByID = make(map[int64]model.StatisticsSite, len(builder.data.sites))
	for _, site := range builder.data.sites {
		builder.sitesByID[site.ID] = site
	}
	builder.customers = make(map[int64]model.StatisticsCustomer, len(builder.data.customers))
	for _, customer := range builder.data.customers {
		builder.customers[customer.ID] = customer
	}
	builder.accounts = make(map[int64]model.StatisticsAccount, len(builder.data.accounts))
	for _, account := range builder.data.accounts {
		builder.accounts[account.ID] = account
	}
	builder.windows = make(map[statisticsSiteBucketKey]model.StatisticsWindow, len(builder.data.windows))
	builder.activeWindows = make(map[statisticsSiteBucketKey]string)
	for _, window := range builder.data.windows {
		key := statisticsSiteBucketKey{SiteID: window.SiteID, Bucket: window.HourTS}
		switch window.RowKind {
		case "", "fact":
			if _, exists := builder.windows[key]; exists {
				return model.ErrStatisticsReadContract
			}
			builder.windows[key] = window
		case "active":
			status := model.CollectionWindowStatusPending
			switch window.ActiveTaskType {
			case constant.TaskTypeUsageBackfill:
				status = constant.SiteStatisticsBackfilling
			case constant.TaskTypeUsageHour, constant.TaskTypeUsageValidation:
			default:
				return model.ErrStatisticsReadContract
			}
			if current := builder.activeWindows[key]; current != constant.SiteStatisticsBackfilling {
				builder.activeWindows[key] = status
			}
		default:
			return model.ErrStatisticsReadContract
		}
	}
	builder.dimensionMetrics = make(map[statisticsDimensionBucketKey]statisticsMetric)
	builder.dimensionSites = make(map[statisticsMetricKey]statisticsMetric)
	builder.trendMetrics = make(map[int64]statisticsMetric)
	builder.siteMetrics = make(map[statisticsSiteBucketKey]statisticsMetric)
	builder.dimensionActive = make(map[statisticsDimensionBucketKey]int64)
	builder.trendActive = make(map[int64]int64)
	builder.siteActive = make(map[statisticsSiteBucketKey]int64)
	builder.dimensionCoverage = make(map[statisticsDimensionBucketKey]*statisticsCoverage)
	builder.dimensionSitesCov = make(map[statisticsMetricKey]*statisticsCoverage)
	builder.trendCoverage = make(map[int64]*statisticsCoverage)
	builder.siteCoverage = make(map[statisticsSiteBucketKey]*statisticsCoverage)
	builder.rangeCoverage = newStatisticsCoverage()
	builder.siteRangeCoverage = make(map[int64]*statisticsCoverage)
	if err := builder.buildDimensions(); err != nil {
		return err
	}
	if err := builder.loadMetrics(); err != nil {
		return err
	}
	if err := builder.loadActiveUsers(); err != nil {
		return err
	}
	return builder.loadCoverage()
}

func (builder *statisticsResponseBuilder) buildDimensions() error {
	switch builder.scope {
	case dto.StatisticsScopeGlobal:
		builder.dimensions = []statisticsDimension{{Key: "global", ID: "global", Name: "全局"}}
	case dto.StatisticsScopeSite:
		for _, site := range builder.data.sites {
			id := strconv.FormatInt(site.ID, 10)
			builder.dimensions = append(builder.dimensions, statisticsDimension{Key: id, ID: id, Name: site.Name, SiteID: site.ID})
		}
	case dto.StatisticsScopeCustomer:
		accountCount := make(map[int64]int)
		sites := make(map[int64]map[int64]struct{})
		for _, account := range builder.data.accounts {
			accountCount[account.CustomerID]++
			if sites[account.CustomerID] == nil {
				sites[account.CustomerID] = make(map[int64]struct{})
			}
			sites[account.CustomerID][account.SiteID] = struct{}{}
		}
		for _, customer := range builder.data.customers {
			id := strconv.FormatInt(customer.ID, 10)
			builder.dimensions = append(builder.dimensions, statisticsDimension{
				Key: id, ID: id, Name: customer.Name, CustomerID: customer.ID,
				AccountCount: accountCount[customer.ID], SiteCount: len(sites[customer.ID]),
			})
		}
	case dto.StatisticsScopeAccount:
		for _, account := range builder.data.accounts {
			id := strconv.FormatInt(account.ID, 10)
			name := account.DisplayName
			if name == "" {
				name = account.Username
			}
			builder.dimensions = append(builder.dimensions, statisticsDimension{
				Key: id, ID: id, Name: name, SiteID: account.SiteID, CustomerID: account.CustomerID,
				RemoteUserID: account.RemoteUserID,
			})
		}
	case dto.StatisticsScopeModel:
		seen := make(map[string]struct{})
		appendModel := func(siteID int64, name string) {
			key := statisticsModelDimensionKey(siteID, name)
			if _, exists := seen[key]; exists {
				return
			}
			seen[key] = struct{}{}
			builder.dimensions = append(builder.dimensions, statisticsDimension{
				Key: key, ID: name, Name: name, SiteID: siteID,
			})
		}
		if len(builder.query.ModelNames) > 0 {
			for _, site := range builder.data.sites {
				for _, name := range builder.query.ModelNames {
					appendModel(site.ID, name)
				}
			}
		}
		for _, row := range builder.data.metrics {
			appendModel(row.SiteID, row.DimensionID)
		}
		for _, row := range builder.data.active {
			if row.RowKind == "dimension" {
				appendModel(row.SiteID, row.DimensionID)
			}
		}
	case dto.StatisticsScopeChannel:
		seen := make(map[string]struct{})
		appendChannel := func(siteID, channelID int64, name string, missing bool) {
			key := statisticsChannelDimensionKey(siteID, channelID)
			if _, exists := seen[key]; exists {
				return
			}
			seen[key] = struct{}{}
			if name == "" {
				if channelID == 0 {
					name = "未知通道"
					missing = false
				} else {
					name = "通道 " + strconv.FormatInt(channelID, 10)
					missing = true
				}
			}
			builder.dimensions = append(builder.dimensions, statisticsDimension{
				Key: key, ID: key, Name: name, SiteID: siteID,
				RemoteChannelID: channelID, RemoteMissing: missing,
			})
		}
		for _, channel := range builder.data.channels {
			appendChannel(channel.SiteID, channel.RemoteChannelID, channel.Name, channel.RemoteMissing)
		}
		for _, key := range builder.request.ChannelKeys {
			appendChannel(key.SiteID, key.ChannelID, "", key.ChannelID != 0)
		}
		for _, row := range builder.data.metrics {
			channelID, err := strconv.ParseInt(row.DimensionID, 10, 64)
			if err != nil || channelID < 0 {
				return model.ErrStatisticsReadContract
			}
			appendChannel(row.SiteID, channelID, "", true)
		}
	case dto.StatisticsScopeGroup, dto.StatisticsScopeNode:
		seen := make(map[string]struct{})
		appendValue := func(siteID int64, value string) {
			key := statisticsModelDimensionKey(siteID, value)
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			name := value
			if name == "" {
				if builder.scope == dto.StatisticsScopeGroup {
					name = "未知分组"
				} else {
					name = "未知节点"
				}
			}
			builder.dimensions = append(builder.dimensions, statisticsDimension{Key: key, ID: value, Name: name, SiteID: siteID})
		}
		var filters []string
		if builder.scope == dto.StatisticsScopeGroup {
			filters = builder.query.UseGroups
		} else {
			filters = builder.query.NodeNames
		}
		for _, site := range builder.data.sites {
			for _, value := range filters {
				appendValue(site.ID, value)
			}
		}
		for _, row := range builder.data.metrics {
			appendValue(row.SiteID, row.DimensionID)
		}
		for _, row := range builder.data.active {
			if row.RowKind == "dimension" {
				appendValue(row.SiteID, row.DimensionID)
			}
		}
	case dto.StatisticsScopeToken:
		seen := make(map[string]struct{})
		appendToken := func(siteID, tokenID int64, name string) {
			key := statisticsChannelDimensionKey(siteID, tokenID)
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			if name == "" {
				if tokenID == 0 {
					name = "未知/已删除 Token"
				} else {
					name = "Token " + strconv.FormatInt(tokenID, 10)
				}
			}
			builder.dimensions = append(builder.dimensions, statisticsDimension{Key: key, ID: key, Name: name, SiteID: siteID, TokenID: tokenID})
		}
		for _, key := range builder.request.TokenKeys {
			appendToken(key.SiteID, key.ChannelID, "")
		}
		for _, row := range builder.data.metrics {
			id, err := strconv.ParseInt(row.DimensionID, 10, 64)
			if err != nil || id < 0 {
				return model.ErrStatisticsReadContract
			}
			appendToken(row.SiteID, id, row.DimensionName)
		}
		for _, row := range builder.data.active {
			if row.RowKind == "dimension" {
				id, err := strconv.ParseInt(row.DimensionID, 10, 64)
				if err != nil || id < 0 {
					return model.ErrStatisticsReadContract
				}
				appendToken(row.SiteID, id, "")
			}
		}
	default:
		return ErrStatisticsInvalid
	}
	sort.SliceStable(builder.dimensions, func(left, right int) bool {
		if builder.dimensions[left].Name != builder.dimensions[right].Name {
			return builder.dimensions[left].Name < builder.dimensions[right].Name
		}
		return builder.dimensions[left].Key < builder.dimensions[right].Key
	})
	return nil
}

func statisticsModelDimensionKey(siteID int64, modelName string) string {
	return strconv.FormatInt(siteID, 10) + "\x00" + modelName
}

func statisticsChannelDimensionKey(siteID, channelID int64) string {
	return strconv.FormatInt(siteID, 10) + ":" + strconv.FormatInt(channelID, 10)
}

func (builder *statisticsResponseBuilder) loadMetrics() error {
	bucketStarts := make(map[int64]int64, len(builder.buckets))
	for _, bucket := range builder.buckets {
		bucketStarts[bucket.Key] = bucket.Start
	}
	for _, row := range builder.data.metrics {
		bucketStart, exists := bucketStarts[row.BucketKey]
		if !exists {
			return model.ErrStatisticsReadContract
		}
		dimension := builder.metricDimensionKey(row)
		if dimension == "" {
			return model.ErrStatisticsReadContract
		}
		value, err := statisticsMetricFromRow(row)
		if err != nil {
			return err
		}
		dimensionKey := statisticsDimensionBucketKey{Dimension: dimension, Bucket: bucketStart}
		current := builder.dimensionMetrics[dimensionKey]
		if err := current.add(value); err != nil {
			return err
		}
		builder.dimensionMetrics[dimensionKey] = current
		dimensionSiteKey := statisticsMetricKey{Dimension: dimension, SiteID: row.SiteID, Bucket: bucketStart}
		current = builder.dimensionSites[dimensionSiteKey]
		if err := current.add(value); err != nil {
			return err
		}
		builder.dimensionSites[dimensionSiteKey] = current
		current = builder.trendMetrics[bucketStart]
		if err := current.add(value); err != nil {
			return err
		}
		builder.trendMetrics[bucketStart] = current
		siteKey := statisticsSiteBucketKey{SiteID: row.SiteID, Bucket: bucketStart}
		current = builder.siteMetrics[siteKey]
		if err := current.add(value); err != nil {
			return err
		}
		builder.siteMetrics[siteKey] = current
		if err := builder.summaryMetrics.add(value); err != nil {
			return err
		}
	}
	return nil
}

func (builder *statisticsResponseBuilder) metricDimensionKey(row model.StatisticsMetricRow) string {
	switch builder.scope {
	case dto.StatisticsScopeGlobal:
		return "global"
	case dto.StatisticsScopeSite, dto.StatisticsScopeCustomer, dto.StatisticsScopeAccount:
		return row.DimensionID
	case dto.StatisticsScopeModel:
		return statisticsModelDimensionKey(row.SiteID, row.DimensionID)
	case dto.StatisticsScopeChannel:
		channelID, err := strconv.ParseInt(row.DimensionID, 10, 64)
		if err != nil || channelID < 0 {
			return ""
		}
		return statisticsChannelDimensionKey(row.SiteID, channelID)
	case dto.StatisticsScopeGroup, dto.StatisticsScopeNode:
		return statisticsModelDimensionKey(row.SiteID, row.DimensionID)
	case dto.StatisticsScopeToken:
		id, err := strconv.ParseInt(row.DimensionID, 10, 64)
		if err != nil || id < 0 {
			return ""
		}
		return statisticsChannelDimensionKey(row.SiteID, id)
	default:
		return ""
	}
}

func statisticsMetricFromRow(row model.StatisticsMetricRow) (statisticsMetric, error) {
	requestCount, err := statisticsParseMetric(row.RequestCount)
	if err != nil {
		return statisticsMetric{}, err
	}
	quota, err := statisticsParseMetric(row.Quota)
	if err != nil {
		return statisticsMetric{}, err
	}
	tokenUsed, err := statisticsParseMetric(row.TokenUsed)
	if err != nil {
		return statisticsMetric{}, err
	}
	if row.LastCalculatedAt < 0 {
		return statisticsMetric{}, model.ErrStatisticsReadContract
	}
	return statisticsMetric{
		RequestCount: requestCount, Quota: quota, TokenUsed: tokenUsed,
		LastCalculatedAt: row.LastCalculatedAt,
	}, nil
}

func statisticsParseMetric(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, model.ErrStatisticsReadContract
	}
	return parsed, nil
}

func (metric *statisticsMetric) add(value statisticsMetric) error {
	var ok bool
	metric.RequestCount, ok = statisticsCheckedAdd(metric.RequestCount, value.RequestCount)
	if !ok {
		return model.ErrStatisticsReadContract
	}
	metric.Quota, ok = statisticsCheckedAdd(metric.Quota, value.Quota)
	if !ok {
		return model.ErrStatisticsReadContract
	}
	metric.TokenUsed, ok = statisticsCheckedAdd(metric.TokenUsed, value.TokenUsed)
	if !ok {
		return model.ErrStatisticsReadContract
	}
	if value.LastCalculatedAt > metric.LastCalculatedAt {
		metric.LastCalculatedAt = value.LastCalculatedAt
	}
	return nil
}

func statisticsCheckedAdd(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || left > math.MaxInt64-right {
		return 0, false
	}
	return left + right, true
}

func (builder *statisticsResponseBuilder) loadActiveUsers() error {
	bucketStarts := make(map[int64]int64, len(builder.buckets))
	for _, bucket := range builder.buckets {
		bucketStarts[bucket.Key] = bucket.Start
	}
	for _, row := range builder.data.active {
		value, err := statisticsParseMetric(row.ActiveUsers)
		if err != nil {
			return err
		}
		switch row.RowKind {
		case "dimension":
			bucketStart, exists := bucketStarts[row.BucketKey]
			if !exists {
				return model.ErrStatisticsReadContract
			}
			dimension := builder.activeDimensionKey(row)
			if dimension == "" {
				return model.ErrStatisticsReadContract
			}
			key := statisticsDimensionBucketKey{Dimension: dimension, Bucket: bucketStart}
			current, ok := statisticsCheckedAdd(builder.dimensionActive[key], value)
			if !ok {
				return model.ErrStatisticsReadContract
			}
			builder.dimensionActive[key] = current
		case "trend":
			bucketStart, exists := bucketStarts[row.BucketKey]
			if !exists {
				return model.ErrStatisticsReadContract
			}
			builder.trendActive[bucketStart] = value
		case "site":
			bucketStart, exists := bucketStarts[row.BucketKey]
			if !exists || row.SiteID <= 0 {
				return model.ErrStatisticsReadContract
			}
			builder.siteActive[statisticsSiteBucketKey{SiteID: row.SiteID, Bucket: bucketStart}] = value
		case "summary":
			builder.summaryActive = value
		default:
			return model.ErrStatisticsReadContract
		}
	}
	return nil
}

func (builder *statisticsResponseBuilder) activeDimensionKey(row model.StatisticsActiveRow) string {
	switch builder.scope {
	case dto.StatisticsScopeGlobal:
		return "global"
	case dto.StatisticsScopeSite, dto.StatisticsScopeCustomer, dto.StatisticsScopeAccount:
		return row.DimensionID
	case dto.StatisticsScopeModel:
		return statisticsModelDimensionKey(row.SiteID, row.DimensionID)
	case dto.StatisticsScopeChannel:
		channelID, err := strconv.ParseInt(row.DimensionID, 10, 64)
		if err != nil || channelID < 0 {
			return ""
		}
		return statisticsChannelDimensionKey(row.SiteID, channelID)
	case dto.StatisticsScopeGroup, dto.StatisticsScopeNode:
		return statisticsModelDimensionKey(row.SiteID, row.DimensionID)
	case dto.StatisticsScopeToken:
		id, err := strconv.ParseInt(row.DimensionID, 10, 64)
		if err != nil || id < 0 {
			return ""
		}
		return statisticsChannelDimensionKey(row.SiteID, id)
	default:
		return ""
	}
}

func (builder *statisticsResponseBuilder) loadCoverage() error {
	switch builder.scope {
	case dto.StatisticsScopeGlobal, dto.StatisticsScopeSite, dto.StatisticsScopeModel, dto.StatisticsScopeChannel,
		dto.StatisticsScopeGroup, dto.StatisticsScopeToken, dto.StatisticsScopeNode:
		return builder.loadSiteCoverage()
	case dto.StatisticsScopeCustomer:
		return builder.loadCustomerCoverage()
	case dto.StatisticsScopeAccount:
		return builder.loadAccountCoverage()
	default:
		return ErrStatisticsInvalid
	}
}

func (builder *statisticsResponseBuilder) loadSiteCoverage() error {
	base := make(map[statisticsSiteBucketKey]*statisticsCoverage)
	for _, site := range builder.data.sites {
		for _, bucket := range builder.buckets {
			coverage := newStatisticsCoverage()
			for hour := bucket.Start; hour < minInt64(bucket.End, floorHour(builder.now)); hour += 3600 {
				if !statisticsSiteExpected(site, hour) {
					continue
				}
				status, verified, window := builder.windowState(site, hour)
				coverage.add(site.ID, status, hour+3600, verified, window.VerifiedAt)
				if status != model.CollectionWindowStatusComplete {
					builder.appendMissing(site, hour, status, window, "site", site.ID)
				}
			}
			key := statisticsSiteBucketKey{SiteID: site.ID, Bucket: bucket.Start}
			base[key] = coverage
			builder.siteCoverage[key] = coverage
			builder.coverageForTrend(bucket.Start).merge(coverage)
			builder.rangeCoverage.merge(coverage)
			builder.coverageForSiteRange(site.ID).merge(coverage)
		}
	}
	for _, dimension := range builder.dimensions {
		switch builder.scope {
		case dto.StatisticsScopeGlobal:
			for _, site := range builder.data.sites {
				for _, bucket := range builder.buckets {
					coverage := base[statisticsSiteBucketKey{SiteID: site.ID, Bucket: bucket.Start}]
					builder.coverageForDimension(dimension.Key, bucket.Start).merge(coverage)
					builder.coverageForDimensionSite(dimension.Key, site.ID, bucket.Start).merge(coverage)
				}
			}
		case dto.StatisticsScopeSite, dto.StatisticsScopeModel, dto.StatisticsScopeChannel,
			dto.StatisticsScopeGroup, dto.StatisticsScopeToken, dto.StatisticsScopeNode:
			for _, bucket := range builder.buckets {
				coverage := base[statisticsSiteBucketKey{SiteID: dimension.SiteID, Bucket: bucket.Start}]
				builder.coverageForDimension(dimension.Key, bucket.Start).merge(coverage)
				builder.coverageForDimensionSite(dimension.Key, dimension.SiteID, bucket.Start).merge(coverage)
			}
		}
	}
	return nil
}

func (builder *statisticsResponseBuilder) loadCustomerCoverage() error {
	groups := make(map[int64]map[int64][]model.StatisticsAccount)
	for _, account := range builder.data.accounts {
		if groups[account.CustomerID] == nil {
			groups[account.CustomerID] = make(map[int64][]model.StatisticsAccount)
		}
		groups[account.CustomerID][account.SiteID] = append(groups[account.CustomerID][account.SiteID], account)
	}
	for _, dimension := range builder.dimensions {
		customer := builder.customers[dimension.CustomerID]
		for siteID, accounts := range groups[dimension.CustomerID] {
			site, exists := builder.sitesByID[siteID]
			if !exists {
				continue
			}
			for _, bucket := range builder.buckets {
				coverage := newStatisticsCoverage()
				for hour := bucket.Start; hour < minInt64(bucket.End, floorHour(builder.now)); hour += 3600 {
					if !statisticsSiteExpected(site, hour) {
						continue
					}
					if customer.StatisticsPausedAt != nil && hour >= *customer.StatisticsPausedAt {
						builder.markPaused(dimension.Key, site.ID, bucket.Start, hour, "customer", customer.ID)
						continue
					}
					expected := false
					registered := false
					for _, account := range accounts {
						if account.RemoteCreatedAt < hour+3600 {
							registered = true
						}
						if statisticsAccountExpected(account, hour) {
							expected = true
							break
						}
					}
					if !expected {
						if registered {
							builder.markPaused(dimension.Key, site.ID, bucket.Start, hour, "customer", customer.ID)
						}
						continue
					}
					status, verified, window := builder.windowState(site, hour)
					coverage.add(site.ID, status, hour+3600, verified, window.VerifiedAt)
					builder.addCoverageUnit(dimension.Key, site.ID, bucket.Start, status, hour+3600, verified, window.VerifiedAt)
					if status != model.CollectionWindowStatusComplete {
						builder.appendMissing(site, hour, status, window, "customer", customer.ID)
					}
				}
				builder.coverageForDimension(dimension.Key, bucket.Start).merge(coverage)
				builder.coverageForDimensionSite(dimension.Key, site.ID, bucket.Start).merge(coverage)
			}
		}
	}
	return nil
}

func (builder *statisticsResponseBuilder) loadAccountCoverage() error {
	for _, dimension := range builder.dimensions {
		account := builder.accounts[mustStatisticsID(dimension.ID)]
		site, exists := builder.sitesByID[account.SiteID]
		if !exists {
			continue
		}
		for _, bucket := range builder.buckets {
			coverage := newStatisticsCoverage()
			for hour := bucket.Start; hour < minInt64(bucket.End, floorHour(builder.now)); hour += 3600 {
				if !statisticsSiteExpected(site, hour) || account.RemoteCreatedAt >= hour+3600 {
					continue
				}
				if account.StatisticsPausedAt != nil && hour >= *account.StatisticsPausedAt {
					builder.markPaused(dimension.Key, site.ID, bucket.Start, hour, "account", account.ID)
					continue
				}
				status, verified, window := builder.windowState(site, hour)
				coverage.add(site.ID, status, hour+3600, verified, window.VerifiedAt)
				builder.addCoverageUnit(dimension.Key, site.ID, bucket.Start, status, hour+3600, verified, window.VerifiedAt)
				if status != model.CollectionWindowStatusComplete {
					builder.appendMissing(site, hour, status, window, "account", account.ID)
				}
			}
			builder.coverageForDimension(dimension.Key, bucket.Start).merge(coverage)
			builder.coverageForDimensionSite(dimension.Key, site.ID, bucket.Start).merge(coverage)
		}
	}
	return nil
}

func statisticsSiteExpected(site model.StatisticsSite, hour int64) bool {
	return site.StatisticsStartAt != nil && *site.StatisticsStartAt < hour+3600 &&
		(site.StatisticsEndAt == nil || *site.StatisticsEndAt > hour)
}

func statisticsAccountExpected(account model.StatisticsAccount, hour int64) bool {
	return account.RemoteCreatedAt < hour+3600 &&
		(account.StatisticsPausedAt == nil || hour < *account.StatisticsPausedAt)
}

func (builder *statisticsResponseBuilder) windowState(
	site model.StatisticsSite,
	hour int64,
) (string, bool, model.StatisticsWindow) {
	key := statisticsSiteBucketKey{SiteID: site.ID, Bucket: hour}
	window := builder.windows[key]
	status := window.Status
	if status != "" && status != model.CollectionWindowStatusComplete && status != model.CollectionWindowStatusMissing &&
		status != model.CollectionWindowStatusUnavailable && status != model.CollectionWindowStatusPending {
		status = model.CollectionWindowStatusPending
	}
	if status == model.CollectionWindowStatusComplete || status == model.CollectionWindowStatusUnavailable {
		return status, builder.windowVerified(hour, window), window
	}
	if site.ManagementStatus == constant.SiteManagementDisabled && site.StatisticsEndAt == nil &&
		site.DisabledAt != nil && hour >= floorHour(*site.DisabledAt) {
		return constant.SiteStatisticsPaused, false, model.StatisticsWindow{}
	}
	if activeStatus := builder.activeWindows[key]; activeStatus != "" {
		return activeStatus, false, model.StatisticsWindow{}
	}
	if status == model.CollectionWindowStatusMissing {
		return status, false, window
	}
	deadlineMinutes := builder.data.rates.UsageDelayMinutes + 10
	if deadlineMinutes < 15 {
		deadlineMinutes = 15
	}
	if builder.now >= hour+3600+int64(deadlineMinutes*60) {
		return model.CollectionWindowStatusMissing, false, window
	}
	if site.StatisticsStatus == constant.SiteStatisticsBackfilling {
		return constant.SiteStatisticsBackfilling, false, model.StatisticsWindow{}
	}
	return model.CollectionWindowStatusPending, false, model.StatisticsWindow{}
}

func (builder *statisticsResponseBuilder) windowVerified(hour int64, window model.StatisticsWindow) bool {
	if window.Status != model.CollectionWindowStatusComplete {
		return false
	}
	if builder.query.Granularity == dto.StatisticsGranularityHour {
		return true
	}
	local := time.Unix(hour, 0).In(statisticsLocation)
	dateEnd := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, statisticsLocation).Unix()
	return window.VerifiedAt != nil && *window.VerifiedAt >= dateEnd
}

func (builder *statisticsResponseBuilder) addCoverageUnit(
	dimension string,
	siteID int64,
	bucketStart int64,
	status string,
	hourEnd int64,
	verified bool,
	verifiedAt *int64,
) {
	builder.coverageForTrend(bucketStart).add(siteID, status, hourEnd, verified, verifiedAt)
	builder.rangeCoverage.add(siteID, status, hourEnd, verified, verifiedAt)
	builder.coverageForSiteBucket(siteID, bucketStart).add(siteID, status, hourEnd, verified, verifiedAt)
	builder.coverageForSiteRange(siteID).add(siteID, status, hourEnd, verified, verifiedAt)
}

func (builder *statisticsResponseBuilder) markPaused(
	dimension string,
	siteID int64,
	bucketStart int64,
	hour int64,
	scope string,
	scopeID int64,
) {
	status := constant.SiteStatisticsPaused
	hourEnd := hour + 3600
	builder.coverageForDimension(dimension, bucketStart).add(siteID, status, hourEnd, false, nil)
	builder.coverageForDimensionSite(dimension, siteID, bucketStart).add(siteID, status, hourEnd, false, nil)
	builder.coverageForTrend(bucketStart).add(siteID, status, hourEnd, false, nil)
	builder.coverageForSiteBucket(siteID, bucketStart).add(siteID, status, hourEnd, false, nil)
	builder.rangeCoverage.add(siteID, status, hourEnd, false, nil)
	builder.coverageForSiteRange(siteID).add(siteID, status, hourEnd, false, nil)
	site := builder.sitesByID[siteID]
	builder.appendMissing(site, hour, status, model.StatisticsWindow{}, scope, scopeID)
}

func (builder *statisticsResponseBuilder) coverageForDimension(dimension string, bucket int64) *statisticsCoverage {
	key := statisticsDimensionBucketKey{Dimension: dimension, Bucket: bucket}
	if builder.dimensionCoverage[key] == nil {
		builder.dimensionCoverage[key] = newStatisticsCoverage()
	}
	return builder.dimensionCoverage[key]
}

func (builder *statisticsResponseBuilder) coverageForDimensionSite(
	dimension string,
	siteID, bucket int64,
) *statisticsCoverage {
	key := statisticsMetricKey{Dimension: dimension, SiteID: siteID, Bucket: bucket}
	if builder.dimensionSitesCov[key] == nil {
		builder.dimensionSitesCov[key] = newStatisticsCoverage()
	}
	return builder.dimensionSitesCov[key]
}

func (builder *statisticsResponseBuilder) coverageForTrend(bucket int64) *statisticsCoverage {
	if builder.trendCoverage[bucket] == nil {
		builder.trendCoverage[bucket] = newStatisticsCoverage()
	}
	return builder.trendCoverage[bucket]
}

func (builder *statisticsResponseBuilder) coverageForSiteBucket(siteID, bucket int64) *statisticsCoverage {
	key := statisticsSiteBucketKey{SiteID: siteID, Bucket: bucket}
	if builder.siteCoverage[key] == nil {
		builder.siteCoverage[key] = newStatisticsCoverage()
	}
	return builder.siteCoverage[key]
}

func (builder *statisticsResponseBuilder) coverageForSiteRange(siteID int64) *statisticsCoverage {
	if builder.siteRangeCoverage[siteID] == nil {
		builder.siteRangeCoverage[siteID] = newStatisticsCoverage()
	}
	return builder.siteRangeCoverage[siteID]
}

func mustStatisticsID(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func (builder *statisticsResponseBuilder) appendMissing(
	site model.StatisticsSite,
	hour int64,
	status string,
	window model.StatisticsWindow,
	scope string,
	scopeID int64,
) {
	reason := builder.statisticsReason(site, hour, status, window, scope, scopeID)
	builder.missing = append(builder.missing, statisticsMissingCandidate{
		SiteID: site.ID, HourTS: hour, Status: status, Reason: reason,
	})
}

func (builder *statisticsResponseBuilder) statisticsReason(
	site model.StatisticsSite,
	hour int64,
	status string,
	window model.StatisticsWindow,
	scope string,
	scopeID int64,
) dto.MessageRef {
	if window.LastErrorCode != "" {
		code := constant.MessageCode(window.LastErrorCode)
		if _, exists := constant.MessageRegistry[code]; exists {
			params := map[string]any{}
			if len(window.LastErrorParams) == 0 || common.Unmarshal(window.LastErrorParams, &params) == nil {
				if dto.ValidateMessageParams(code, params) == nil {
					return dto.MessageRef{Code: code, Params: params, TechnicalDetail: ""}
				}
			}
		}
	}
	siteID := strconv.FormatInt(site.ID, 10)
	start := hour
	end := hour + 3600
	switch status {
	case model.CollectionWindowStatusMissing:
		return dto.MustMessageRef(constant.MessageDataWindowMissing, map[string]any{
			"site_id": siteID, "start_timestamp": start, "end_timestamp": end,
		}, "")
	case model.CollectionWindowStatusUnavailable:
		return dto.MustMessageRef(constant.MessageDataUpstreamUnavailable, map[string]any{
			"site_id": siteID, "start_timestamp": start, "end_timestamp": end,
		}, "")
	case constant.SiteStatisticsPaused:
		if scopeID <= 0 || scope == "global" || scope == "model" || scope == "channel" {
			scope = "site"
			scopeID = site.ID
		}
		return dto.MustMessageRef(constant.MessageDataScopePaused, map[string]any{
			"scope_type": scope, "scope_id": strconv.FormatInt(scopeID, 10),
			"start_timestamp": start, "end_timestamp": end,
		}, "")
	case constant.SiteStatisticsBackfilling:
		return dto.MustMessageRef(constant.MessageDataBackfilling, map[string]any{
			"scope_type": "site", "scope_id": siteID, "progress": float64(0),
		}, "")
	default:
		var id any
		if scopeID > 0 && scope != "global" && scope != "model" && scope != "channel" {
			id = strconv.FormatInt(scopeID, 10)
		}
		return dto.MustMessageRef(constant.MessageDataPending, map[string]any{
			"scope_type": scope, "scope_id": id, "progress": float64(0),
		}, "")
	}
}

func (builder *statisticsResponseBuilder) missingRanges() ([]dto.MissingRange, int, bool, []string) {
	type key struct {
		SiteID int64
		Hour   int64
		Status string
		Code   constant.MessageCode
	}
	deduplicated := make(map[key]statisticsMissingCandidate)
	missingSites := make(map[int64]struct{})
	for _, candidate := range builder.missing {
		if candidate.SiteID <= 0 || candidate.HourTS < builder.query.StartTimestamp || candidate.HourTS >= builder.query.EndTimestamp {
			continue
		}
		itemKey := key{SiteID: candidate.SiteID, Hour: candidate.HourTS, Status: candidate.Status, Code: candidate.Reason.Code}
		if _, exists := deduplicated[itemKey]; !exists {
			deduplicated[itemKey] = candidate
		}
		missingSites[candidate.SiteID] = struct{}{}
	}
	items := make([]statisticsMissingCandidate, 0, len(deduplicated))
	for _, candidate := range deduplicated {
		items = append(items, candidate)
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].SiteID != items[right].SiteID {
			return items[left].SiteID < items[right].SiteID
		}
		if items[left].Status != items[right].Status {
			return items[left].Status < items[right].Status
		}
		if items[left].Reason.Code != items[right].Reason.Code {
			return items[left].Reason.Code < items[right].Reason.Code
		}
		return items[left].HourTS < items[right].HourTS
	})
	ranges := make([]dto.MissingRange, 0)
	for _, item := range items {
		last := len(ranges) - 1
		if last >= 0 && ranges[last].SiteID == strconv.FormatInt(item.SiteID, 10) &&
			ranges[last].Status == item.Status && ranges[last].Reason.Code == item.Reason.Code &&
			ranges[last].EndTimestamp == item.HourTS {
			ranges[last].EndTimestamp = item.HourTS + 3600
			continue
		}
		ranges = append(ranges, dto.MissingRange{
			SiteID: strconv.FormatInt(item.SiteID, 10), Status: item.Status,
			StartTimestamp: item.HourTS, EndTimestamp: item.HourTS + 3600, Reason: item.Reason,
		})
	}
	total := len(ranges)
	sort.Slice(ranges, func(left, right int) bool {
		if ranges[left].StartTimestamp != ranges[right].StartTimestamp {
			return ranges[left].StartTimestamp > ranges[right].StartTimestamp
		}
		return ranges[left].SiteID < ranges[right].SiteID
	})
	truncated := total > 100
	if truncated {
		ranges = ranges[:100]
	}
	siteIDs := make([]int64, 0, len(missingSites))
	for siteID := range missingSites {
		siteIDs = append(siteIDs, siteID)
	}
	sort.Slice(siteIDs, func(left, right int) bool { return siteIDs[left] < siteIDs[right] })
	encodedSiteIDs := make([]string, len(siteIDs))
	for index, siteID := range siteIDs {
		encodedSiteIDs[index] = strconv.FormatInt(siteID, 10)
	}
	return ranges, total, truncated, encodedSiteIDs
}

func (builder *statisticsResponseBuilder) buildSummary() dto.StatisticsSummary {
	requestCount, quota, tokenUsed, activeUsers := builder.metricPointers(
		builder.summaryMetrics, builder.rangeCoverage, builder.summaryActive,
	)
	status := builder.rangeCoverage.status()
	return dto.StatisticsSummary{
		RequestCount: requestCount, Quota: quota, TokenUsed: tokenUsed, ActiveUsers: activeUsers,
		DataStatus: status, IsPartial: status == model.UsageAggregationStatusPartial,
	}
}

func (builder *statisticsResponseBuilder) buildTrend() []dto.TrendPoint {
	result := make([]dto.TrendPoint, 0, len(builder.buckets))
	for _, bucket := range builder.buckets {
		coverage := builder.coverageForTrend(bucket.Start)
		metric := builder.trendMetrics[bucket.Start]
		requestCount, quota, tokenUsed, activeUsers := builder.metricPointers(metric, coverage, builder.trendActive[bucket.Start])
		completeSites, expectedSites := coverage.siteCounts()
		var asOf *int64
		if coverage.AsOf > 0 {
			value := coverage.AsOf
			asOf = &value
		}
		result = append(result, dto.TrendPoint{
			BucketStart: bucket.Start, BucketEnd: bucket.End,
			RequestCount: requestCount, Quota: quota, TokenUsed: tokenUsed, ActiveUsers: activeUsers,
			DataStatus: coverage.status(), IsFinal: coverage.final(builder.query.Granularity, bucket.End, builder.now),
			AsOf: asOf, CompleteSiteCount: completeSites, ExpectedSiteCount: expectedSites,
			SiteBreakdown: builder.buildBucketSiteBreakdown(bucket.Start),
			Reason:        builder.reasonForBucket(bucket, coverage),
		})
	}
	return result
}

func (builder *statisticsResponseBuilder) metricPointers(
	metric statisticsMetric,
	coverage *statisticsCoverage,
	active int64,
) (*string, *string, *string, *string) {
	if coverage == nil || coverage.Complete == 0 {
		return nil, nil, nil, nil
	}
	requestCount := strconv.FormatInt(metric.RequestCount, 10)
	quota := strconv.FormatInt(metric.Quota, 10)
	tokenUsed := strconv.FormatInt(metric.TokenUsed, 10)
	if builder.scope == dto.StatisticsScopeAccount && coverage.Complete != coverage.Expected {
		return &requestCount, &quota, &tokenUsed, nil
	}
	activeUsers := strconv.FormatInt(active, 10)
	return &requestCount, &quota, &tokenUsed, &activeUsers
}

func (builder *statisticsResponseBuilder) reasonForBucket(
	bucket statisticsBucket,
	coverage *statisticsCoverage,
) *dto.MessageRef {
	status := coverage.status()
	if status == model.CollectionWindowStatusComplete {
		return nil
	}
	if status == model.UsageAggregationStatusPartial {
		completeSites, expectedSites := coverage.siteCounts()
		reason := dto.MustMessageRef(constant.MessageDataPartialSites, map[string]any{
			"complete_site_count": completeSites, "expected_site_count": expectedSites,
		}, "")
		return &reason
	}
	priority := statisticsStatusPriority(status)
	for _, candidate := range builder.missing {
		if candidate.HourTS >= bucket.Start && candidate.HourTS < bucket.End &&
			statisticsStatusPriority(candidate.Status) == priority {
			reason := candidate.Reason
			return &reason
		}
	}
	var id any
	if len(builder.dimensions) == 1 {
		if parsed := mustStatisticsID(builder.dimensions[0].ID); parsed > 0 {
			id = strconv.FormatInt(parsed, 10)
		}
	}
	code := constant.MessageDataPending
	if status == constant.SiteStatisticsBackfilling {
		code = constant.MessageDataBackfilling
	}
	reason := dto.MustMessageRef(code, map[string]any{
		"scope_type": builder.scope, "scope_id": id, "progress": float64(0),
	}, "")
	return &reason
}

func statisticsStatusPriority(status string) int {
	switch status {
	case model.CollectionWindowStatusMissing:
		return 5
	case model.CollectionWindowStatusUnavailable:
		return 4
	case constant.SiteStatisticsPaused:
		return 3
	case constant.SiteStatisticsBackfilling:
		return 2
	case model.CollectionWindowStatusPending:
		return 1
	default:
		return 0
	}
}

func (builder *statisticsResponseBuilder) buildBucketSiteBreakdown(bucket int64) []dto.SiteQuotaBreakdown {
	result := make([]dto.SiteQuotaBreakdown, 0, len(builder.data.sites))
	for _, site := range builder.data.sites {
		key := statisticsSiteBucketKey{SiteID: site.ID, Bucket: bucket}
		coverage := builder.siteCoverage[key]
		metric, metricExists := builder.siteMetrics[key]
		if coverage == nil && !metricExists {
			continue
		}
		result = append(result, builder.siteQuotaBreakdown(site, metric, coverage))
	}
	return result
}

func (builder *statisticsResponseBuilder) buildDimensionSiteBreakdown(
	dimension string,
	bucket int64,
) []dto.SiteQuotaBreakdown {
	result := make([]dto.SiteQuotaBreakdown, 0)
	for _, site := range builder.data.sites {
		key := statisticsMetricKey{Dimension: dimension, SiteID: site.ID, Bucket: bucket}
		coverage := builder.dimensionSitesCov[key]
		metric, metricExists := builder.dimensionSites[key]
		if coverage == nil && !metricExists {
			continue
		}
		result = append(result, builder.siteQuotaBreakdown(site, metric, coverage))
	}
	return result
}

func (builder *statisticsResponseBuilder) buildRangeSiteBreakdown() ([]dto.SiteQuotaBreakdown, error) {
	metrics := make(map[int64]statisticsMetric)
	for key, metric := range builder.siteMetrics {
		current := metrics[key.SiteID]
		if err := current.add(metric); err != nil {
			return nil, err
		}
		metrics[key.SiteID] = current
	}
	result := make([]dto.SiteQuotaBreakdown, 0, len(builder.data.sites))
	for _, site := range builder.data.sites {
		coverage := builder.siteRangeCoverage[site.ID]
		metric, metricExists := metrics[site.ID]
		if coverage == nil && !metricExists {
			continue
		}
		result = append(result, builder.siteQuotaBreakdown(site, metric, coverage))
	}
	return result, nil
}

func (builder *statisticsResponseBuilder) siteQuotaBreakdown(
	site model.StatisticsSite,
	metric statisticsMetric,
	coverage *statisticsCoverage,
) dto.SiteQuotaBreakdown {
	var quota *string
	if coverage != nil && coverage.Complete > 0 {
		value := strconv.FormatInt(metric.Quota, 10)
		quota = &value
	}
	rate := statisticsEffectiveRate(site, builder.data.rates)
	status := model.CollectionWindowStatusPending
	if coverage != nil {
		status = coverage.status()
	}
	return dto.SiteQuotaBreakdown{
		SiteID: strconv.FormatInt(site.ID, 10), SiteName: site.Name, Quota: quota,
		QuotaPerUnit: rate.QuotaPerUnit, USDExchangeRate: rate.USDExchangeRate,
		RateSource: rate.Source, RateUpdatedAt: rate.UpdatedAt, DataStatus: status,
	}
}

func statisticsEffectiveRate(site model.StatisticsSite, fallback model.StatisticsFallbackRates) dto.RateInfo {
	if site.LastRateAt != nil && statisticsPositiveDecimal(site.QuotaPerUnit) && statisticsPositiveDecimal(site.USDExchangeRate) {
		return dto.RateInfo{
			QuotaPerUnit: site.QuotaPerUnit, USDExchangeRate: site.USDExchangeRate,
			Source: "site", UpdatedAt: site.LastRateAt,
		}
	}
	if site.LastRateAt == nil && statisticsPositiveDecimalString(fallback.QuotaPerUnit) &&
		statisticsPositiveDecimalString(fallback.USDExchangeRate) {
		quotaPerUnit := fallback.QuotaPerUnit
		exchangeRate := fallback.USDExchangeRate
		return dto.RateInfo{
			QuotaPerUnit: &quotaPerUnit, USDExchangeRate: &exchangeRate, Source: "fallback",
		}
	}
	return dto.RateInfo{Source: "unavailable"}
}

func statisticsPositiveDecimal(value *string) bool {
	return value != nil && statisticsPositiveDecimalString(*value)
}

func statisticsPositiveDecimalString(value string) bool {
	if value == "" {
		return false
	}
	parsed, ok := new(big.Rat).SetString(value)
	return ok && parsed.Sign() > 0
}

type statisticsBreakdownRow struct {
	Item        dto.StatisticsBreakdownItem
	Key         string
	Name        string
	BucketStart int64
	Metric      statisticsMetric
	ActiveUsers int64
	Known       bool
}

func (builder *statisticsResponseBuilder) buildBreakdown() (
	common.PageData[dto.StatisticsBreakdownItem],
	error,
) {
	rows := make([]statisticsBreakdownRow, 0, len(builder.dimensions)*len(builder.buckets))
	for _, dimension := range builder.dimensions {
		for _, bucket := range builder.buckets {
			key := statisticsDimensionBucketKey{Dimension: dimension.Key, Bucket: bucket.Start}
			coverage := builder.dimensionCoverage[key]
			if coverage == nil {
				coverage = newStatisticsCoverage()
			}
			metric := builder.dimensionMetrics[key]
			active := builder.dimensionActive[key]
			item, err := builder.breakdownItem(dimension, bucket, metric, active, coverage)
			if err != nil {
				return common.PageData[dto.StatisticsBreakdownItem]{}, err
			}
			rows = append(rows, statisticsBreakdownRow{
				Item: item, Key: dimension.Key, Name: dimension.Name, BucketStart: bucket.Start,
				Metric: metric, ActiveUsers: active, Known: coverage.Complete > 0,
			})
		}
	}
	sort.SliceStable(rows, func(left, right int) bool {
		return builder.breakdownLess(rows[left], rows[right])
	})
	total := int64(len(rows))
	start, end := 0, len(rows)
	page, pageSize := 1, len(rows)
	if !builder.unpaginated {
		start = builder.query.Offset()
		if start > len(rows) {
			start = len(rows)
		}
		end = start + builder.query.PageSize
		if end > len(rows) {
			end = len(rows)
		}
		page, pageSize = builder.query.Page, builder.query.PageSize
	} else if pageSize == 0 {
		pageSize = 1
	}
	items := make([]dto.StatisticsBreakdownItem, 0, end-start)
	for _, row := range rows[start:end] {
		items = append(items, row.Item)
	}
	return common.NewPageData(page, pageSize, total, items), nil
}

func (builder *statisticsResponseBuilder) breakdownItem(
	dimension statisticsDimension,
	bucket statisticsBucket,
	metric statisticsMetric,
	active int64,
	coverage *statisticsCoverage,
) (dto.StatisticsBreakdownItem, error) {
	requestCount, quota, tokenUsed, activeUsers := builder.metricPointers(metric, coverage, active)
	var siteID, siteName *string
	if dimension.SiteID > 0 {
		id := strconv.FormatInt(dimension.SiteID, 10)
		name := builder.sitesByID[dimension.SiteID].Name
		siteID = &id
		siteName = &name
	}
	var asOf *int64
	if coverage.AsOf > 0 {
		value := coverage.AsOf
		asOf = &value
	}
	base := dto.StatisticsBreakdownBase{
		DimensionID: dimension.ID, DimensionName: dimension.Name, SiteID: siteID, SiteName: siteName,
		BucketStart: bucket.Start, BucketEnd: bucket.End,
		RequestCount: requestCount, Quota: quota, TokenUsed: tokenUsed, ActiveUsers: activeUsers,
		DataStatus: coverage.status(), IsFinal: coverage.final(builder.query.Granularity, bucket.End, builder.now),
		AsOf: asOf, SiteBreakdown: builder.buildDimensionSiteBreakdown(dimension.Key, bucket.Start),
		CompletenessRate: coverage.completenessRate(),
	}
	switch builder.scope {
	case dto.StatisticsScopeGlobal:
		completeSites, expectedSites := coverage.siteCounts()
		return dto.GlobalStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeGlobal,
			CompleteSiteCount: completeSites, ExpectedSiteCount: expectedSites,
		}, nil
	case dto.StatisticsScopeSite:
		site, exists := builder.sitesByID[dimension.SiteID]
		if !exists {
			return nil, model.ErrStatisticsReadContract
		}
		return dto.SiteStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeSite,
			ManagementStatus: site.ManagementStatus, OnlineStatus: site.OnlineStatus, AuthStatus: site.AuthStatus,
			StatisticsStatus: site.StatisticsStatus, HealthStatus: site.HealthStatus,
			Rate: statisticsEffectiveRate(site, builder.data.rates),
		}, nil
	case dto.StatisticsScopeCustomer:
		return dto.CustomerStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeCustomer,
			AccountCount: dimension.AccountCount, SiteCount: dimension.SiteCount,
		}, nil
	case dto.StatisticsScopeAccount:
		customer, exists := builder.customers[dimension.CustomerID]
		if !exists {
			return nil, model.ErrStatisticsReadContract
		}
		return dto.AccountStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeAccount,
			CustomerID: strconv.FormatInt(customer.ID, 10), CustomerName: customer.Name,
			RemoteUserID: strconv.FormatInt(dimension.RemoteUserID, 10),
		}, nil
	case dto.StatisticsScopeModel:
		return dto.ModelStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeModel, ModelName: dimension.Name,
		}, nil
	case dto.StatisticsScopeChannel:
		return dto.ChannelStatisticsBreakdown{
			StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeChannel,
			RemoteChannelID: strconv.FormatInt(dimension.RemoteChannelID, 10), RemoteMissing: dimension.RemoteMissing,
		}, nil
	case dto.StatisticsScopeGroup:
		return dto.GroupStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeGroup, UseGroup: dimension.ID}, nil
	case dto.StatisticsScopeToken:
		return dto.TokenStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeToken, TokenID: strconv.FormatInt(dimension.TokenID, 10), TokenName: dimension.Name}, nil
	case dto.StatisticsScopeNode:
		return dto.NodeStatisticsBreakdown{StatisticsBreakdownBase: base, DimensionType: dto.StatisticsScopeNode, NodeName: dimension.ID}, nil
	default:
		return nil, ErrStatisticsInvalid
	}
}

func (builder *statisticsResponseBuilder) breakdownLess(left, right statisticsBreakdownRow) bool {
	metricSort := builder.query.SortBy == "request_count" || builder.query.SortBy == "quota" ||
		builder.query.SortBy == "token_used" || builder.query.SortBy == "active_users"
	if metricSort && left.Known != right.Known {
		return left.Known
	}
	order := 0
	switch builder.query.SortBy {
	case "name":
		order = strings.Compare(left.Name, right.Name)
	case "bucket_start":
		order = compareStatisticsInt64(left.BucketStart, right.BucketStart)
	case "request_count":
		order = compareStatisticsKnownMetric(left.Known, left.Metric.RequestCount, right.Known, right.Metric.RequestCount)
	case "quota":
		order = compareStatisticsKnownMetric(left.Known, left.Metric.Quota, right.Known, right.Metric.Quota)
	case "token_used":
		order = compareStatisticsKnownMetric(left.Known, left.Metric.TokenUsed, right.Known, right.Metric.TokenUsed)
	case "active_users":
		order = compareStatisticsKnownMetric(left.Known, left.ActiveUsers, right.Known, right.ActiveUsers)
	}
	if order == 0 {
		order = strings.Compare(left.Name, right.Name)
	}
	if order == 0 {
		order = compareStatisticsInt64(left.BucketStart, right.BucketStart)
	}
	if order != 0 {
		if builder.query.SortOrder == "desc" {
			return order > 0
		}
		return order < 0
	}
	return left.Key < right.Key
}

func compareStatisticsKnownMetric(leftKnown bool, left int64, rightKnown bool, right int64) int {
	if leftKnown != rightKnown {
		if leftKnown {
			return -1
		}
		return 1
	}
	return compareStatisticsInt64(left, right)
}

func compareStatisticsInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func (builder *statisticsResponseBuilder) buildCompleteness() dto.Completeness {
	ranges, total, truncated, missingSites := builder.missingRanges()
	completeSites, expectedSites := 0, 0
	if len(builder.buckets) > 0 {
		completeSites, expectedSites = builder.coverageForTrend(builder.buckets[len(builder.buckets)-1].Start).siteCounts()
	}
	var lastVerifiedAt *int64
	if builder.rangeCoverage.LastVerified > 0 {
		value := builder.rangeCoverage.LastVerified
		lastVerifiedAt = &value
	}
	return dto.Completeness{
		DataStatus: builder.rangeCoverage.status(), CompleteSiteCount: completeSites, ExpectedSiteCount: expectedSites,
		UnitType: statisticsUnitType(builder.scope), CompleteUnitCount: builder.rangeCoverage.Complete,
		ExpectedUnitCount: builder.rangeCoverage.Expected, CompletenessRate: builder.rangeCoverage.completenessRate(),
		MissingSiteIDs: missingSites, MissingRanges: ranges, MissingRangeTotal: total,
		MissingRangesTruncated: truncated, LastVerifiedAt: lastVerifiedAt,
	}
}
