package service

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strconv"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const dashboardCurrentSnapshotMaxAgeSeconds int64 = 120

type DashboardReaderOptions struct {
	Database *gorm.DB
	Alerts   *AlertService
	Clock    common.Clock
}

// DashboardReader adapts the current-state tables and AlertService to the
// narrow read contracts consumed by DashboardService.
type DashboardReader struct {
	database *gorm.DB
	alerts   *AlertService
	clock    common.Clock
}

func NewDashboardReader(options DashboardReaderOptions) (*DashboardReader, error) {
	if options.Database == nil || options.Alerts == nil || options.Clock == nil {
		return nil, errors.New("dashboard reader dependencies are required")
	}
	return &DashboardReader{database: options.Database, alerts: options.Alerts, clock: options.Clock}, nil
}

func (reader *DashboardReader) ReadDashboardSiteHealth(ctx context.Context) ([]DashboardSiteHealthSnapshot, error) {
	if reader == nil || reader.database == nil {
		return nil, ErrDashboardRead
	}
	var sites []model.Site
	if err := reader.database.WithContext(ctx).Order("id ASC").Find(&sites).Error; err != nil {
		return nil, err
	}
	result := make([]DashboardSiteHealthSnapshot, 0, len(sites))
	for _, site := range sites {
		result = append(result, DashboardSiteHealthSnapshot{
			SiteID: site.ID, SiteName: site.Name, ManagementStatus: site.ManagementStatus,
			OnlineStatus: site.OnlineStatus, AuthStatus: site.AuthStatus,
			StatisticsStatus: site.StatisticsStatus, HealthStatus: site.HealthStatus, UpdatedAt: site.UpdatedAt,
		})
	}
	return result, nil
}

func (reader *DashboardReader) ReadDashboardAlerts(ctx context.Context, limit int) (DashboardAlertSnapshot, error) {
	if reader == nil || reader.alerts == nil || limit < 1 || limit > 100 {
		return DashboardAlertSnapshot{}, ErrDashboardRead
	}
	summary, err := reader.alerts.Summary(ctx)
	if err != nil {
		return DashboardAlertSnapshot{}, err
	}
	page, err := reader.alerts.List(ctx, dto.AlertListQuery{
		Page: 1, PageSize: limit, Statuses: []string{dto.AlertStatusFiring},
		SortBy: "last_fired_at", SortOrder: "desc",
	})
	if err != nil {
		return DashboardAlertSnapshot{}, err
	}
	return DashboardAlertSnapshot{Summary: summary, Latest: page.Items}, nil
}

func (reader *DashboardReader) ReadDashboardRealtime(ctx context.Context) (DashboardRealtimeSnapshot, error) {
	if reader == nil || reader.database == nil || reader.clock == nil {
		return DashboardRealtimeSnapshot{}, ErrDashboardRead
	}
	now := reader.clock.Now().Unix()
	if now <= 0 {
		return DashboardRealtimeSnapshot{}, model.ErrStatisticsReadContract
	}
	var sites []model.Site
	if err := reader.database.WithContext(ctx).Order("id ASC").Find(&sites).Error; err != nil {
		return DashboardRealtimeSnapshot{}, err
	}

	snapshot := DashboardRealtimeSnapshot{}
	expectedSiteIDs := make([]int64, 0, len(sites))
	for _, site := range sites {
		snapshot.SiteCount++
		switch site.OnlineStatus {
		case constant.SiteOnlineOnline:
			snapshot.OnlineSiteCount++
		case constant.SiteOnlineOffline:
			snapshot.OfflineSiteCount++
		}
		if dashboardCurrentSiteExpected(site) {
			expectedSiteIDs = append(expectedSiteIDs, site.ID)
		}
	}
	if err := reader.readDashboardEntityCounts(ctx, &snapshot); err != nil {
		return DashboardRealtimeSnapshot{}, err
	}
	if err := reader.readDashboardActiveAccounts(ctx, now, &snapshot); err != nil {
		return DashboardRealtimeSnapshot{}, err
	}
	if err := reader.readDashboardRealtimeTotals(sites, expectedSiteIDs, now, &snapshot); err != nil {
		return DashboardRealtimeSnapshot{}, err
	}
	if err := reader.readDashboardResourceTotals(ctx, sites, expectedSiteIDs, now, &snapshot); err != nil {
		return DashboardRealtimeSnapshot{}, err
	}
	return snapshot, nil
}

func (reader *DashboardReader) readDashboardEntityCounts(
	ctx context.Context,
	snapshot *DashboardRealtimeSnapshot,
) error {
	type counts struct {
		CustomerCount       int64 `gorm:"column:customer_count"`
		ManagedAccountCount int64 `gorm:"column:managed_account_count"`
	}
	var value counts
	if err := reader.database.WithContext(ctx).Raw(`SELECT
  (SELECT COUNT(*) FROM customer) AS customer_count,
  (SELECT COUNT(*) FROM account WHERE managed_status = 'active') AS managed_account_count`).Scan(&value).Error; err != nil {
		return err
	}
	if value.CustomerCount > math.MaxInt || value.ManagedAccountCount > math.MaxInt {
		return model.ErrStatisticsReadContract
	}
	snapshot.CustomerCount = int(value.CustomerCount)
	snapshot.ManagedAccountCount = int(value.ManagedAccountCount)
	return nil
}

func (reader *DashboardReader) readDashboardActiveAccounts(
	ctx context.Context,
	now int64,
	snapshot *DashboardRealtimeSnapshot,
) error {
	start, _ := dashboardTodayRange(reader.clock.Now())
	type activeAccountCount struct {
		Count            int64 `gorm:"column:active_count"`
		HasCompleteUsage bool  `gorm:"column:has_complete_usage"`
	}
	var value activeAccountCount
	if err := reader.database.WithContext(ctx).Raw(`SELECT
	  (SELECT COUNT(DISTINCT a.id)
	     FROM usage_fact_hourly f
	     JOIN account a ON a.site_id = f.site_id AND a.remote_user_id = f.remote_user_id
	    WHERE f.hour_ts >= ? AND f.hour_ts < ? AND a.managed_status = 'active') AS active_count,
  EXISTS(SELECT 1 FROM collection_window w
          WHERE w.hour_ts >= ? AND w.hour_ts < ? AND w.status = 'complete') AS has_complete_usage`,
		start, now, start, now).Scan(&value).Error; err != nil {
		return err
	}
	if value.HasCompleteUsage {
		active := strconv.FormatInt(value.Count, 10)
		snapshot.ActiveAccountsToday = &active
	}
	return nil
}

func (reader *DashboardReader) readDashboardRealtimeTotals(
	sites []model.Site,
	expectedSiteIDs []int64,
	now int64,
	snapshot *DashboardRealtimeSnapshot,
) error {
	expected := make(map[int64]struct{}, len(expectedSiteIDs))
	for _, siteID := range expectedSiteIDs {
		expected[siteID] = struct{}{}
	}
	rpm := new(big.Int)
	tpm := new(big.Int)
	var asOf *int64
	for _, site := range sites {
		if _, exists := expected[site.ID]; !exists {
			continue
		}
		snapshot.RealtimeExpectedSiteCount++
		if site.LastRealtimeStatAt == nil || *site.LastRealtimeStatAt < now-dashboardCurrentSnapshotMaxAgeSeconds ||
			site.OnlineStatus == constant.SiteOnlineOffline {
			snapshot.StaleSiteIDs = append(snapshot.StaleSiteIDs, site.ID)
			continue
		}
		if site.CurrentRPM < 0 || site.CurrentTPM < 0 {
			return model.ErrStatisticsReadContract
		}
		snapshot.RealtimeCompleteSiteCount++
		rpm.Add(rpm, big.NewInt(site.CurrentRPM))
		tpm.Add(tpm, big.NewInt(site.CurrentTPM))
		asOf = dashboardOldestTimestamp(asOf, *site.LastRealtimeStatAt)
	}
	snapshot.RealtimeDataStatus = dashboardCurrentDataStatus(
		snapshot.RealtimeCompleteSiteCount, snapshot.RealtimeExpectedSiteCount,
	)
	if snapshot.RealtimeCompleteSiteCount > 0 {
		rpmValue, tpmValue := rpm.String(), tpm.String()
		snapshot.RPM, snapshot.TPM, snapshot.RealtimeAsOf = &rpmValue, &tpmValue, asOf
	}
	return nil
}

func (reader *DashboardReader) readDashboardResourceTotals(
	ctx context.Context,
	sites []model.Site,
	expectedSiteIDs []int64,
	now int64,
	snapshot *DashboardRealtimeSnapshot,
) error {
	snapshot.ResourceExpectedSiteCount = len(expectedSiteIDs)
	snapshot.ResourceDataStatus = dashboardCurrentDataStatus(0, snapshot.ResourceExpectedSiteCount)
	if len(expectedSiteIDs) == 0 {
		return nil
	}
	type latestResource struct {
		SiteID              int64 `gorm:"column:site_id"`
		MinuteTS            int64 `gorm:"column:minute_ts"`
		InstanceCount       int64 `gorm:"column:instance_count"`
		OnlineInstanceCount int64 `gorm:"column:online_instance_count"`
	}
	var rows []latestResource
	if err := reader.database.WithContext(ctx).Raw(`SELECT site_id, minute_ts, instance_count, online_instance_count
FROM (
  SELECT site_id, minute_ts, instance_count, online_instance_count,
	         ROW_NUMBER() OVER (PARTITION BY site_id ORDER BY minute_ts DESC, id DESC) AS sample_rank
  FROM site_status_minutely WHERE site_id IN ?
) ranked WHERE sample_rank = 1`, expectedSiteIDs).Scan(&rows).Error; err != nil {
		return err
	}
	latest := make(map[int64]latestResource, len(rows))
	for _, row := range rows {
		latest[row.SiteID] = row
	}
	instanceCount, onlineInstanceCount := int64(0), int64(0)
	var asOf *int64
	for _, site := range sites {
		if !dashboardCurrentSiteExpected(site) {
			continue
		}
		row, exists := latest[site.ID]
		if !exists || row.MinuteTS < now-dashboardCurrentSnapshotMaxAgeSeconds ||
			site.OnlineStatus == constant.SiteOnlineOffline {
			snapshot.ResourceStaleSiteIDs = append(snapshot.ResourceStaleSiteIDs, site.ID)
			continue
		}
		if row.InstanceCount < 0 || row.OnlineInstanceCount < 0 || row.OnlineInstanceCount > row.InstanceCount ||
			instanceCount > math.MaxInt64-row.InstanceCount || onlineInstanceCount > math.MaxInt64-row.OnlineInstanceCount {
			return model.ErrStatisticsReadContract
		}
		snapshot.ResourceCompleteSiteCount++
		instanceCount += row.InstanceCount
		onlineInstanceCount += row.OnlineInstanceCount
		asOf = dashboardOldestTimestamp(asOf, row.MinuteTS)
	}
	snapshot.ResourceDataStatus = dashboardCurrentDataStatus(
		snapshot.ResourceCompleteSiteCount, snapshot.ResourceExpectedSiteCount,
	)
	if snapshot.ResourceCompleteSiteCount > 0 {
		if instanceCount > math.MaxInt || onlineInstanceCount > math.MaxInt {
			return model.ErrStatisticsReadContract
		}
		instances, online := int(instanceCount), int(onlineInstanceCount)
		snapshot.InstanceCount, snapshot.OnlineInstanceCount, snapshot.ResourceAsOf = &instances, &online, asOf
	}
	return nil
}

func dashboardCurrentSiteExpected(site model.Site) bool {
	return site.ID > 0 && site.ManagementStatus == constant.SiteManagementActive &&
		site.AuthStatus == constant.SiteAuthAuthorized && site.StatisticsEndAt == nil
}

func dashboardCurrentDataStatus(complete, expected int) string {
	switch {
	case complete == expected:
		return model.CollectionWindowStatusComplete
	case complete > 0:
		return model.UsageAggregationStatusPartial
	default:
		return model.CollectionWindowStatusMissing
	}
}

func dashboardOldestTimestamp(current *int64, candidate int64) *int64 {
	if current == nil || candidate < *current {
		value := candidate
		return &value
	}
	return current
}
