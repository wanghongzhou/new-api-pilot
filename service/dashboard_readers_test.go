package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestDashboardReaderMySQLCurrentCoverageEntitiesHealthAndAlerts(t *testing.T) {
	database := openSiteTestTransaction(t)
	location := time.FixedZone("Asia/Shanghai", 8*3600)
	now := time.Date(2034, 3, 4, 12, 2, 30, 0, location)
	clock := testsupport.NewFakeClock(now)
	today := time.Date(2034, 3, 4, 0, 0, 0, 0, location).Unix()
	recentRealtime := now.Unix() - 30
	staleRealtime := now.Unix() - dashboardCurrentSnapshotMaxAgeSeconds - 1
	sites := []model.Site{
		{
			Name: "Dashboard Fresh", BaseURL: "https://dashboard-fresh.example", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady,
			HealthStatus: constant.SiteHealthOK, CurrentRPM: 9_007_199_254_740_993, CurrentTPM: 17,
			LastRealtimeStatAt: &recentRealtime, CreatedAt: today, UpdatedAt: now.Unix(),
		},
		{
			Name: "Dashboard Stale", BaseURL: "https://dashboard-stale.example", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOffline,
			AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsPartial,
			HealthStatus: constant.SiteHealthWarning, CurrentRPM: 11, CurrentTPM: 19,
			LastRealtimeStatAt: &staleRealtime, CreatedAt: today, UpdatedAt: now.Unix(),
		},
		{
			Name: "Dashboard Disabled", BaseURL: "https://dashboard-disabled.example", ConfigVersion: 1,
			ManagementStatus: constant.SiteManagementDisabled, OnlineStatus: constant.SiteOnlineUnknown,
			AuthStatus: constant.SiteAuthUnauthorized, StatisticsStatus: constant.SiteStatisticsPaused,
			HealthStatus: constant.SiteHealthUnavailable, CreatedAt: today, UpdatedAt: now.Unix(),
		},
	}
	if err := database.Create(&sites).Error; err != nil {
		t.Fatalf("create dashboard sites: %v", err)
	}
	customers := []model.Customer{
		{Name: "Dashboard Customer", Status: dto.CustomerStatusUsing, CreatedAt: today, UpdatedAt: now.Unix()},
		{Name: "Dashboard Disabled Customer", Status: dto.CustomerStatusDisabled, CreatedAt: today, UpdatedAt: now.Unix()},
	}
	if err := database.Create(&customers).Error; err != nil {
		t.Fatalf("create dashboard customers: %v", err)
	}
	accounts := []model.Account{
		{SiteID: sites[0].ID, CustomerID: customers[0].ID, RemoteUserID: 101, RemoteCreatedAt: today,
			Username: "active", RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusActive,
			CreatedAt: today, UpdatedAt: now.Unix()},
		{SiteID: sites[0].ID, CustomerID: customers[0].ID, RemoteUserID: 102, RemoteCreatedAt: today,
			Username: "archived", RemoteState: model.AccountRemoteStateNormal, ManagedStatus: model.AccountManagedStatusArchived,
			CreatedAt: today, UpdatedAt: now.Unix()},
	}
	if err := database.Create(&accounts).Error; err != nil {
		t.Fatalf("create dashboard accounts: %v", err)
	}
	facts := []model.UsageFactHourly{
		{SiteID: sites[0].ID, RemoteUserID: 101, ModelName: "model", HourTS: today, RequestCount: 1, CollectedAt: now.Unix()},
		{SiteID: sites[0].ID, RemoteUserID: 102, ModelName: "model", HourTS: today, RequestCount: 1, CollectedAt: now.Unix()},
	}
	if err := database.Create(&facts).Error; err != nil {
		t.Fatalf("create dashboard usage facts: %v", err)
	}
	if err := database.Create(&model.CollectionWindow{
		SiteID: sites[0].ID, HourTS: today, Status: model.CollectionWindowStatusComplete, UpdatedAt: now.Unix(),
	}).Error; err != nil {
		t.Fatalf("create dashboard collection window: %v", err)
	}
	resourceMinute := now.Unix() - now.Unix()%60 - 60
	if err := database.Create(&model.SiteStatusMinutely{
		SiteID: sites[0].ID, MinuteTS: resourceMinute, InstanceCount: 3, OnlineInstanceCount: 2,
		HealthStatus: constant.SiteHealthOK, CreatedAt: now.Unix(),
	}).Error; err != nil {
		t.Fatalf("create dashboard resource sample: %v", err)
	}

	alertService, err := NewAlertService(AlertServiceOptions{Database: database, Clock: clock})
	if err != nil {
		t.Fatalf("create dashboard alert service: %v", err)
	}
	rule := model.AlertRule{
		RuleKey: "dashboard_reader_test", Name: "Auth Expired", Enabled: true, Level: dto.AlertLevelCritical,
		Metric: "site.auth_expired", CompareOperator: "==", ForTimes: 1, ScopeType: dto.AlertScopeGlobal,
		CreatedAt: today, UpdatedAt: now.Unix(),
	}
	if err := database.Create(&rule).Error; err != nil {
		t.Fatalf("create dashboard alert rule: %v", err)
	}
	siteID := strconv.FormatInt(sites[1].ID, 10)
	message := dto.MustMessageRef(constant.MessageAlertAuthExpired, map[string]any{
		"site_id": siteID, "site_name": sites[1].Name,
	}, "")
	encodedMessageParams, err := common.Marshal(message.Params)
	if err != nil {
		t.Fatalf("marshal dashboard alert params: %v", err)
	}
	messageParams := string(encodedMessageParams)
	activeKey := "dashboard-reader-alert"
	firedAt := now.Unix() - 10
	event := model.AlertEvent{
		RuleID: rule.ID, RuleKey: rule.RuleKey, SiteID: &sites[1].ID, TargetType: "site", TargetKey: siteID,
		ActiveKey: &activeKey, Level: dto.AlertLevelCritical, Status: dto.AlertStatusFiring,
		ConsecutiveCount: 1, MessageCode: string(message.Code), MessageParams: &messageParams,
		FirstObservedAt: firedAt, FirstFiredAt: &firedAt, LastFiredAt: &firedAt,
		CreatedAt: firedAt, UpdatedAt: firedAt,
	}
	if err := database.Create(&event).Error; err != nil {
		t.Fatalf("create dashboard alert event: %v", err)
	}

	reader, err := NewDashboardReader(DashboardReaderOptions{Database: database, Alerts: alertService, Clock: clock})
	if err != nil {
		t.Fatalf("create dashboard reader: %v", err)
	}
	snapshot, err := reader.ReadDashboardRealtime(context.Background())
	if err != nil {
		t.Fatalf("read dashboard realtime: %v", err)
	}
	if snapshot.SiteCount != 3 || snapshot.OnlineSiteCount != 1 || snapshot.OfflineSiteCount != 1 ||
		snapshot.CustomerCount != 2 || snapshot.ManagedAccountCount != 1 || dashboardString(snapshot.ActiveAccountsToday) != "1" ||
		dashboardString(snapshot.RPM) != "9007199254740993" || dashboardString(snapshot.TPM) != "17" ||
		snapshot.RealtimeCompleteSiteCount != 1 || snapshot.RealtimeExpectedSiteCount != 2 ||
		snapshot.RealtimeDataStatus != model.UsageAggregationStatusPartial || len(snapshot.StaleSiteIDs) != 1 ||
		snapshot.InstanceCount == nil || *snapshot.InstanceCount != 3 || snapshot.OnlineInstanceCount == nil ||
		*snapshot.OnlineInstanceCount != 2 || snapshot.ResourceCompleteSiteCount != 1 ||
		snapshot.ResourceExpectedSiteCount != 2 || snapshot.ResourceDataStatus != model.UsageAggregationStatusPartial ||
		len(snapshot.ResourceStaleSiteIDs) != 1 {
		t.Fatalf("dashboard realtime snapshot = %#v", snapshot)
	}
	health, err := reader.ReadDashboardSiteHealth(context.Background())
	if err != nil || len(health) != 3 || health[0].SiteName != sites[0].Name {
		t.Fatalf("dashboard site health = %#v, %v", health, err)
	}
	alerts, err := reader.ReadDashboardAlerts(context.Background(), 5)
	if err != nil || alerts.Summary.FiringCount != 1 || alerts.Summary.CriticalCount != 1 ||
		len(alerts.Latest) != 1 || alerts.Latest[0].ID != strconv.FormatInt(event.ID, 10) {
		t.Fatalf("dashboard alerts = %#v, %v", alerts, err)
	}
}
