package testsupport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

const maximumCapturedRequestBytes = 1 << 20

type CapturedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

type FakeUpstream struct {
	server   *httptest.Server
	testing  testing.TB
	fixture  string
	scenario string
	routes   []loadedUpstreamRoute

	mu       sync.Mutex
	requests []CapturedRequest
}

func NewFakeUpstream(t testing.TB, scenario string) *FakeUpstream {
	t.Helper()
	fixtureDir := DesignFixturePath("f02-upstream")
	manifest := loadUpstreamManifest(t, fixtureDir)
	definition, ok := manifest.Scenarios[scenario]
	if !ok {
		names := make([]string, 0, len(manifest.Scenarios))
		for name := range manifest.Scenarios {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Fatalf("unknown F02 scenario %q; available: %s", scenario, strings.Join(names, ", "))
	}

	fake := &FakeUpstream{
		testing:  t,
		fixture:  fixtureDir,
		scenario: scenario,
		routes:   loadUpstreamRoutes(t, fixtureDir, definition.Routes),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	t.Cleanup(fake.Close)
	return fake
}

func (fake *FakeUpstream) URL() string {
	return fake.server.URL
}

func (fake *FakeUpstream) Client() *http.Client {
	return fake.server.Client()
}

func (fake *FakeUpstream) Close() {
	if fake.server != nil {
		fake.server.Close()
		fake.server = nil
	}
}

func (fake *FakeUpstream) Requests() []CapturedRequest {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	requests := make([]CapturedRequest, len(fake.requests))
	for index, request := range fake.requests {
		requests[index] = CapturedRequest{
			Method: request.Method,
			Path:   request.Path,
			Query:  cloneValues(request.Query),
			Header: request.Header.Clone(),
			Body:   append([]byte(nil), request.Body...),
		}
	}
	return requests
}

func (fake *FakeUpstream) Count(routeID string) int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	count := 0
	for _, request := range fake.requests {
		for _, route := range fake.routes {
			if route.definition.ID == routeID && route.matches(request.Method, request.Path, request.Query) {
				count++
				break
			}
		}
	}
	return count
}

func (fake *FakeUpstream) serveHTTP(response http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, maximumCapturedRequestBytes+1))
	if err != nil {
		fake.fail(response, http.StatusInternalServerError, "read request body: %v", err)
		return
	}
	if len(body) > maximumCapturedRequestBytes {
		fake.fail(response, http.StatusRequestEntityTooLarge, "request body exceeds %d bytes", maximumCapturedRequestBytes)
		return
	}

	captured := CapturedRequest{
		Method: request.Method,
		Path:   request.URL.Path,
		Query:  cloneValues(request.URL.Query()),
		Header: request.Header.Clone(),
		Body:   body,
	}
	fake.mu.Lock()
	fake.requests = append(fake.requests, captured)
	fake.mu.Unlock()

	var matched *loadedUpstreamRoute
	for index := range fake.routes {
		if fake.routes[index].matches(captured.Method, captured.Path, captured.Query) {
			matched = &fake.routes[index]
			break
		}
	}
	if matched == nil {
		fake.fail(response, http.StatusNotFound, "unexpected %s %s?%s in F02 scenario %q", request.Method, request.URL.Path, request.URL.RawQuery, fake.scenario)
		return
	}

	if problem := matched.validateHeaders(request.Header); problem != "" {
		fake.fail(response, http.StatusBadRequest, "route %q: %s", matched.definition.ID, problem)
		return
	}
	if matched.definition.Disconnect {
		disconnect(response)
		return
	}

	for name, value := range matched.definition.Headers {
		response.Header().Set(name, value)
	}
	if response.Header().Get("Content-Type") == "" {
		response.Header().Set("Content-Type", "application/json")
	}
	status := matched.definition.Status
	if status == 0 {
		status = http.StatusOK
	}
	response.WriteHeader(status)
	if len(matched.body) > 0 {
		if _, err := response.Write(matched.body); err != nil {
			fake.testing.Errorf("F02 route %q write response: %v", matched.definition.ID, err)
		}
	}
}

func (fake *FakeUpstream) fail(response http.ResponseWriter, status int, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fake.testing.Errorf("fake upstream: %s", message)
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]any{
		"success": false,
		"message": message,
	})
}

func DesignFixturePath(parts ...string) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("testsupport: resolve fixture path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "design"))
	return filepath.Join(append([]string{root}, parts...)...)
}

type upstreamManifest struct {
	SchemaVersion   int                         `json:"schema_version"`
	FixtureID       string                      `json:"fixture_id"`
	Description     string                      `json:"description"`
	FixedNowUnix    int64                       `json:"fixed_now_unix"`
	Scenarios       map[string]upstreamScenario `json:"scenarios"`
}

type upstreamScenario struct {
	Routes []upstreamRoute `json:"routes"`
}

type upstreamRoute struct {
	ID               string            `json:"id"`
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	Query            map[string]string `json:"query"`
	Status           int               `json:"status"`
	Headers          map[string]string `json:"headers"`
	ResponseFile     string            `json:"response_file"`
	ExpectedHeaders  map[string]string `json:"expected_headers"`
	RequiredHeaders  []string          `json:"required_headers"`
	ForbiddenHeaders []string          `json:"forbidden_headers"`
	Disconnect       bool              `json:"disconnect"`
}

type loadedUpstreamRoute struct {
	definition upstreamRoute
	body       []byte
}

func loadUpstreamManifest(t testing.TB, fixtureDir string) upstreamManifest {
	t.Helper()
	file, err := os.Open(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatalf("open F02 manifest: %v", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest upstreamManifest
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode F02 manifest: %v", err)
	}
	if manifest.SchemaVersion != 1 || manifest.FixtureID != "F02" {
		t.Fatalf("unexpected F02 manifest identity: version=%d id=%q", manifest.SchemaVersion, manifest.FixtureID)
	}
	return manifest
}

func loadUpstreamRoutes(t testing.TB, fixtureDir string, definitions []upstreamRoute) []loadedUpstreamRoute {
	t.Helper()
	routes := make([]loadedUpstreamRoute, 0, len(definitions))
	identities := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" || definition.Method == "" || definition.Path == "" {
			t.Fatalf("F02 route must have id, method, and path: %+v", definition)
		}
		identity := definition.Method + " " + definition.Path + "?" + encodeQuery(definition.Query)
		if _, exists := identities[identity]; exists {
			t.Fatalf("duplicate F02 route identity %s", identity)
		}
		identities[identity] = struct{}{}

		var body []byte
		if definition.ResponseFile != "" {
			responsePath, err := safeFixturePath(fixtureDir, definition.ResponseFile)
			if err != nil {
				t.Fatalf("F02 route %q response path: %v", definition.ID, err)
			}
			body, err = os.ReadFile(responsePath)
			if err != nil {
				t.Fatalf("read F02 route %q response: %v", definition.ID, err)
			}
			if !json.Valid(body) {
				t.Fatalf("F02 route %q response is not valid JSON", definition.ID)
			}
		}
		routes = append(routes, loadedUpstreamRoute{definition: definition, body: body})
	}
	return routes
}

func (route loadedUpstreamRoute) matches(method string, path string, query url.Values) bool {
	if !strings.EqualFold(route.definition.Method, method) || route.definition.Path != path {
		return false
	}
	if len(query) != len(route.definition.Query) {
		return false
	}
	for name, expected := range route.definition.Query {
		values, ok := query[name]
		if !ok || len(values) != 1 || values[0] != expected {
			return false
		}
	}
	return true
}

func (route loadedUpstreamRoute) validateHeaders(header http.Header) string {
	for name, expected := range route.definition.ExpectedHeaders {
		if actual := header.Get(name); actual != expected {
			return fmt.Sprintf("header %s = %q, want %q", name, actual, expected)
		}
	}
	for _, name := range route.definition.RequiredHeaders {
		if strings.TrimSpace(header.Get(name)) == "" {
			return fmt.Sprintf("required header %s is empty", name)
		}
	}
	for _, name := range route.definition.ForbiddenHeaders {
		if header.Get(name) != "" {
			return fmt.Sprintf("forbidden header %s was sent", name)
		}
	}
	return ""
}

func safeFixturePath(root string, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("absolute path is not allowed")
	}
	resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes fixture directory")
	}
	return resolved, nil
}

func encodeQuery(query map[string]string) string {
	values := make(url.Values, len(query))
	for name, value := range query {
		values.Set(name, value)
	}
	return values.Encode()
}

func cloneValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for name, entries := range values {
		clone[name] = append([]string(nil), entries...)
	}
	return clone
}

func disconnect(response http.ResponseWriter) {
	hijacker, ok := response.(http.Hijacker)
	if !ok {
		panic("testsupport: response writer cannot disconnect")
	}
	connection, buffer, err := hijacker.Hijack()
	if err != nil {
		panic(fmt.Sprintf("testsupport: disconnect: %v", err))
	}
	closeBufferedConnection(connection, buffer)
}

func closeBufferedConnection(connection net.Conn, buffer *bufio.ReadWriter) {
	_ = buffer.Flush()
	_ = connection.Close()
}
