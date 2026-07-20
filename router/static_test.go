package router

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
)

func TestStaticWebAssetsServeExactFilesCacheValidatorsAndHead(t *testing.T) {
	index := []byte("<!doctype html><html><body>frontend-shell</body></html>")
	javascript := []byte(`console.log("loaded")`)
	engine := newStaticFixtureEngine(t, fstest.MapFS{
		"index.html":                             {Data: index},
		"favicon.svg":                            {Data: []byte("<svg></svg>")},
		"static/js/app.0123456789abcdef.js":      {Data: javascript},
		"static/css/app.0123456789abcdef.css":    {Data: []byte("body{}")},
		"static/font/app.0123456789abcdef.woff2": {Data: []byte("font")},
	})

	response := performRequest(engine, http.MethodGet, "/", "127.0.0.1:1000")
	if response.Code != http.StatusOK || response.Body.String() != string(index) {
		t.Fatalf("index response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != revalidateCacheControl {
		t.Fatalf("index Cache-Control = %q", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("index Content-Type = %q", got)
	}
	indexETag := response.Header().Get("ETag")
	if !strings.HasPrefix(indexETag, `"`) || !strings.HasSuffix(indexETag, `"`) {
		t.Fatalf("index ETag = %q", indexETag)
	}

	response = performRequest(engine, http.MethodGet, "/static/js/app.0123456789abcdef.js", "127.0.0.1:1000")
	if response.Code != http.StatusOK || response.Body.String() != string(javascript) {
		t.Fatalf("asset response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != immutableCacheControl {
		t.Fatalf("asset Cache-Control = %q", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("asset Content-Type = %q", got)
	}
	assetETag := response.Header().Get("ETag")
	if assetETag == "" {
		t.Fatal("asset ETag is empty")
	}

	response = performStaticRequest(
		engine,
		http.MethodGet,
		"/static/js/app.0123456789abcdef.js",
		map[string]string{"If-None-Match": assetETag},
	)
	if response.Code != http.StatusNotModified || response.Body.Len() != 0 {
		t.Fatalf("conditional asset response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("ETag"); got != assetETag {
		t.Fatalf("conditional asset ETag = %q, want %q", got, assetETag)
	}

	response = performRequest(engine, http.MethodHead, "/static/js/app.0123456789abcdef.js", "127.0.0.1:1000")
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("HEAD asset response = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Length"); got != strconv.Itoa(len(javascript)) {
		t.Fatalf("HEAD asset Content-Length = %q", got)
	}
	if got := response.Header().Get("ETag"); got != assetETag {
		t.Fatalf("HEAD asset ETag = %q, want %q", got, assetETag)
	}

	response = performRequest(engine, http.MethodGet, "/favicon.svg", "127.0.0.1:1000")
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != revalidateCacheControl {
		t.Fatalf("unhashed asset response = %d headers=%v", response.Code, response.Header())
	}
}

func TestStaticWebAssetsFallbackWithoutTakingReservedRoutes(t *testing.T) {
	index := []byte("<!doctype html><html><body>frontend-shell</body></html>")
	engine := newStaticFixtureEngine(t, fstest.MapFS{"index.html": {Data: index}})

	for _, target := range []string{"/exports?exportId=9007199254740993", "/nested/client/route", "/missing.js"} {
		response := performRequest(engine, http.MethodGet, target, "127.0.0.1:1000")
		if response.Code != http.StatusOK || response.Body.String() != string(index) {
			t.Errorf("fallback %s = %d %q", target, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != revalidateCacheControl {
			t.Errorf("fallback %s Cache-Control = %q", target, response.Header().Get("Cache-Control"))
		}
	}

	response := performRequest(engine, http.MethodHead, "/exports/active", "127.0.0.1:1000")
	if response.Code != http.StatusOK || response.Body.Len() != 0 || response.Header().Get("Content-Length") != strconv.Itoa(len(index)) {
		t.Fatalf("fallback HEAD = %d body=%q headers=%v", response.Code, response.Body.String(), response.Header())
	}

	response = performRequest(engine, http.MethodGet, "/api/not-registered", "127.0.0.1:1000")
	var envelope common.APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode API envelope: %v", err)
	}
	if response.Code != http.StatusNotFound || envelope.Code != constant.CodeNotFound || envelope.RequestID == "" {
		t.Fatalf("unknown API response = %d %#v", response.Code, envelope)
	}
	if strings.Contains(response.Body.String(), "frontend-shell") {
		t.Fatal("unknown API route used the SPA fallback")
	}

	for _, testCase := range []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodGet, path: "/healthz", status: http.StatusOK},
		{method: http.MethodGet, path: "/readyz", status: http.StatusServiceUnavailable},
		{method: http.MethodGet, path: "/metrics", status: http.StatusNotFound},
		{method: http.MethodGet, path: "/healthz/missing", status: http.StatusNotFound},
		{method: http.MethodGet, path: "/readyz/missing", status: http.StatusNotFound},
		{method: http.MethodGet, path: "/metrics/missing", status: http.StatusNotFound},
		{method: http.MethodPost, path: "/exports", status: http.StatusNotFound},
	} {
		response = performRequest(engine, testCase.method, testCase.path, "127.0.0.1:1000")
		if response.Code != testCase.status || strings.Contains(response.Body.String(), "frontend-shell") {
			t.Errorf("reserved request %s %s = %d %q", testCase.method, testCase.path, response.Code, response.Body.String())
		}
	}

	withoutAssets := newStaticFixtureEngine(t, nil)
	response = performRequest(withoutAssets, http.MethodGet, "/", "127.0.0.1:1000")
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "frontend-shell") {
		t.Fatalf("local build root response = %d %q", response.Code, response.Body.String())
	}
}

func TestStaticWebAssetsRejectTraversalHiddenFilesAndInvalidBundles(t *testing.T) {
	engine := newStaticFixtureEngine(t, fstest.MapFS{
		"index.html":     {Data: []byte("frontend-shell")},
		"secret.txt":     {Data: []byte("top-secret")},
		".env":           {Data: []byte("hidden-secret")},
		"nested/.secret": {Data: []byte("nested-secret")},
	})

	for _, target := range []string{
		"/../secret.txt",
		"/%2e%2e/secret.txt",
		"/safe/../secret.txt",
		"/safe/%2e%2e/secret.txt",
		"/.env",
		"/nested/.secret",
		"/static%5csecret.txt",
	} {
		response := performRequest(engine, http.MethodGet, target, "127.0.0.1:1000")
		if response.Code != http.StatusNotFound {
			t.Errorf("unsafe path %s status = %d", target, response.Code)
		}
		if body := response.Body.String(); strings.Contains(body, "top-secret") || strings.Contains(body, "hidden-secret") || strings.Contains(body, "frontend-shell") {
			t.Errorf("unsafe path %s exposed content %q", target, body)
		}
	}

	if _, err := newStaticServer(fstest.MapFS{"static/app.js": {Data: []byte("app")}}); err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("bundle without index error = %v", err)
	}
	if _, err := newStaticServer(fstest.MapFS{"index.html": {Data: nil}}); err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("bundle with empty index error = %v", err)
	}
}

func newStaticFixtureEngine(t *testing.T, assets fs.FS) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine, err := New(Options{
		Config:    config.Config{AppEnv: config.EnvironmentTest},
		WebAssets: assets,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return engine
}

func performStaticRequest(handler http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.RemoteAddr = "127.0.0.1:1000"
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
