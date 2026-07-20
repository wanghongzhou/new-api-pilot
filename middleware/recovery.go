package middleware

import (
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/constant"
)

func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		log.Printf("panic recovered request_id=%s method=%s route=%s", common.RequestID(c), c.Request.Method, c.FullPath())
		common.AbortError(c, http.StatusInternalServerError, constant.CodeInternalError, "Internal server error", nil)
	})
}
