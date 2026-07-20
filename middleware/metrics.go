package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
)

func HTTPMetrics(metrics *common.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		metrics.ObserveHTTP(c.Request.Method, route, c.Writer.Status(), time.Since(startedAt))
	}
}
