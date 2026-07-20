package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
)

func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		log.Printf(
			"http request request_id=%s method=%s route=%s status=%d duration_ms=%d",
			common.RequestID(c),
			c.Request.Method,
			route,
			c.Writer.Status(),
			time.Since(startedAt).Milliseconds(),
		)
	}
}
