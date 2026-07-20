package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

const (
	statusVersion       = "v0.6.11-fixture"
	f02RootToken        = "fixture-root-token"
	f02HourStart  int64 = 1768406400
)

type f02Manifest struct {
	FixedNowUnix int64                  `json:"fixed_now_unix"`
	Scenarios    map[string]f02Scenario `json:"scenarios"`
}

type f02Scenario struct {
	Routes []f02Route `json:"routes"`
}

type f02Route struct {
	ID               string            `json:"id"`
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	Query            map[string]string `json:"query"`
	ResponseFile     string            `json:"response_file"`
	ExpectedHeaders  map[string]string `json:"expected_headers"`
	RequiredHeaders  []string          `json:"required_headers"`
	ForbiddenHeaders []string          `json:"forbidden_headers"`
	Headers          map[string]string `json:"headers"`
	Status           int               `json:"status"`
	Disconnect       bool              `json:"disconnect"`
}

type f02Server struct {
	t      *testing.T
	server *httptest.Server
	routes []f02Route
	mu     sync.Mutex
	hits   map[string]int
	errors []string
}

func TestNewAPIClientF02Scenarios(t *testing.T) {
	manifest := loadF02Manifest(t)
	fixedNow := time.Unix(manifest.FixedNowUnix, 0)

	t.Run("supported", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["supported"], fixedNow)
		status, err := client.Status(context.Background(), "f02-status")
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if status.Version != statusVersion || status.QuotaPerUnit != "500000" || status.USDExchangeRate != "7.2" {
			t.Fatalf("unexpected status: %+v", status)
		}
		identity, err := client.Self(context.Background(), "f02-self")
		if err != nil || identity.ID != 1 {
			t.Fatalf("self: %+v, %v", identity, err)
		}
		root, err := client.GetUser(context.Background(), "f02-root", 1)
		if err != nil {
			t.Fatalf("root user: %v", err)
		}
		if root.Quota != 9007199254740993 || root.UsedQuota != 9007199254740994 || root.RequestCount != 9007199254740995 {
			t.Fatalf("bigint precision lost: %+v", root)
		}
		users, err := client.SnapshotUsers(context.Background(), "f02-users")
		if err != nil {
			t.Fatalf("users snapshot: %v", err)
		}
		if users.Total != 4 || len(users.Items) != 4 || !users.Items[1].Deleted || users.Items[3].Deleted {
			t.Fatalf("unexpected users snapshot: %+v", users)
		}
		channels, err := client.SnapshotChannels(context.Background(), "f02-channels")
		if err != nil || channels.Total != 2 || len(channels.Items) != 2 {
			t.Fatalf("channels snapshot: %+v, %v", channels, err)
		}
		flow, err := client.FlowHour(context.Background(), "f02-flow", f02HourStart)
		if err != nil || len(flow) != 3 || flow[1].ModelName != "model-a" || flow[1].ChannelID != 0 {
			t.Fatalf("flow: %+v, %v", flow, err)
		}
		data, err := client.DataHour(context.Background(), "f02-data", f02HourStart)
		if err != nil || len(data) != 2 {
			t.Fatalf("data: %+v, %v", data, err)
		}
		if err := ValidateFlowDataConsistency(flow, data); err != nil {
			t.Fatalf("flow/data consistency: %v", err)
		}
		instances, err := client.Instances(context.Background(), "f02-instances")
		if err != nil || len(instances) != 2 || instances[0].Hostname != instances[1].Hostname || *instances[0].IsMaster != true {
			t.Fatalf("instances: %+v, %v", instances, err)
		}
		stat, err := client.LogStat(context.Background(), "f02-realtime")
		if err != nil || stat.RPM != 120 || stat.TPM != 24000 {
			t.Fatalf("log stat: %+v, %v", stat, err)
		}
		performance, err := client.PerformanceSummary(context.Background(), "f02-performance", 24)
		if err != nil || len(performance.Models) != 2 || performance.Models[1].RequestCount != 30 {
			t.Fatalf("performance summary: %+v, %v", performance, err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("export_disabled", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["export_disabled"], fixedNow)
		status, err := client.Status(context.Background(), "f02-export")
		if !errors.Is(err, ErrUpstreamExportDisabled) || status.DataExportEnabled {
			t.Fatalf("expected export disabled, got %+v, %v", status, err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("flow_username_canonicalization", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["flow_username_canonicalization"], fixedNow)
		flow, err := client.FlowHour(context.Background(), "f02-flow-username", f02HourStart)
		if err != nil || len(flow) != 1 || flow[0].Username != "alpha-user" ||
			flow[0].RequestCount != 10 || flow[0].Quota != 100 || flow[0].TokenUsed != 1000 {
			t.Fatalf("unexpected canonical flow username: %+v, %v", flow, err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("flow_data_mismatch", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["flow_data_mismatch"], fixedNow)
		flow, err := client.FlowHour(context.Background(), "f02-flow-mismatch", f02HourStart)
		if err != nil {
			t.Fatalf("flow: %v", err)
		}
		data, err := client.DataHour(context.Background(), "f02-data-mismatch", f02HourStart)
		if err != nil {
			t.Fatalf("data: %v", err)
		}
		if err := ValidateFlowDataConsistency(flow, data); !errors.Is(err, ErrUpstreamDataMismatch) {
			t.Fatalf("expected data mismatch, got %v", err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("users_total_drift", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["users_total_drift"], fixedNow)
		if _, err := client.SnapshotUsers(context.Background(), "f02-user-drift"); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("expected invalid snapshot, got %v", err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("instances_empty", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["instances_empty"], fixedNow)
		instances, err := client.Instances(context.Background(), "f02-empty-instances")
		if err != nil || len(instances) != 0 || instances == nil {
			t.Fatalf("expected a complete empty snapshot, got %#v, %v", instances, err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("instances_invalid", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["instances_invalid"], fixedNow)
		if _, err := client.Instances(context.Background(), "f02-invalid-instances"); !errors.Is(err, ErrUpstreamResponseInvalid) {
			t.Fatalf("expected invalid instances response, got %v", err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("token_rotation_disconnect", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["token_rotation_disconnect"], fixedNow)
		_, token, err := client.LoginAndGenerateAccessToken(context.Background(), "f02-token", "root", "fixture-password")
		if !errors.Is(err, ErrUpstreamTokenRotationResultUnknown) || token != "" {
			t.Fatalf("expected unknown token rotation result, got token=%q err=%v", token, err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("upstream_429", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["upstream_429"], fixedNow)
		_, err := client.FlowHour(context.Background(), "f02-429", f02HourStart)
		var typed *UpstreamRequestError
		if !errors.As(err, &typed) || !errors.Is(err, ErrUpstreamRateLimited) || typed.RetryAfter != 2*time.Minute {
			t.Fatalf("expected typed rate limit, got %T %v", err, err)
		}
		fixture.assertAllRoutesHitOnce()
	})

	t.Run("upstream_500", func(t *testing.T) {
		fixture, client := newF02Client(t, manifest.Scenarios["upstream_500"], fixedNow)
		if _, err := client.FlowHour(context.Background(), "f02-500", f02HourStart); !errors.Is(err, ErrUpstreamRemote) {
			t.Fatalf("expected upstream server error, got %v", err)
		}
		fixture.assertAllRoutesHitOnce()
	})
}

func loadF02Manifest(t *testing.T) f02Manifest {
	t.Helper()
	path := filepath.Join("..", "testdata", "design", "f02-upstream", "manifest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read F02 manifest: %v", err)
	}
	var manifest f02Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode F02 manifest: %v", err)
	}
	return manifest
}

func newF02Client(t *testing.T, scenario f02Scenario, now time.Time) (*f02Server, *NewAPIClient) {
	t.Helper()
	fixture := &f02Server{t: t, routes: scenario.Routes, hits: make(map[string]int)}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	serverURL, err := url.Parse(fixture.server.URL)
	if err != nil {
		t.Fatalf("parse fixture server URL: %v", err)
	}
	logicalBaseURL := "http://fixture.example:" + serverURL.Port()
	resolver := staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}}
	dialer := redirectingUpstreamDialer{target: serverURL.Host}
	client, err := newNewAPIClient(NewAPIClientOptions{
		BaseURL:             logicalBaseURL,
		CredentialOrigin:    logicalBaseURL,
		AccessToken:         f02RootToken,
		RootUserID:          1,
		AllowedHostSuffixes: []string{"fixture.example"},
	}, newAPIClientDependencies{
		transport: upstreamTransportDependencies{resolver: resolver, dialer: dialer},
		now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("create F02 client: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return fixture, client
}

func (fixture *f02Server) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	route := fixture.matchRoute(request)
	if route == nil {
		fixture.recordError(fmt.Sprintf("unexpected route %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery))
		http.Error(writer, "unexpected fixture route", http.StatusNotFound)
		return
	}
	fixture.mu.Lock()
	fixture.hits[route.ID]++
	fixture.mu.Unlock()
	if request.Header.Get("X-Request-ID") == "" {
		fixture.recordError(route.ID + ": missing X-Request-ID")
	}
	if request.Header.Get("User-Agent") != NewAPIClientUserAgent {
		fixture.recordError(route.ID + ": unexpected User-Agent")
	}
	if (route.ID == "login" || route.ID == "token-disconnect") && request.Header.Get("Authorization") != "" {
		fixture.recordError(route.ID + ": temporary session sent Authorization")
	}
	if route.ID == "login" && request.Header.Get("New-Api-User") != "" {
		fixture.recordError(route.ID + ": login sent New-Api-User")
	}
	for name, expected := range route.ExpectedHeaders {
		if actual := request.Header.Get(name); actual != expected {
			fixture.recordError(fmt.Sprintf("%s: header %s=%q, want %q", route.ID, name, actual, expected))
		}
	}
	for _, name := range route.RequiredHeaders {
		if request.Header.Get(name) == "" {
			fixture.recordError(fmt.Sprintf("%s: required header %s is empty", route.ID, name))
		}
	}
	for _, name := range route.ForbiddenHeaders {
		if request.Header.Get(name) != "" {
			fixture.recordError(fmt.Sprintf("%s: forbidden header %s was sent", route.ID, name))
		}
	}
	if route.Disconnect {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			fixture.recordError(route.ID + ": response writer cannot disconnect")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			fixture.recordError(route.ID + ": hijack failed")
			return
		}
		_ = connection.Close()
		return
	}
	for name, value := range route.Headers {
		writer.Header().Set(name, value)
	}
	if route.ID == "login" {
		http.SetCookie(writer, &http.Cookie{Name: "session", Value: "fixture", Path: "/api"})
	}
	status := route.Status
	if status == 0 {
		status = http.StatusOK
	}
	writer.WriteHeader(status)
	if route.ResponseFile == "" {
		return
	}
	payload, err := os.ReadFile(filepath.Join("..", "testdata", "design", "f02-upstream", filepath.FromSlash(route.ResponseFile)))
	if err != nil {
		fixture.recordError(route.ID + ": response fixture cannot be read")
		return
	}
	_, _ = writer.Write(payload)
}

func (fixture *f02Server) matchRoute(request *http.Request) *f02Route {
	for index := range fixture.routes {
		route := &fixture.routes[index]
		if request.Method != route.Method || request.URL.Path != route.Path {
			continue
		}
		if route.Query != nil && len(request.URL.Query()) != len(route.Query) {
			continue
		}
		matched := true
		for name, expected := range route.Query {
			if request.URL.Query().Get(name) != expected {
				matched = false
				break
			}
		}
		if matched {
			return route
		}
	}
	return nil
}

func (fixture *f02Server) recordError(message string) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.errors = append(fixture.errors, message)
}

func (fixture *f02Server) assertAllRoutesHitOnce() {
	fixture.t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for _, route := range fixture.routes {
		if fixture.hits[route.ID] != 1 {
			fixture.t.Errorf("route %s hit %d times, want 1", route.ID, fixture.hits[route.ID])
		}
	}
	for _, message := range fixture.errors {
		fixture.t.Error(message)
	}
}

type staticUpstreamResolver struct {
	addresses []netip.Addr
	err       error
}

func (resolver staticUpstreamResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return append([]netip.Addr(nil), resolver.addresses...), resolver.err
}

type redirectingUpstreamDialer struct {
	target string
	mu     *sync.Mutex
	seen   *[]string
}

func (dialer redirectingUpstreamDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if dialer.mu != nil && dialer.seen != nil {
		dialer.mu.Lock()
		*dialer.seen = append(*dialer.seen, address)
		dialer.mu.Unlock()
	}
	var netDialer net.Dialer
	return netDialer.DialContext(ctx, network, dialer.target)
}

func logicalURLForServer(t *testing.T, server *httptest.Server, hostname string) string {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return "http://" + hostname + ":" + strconv.Itoa(mustPort(t, parsed.Port()))
}

func mustPort(t *testing.T, raw string) int {
	t.Helper()
	port, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return port
}
