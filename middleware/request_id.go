package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"new-api-pilot/constant"
)

const RequestIDHeader = "X-Request-ID"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

var (
	requestIDEntropy         = rand.Read
	requestIDFallbackCounter atomic.Uint64
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if !requestIDPattern.MatchString(requestID) {
			requestID = newRequestID()
		}
		c.Set(constant.ContextRequestID, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func newRequestID() string {
	random := make([]byte, 16)
	if count, err := requestIDEntropy(random); err == nil && count == len(random) {
		return "req_" + hex.EncodeToString(random)
	}
	// Request IDs are correlation identifiers, not authentication secrets. A
	// process-local fallback avoids panicking before Recovery enters the chain.
	sequence := requestIDFallbackCounter.Add(1)
	return fmt.Sprintf("req_fallback_%x_%x", uint64(time.Now().UnixNano()), sequence)
}
