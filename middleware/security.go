package middleware

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
)

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		c.Next()
	}
}

func OriginGuard(appEnv, publicOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if appEnv != config.EnvironmentProduction || !isBrowserWrite(c.Request) {
			c.Next()
			return
		}
		origins := c.Request.Header.Values("Origin")
		if len(origins) != 1 || origins[0] == "" || origins[0] != publicOrigin {
			common.AbortError(c, http.StatusForbidden, constant.CodeOriginForbidden, "Request origin is not allowed", nil)
			return
		}
		c.Next()
	}
}

func AllowCIDRs(prefixes []netip.Prefix) gin.HandlerFunc {
	return func(c *gin.Context) {
		address, err := netip.ParseAddr(c.ClientIP())
		if err != nil || !prefixContains(prefixes, address.Unmap()) {
			common.AbortError(c, http.StatusForbidden, constant.CodeForbidden, "Access denied", nil)
			return
		}
		c.Next()
	}
}

func isBrowserWrite(request *http.Request) bool {
	if !strings.HasPrefix(request.URL.Path, "/api") {
		return false
	}
	switch request.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func prefixContains(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
