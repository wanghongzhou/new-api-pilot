package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
)

type fakeChannelInventoryApp struct {
	q dto.ChannelInventoryQuery
	s dto.ChannelInventoryStatisticsQuery
}

func (a *fakeChannelInventoryApp) List(_ context.Context, q dto.ChannelInventoryQuery) (dto.ChannelInventoryPage, error) {
	a.q = q
	return dto.ChannelInventoryPage{Items: []dto.ChannelInventoryItem{}, Page: q.Page, PageSize: q.PageSize, DataStatus: "complete"}, nil
}
func (a *fakeChannelInventoryApp) Statistics(_ context.Context, q dto.ChannelInventoryStatisticsQuery) (dto.ChannelInventoryStatisticsResponse, error) {
	a.s = q
	return dto.ChannelInventoryStatisticsResponse{Trend: []dto.ChannelInventoryTrendPoint{}, DataStatus: "complete"}, nil
}
func TestChannelInventoryControllerStrictQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &fakeChannelInventoryApp{}
	c := NewChannelInventoryController(a)
	e := gin.New()
	e.GET("/channels", c.Global)
	e.GET("/stats", c.GlobalStatistics)
	e.GET("/sites/:id/channels", c.Site)
	r := httptest.NewRecorder()
	e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/channels?p=2&page_size=10&site_ids=3&types=1&statuses=1&groups=vip&tags=prod&states=normal&min_balance=0.1&max_response_time_ms=900", nil))
	if r.Code != 200 || a.q.Page != 2 || a.q.MaxResponseTimeMS == nil || *a.q.MaxResponseTimeMS != 900 {
		t.Fatalf("channel query=%#v code=%d body=%s", a.q, r.Code, r.Body.String())
	}
	for _, target := range []string{"/channels?p=1&page_size=20&min_balance=01", "/channels?p=1&page_size=20&types=bad", "/channels?unknown=value", "/channels?p=1&p=2", "/sites/2/channels?site_ids=3", "/stats?start_timestamp=3601&end_timestamp=7200", "/stats?start_timestamp=3600&end_timestamp=7200&types=bad", "/stats?start_timestamp=3600&start_timestamp=7200&end_timestamp=10800"} {
		r = httptest.NewRecorder()
		e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, target, nil))
		if r.Code != 400 {
			t.Fatalf("invalid %s=%d %s", target, r.Code, r.Body.String())
		}
	}
	r = httptest.NewRecorder()
	e.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/channels", strings.NewReader(`{}`)))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("non-empty channel body=%d %s", r.Code, r.Body.String())
	}
}
