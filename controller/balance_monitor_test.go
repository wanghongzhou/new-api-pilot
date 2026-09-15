package controller

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"new-api-pilot/dto"
	"strings"
	"testing"
)

type balanceFake struct{ writes int }

func (f *balanceFake) Ingest(context.Context, dto.BalanceIngest) error { f.writes++; return nil }
func (f *balanceFake) List(context.Context, dto.BalanceQuery) (dto.BalancePage, error) {
	return dto.BalancePage{Total: "0"}, nil
}
func TestBalanceReceiverRejectsUnauthorizedAndMalformed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &balanceFake{}
	c := NewBalanceMonitorController(fake, strings.Repeat("a", 32))
	e := gin.New()
	e.POST("/ingest", c.Ingest)
	for _, test := range []struct {
		token, body string
		status      int
	}{{"", "{}", 401}, {"Bearer wrong", "{}", 401}, {"Bearer " + strings.Repeat("a", 32), "{}", 400}} {
		request := httptest.NewRequest("POST", "/ingest", strings.NewReader(test.body))
		request.Header.Set("Authorization", test.token)
		result := httptest.NewRecorder()
		e.ServeHTTP(result, request)
		if result.Code != test.status {
			t.Fatalf("got %d want %d", result.Code, test.status)
		}
	}
	if fake.writes != 0 {
		t.Fatal("invalid request persisted")
	}
}
