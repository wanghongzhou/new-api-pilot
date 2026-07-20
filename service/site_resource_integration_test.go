package service

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	testsupport "new-api-pilot/tests/support"
)

func TestSiteResourceStatusReadsAllResourceTablesAndKeepsNodeOwnershipBinary(t *testing.T) {
	tx := openSiteTestTransaction(t)
	dayStart := time.Date(2025, time.July, 10, 0, 0, 0, 0, siteResourceLocation).Unix()
	now := dayStart + 2*24*60*60
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{
		authenticated: authorizedTestSiteClient(now), public: authorizedTestSiteClient(now),
	})
	repository := model.NewSiteRepository(tx)
	monitoringStart := dayStart
	site := newTestSite(now, "https://resource-all.example")
	site.MonitoringStartAt = &monitoringStart
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create resource site: %v", err)
	}
	nodeName := "Node-A"
	if err := tx.Create(&model.SiteInstance{
		SiteID: site.ID, NodeName: nodeName, Hostname: "node-a.example", UpstreamStatus: "online",
		CurrentStatus: "online", FirstSeenAt: dayStart + 5, LastSyncedAt: now,
		CreatedAt: dayStart + 5, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create resource instance: %v", err)
	}
	cpu, memory, disk := 12.5, 34.5, 56.5
	zero, one := 0, 1
	if err := tx.Create(&model.SiteStatusMinutely{
		SiteID: site.ID, MinuteTS: dayStart, InstanceCount: zero, OnlineInstanceCount: zero,
		CPUMaxPercent: nil, CPUAvgPercent: nil, MemoryMaxPercent: nil, MemoryAvgPercent: nil,
		DiskMaxUsedPercent: nil, HealthStatus: constant.SiteHealthOK, CreatedAt: dayStart + 30,
	}).Error; err != nil {
		t.Fatalf("create site minute: %v", err)
	}
	if err := tx.Create(&model.SiteInstanceStatusMinutely{
		SiteID: site.ID, NodeName: nodeName, MinuteTS: dayStart, Status: "online",
		CPUPercent: &cpu, MemoryPercent: &memory, DiskUsedPercent: &disk, CreatedAt: dayStart + 30,
	}).Error; err != nil {
		t.Fatalf("create instance minute: %v", err)
	}
	if err := tx.Exec(`INSERT INTO site_status_hourly
  (site_id, hour_ts, instance_count_max, online_instance_count_min,
   cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
   disk_max_used_percent, abnormal_samples, sample_count, expected_sample_count,
   data_status, health_status, last_calculated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 60, 60, 'complete', 'ok', ?)`,
		site.ID, dayStart, one, one, cpu, cpu, memory, memory, disk, dayStart+3600).Error; err != nil {
		t.Fatalf("create site hour: %v", err)
	}
	if err := tx.Exec(`INSERT INTO site_instance_status_hourly
  (site_id, node_name, hour_ts, cpu_max_percent, cpu_avg_percent,
   memory_max_percent, memory_avg_percent, disk_max_used_percent, disk_last_used_percent,
   online_samples, abnormal_samples, sample_count, expected_sample_count, data_status, last_calculated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 60, 0, 60, 60, 'complete', ?)`,
		site.ID, nodeName, dayStart, cpu, cpu, memory, memory, disk, disk, dayStart+3600).Error; err != nil {
		t.Fatalf("create instance hour: %v", err)
	}
	dateKey := siteResourceDateKey(dayStart)
	if err := tx.Exec(`INSERT INTO site_status_daily
  (site_id, date_key, instance_count_max, online_instance_count_min,
   cpu_max_percent, cpu_avg_percent, memory_max_percent, memory_avg_percent,
   disk_max_used_percent, abnormal_samples, sample_count, expected_sample_count,
   data_status, health_status, is_final, last_calculated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 1440, 1440, 'complete', 'ok', 1, ?)`,
		site.ID, dateKey, one, one, cpu, cpu, memory, memory, disk, dayStart+24*60*60).Error; err != nil {
		t.Fatalf("create site day: %v", err)
	}
	if err := tx.Exec(`INSERT INTO site_instance_status_daily
  (site_id, node_name, date_key, cpu_max_percent, cpu_avg_percent,
   memory_max_percent, memory_avg_percent, disk_max_used_percent, disk_last_used_percent,
   online_samples, abnormal_samples, sample_count, expected_sample_count, data_status, is_final, last_calculated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1440, 0, 1440, 1440, 'complete', 1, ?)`,
		site.ID, nodeName, dateKey, cpu, cpu, memory, memory, disk, disk, dayStart+24*60*60).Error; err != nil {
		t.Fatalf("create instance day: %v", err)
	}

	queries := []struct {
		name         string
		query        dto.ResourceQuery
		wantDiskLast bool
		wantFinal    bool
	}{
		{name: "site minute", query: resourceIntegrationQuery(dayStart, dayStart+60, "minute", nil), wantFinal: true},
		{name: "instance minute", query: resourceIntegrationQuery(dayStart, dayStart+60, "minute", &nodeName), wantDiskLast: true, wantFinal: true},
		{name: "site hour", query: resourceIntegrationQuery(dayStart, dayStart+3600, "hour", nil), wantFinal: true},
		{name: "instance hour", query: resourceIntegrationQuery(dayStart, dayStart+3600, "hour", &nodeName), wantDiskLast: true, wantFinal: true},
		{name: "site day", query: resourceIntegrationQuery(dayStart, dayStart+24*60*60, "day", nil), wantFinal: true},
		{name: "instance day", query: resourceIntegrationQuery(dayStart, dayStart+24*60*60, "day", &nodeName), wantDiskLast: true, wantFinal: true},
	}
	for _, test := range queries {
		t.Run(test.name, func(t *testing.T) {
			response, err := sites.ResourceStatus(context.Background(), site.ID, test.query)
			if err != nil {
				t.Fatalf("ResourceStatus() error = %v", err)
			}
			if response.SiteID != strconv.FormatInt(site.ID, 10) || len(response.Trend) != 1 || response.Summary == nil {
				t.Fatalf("resource response = %#v", response)
			}
			point := response.Trend[0]
			if point.DataStatus != "complete" || point.Reason != nil || point.AsOf == nil || point.IsFinal != test.wantFinal {
				t.Fatalf("resource point = %#v", point)
			}
			if (point.DiskLastUsedPercent != nil) != test.wantDiskLast {
				t.Fatalf("disk_last_used_percent = %#v, want present=%v", point.DiskLastUsedPercent, test.wantDiskLast)
			}
		})
	}

	wrongCase := "node-a"
	response, err := sites.ResourceStatus(context.Background(), site.ID,
		resourceIntegrationQuery(dayStart, dayStart+60, "minute", &wrongCase))
	if err != nil {
		t.Fatalf("wrong-case ResourceStatus() error = %v", err)
	}
	if response.Summary != nil || len(response.Trend) != 0 {
		t.Fatalf("binary node ownership leaked wrong-case data: %#v", response)
	}
}

func TestSiteResourceStatusUsesRetentionPauseAndMissingContracts(t *testing.T) {
	tx := openSiteTestTransaction(t)
	start := time.Date(2025, time.July, 13, 12, 0, 0, 0, siteResourceLocation).Unix()
	now := start + 4*60
	clock := testsupport.NewFakeClock(time.Unix(now, 0))
	sites := newIntegrationSiteService(t, tx, clock, &testSiteClientFactory{
		authenticated: authorizedTestSiteClient(now), public: authorizedTestSiteClient(now),
	})
	repository := model.NewSiteRepository(tx)
	monitoringStart := start
	site := newTestSite(now, "https://resource-coverage.example")
	site.MonitoringStartAt = &monitoringStart
	if err := repository.Create(context.Background(), &site); err != nil {
		t.Fatalf("create coverage site: %v", err)
	}
	pauseEnd := start + 2*60
	if err := tx.Create(&model.SiteMonitoringPause{
		SiteID: site.ID, StartMinuteTS: start + 60, EndMinuteTS: &pauseEnd,
		Reason: "management_disabled", CreatedAt: start + 60,
	}).Error; err != nil {
		t.Fatalf("create resource pause: %v", err)
	}
	if err := tx.Create(&model.SiteStatusMinutely{
		SiteID: site.ID, MinuteTS: start, InstanceCount: 0, OnlineInstanceCount: 0,
		HealthStatus: constant.SiteHealthOK, CreatedAt: start + 30,
	}).Error; err != nil {
		t.Fatalf("create zero-instance minute: %v", err)
	}
	response, err := sites.ResourceStatus(context.Background(), site.ID,
		resourceIntegrationQuery(start, start+3*60, "minute", nil))
	if err != nil {
		t.Fatalf("coverage ResourceStatus() error = %v", err)
	}
	if len(response.Trend) != 3 || response.Trend[0].DataStatus != "complete" ||
		response.Trend[1].DataStatus != "paused" || response.Trend[2].DataStatus != "missing" {
		t.Fatalf("coverage trend = %#v", response.Trend)
	}
	if response.Trend[0].InstanceCount == nil || *response.Trend[0].InstanceCount != 0 ||
		response.Trend[2].InstanceCount != nil || response.Trend[2].CPUMaxPercent != nil {
		t.Fatalf("zero/missing metric contract = %#v", response.Trend)
	}
	if response.Summary == nil || response.Summary.DataStatus != "partial" ||
		response.Summary.SampleCount != 1 || response.Summary.ExpectedSampleCount != 2 {
		t.Fatalf("coverage summary = %#v", response.Summary)
	}

	if err := tx.Exec(`UPDATE platform_setting SET setting_value = '1', value_type = 'int', is_secret = 0
WHERE setting_key = 'collector.minute_retention_days'`).Error; err != nil {
		t.Fatalf("set resource retention: %v", err)
	}
	oldStart := floorMinute(now) - 24*60*60 - 60
	_, err = sites.ResourceStatus(context.Background(), site.ID,
		resourceIntegrationQuery(oldStart, oldStart+60, "minute", nil))
	if !errors.Is(err, ErrSiteResourceRange) {
		t.Fatalf("old minute range error = %v, want ErrSiteResourceRange", err)
	}
}

func TestSiteResourceStatusUsesFixedSQLCountForLongTrend(t *testing.T) {
	tx := openSiteTestTransaction(t)
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, siteResourceLocation).Unix()
	end := time.Date(2025, time.January, 1, 0, 0, 0, 0, siteResourceLocation).Unix()
	clock := testsupport.NewFakeClock(time.Unix(end+3600, 0))
	monitoringStart := start
	site := newTestSite(clock.Now().Unix(), "https://resource-query-count.example")
	site.MonitoringStartAt = &monitoringStart
	if err := model.NewSiteRepository(tx).Create(context.Background(), &site); err != nil {
		t.Fatalf("create counted resource site: %v", err)
	}
	counter := &resourceQueryCounter{Interface: tx.Logger}
	counted := tx.Session(&gorm.Session{Logger: counter})
	sites := newIntegrationSiteService(t, counted, clock, &testSiteClientFactory{
		authenticated: authorizedTestSiteClient(clock.Now().Unix()),
		public:        authorizedTestSiteClient(clock.Now().Unix()),
	})
	response, err := sites.ResourceStatus(context.Background(), site.ID,
		resourceIntegrationQuery(start, end, "hour", nil))
	if err != nil {
		t.Fatalf("long ResourceStatus() error = %v", err)
	}
	if len(response.Trend) != 366*24 {
		t.Fatalf("long resource trend length = %d, want %d", len(response.Trend), 366*24)
	}
	if got := counter.statements.Load(); got != 3 {
		t.Fatalf("resource SQL statements = %d, want 3 fixed reads", got)
	}
}

type resourceQueryCounter struct {
	logger.Interface
	statements atomic.Int64
}

func (counter *resourceQueryCounter) Trace(
	ctx context.Context,
	begin time.Time,
	query func() (string, int64),
	err error,
) {
	counter.statements.Add(1)
	counter.Interface.Trace(ctx, begin, query, err)
}

func resourceIntegrationQuery(start, end int64, granularity string, nodeName *string) dto.ResourceQuery {
	return dto.ResourceQuery{
		StartTimestamp: start, EndTimestamp: end, Granularity: granularity, NodeName: nodeName,
	}
}
