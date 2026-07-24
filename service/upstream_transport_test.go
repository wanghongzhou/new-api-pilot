package service

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testStatusEnvelope = `{"success":true,"message":"","data":{"version":"test-version","system_name":"Test","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true}}`

func TestNormalizeUpstreamBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "canonical origin", raw: "  HTTPS://ExAmPle.COM:443/Api/  ", want: "https://example.com/Api"},
		{name: "default HTTP port", raw: "http://Example.com:080", want: "http://example.com"},
		{name: "nondefault port", raw: "https://Example.com:8443/base", want: "https://example.com:8443/base"},
		{name: "IDNA", raw: "https://\u4f8b\u5b50.\u6d4b\u8bd5/Api", want: "https://xn--fsqu00a.xn--0zwm56d/Api"},
		{name: "IPv6", raw: "https://[2001:db8::1]:443/", want: "https://[2001:db8::1]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NormalizeUpstreamBaseURL(test.raw)
			if err != nil || actual != test.want {
				t.Fatalf("NormalizeUpstreamBaseURL() = %q, %v; want %q", actual, err, test.want)
			}
		})
	}

	invalid := []string{
		"/relative",
		"ftp://example.com",
		"http://user:secret-password@example.com",
		"http://example.com/path?secret=value",
		"http://example.com/path#fragment",
		"http://example.com:0",
		"http://example.com:65536",
		"http://example.com/a/../b",
		"http://example.com/a/%2e%2e/b",
		"http://example.com/a%2fb",
		"http://[::ffff:127.0.0.1]",
		"http://user:secret-password@%zz",
	}
	for _, raw := range invalid {
		t.Run("invalid "+raw, func(t *testing.T) {
			_, err := NormalizeUpstreamBaseURL(raw)
			if err == nil {
				t.Fatalf("expected %q to be rejected", raw)
			}
			if strings.Contains(err.Error(), "secret-password") || strings.Contains(err.Error(), "secret=value") {
				t.Fatalf("normalization error leaked URL contents: %v", err)
			}
		})
	}
}

func TestUpstreamNetworkPolicy(t *testing.T) {
	policy, err := newUpstreamNetworkPolicy([]string{"example.com"}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"example.com", "api.example.com", "deep.api.example.com"} {
		if err := policy.validateHost(host); err != nil {
			t.Errorf("expected host %q to match: %v", host, err)
		}
	}
	for _, host := range []string{"evil-example.com", "example.com.evil", "badexample.com"} {
		if err := policy.validateHost(host); !errors.Is(err, ErrUpstreamAddressForbidden) {
			t.Errorf("expected host %q to fail on label boundary, got %v", host, err)
		}
	}

	private := netip.MustParseAddr("10.1.2.3")
	if err := policy.validateAddress(private); !errors.Is(err, ErrUpstreamAddressForbidden) {
		t.Fatalf("private address without CIDR was allowed: %v", err)
	}
	privatePolicy, err := newUpstreamNetworkPolicy(nil, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := privatePolicy.validateAddress(private); err != nil {
		t.Fatalf("explicit private CIDR was rejected: %v", err)
	}
	if err := privatePolicy.validateAddress(netip.MustParseAddr("192.0.2.10")); !errors.Is(err, ErrUpstreamAddressForbidden) {
		t.Fatalf("configured CIDR was not applied to a public address: %v", err)
	}

	alwaysForbidden := []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("169.254.169.254"),
		netip.MustParseAddr("224.0.0.1"),
		netip.MustParseAddr("0.0.0.0"),
		netip.MustParseAddr("100.100.100.200"),
		netip.MustParseAddr("240.0.0.1"),
		netip.MustParseAddr("::1"),
		netip.MustParseAddr("fe80::1"),
		netip.MustParseAddr("ff02::1"),
		netip.MustParseAddr("fd00:ec2::254"),
		netip.MustParseAddr("::ffff:127.0.0.1"),
		netip.MustParseAddr("64:ff9b::7f00:1"),
		netip.MustParseAddr("64:ff9b::a9fe:a9fe"),
		netip.MustParseAddr("64:ff9b::a00:1"),
		netip.MustParseAddr("64:ff9b:1::a00:1"),
		netip.MustParseAddr("2002:7f00:1::"),
		netip.MustParseAddr("2002:a9fe:a9fe::"),
		netip.MustParseAddr("2002:a00:1::"),
		netip.MustParseAddr("2001:0:4136:e378:8000:63bf:80ff:fffe"),
		netip.MustParseAddr("2001:0:4136:e378:8000:63bf:5601:5601"),
		netip.MustParseAddr("2001:0:4136:e378:8000:63bf:f5ff:fffe"),
		netip.MustParseAddr("::7f00:1"),
		netip.MustParseAddr("::a9fe:a9fe"),
		netip.MustParseAddr("::a00:1"),
		netip.MustParseAddr("::ffff:0:7f00:1"),
		netip.MustParseAddr("::ffff:0:a9fe:a9fe"),
		netip.MustParseAddr("::ffff:0:a00:1"),
		netip.MustParseAddr("2001:db8::5efe:7f00:1"),
		netip.MustParseAddr("2001:db8::5efe:a9fe:a9fe"),
		netip.MustParseAddr("2001:db8::200:5efe:a00:1"),
	}
	for _, address := range alwaysForbidden {
		prefix := netip.PrefixFrom(address.Unmap(), address.Unmap().BitLen())
		explicit, err := newUpstreamNetworkPolicy(nil, []netip.Prefix{prefix}, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := explicit.validateAddress(address); !errors.Is(err, ErrUpstreamAddressForbidden) {
			t.Errorf("special address %s was allowed by an explicit CIDR: %v", address, err)
		}
	}
}

func TestDevelopmentUpstreamNetworkPolicyAllowsPrivateAddresses(t *testing.T) {
	policy, err := newUpstreamNetworkPolicy(nil, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"10.1.2.3", "172.16.5.4", "192.168.8.20", "fd12:3456:789a::1"} {
		address := netip.MustParseAddr(raw)
		if err := policy.validateAddress(address); err != nil {
			t.Errorf("development private address %s was rejected: %v", address, err)
		}
	}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "224.0.0.1", "::1", "fe80::1"} {
		address := netip.MustParseAddr(raw)
		if err := policy.validateAddress(address); !errors.Is(err, ErrUpstreamAddressForbidden) {
			t.Errorf("development special-use address %s was allowed: %v", address, err)
		}
	}
}

func TestUpstreamDNSRebindingAndPinnedDial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Connection", "close")
		_, _ = writer.Write([]byte(testStatusEnvelope))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	resolver := &sequenceUpstreamResolver{responses: [][]netip.Addr{
		{netip.MustParseAddr("192.0.2.10")},
		{netip.MustParseAddr("127.0.0.1")},
	}}
	var mu sync.Mutex
	seen := make([]string, 0, 2)
	client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
		resolver, &redirectingUpstreamDialer{target: parsed.Host, mu: &mu, seen: &seen}, testClientSettings{})
	if _, err := client.Status(context.Background(), "dns-first"); err != nil {
		t.Fatalf("first status: %v", err)
	}
	if _, err := client.Status(context.Background(), "dns-second"); !errors.Is(err, ErrUpstreamAddressForbidden) {
		t.Fatalf("DNS rebinding was not rejected: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != net.JoinHostPort("192.0.2.10", parsed.Port()) {
		t.Fatalf("dialer did not receive only the validated pinned IP: %#v", seen)
	}
}

func TestNewAPIClientDoesNotReplayGET(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		switch requests.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(testStatusEnvelope))
		case 2:
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
		default:
			_, _ = writer.Write([]byte(testStatusEnvelope))
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
		staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
		redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{})
	if _, err := client.Status(context.Background(), "no-replay-first"); err != nil {
		t.Fatalf("prime connection: %v", err)
	}
	if _, err := client.Status(context.Background(), "no-replay-second"); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("expected one failed attempt, got %v", err)
	}
	if actual := requests.Load(); actual != 2 {
		t.Fatalf("logical GET was sent %d HTTP times, want 2 requests total", actual)
	}
}

func TestUpstreamRedirectPolicy(t *testing.T) {
	t.Run("same origin", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/api/status" {
				http.Redirect(writer, request, "/status-final", http.StatusFound)
				return
			}
			_, _ = writer.Write([]byte(testStatusEnvelope))
		}))
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
			staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
			redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{})
		if _, err := client.Status(context.Background(), "redirect-same"); err != nil {
			t.Fatalf("same-origin redirect failed: %v", err)
		}
	})

	for _, test := range []struct {
		name     string
		location func(port string) string
	}{
		{name: "cross host", location: func(port string) string { return "http://other.example:" + port + "/status-final" }},
		{name: "cross port", location: func(string) string { return "http://fixture.example:1/status-final" }},
		{name: "cross scheme", location: func(port string) string { return "https://fixture.example:" + port + "/status-final" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var redirected atomic.Int32
			var location string
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/status" {
					redirected.Add(1)
				}
				writer.Header().Set("Location", location)
				writer.WriteHeader(http.StatusFound)
			}))
			defer server.Close()
			parsed, _ := url.Parse(server.URL)
			location = test.location(parsed.Port())
			client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
				staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
				redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{})
			if _, err := client.Status(context.Background(), "redirect-blocked"); !errors.Is(err, ErrUpstreamAddressForbidden) {
				t.Fatalf("redirect was not blocked: %v", err)
			}
			if redirected.Load() != 0 {
				t.Fatal("blocked redirect reached its destination")
			}
		})
	}
}

func TestUpstreamTLSCustomCA(t *testing.T) {
	var downgradeLocation atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if location, ok := downgradeLocation.Load().(string); ok && location != "" {
			writer.Header().Set("Location", location)
			writer.WriteHeader(http.StatusFound)
			return
		}
		_, _ = writer.Write([]byte(testStatusEnvelope))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	if len(server.Certificate().DNSNames) == 0 {
		t.Fatal("httptest certificate has no DNS SAN")
	}
	hostname := server.Certificate().DNSNames[0]
	logicalURL := logicalTestBaseURL(t, server, hostname, "https")
	resolver := staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}}
	dialer := redirectingUpstreamDialer{target: parsed.Host}
	withoutCA := newTestUpstreamClient(t, logicalURL, parsed.Host, resolver, dialer, testClientSettings{})
	if _, err := withoutCA.Status(context.Background(), "tls-untrusted"); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("untrusted TLS certificate was accepted: %v", err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	caPath := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(caPath, certificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	withCA := newTestUpstreamClient(t, logicalURL, parsed.Host, resolver, dialer, testClientSettings{caFile: caPath})
	if _, err := withCA.Status(context.Background(), "tls-trusted"); err != nil {
		t.Fatalf("custom CA was not trusted: %v", err)
	}
	downgradeLocation.Store("http://" + hostname + ":" + parsed.Port() + "/status-final")
	if _, err := withCA.Status(context.Background(), "tls-downgrade"); !errors.Is(err, ErrUpstreamAddressForbidden) {
		t.Fatalf("HTTPS downgrade redirect was not blocked: %v", err)
	}
}

func TestUpstreamTransportIgnoresEnvironmentProxy(t *testing.T) {
	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		proxyHits.Add(1)
		http.Error(writer, "proxy must not be used", http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(testStatusEnvelope))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
		staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
		redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{})
	if _, err := client.Status(context.Background(), "proxy-disabled"); err != nil {
		t.Fatalf("direct request failed: %v", err)
	}
	if proxyHits.Load() != 0 {
		t.Fatal("upstream request used the environment proxy")
	}
}

func TestUpstreamTimeouts(t *testing.T) {
	t.Run("connect includes DNS", func(t *testing.T) {
		client := newTestUpstreamClient(t, "http://fixture.example", "", blockingUpstreamResolver{},
			redirectingUpstreamDialer{}, testClientSettings{
				connectTimeout: 20 * time.Millisecond,
				headerTimeout:  200 * time.Millisecond,
				requestTimeout: 500 * time.Millisecond,
				exportTimeout:  500 * time.Millisecond,
			})
		started := time.Now()
		if _, err := client.Status(context.Background(), "timeout-connect"); !errors.Is(err, ErrUpstreamUnavailable) {
			t.Fatalf("expected connect timeout, got %v", err)
		}
		if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
			t.Fatalf("connect timeout did not bound DNS: %v", elapsed)
		}
	})

	t.Run("response header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = writer.Write([]byte(testStatusEnvelope))
		}))
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
			staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
			redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{
				connectTimeout: 50 * time.Millisecond, headerTimeout: 20 * time.Millisecond,
				requestTimeout: 200 * time.Millisecond, exportTimeout: 200 * time.Millisecond,
			})
		if _, err := client.Status(context.Background(), "timeout-header"); !errors.Is(err, ErrUpstreamUnavailable) {
			t.Fatalf("expected response header timeout, got %v", err)
		}
	})

	t.Run("total response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"success":`))
			writer.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
			_, _ = writer.Write([]byte(`true}`))
		}))
		defer server.Close()
		parsed, _ := url.Parse(server.URL)
		client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
			staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
			redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{
				connectTimeout: 50 * time.Millisecond, headerTimeout: 50 * time.Millisecond,
				requestTimeout: 25 * time.Millisecond, exportTimeout: 200 * time.Millisecond,
			})
		if _, err := client.Status(context.Background(), "timeout-total"); !errors.Is(err, ErrUpstreamUnavailable) {
			t.Fatalf("expected total timeout, got %v", err)
		}
	})
}

func TestUpstreamResponseLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		chunked bool
	}{
		{name: "declared length"},
		{name: "chunked", chunked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if test.chunked {
					writer.(http.Flusher).Flush()
				} else {
					writer.Header().Set("Content-Length", "129")
				}
				_, _ = writer.Write([]byte(strings.Repeat("x", 129)))
			}))
			defer server.Close()
			parsed, _ := url.Parse(server.URL)
			client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
				staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
				redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{maxResponseBytes: 128})
			_, err := client.Status(context.Background(), "response-limit")
			if !errors.Is(err, ErrUpstreamResponseTooLarge) {
				t.Fatalf("oversized response was not rejected: %v", err)
			}
			var requestError *UpstreamRequestError
			if !errors.As(err, &requestError) || requestError.ResponseBytes != 129 || requestError.LimitBytes != 128 {
				t.Fatalf("oversized response metadata = %#v", requestError)
			}
		})
	}
}

func TestUpstreamResponseBodiesCloseAndConnectionsReuse(t *testing.T) {
	var newConnections atomic.Int32
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"success":false,"message":"ignored"}`))
			return
		}
		_, _ = writer.Write([]byte(testStatusEnvelope))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
		staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
		redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{})
	if _, err := client.Status(context.Background(), "reuse-error"); !errors.Is(err, ErrUpstreamRemote) {
		t.Fatalf("expected first server error, got %v", err)
	}
	if _, err := client.Status(context.Background(), "reuse-success"); err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if actual := newConnections.Load(); actual != 1 {
		t.Fatalf("response body was not drained/closed for reuse; connections=%d", actual)
	}
}

type testClientSettings struct {
	caFile           string
	connectTimeout   time.Duration
	headerTimeout    time.Duration
	requestTimeout   time.Duration
	exportTimeout    time.Duration
	maxResponseBytes int64
	withCredentials  bool
	credentialOrigin string
}

func newTestUpstreamClient(
	t *testing.T,
	baseURL string,
	_ string,
	resolver upstreamResolver,
	dialer upstreamContextDialer,
	settings testClientSettings,
) *NewAPIClient {
	t.Helper()
	options := NewAPIClientOptions{
		BaseURL:             baseURL,
		AllowedHostSuffixes: []string{"fixture.example", "example.com"},
		CAFile:              settings.caFile,
		ConnectTimeout:      settings.connectTimeout,
		HeaderTimeout:       settings.headerTimeout,
		RequestTimeout:      settings.requestTimeout,
		ExportTimeout:       settings.exportTimeout,
	}
	if settings.withCredentials {
		options.AccessToken = "test-root-token"
		options.RootUserID = 1
		options.CredentialOrigin = settings.credentialOrigin
		if options.CredentialOrigin == "" {
			options.CredentialOrigin = baseURL
		}
	}
	client, err := newNewAPIClient(options, newAPIClientDependencies{
		transport:              upstreamTransportDependencies{resolver: resolver, dialer: dialer},
		allowNonDesignTimeouts: true,
		maxResponseBytes:       settings.maxResponseBytes,
	})
	if err != nil {
		t.Fatalf("create test upstream client: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func logicalTestBaseURL(t *testing.T, server *httptest.Server, hostname, scheme string) string {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s://%s:%s", scheme, hostname, parsed.Port())
}

type sequenceUpstreamResolver struct {
	mu        sync.Mutex
	responses [][]netip.Addr
	calls     int
}

func (resolver *sequenceUpstreamResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	index := resolver.calls
	resolver.calls++
	if index >= len(resolver.responses) {
		index = len(resolver.responses) - 1
	}
	return append([]netip.Addr(nil), resolver.responses[index]...), nil
}

type blockingUpstreamResolver struct{}

func (blockingUpstreamResolver) LookupNetIP(ctx context.Context, _, _ string) ([]netip.Addr, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
