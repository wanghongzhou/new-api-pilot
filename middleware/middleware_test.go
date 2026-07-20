package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
)

func TestRequestIDPreservesValidAndReplacesInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestID())
	engine.GET("/", func(c *gin.Context) { common.WriteSuccess(c, http.StatusOK, nil) })

	valid := httptest.NewRequest(http.MethodGet, "/", nil)
	valid.Header.Set(RequestIDHeader, "request.valid-1")
	validResponse := httptest.NewRecorder()
	engine.ServeHTTP(validResponse, valid)
	if validResponse.Header().Get(RequestIDHeader) != "request.valid-1" {
		t.Fatalf("valid request ID was not preserved: %s", validResponse.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodGet, "/", nil)
	invalid.Header.Set(RequestIDHeader, "contains spaces")
	invalidResponse := httptest.NewRecorder()
	engine.ServeHTTP(invalidResponse, invalid)
	generated := invalidResponse.Header().Get(RequestIDHeader)
	if generated == "" || generated == "contains spaces" || len(generated) > 64 {
		t.Fatalf("invalid generated request ID %q", generated)
	}
}

func TestProductionOriginGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestID(), OriginGuard(config.EnvironmentProduction, "https://pilot.example.com"))
	engine.POST("/api/write", func(c *gin.Context) { common.WriteSuccess(c, http.StatusOK, nil) })

	for _, test := range []struct {
		name    string
		origins []string
		status  int
	}{
		{name: "missing", status: http.StatusForbidden},
		{name: "empty", origins: []string{""}, status: http.StatusForbidden},
		{name: "null", origins: []string{"null"}, status: http.StatusForbidden},
		{name: "other", origins: []string{"https://other.example.com"}, status: http.StatusForbidden},
		{name: "host case", origins: []string{"https://PILOT.example.com"}, status: http.StatusForbidden},
		{name: "default port", origins: []string{"https://pilot.example.com:443"}, status: http.StatusForbidden},
		{name: "comma joined", origins: []string{"https://pilot.example.com, https://other.example.com"}, status: http.StatusForbidden},
		{name: "duplicate same", origins: []string{"https://pilot.example.com", "https://pilot.example.com"}, status: http.StatusForbidden},
		{name: "duplicate mixed", origins: []string{"https://pilot.example.com", "https://other.example.com"}, status: http.StatusForbidden},
		{name: "exact", origins: []string{"https://pilot.example.com"}, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/write", nil)
			for _, origin := range test.origins {
				request.Header.Add("Origin", origin)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("origins %#v status = %d, want %d", test.origins, response.Code, test.status)
			}
		})
	}
}

func TestCIDRGuardAndSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prefix := netip.MustParsePrefix("127.0.0.0/8")
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil)
	engine.Use(RequestID(), SecurityHeaders(), AllowCIDRs([]netip.Prefix{prefix}))
	engine.GET("/metrics", func(c *gin.Context) { c.Status(http.StatusOK) })

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("allowed response = %d headers=%v", response.Code, response.Header())
	}

	request = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("denied response status = %d", response.Code)
	}
}

type fixedIdentityResolver struct {
	identity Identity
	err      error
}

func (resolver fixedIdentityResolver) ResolveIdentity(*gin.Context) (Identity, error) {
	return resolver.identity, resolver.err
}

func TestAuthMiddlewareUsesResolverHeaderAndRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := fixedIdentityResolver{identity: Identity{ID: "9007199254740993", Role: constant.RoleViewer, Status: 1}}
	engine := gin.New()
	engine.Use(RequestID(), UserAuth(resolver))
	engine.GET("/read", func(c *gin.Context) { common.WriteSuccess(c, http.StatusOK, nil) })
	engine.POST("/admin", AdminAuth(), func(c *gin.Context) { common.WriteSuccess(c, http.StatusOK, nil) })

	request := httptest.NewRequest(http.MethodGet, "/read", nil)
	request.Header.Set("New-Api-User", "9007199254740993")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("viewer read status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/admin", nil)
	request.Header.Set("New-Api-User", "9007199254740993")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer admin status = %d", response.Code)
	}

	badResolver := fixedIdentityResolver{err: errors.New("invalid session")}
	badEngine := gin.New()
	badEngine.Use(RequestID(), UserAuth(badResolver))
	badEngine.GET("/read", func(c *gin.Context) {})
	request = httptest.NewRequest(http.MethodGet, "/read", nil)
	response = httptest.NewRecorder()
	badEngine.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid identity status = %d", response.Code)
	}
}

func TestRecoveryUsesStableEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var applicationLog bytes.Buffer
	var ginRecoveryLog bytes.Buffer
	previousLogWriter := log.Writer()
	previousGinWriter := gin.DefaultErrorWriter
	log.SetOutput(&applicationLog)
	gin.DefaultErrorWriter = &ginRecoveryLog
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
		gin.DefaultErrorWriter = previousGinWriter
	})
	const secret = "a45-panic-secret-never-log"
	engine := gin.New()
	engine.Use(RequestID(), Recovery())
	engine.GET("/panic", func(c *gin.Context) { panic(secret) })
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic?access_token="+secret, nil)
	request.Header.Set("Authorization", secret)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d", response.Code)
	}
	var envelope common.APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Code != constant.CodeInternalError || envelope.RequestID == "" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	combined := applicationLog.String() + ginRecoveryLog.String() + response.Body.String()
	for _, forbidden := range []string{secret, "access_token", "Authorization"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("panic handling leaked %q: %s", forbidden, combined)
		}
	}
	if !strings.Contains(applicationLog.String(), "panic recovered request_id=") ||
		!strings.Contains(applicationLog.String(), "method=GET route=/panic") {
		t.Fatalf("safe panic log is incomplete: %q", applicationLog.String())
	}
}
