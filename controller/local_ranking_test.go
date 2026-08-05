package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/dto"
	"new-api-pilot/service"
)

type failingLocalRankingApplication struct{ err error }

func (app failingLocalRankingApplication) Query(context.Context, dto.LocalRankingQuery, string) (dto.LocalRankingResponse, error) {
	return dto.LocalRankingResponse{}, app.err
}

func TestLocalRankingControllerMapsOnlyValidationErrorsToBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		err  error
		want int
	}{{name: "validation", err: service.ErrStatisticsInvalid, want: http.StatusBadRequest}, {name: "database", err: errors.New("database unavailable"), want: http.StatusInternalServerError}} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/rankings", NewLocalRankingController(failingLocalRankingApplication{err: test.err}).GlobalModels)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/rankings?period=today", nil))
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
