package controller

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/service"
	"strings"
	"testing"
)

type fakePerformanceHistoryApp struct {
	q             dto.PerformanceHistoryQuery
	statisticsErr error
}

func (a *fakePerformanceHistoryApp) List(_ context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryPage, error) {
	a.q = q
	return dto.PerformanceHistoryPage{Items: []dto.PerformanceHistoryItem{}, Page: q.Page, PageSize: q.PageSize, DataStatus: "complete"}, nil
}
func (a *fakePerformanceHistoryApp) Statistics(_ context.Context, q dto.PerformanceHistoryQuery) (dto.PerformanceHistoryStatisticsResponse, error) {
	a.q = q
	return dto.PerformanceHistoryStatisticsResponse{Trend: []dto.PerformanceHistoryItem{}, SiteBreakdown: []dto.PerformanceHistoryItem{}, AggregationStatus: "unavailable", DataStatus: "complete"}, a.statisticsErr
}

func TestPerformanceHistoryStatisticsRejectsOversizedResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &fakePerformanceHistoryApp{statisticsErr: service.ErrPerformanceHistoryTooLarge}
	c := NewPerformanceHistoryController(a)
	e := gin.New()
	e.GET("/stats", c.GlobalStatistics)
	r := httptest.NewRecorder()
	e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/stats?start_timestamp=100&end_timestamp=200", nil))
	if r.Code != http.StatusRequestEntityTooLarge || !strings.Contains(r.Body.String(), constant.CodePayloadTooLarge) {
		t.Fatalf("oversized statistics=%d %s", r.Code, r.Body.String())
	}
}
func TestPerformanceHistoryControllerQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &fakePerformanceHistoryApp{}
	c := NewPerformanceHistoryController(a)
	e := gin.New()
	e.GET("/history", c.Global)
	e.GET("/stats", c.GlobalStatistics)
	r := httptest.NewRecorder()
	e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/history?start_timestamp=100&end_timestamp=200&site_ids=3&model_names=gpt&groups=vip&p=2&page_size=10", nil))
	if r.Code != 200 || a.q.Page != 2 || len(a.q.SiteIDs) != 1 || a.q.ModelNames[0] != "gpt" {
		t.Fatalf("history query=%#v code=%d body=%s", a.q, r.Code, r.Body.String())
	}
	for _, target := range []string{"/history?start_timestamp=200&end_timestamp=100&p=1&page_size=20", "/history?start_timestamp=100&end_timestamp=200&site_ids=bad&p=1&page_size=20"} {
		r = httptest.NewRecorder()
		e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, target, nil))
		if r.Code != 400 {
			t.Fatalf("invalid %s=%d %s", target, r.Code, r.Body.String())
		}
	}
}
