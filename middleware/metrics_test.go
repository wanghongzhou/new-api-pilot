package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
)

func TestHTTPMetricsUseGinRouteTemplatesAndFixedUnmatchedLabel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := common.NewMetrics()
	engine := gin.New()
	engine.Use(HTTPMetrics(metrics))
	engine.GET("/api/sites/:id", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": c.Param("id")})
	})
	engine.NoRoute(func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	performMetricsRequest(engine, "/api/sites/9007199254740993?request_id=secret")
	performMetricsRequest(engine, "/not-registered/secret-value")

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{
		`method="GET",route="/api/sites/:id",status_class="2xx"`,
		`method="GET",route="unmatched",status_class="4xx"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics scrape missing %q\n%s", expected, body)
		}
	}
	for _, forbidden := range []string{"9007199254740993", "request_id", "secret-value"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics scrape leaked request value %q", forbidden)
		}
	}
}

func performMetricsRequest(handler http.Handler, target string) {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
}
