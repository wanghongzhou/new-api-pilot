package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

var (
	ErrDashboardInvalid = errors.New("dashboard query is invalid")
	ErrDashboardRead    = errors.New("dashboard query failed")
)

type DashboardStatisticsReader interface {
	Global(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Sites(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Customers(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Models(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
	Channels(context.Context, dto.StatisticsQuery) (dto.StatisticsResponse, error)
}

type DashboardSiteHealthSnapshot struct {
	SiteID           int64
	SiteName         string
	ManagementStatus string
	OnlineStatus     string
	AuthStatus       string
	StatisticsStatus string
	HealthStatus     string
	UpdatedAt        int64
}

type SiteHealthReader interface {
	ReadDashboardSiteHealth(context.Context) ([]DashboardSiteHealthSnapshot, error)
}

type DashboardAlertSnapshot struct {
	Summary dto.AlertSummary
	Latest  []dto.AlertEventItem
}

type AlertSummaryReader interface {
	ReadDashboardAlerts(context.Context, int) (DashboardAlertSnapshot, error)
}

type DashboardRealtimeSnapshot struct {
	ActiveAccountsToday       *string
	SiteCount                 int
	OnlineSiteCount           int
	OfflineSiteCount          int
	CustomerCount             int
	ManagedAccountCount       int
	InstanceCount             *int
	OnlineInstanceCount       *int
	ResourceCompleteSiteCount int
	ResourceExpectedSiteCount int
	ResourceStaleSiteIDs      []int64
	ResourceDataStatus        string
	ResourceAsOf              *int64
	ResourceReason            *dto.MessageRef
	RPM                       *string
	TPM                       *string
	RealtimeCompleteSiteCount int
	RealtimeExpectedSiteCount int
	StaleSiteIDs              []int64
	RealtimeDataStatus        string
	RealtimeAsOf              *int64
	RealtimeReason            *dto.MessageRef
}

type RealtimeReader interface {
	ReadDashboardRealtime(context.Context) (DashboardRealtimeSnapshot, error)
}

type DashboardService struct {
	statistics DashboardStatisticsReader
	siteHealth SiteHealthReader
	alerts     AlertSummaryReader
	realtime   RealtimeReader
	clock      common.Clock
}

type DashboardServiceOptions struct {
	Statistics DashboardStatisticsReader
	SiteHealth SiteHealthReader
	Alerts     AlertSummaryReader
	Realtime   RealtimeReader
	Clock      common.Clock
}

func NewDashboardService(options DashboardServiceOptions) (*DashboardService, error) {
	if options.Statistics == nil || options.SiteHealth == nil || options.Alerts == nil ||
		options.Realtime == nil || options.Clock == nil {
		return nil, errors.New("dashboard service dependencies are required")
	}
	return &DashboardService{
		statistics: options.Statistics,
		siteHealth: options.SiteHealth,
		alerts:     options.Alerts,
		realtime:   options.Realtime,
		clock:      options.Clock,
	}, nil
}

func (service *DashboardService) Summary(ctx context.Context) (dto.DashboardSummary, error) {
	if !service.ready() {
		return dto.DashboardSummary{}, ErrDashboardRead
	}
	start, end := dashboardTodayRange(service.clock.Now())
	statistics, err := service.statistics.Global(ctx, dashboardStatisticsQuery(start, end, 1, "bucket_start"))
	if err != nil {
		return dto.DashboardSummary{}, errors.Join(ErrDashboardRead, err)
	}
	point, err := dashboardSinglePoint(statistics, start, end)
	if err != nil {
		return dto.DashboardSummary{}, errors.Join(ErrDashboardRead, err)
	}
	realtime, err := service.realtime.ReadDashboardRealtime(ctx)
	if err != nil {
		return dto.DashboardSummary{}, errors.Join(ErrDashboardRead, err)
	}
	realtime.ResourceReason = dashboardCurrentReason(
		realtime.ResourceDataStatus, realtime.ResourceCompleteSiteCount,
		realtime.ResourceExpectedSiteCount, realtime.ResourceReason,
	)
	realtime.RealtimeReason = dashboardCurrentReason(
		realtime.RealtimeDataStatus, realtime.RealtimeCompleteSiteCount,
		realtime.RealtimeExpectedSiteCount, realtime.RealtimeReason,
	)
	if err := validateDashboardRealtime(realtime); err != nil {
		return dto.DashboardSummary{}, errors.Join(ErrDashboardRead, err)
	}
	if (point.RequestCount == nil) != (realtime.ActiveAccountsToday == nil) {
		return dto.DashboardSummary{}, errors.Join(ErrDashboardRead, model.ErrStatisticsReadContract)
	}
	return dto.DashboardSummary{
		Today:                     dashboardUsageSummary(point),
		ActiveAccountsToday:       realtime.ActiveAccountsToday,
		SiteCount:                 realtime.SiteCount,
		OnlineSiteCount:           realtime.OnlineSiteCount,
		OfflineSiteCount:          realtime.OfflineSiteCount,
		CustomerCount:             realtime.CustomerCount,
		ManagedAccountCount:       realtime.ManagedAccountCount,
		InstanceCount:             realtime.InstanceCount,
		OnlineInstanceCount:       realtime.OnlineInstanceCount,
		ResourceCompleteSiteCount: realtime.ResourceCompleteSiteCount,
		ResourceExpectedSiteCount: realtime.ResourceExpectedSiteCount,
		ResourceStaleSiteIDs:      dashboardIDStrings(realtime.ResourceStaleSiteIDs),
		ResourceDataStatus:        realtime.ResourceDataStatus,
		ResourceAsOf:              realtime.ResourceAsOf,
		ResourceReason:            realtime.ResourceReason,
		RPM:                       realtime.RPM,
		TPM:                       realtime.TPM,
		RealtimeCompleteSiteCount: realtime.RealtimeCompleteSiteCount,
		RealtimeExpectedSiteCount: realtime.RealtimeExpectedSiteCount,
		StaleSiteIDs:              dashboardIDStrings(realtime.StaleSiteIDs),
		RealtimeDataStatus:        realtime.RealtimeDataStatus,
		RealtimeAsOf:              realtime.RealtimeAsOf,
		RealtimeReason:            realtime.RealtimeReason,
	}, nil
}

func (service *DashboardService) Trend(
	ctx context.Context,
	query dto.DashboardTrendQuery,
) ([]dto.TrendPoint, error) {
	if !service.ready() {
		return nil, ErrDashboardRead
	}
	query.Normalize()
	if query.Validate() != nil {
		return nil, ErrDashboardInvalid
	}
	start, end := dashboardDayRange(service.clock.Now(), query.Days)
	statistics, err := service.statistics.Global(ctx, dashboardStatisticsQuery(start, end, 1, "bucket_start"))
	if err != nil {
		return nil, errors.Join(ErrDashboardRead, err)
	}
	if err := validateDashboardTrend(statistics, start, end, query.Days); err != nil {
		return nil, errors.Join(ErrDashboardRead, err)
	}
	return append([]dto.TrendPoint(nil), statistics.Trend...), nil
}

func (service *DashboardService) Top(
	ctx context.Context,
	query dto.DashboardTopQuery,
) ([]dto.DashboardRankingItem, error) {
	if !service.ready() {
		return nil, ErrDashboardRead
	}
	query.Normalize()
	if query.Validate() != nil {
		return nil, ErrDashboardInvalid
	}
	start, end := dashboardTodayRange(service.clock.Now())
	statisticsQuery := dashboardStatisticsQuery(start, end, query.Limit, query.Metric)
	var (
		statistics dto.StatisticsResponse
		err        error
	)
	switch query.Type {
	case dto.DashboardTopTypeSite:
		statistics, err = service.statistics.Sites(ctx, statisticsQuery)
	case dto.DashboardTopTypeCustomer:
		statistics, err = service.statistics.Customers(ctx, statisticsQuery)
	case dto.DashboardTopTypeModel:
		statistics, err = service.statistics.Models(ctx, statisticsQuery)
	case dto.DashboardTopTypeChannel:
		statistics, err = service.statistics.Channels(ctx, statisticsQuery)
	default:
		return nil, ErrDashboardInvalid
	}
	if err != nil {
		return nil, errors.Join(ErrDashboardRead, err)
	}
	items, err := dashboardRankingItems(statistics, query, start, end)
	if err != nil {
		return nil, errors.Join(ErrDashboardRead, err)
	}
	return items, nil
}

func (service *DashboardService) Health(ctx context.Context) (dto.DashboardHealth, error) {
	if !service.ready() {
		return dto.DashboardHealth{}, ErrDashboardRead
	}
	today, _ := dashboardTodayRange(service.clock.Now())
	yesterday := time.Unix(today, 0).In(dashboardLocation).AddDate(0, 0, -1).Unix()
	statistics, err := service.statistics.Global(ctx, dashboardStatisticsQuery(yesterday, today, 1, "bucket_start"))
	if err != nil {
		return dto.DashboardHealth{}, errors.Join(ErrDashboardRead, err)
	}
	point, err := dashboardSinglePoint(statistics, yesterday, today)
	if err != nil {
		return dto.DashboardHealth{}, errors.Join(ErrDashboardRead, err)
	}
	sites, err := service.siteHealth.ReadDashboardSiteHealth(ctx)
	if err != nil {
		return dto.DashboardHealth{}, errors.Join(ErrDashboardRead, err)
	}
	if err := validateDashboardSites(sites); err != nil {
		return dto.DashboardHealth{}, errors.Join(ErrDashboardRead, err)
	}
	alerts, err := service.alerts.ReadDashboardAlerts(ctx, 5)
	if err != nil {
		return dto.DashboardHealth{}, errors.Join(ErrDashboardRead, err)
	}
	if err := validateDashboardAlerts(alerts); err != nil {
		return dto.DashboardHealth{}, errors.Join(ErrDashboardRead, err)
	}
	items, authExpired, statisticsNotReady := dashboardHealthSites(sites)
	status, reason := dashboardValidationStatus(point)
	latest := append([]dto.AlertEventItem(nil), alerts.Latest...)
	if latest == nil {
		latest = []dto.AlertEventItem{}
	}
	sort.SliceStable(latest, func(left, right int) bool {
		leftAt := dashboardAlertTimestamp(latest[left])
		rightAt := dashboardAlertTimestamp(latest[right])
		if leftAt != rightAt {
			return leftAt > rightAt
		}
		leftID, _ := strconv.ParseInt(latest[left].ID, 10, 64)
		rightID, _ := strconv.ParseInt(latest[right].ID, 10, 64)
		return leftID > rightID
	})
	return dto.DashboardHealth{
		FiringAlertCount: alerts.Summary.FiringCount, CriticalAlertCount: alerts.Summary.CriticalCount,
		WarningAlertCount: alerts.Summary.WarningCount, AuthExpiredSiteIDs: authExpired,
		StatisticsNotReadySiteIDs: statisticsNotReady, YesterdayValidationStatus: status,
		Completeness: statistics.Completeness, LatestAlerts: latest, Sites: items,
		AsOf: point.AsOf, IsFinal: point.IsFinal, Reason: reason,
	}, nil
}

func (service *DashboardService) ready() bool {
	return service != nil && service.statistics != nil && service.siteHealth != nil &&
		service.alerts != nil && service.realtime != nil && service.clock != nil
}

var dashboardLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func dashboardTodayRange(now time.Time) (int64, int64) {
	local := now.In(dashboardLocation)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, dashboardLocation)
	return start.Unix(), start.AddDate(0, 0, 1).Unix()
}

func dashboardDayRange(now time.Time, days int) (int64, int64) {
	_, end := dashboardTodayRange(now)
	return time.Unix(end, 0).In(dashboardLocation).AddDate(0, 0, -days).Unix(), end
}

func dashboardStatisticsQuery(start, end int64, pageSize int, sortBy string) dto.StatisticsQuery {
	return dto.StatisticsQuery{
		StartTimestamp: start, EndTimestamp: end, Granularity: dto.StatisticsGranularityDay,
		Page: 1, PageSize: pageSize, SortBy: sortBy, SortOrder: "desc",
	}
}

func dashboardSinglePoint(response dto.StatisticsResponse, start, end int64) (dto.TrendPoint, error) {
	if response.Scope != dto.StatisticsScopeGlobal || response.Granularity != dto.StatisticsGranularityDay ||
		response.Range.StartTimestamp != start || response.Range.EndTimestamp != end || len(response.Trend) != 1 ||
		response.Trend[0].BucketStart != start || response.Trend[0].BucketEnd != end {
		return dto.TrendPoint{}, model.ErrStatisticsReadContract
	}
	return response.Trend[0], nil
}

func validateDashboardTrend(response dto.StatisticsResponse, start, end int64, days int) error {
	if response.Scope != dto.StatisticsScopeGlobal || response.Granularity != dto.StatisticsGranularityDay ||
		response.Range.StartTimestamp != start || response.Range.EndTimestamp != end || len(response.Trend) != days {
		return model.ErrStatisticsReadContract
	}
	cursor := start
	for _, point := range response.Trend {
		next := time.Unix(cursor, 0).In(dashboardLocation).AddDate(0, 0, 1).Unix()
		if point.BucketStart != cursor || point.BucketEnd != next {
			return model.ErrStatisticsReadContract
		}
		cursor = next
	}
	if cursor != end {
		return model.ErrStatisticsReadContract
	}
	return nil
}

func dashboardUsageSummary(point dto.TrendPoint) dto.DashboardUsageSummary {
	breakdown := append([]dto.SiteQuotaBreakdown(nil), point.SiteBreakdown...)
	if breakdown == nil {
		breakdown = []dto.SiteQuotaBreakdown{}
	}
	return dto.DashboardUsageSummary{
		UsageSummary: dto.UsageSummary{
			RequestCount: point.RequestCount, Quota: point.Quota, TokenUsed: point.TokenUsed,
			ActiveUsers: point.ActiveUsers, AsOf: point.AsOf, DataStatus: point.DataStatus, IsFinal: point.IsFinal,
		},
		SiteBreakdown: breakdown,
		Reason:        point.Reason,
	}
}

func dashboardRankingItems(
	response dto.StatisticsResponse,
	query dto.DashboardTopQuery,
	start, end int64,
) ([]dto.DashboardRankingItem, error) {
	expectedScope := query.Type
	switch query.Type {
	case dto.DashboardTopTypeSite, dto.DashboardTopTypeCustomer,
		dto.DashboardTopTypeModel, dto.DashboardTopTypeChannel:
	default:
		return nil, model.ErrStatisticsReadContract
	}
	if response.Scope != expectedScope || response.Granularity != dto.StatisticsGranularityDay ||
		response.Range.StartTimestamp != start || response.Range.EndTimestamp != end ||
		len(response.Breakdown.Items) > query.Limit {
		return nil, model.ErrStatisticsReadContract
	}
	result := make([]dto.DashboardRankingItem, 0, len(response.Breakdown.Items))
	for _, raw := range response.Breakdown.Items {
		var base dto.StatisticsBreakdownBase
		switch item := raw.(type) {
		case dto.SiteStatisticsBreakdown:
			if query.Type != dto.DashboardTopTypeSite {
				return nil, model.ErrStatisticsReadContract
			}
			base = item.StatisticsBreakdownBase
		case dto.CustomerStatisticsBreakdown:
			if query.Type != dto.DashboardTopTypeCustomer {
				return nil, model.ErrStatisticsReadContract
			}
			base = item.StatisticsBreakdownBase
		case dto.ModelStatisticsBreakdown:
			if query.Type != dto.DashboardTopTypeModel {
				return nil, model.ErrStatisticsReadContract
			}
			base = item.StatisticsBreakdownBase
		case dto.ChannelStatisticsBreakdown:
			if query.Type != dto.DashboardTopTypeChannel {
				return nil, model.ErrStatisticsReadContract
			}
			base = item.StatisticsBreakdownBase
		default:
			return nil, model.ErrStatisticsReadContract
		}
		value := base.RequestCount
		if query.Metric == dto.DashboardTopMetricQuota {
			value = base.Quota
		}
		if !dashboardRankingIdentity(query.Type, base.DimensionID, base.SiteID) || !dashboardMetricString(value) ||
			!dashboardDataStatus(base.DataStatus) || !dashboardTimestamp(base.AsOf) ||
			(query.Type == dto.DashboardTopTypeCustomer && base.SiteID != nil) {
			return nil, model.ErrStatisticsReadContract
		}
		breakdown := append([]dto.SiteQuotaBreakdown(nil), base.SiteBreakdown...)
		if breakdown == nil {
			breakdown = []dto.SiteQuotaBreakdown{}
		}
		result = append(result, dto.DashboardRankingItem{
			DimensionType: query.Type, DimensionID: base.DimensionID, DimensionName: base.DimensionName,
			SiteID: base.SiteID, Value: value, DataStatus: base.DataStatus, SiteBreakdown: breakdown,
			AsOf: base.AsOf, IsFinal: base.IsFinal,
			Reason: dashboardRankingReason(
				base.DataStatus, query.Type, base.DimensionID, breakdown, base.BucketStart, base.BucketEnd,
			),
		})
	}
	return result, nil
}

func dashboardRankingIdentity(dimensionType, dimensionID string, siteID *string) bool {
	switch dimensionType {
	case dto.DashboardTopTypeSite:
		return dashboardPositiveIDString(dimensionID) && siteID != nil && *siteID == dimensionID
	case dto.DashboardTopTypeCustomer:
		return dashboardPositiveIDString(dimensionID) && siteID == nil
	case dto.DashboardTopTypeModel:
		return strings.TrimSpace(dimensionID) != "" && utf8.ValidString(dimensionID) &&
			siteID != nil && dashboardPositiveIDString(*siteID)
	case dto.DashboardTopTypeChannel:
		left, right, found := strings.Cut(dimensionID, ":")
		channelID, err := strconv.ParseInt(right, 10, 64)
		return found && dashboardPositiveIDString(left) && err == nil && channelID >= 0 &&
			strconv.FormatInt(channelID, 10) == right && siteID != nil && *siteID == left
	default:
		return false
	}
}

func dashboardRankingReason(
	status, scope, id string,
	breakdown []dto.SiteQuotaBreakdown,
	start, end int64,
) *dto.MessageRef {
	if status == model.CollectionWindowStatusComplete {
		return nil
	}
	if status == model.UsageAggregationStatusPartial {
		complete := 0
		for _, site := range breakdown {
			if site.DataStatus == model.CollectionWindowStatusComplete {
				complete++
			}
		}
		expected := len(breakdown)
		if expected == 0 {
			expected = 1
		}
		reason := dto.MustMessageRef(constant.MessageDataPartialSites, map[string]any{
			"complete_site_count": complete, "expected_site_count": expected,
		}, "")
		return &reason
	}
	for _, site := range breakdown {
		if site.DataStatus != status {
			continue
		}
		var reason dto.MessageRef
		switch status {
		case model.CollectionWindowStatusMissing:
			reason = dto.MustMessageRef(constant.MessageDataWindowMissing, map[string]any{
				"site_id": site.SiteID, "start_timestamp": start, "end_timestamp": end,
			}, "")
		case model.CollectionWindowStatusUnavailable:
			reason = dto.MustMessageRef(constant.MessageDataUpstreamUnavailable, map[string]any{
				"site_id": site.SiteID, "start_timestamp": start, "end_timestamp": end,
			}, "")
		case constant.SiteStatisticsPaused:
			reason = dto.MustMessageRef(constant.MessageDataScopePaused, map[string]any{
				"scope_type": scope, "scope_id": id, "start_timestamp": start, "end_timestamp": end,
			}, "")
		default:
			continue
		}
		return &reason
	}
	if status == constant.SiteStatisticsBackfilling {
		reason := dto.MustMessageRef(constant.MessageDataBackfilling, map[string]any{
			"scope_type": scope, "scope_id": id, "progress": float64(0),
		}, "")
		return &reason
	}
	reason := dto.MustMessageRef(constant.MessageDataPending, map[string]any{
		"scope_type": scope, "scope_id": id, "progress": float64(0),
	}, "")
	return &reason
}

func validateDashboardRealtime(snapshot DashboardRealtimeSnapshot) error {
	if snapshot.SiteCount < 0 || snapshot.OnlineSiteCount < 0 || snapshot.OfflineSiteCount < 0 ||
		snapshot.CustomerCount < 0 || snapshot.ManagedAccountCount < 0 ||
		snapshot.OnlineSiteCount+snapshot.OfflineSiteCount > snapshot.SiteCount ||
		snapshot.ResourceCompleteSiteCount < 0 || snapshot.ResourceExpectedSiteCount < 0 ||
		snapshot.ResourceCompleteSiteCount > snapshot.ResourceExpectedSiteCount ||
		snapshot.RealtimeCompleteSiteCount < 0 || snapshot.RealtimeExpectedSiteCount < 0 ||
		snapshot.RealtimeCompleteSiteCount > snapshot.RealtimeExpectedSiteCount ||
		!dashboardDataStatus(snapshot.ResourceDataStatus) || !dashboardDataStatus(snapshot.RealtimeDataStatus) ||
		!dashboardMetricString(snapshot.ActiveAccountsToday) || !dashboardMetricString(snapshot.RPM) ||
		!dashboardMetricString(snapshot.TPM) || !dashboardTimestamp(snapshot.ResourceAsOf) ||
		!dashboardTimestamp(snapshot.RealtimeAsOf) || !dashboardMessage(snapshot.ResourceReason) ||
		!dashboardMessage(snapshot.RealtimeReason) || !dashboardIDs(snapshot.ResourceStaleSiteIDs) ||
		!dashboardIDs(snapshot.StaleSiteIDs) {
		return model.ErrStatisticsReadContract
	}
	if (snapshot.InstanceCount == nil) != (snapshot.OnlineInstanceCount == nil) {
		return model.ErrStatisticsReadContract
	}
	if snapshot.InstanceCount != nil && (*snapshot.InstanceCount < 0 || *snapshot.OnlineInstanceCount < 0 ||
		*snapshot.OnlineInstanceCount > *snapshot.InstanceCount) {
		return model.ErrStatisticsReadContract
	}
	if (snapshot.RPM == nil) != (snapshot.TPM == nil) {
		return model.ErrStatisticsReadContract
	}
	if !dashboardCoverageStatus(
		snapshot.ResourceDataStatus, snapshot.ResourceCompleteSiteCount, snapshot.ResourceExpectedSiteCount,
	) || !dashboardCoverageStatus(
		snapshot.RealtimeDataStatus, snapshot.RealtimeCompleteSiteCount, snapshot.RealtimeExpectedSiteCount,
	) {
		return model.ErrStatisticsReadContract
	}
	if (snapshot.ResourceCompleteSiteCount == 0) != (snapshot.InstanceCount == nil) ||
		(snapshot.RealtimeCompleteSiteCount == 0) != (snapshot.RPM == nil) {
		return model.ErrStatisticsReadContract
	}
	if snapshot.ResourceDataStatus == model.CollectionWindowStatusComplete && snapshot.ResourceReason != nil ||
		snapshot.ResourceDataStatus != model.CollectionWindowStatusComplete && snapshot.ResourceReason == nil ||
		snapshot.RealtimeDataStatus == model.CollectionWindowStatusComplete && snapshot.RealtimeReason != nil ||
		snapshot.RealtimeDataStatus != model.CollectionWindowStatusComplete && snapshot.RealtimeReason == nil {
		return model.ErrStatisticsReadContract
	}
	return nil
}

func dashboardCoverageStatus(status string, complete, expected int) bool {
	switch status {
	case model.CollectionWindowStatusComplete:
		return complete == expected
	case model.UsageAggregationStatusPartial:
		return complete > 0 && complete < expected
	default:
		return complete == 0
	}
}

func dashboardCurrentReason(
	status string,
	complete, expected int,
	reason *dto.MessageRef,
) *dto.MessageRef {
	if status == model.CollectionWindowStatusComplete || reason != nil {
		return reason
	}
	if expected > 0 {
		value := dto.MustMessageRef(constant.MessageDataPartialSites, map[string]any{
			"complete_site_count": complete, "expected_site_count": expected,
		}, "")
		return &value
	}
	value := dto.MustMessageRef(constant.MessageDataPending, map[string]any{
		"scope_type": dto.StatisticsScopeGlobal, "scope_id": nil, "progress": float64(0),
	}, "")
	return &value
}

func validateDashboardSites(sites []DashboardSiteHealthSnapshot) error {
	seen := make(map[int64]struct{}, len(sites))
	for _, site := range sites {
		if site.SiteID <= 0 || site.SiteName == "" || site.UpdatedAt <= 0 ||
			!dto.ValidSiteManagementStatus(site.ManagementStatus) || !dto.ValidSiteOnlineStatus(site.OnlineStatus) ||
			!dto.ValidSiteAuthStatus(site.AuthStatus) || !dto.ValidSiteStatisticsStatus(site.StatisticsStatus) ||
			!dto.ValidSiteHealthStatus(site.HealthStatus) {
			return model.ErrStatisticsReadContract
		}
		if _, exists := seen[site.SiteID]; exists {
			return model.ErrStatisticsReadContract
		}
		seen[site.SiteID] = struct{}{}
	}
	return nil
}

func validateDashboardAlerts(snapshot DashboardAlertSnapshot) error {
	if snapshot.Summary.FiringCount < 0 || snapshot.Summary.CriticalCount < 0 ||
		snapshot.Summary.WarningCount < 0 || snapshot.Summary.ResolvedTodayCount < 0 ||
		snapshot.Summary.CriticalCount+snapshot.Summary.WarningCount > snapshot.Summary.FiringCount ||
		snapshot.Summary.UpdatedAt <= 0 || len(snapshot.Latest) > 5 {
		return model.ErrStatisticsReadContract
	}
	for _, alert := range snapshot.Latest {
		if !dashboardPositiveIDString(alert.ID) || !dashboardPositiveIDString(alert.RuleID) ||
			(alert.SiteID != nil && !dashboardPositiveIDString(*alert.SiteID)) ||
			alert.Status != dto.AlertStatusFiring || !dashboardAlertLevel(alert.Level) || alert.FirstObservedAt <= 0 ||
			dto.ValidateMessageParams(alert.Message.Code, alert.Message.Params) != nil {
			return model.ErrStatisticsReadContract
		}
	}
	return nil
}

func dashboardAlertLevel(level string) bool {
	return level == dto.AlertLevelInfo || level == dto.AlertLevelWarning || level == dto.AlertLevelCritical
}

func dashboardAlertTimestamp(alert dto.AlertEventItem) int64 {
	if alert.LastFiredAt != nil {
		return *alert.LastFiredAt
	}
	if alert.FirstFiredAt != nil {
		return *alert.FirstFiredAt
	}
	return alert.FirstObservedAt
}

func dashboardHealthSites(
	sites []DashboardSiteHealthSnapshot,
) ([]dto.DashboardSiteHealthItem, []string, []string) {
	items := make([]dto.DashboardSiteHealthItem, 0, len(sites))
	authExpired := make([]int64, 0)
	statisticsNotReady := make([]int64, 0)
	for _, site := range sites {
		items = append(items, dto.DashboardSiteHealthItem{
			SiteID: strconv.FormatInt(site.SiteID, 10), SiteName: site.SiteName,
			ManagementStatus: site.ManagementStatus, OnlineStatus: site.OnlineStatus,
			AuthStatus: site.AuthStatus, StatisticsStatus: site.StatisticsStatus,
			HealthStatus: site.HealthStatus, UpdatedAt: site.UpdatedAt,
		})
		if site.ManagementStatus == constant.SiteManagementActive && site.AuthStatus == constant.SiteAuthExpired {
			authExpired = append(authExpired, site.SiteID)
		}
		if site.ManagementStatus == constant.SiteManagementActive && site.StatisticsStatus != constant.SiteStatisticsReady {
			statisticsNotReady = append(statisticsNotReady, site.SiteID)
		}
	}
	sort.SliceStable(items, func(left, right int) bool {
		leftPriority := dashboardHealthPriority(items[left])
		rightPriority := dashboardHealthPriority(items[right])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if items[left].SiteName != items[right].SiteName {
			return items[left].SiteName < items[right].SiteName
		}
		return items[left].SiteID < items[right].SiteID
	})
	return items, dashboardIDStrings(authExpired), dashboardIDStrings(statisticsNotReady)
}

func dashboardHealthPriority(site dto.DashboardSiteHealthItem) int {
	switch {
	case site.HealthStatus == constant.SiteHealthCritical:
		return 0
	case site.HealthStatus == constant.SiteHealthWarning:
		return 1
	case site.OnlineStatus == constant.SiteOnlineOffline:
		return 2
	case site.AuthStatus == constant.SiteAuthExpired:
		return 3
	case site.StatisticsStatus != constant.SiteStatisticsReady:
		return 4
	case site.HealthStatus == constant.SiteHealthUnavailable:
		return 5
	default:
		return 6
	}
}

func dashboardValidationStatus(point dto.TrendPoint) (string, *dto.MessageRef) {
	if point.DataStatus == model.CollectionWindowStatusComplete && !point.IsFinal {
		reason := dto.MustMessageRef(constant.MessageDataPending, map[string]any{
			"scope_type": dto.StatisticsScopeGlobal, "scope_id": nil, "progress": float64(0),
		}, "")
		return model.CollectionWindowStatusPending, &reason
	}
	return point.DataStatus, point.Reason
}

func dashboardIDStrings(ids []int64) []string {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	sort.Slice(unique, func(left, right int) bool { return unique[left] < unique[right] })
	result := make([]string, len(unique))
	for index, id := range unique {
		result[index] = strconv.FormatInt(id, 10)
	}
	return result
}

func dashboardIDs(ids []int64) bool {
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func dashboardMetricString(value *string) bool {
	if value == nil {
		return true
	}
	parsed, err := strconv.ParseInt(*value, 10, 64)
	return err == nil && parsed >= 0 && strconv.FormatInt(parsed, 10) == *value
}

func dashboardPositiveIDString(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func dashboardTimestamp(value *int64) bool {
	return value == nil || *value > 0
}

func dashboardMessage(message *dto.MessageRef) bool {
	return message == nil || dto.ValidateMessageParams(message.Code, message.Params) == nil
}

func dashboardDataStatus(status string) bool {
	switch status {
	case model.CollectionWindowStatusComplete, model.UsageAggregationStatusPartial,
		model.CollectionWindowStatusMissing, model.CollectionWindowStatusUnavailable,
		model.CollectionWindowStatusPending, constant.SiteStatisticsPaused,
		constant.SiteStatisticsBackfilling:
		return true
	default:
		return false
	}
}
