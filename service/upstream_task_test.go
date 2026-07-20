package service

import (
	"context"
	"encoding/json"
	"fmt"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
	"strings"
	"testing"
	"time"
)

func TestTaskMetricUsesNullWithoutDenominator(t *testing.T) {
	metric := taskMetric(model.UpstreamTaskMetricRow{Total: 1, Running: 1})
	if metric.SuccessRate != nil || metric.AvgQueueSeconds != nil || metric.AvgRunSeconds != nil || metric.AvgTotalSeconds != nil {
		t.Fatalf("undefined metrics must be null: %#v", metric)
	}
	page := dto.UpstreamTaskPageResponse{Items: []dto.UpstreamTaskItem{}, Total: 0, Page: 2, PageSize: 20, DataStatus: "pending"}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"page":2`) || !strings.Contains(text, `"page_size":20`) || strings.Contains(text, `"Page"`) || strings.Contains(text, `"PageSize"`) {
		t.Fatalf("pagination JSON contract mismatch: %s", text)
	}
}

func TestTaskBreakdownUsesOverallCompletenessOutsideSiteDimension(t *testing.T) {
	rows := []model.UpstreamTaskMetricRow{{DimensionID: "video", DimensionName: "video", Total: 1}}
	breakdown := taskBreakdown(rows, "partial", nil)
	if len(breakdown) != 1 || breakdown[0].DataStatus != "partial" {
		t.Fatalf("breakdown=%#v", breakdown)
	}
}

func TestUpstreamTaskListAsOfCoversFullFilteredResultNotCurrentPage(t *testing.T) {
	db := openUpstreamLogExportDatabase(t)
	now := int64(2101215000)
	site := model.Site{Name: "Task List", BaseURL: fmt.Sprintf("https://task-list-%d.example", time.Now().UnixNano()), ConfigVersion: 1, ManagementStatus: constant.SiteManagementActive, OnlineStatus: constant.SiteOnlineOnline, AuthStatus: constant.SiteAuthAuthorized, StatisticsStatus: constant.SiteStatisticsReady, HealthStatus: constant.SiteHealthOK, CreatedAt: now, UpdatedAt: now}
	if err := db.GORM.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.SiteUpstreamTask{
		{SiteID: site.ID, RemoteID: 2, RemoteCreatedAt: now - 30, RemoteUpdatedAt: now - 20, TaskID: "page-first", RemoteStatus: "SUCCESS", SubmitTime: now - 30, FinishTime: now - 20, SourceHash: strings.Repeat("a", 64), ConfigVersion: 1, FirstSeenAt: now - 20, LastSeenAt: now - 20, CollectedAt: now - 20, CreatedAt: now - 20, UpdatedAt: now - 20},
		{SiteID: site.ID, RemoteID: 1, RemoteCreatedAt: now - 30, RemoteUpdatedAt: now - 10, TaskID: "newer-off-page", RemoteStatus: "SUCCESS", SubmitTime: now - 30, FinishTime: now - 10, SourceHash: strings.Repeat("b", 64), ConfigVersion: 1, FirstSeenAt: now - 10, LastSeenAt: now - 10, CollectedAt: now - 10, CreatedAt: now - 10, UpdatedAt: now - 10},
	}
	if err := db.GORM.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	successAt := now
	if err := db.GORM.Create(&model.SiteUpstreamTaskCollectionState{SiteID: site.ID, OverlapStart: now - 48*3600, LastSuccessAt: &successAt, ObservedCount: 2, ConfigVersion: 1, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	service, err := NewUpstreamTaskService(db.GORM)
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.List(context.Background(), dto.UpstreamTaskQuery{Page: 1, PageSize: 1, SiteIDs: []int64{site.ID}})
	if err != nil || len(page.Items) != 1 || page.Items[0].TaskID != "page-first" || page.AsOf == nil || *page.AsOf != now-10 || page.DataStatus != "complete" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
}
