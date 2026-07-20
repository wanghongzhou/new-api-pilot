package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/dns/dnsmessage"

	"new-api-pilot/common"
	"new-api-pilot/config"
	"new-api-pilot/constant"
	"new-api-pilot/middleware"
	"new-api-pilot/service"
)

const (
	a45AcceptanceID = "A45"
	a45IsolatedEnv  = "A45_ISOLATED_NETWORK"
)

func TestA45SecurityBoundaryAcceptance(t *testing.T) {
	if os.Getenv(a45IsolatedEnv) != "true" {
		if os.Getenv("ACCEPTANCE_ID") == a45AcceptanceID {
			t.Fatalf("%s=true is required for A45 acceptance", a45IsolatedEnv)
		}
		t.Skip("A45 isolated local network is not enabled")
	}
	gin.SetMode(gin.TestMode)
	t.Run("origin fail closed", testA45OriginFailClosed)
	t.Run("trusted proxy boundary", testA45TrustedProxyBoundary)
	t.Run("upstream DNS TLS and credential boundary", testA45UpstreamBoundary)
	t.Run("sensitive response and logs", testA45SensitiveResponseAndLogs)
}

func testA45OriginFailClosed(t *testing.T) {
	engine := gin.New()
	engine.Use(
		middleware.RequestID(),
		middleware.SecurityHeaders(),
		middleware.OriginGuard(config.EnvironmentProduction, "https://pilot.example.com"),
	)
	engine.Any("/api/security", func(c *gin.Context) { common.WriteSuccess(c, http.StatusOK, nil) })
	engine.POST("/healthz", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		denied := httptest.NewRequest(method, "/api/security", nil)
		deniedResponse := httptest.NewRecorder()
		engine.ServeHTTP(deniedResponse, denied)
		assertA45OriginResponse(t, deniedResponse, http.StatusForbidden)

		allowed := httptest.NewRequest(method, "/api/security", nil)
		allowed.Header.Set("Origin", "https://pilot.example.com")
		allowedResponse := httptest.NewRecorder()
		engine.ServeHTTP(allowedResponse, allowed)
		assertA45OriginResponse(t, allowedResponse, http.StatusOK)
	}

	invalidOrigins := [][]string{
		{""},
		{"null"},
		{"https://PILOT.example.com"},
		{"https://pilot.example.com:443"},
		{"https://pilot.example.com, https://other.example.com"},
		{"https://pilot.example.com", "https://pilot.example.com"},
		{"https://pilot.example.com", "https://other.example.com"},
	}
	for _, origins := range invalidOrigins {
		request := httptest.NewRequest(http.MethodPost, "/api/security", nil)
		for _, origin := range origins {
			request.Header.Add("Origin", origin)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		assertA45OriginResponse(t, response, http.StatusForbidden)
	}

	read := httptest.NewRequest(http.MethodGet, "/api/security", nil)
	readResponse := httptest.NewRecorder()
	engine.ServeHTTP(readResponse, read)
	assertA45OriginResponse(t, readResponse, http.StatusOK)
	health := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	engine.ServeHTTP(healthResponse, health)
	if healthResponse.Code != http.StatusNoContent {
		t.Fatalf("non-browser health write status=%d", healthResponse.Code)
	}
}

func assertA45OriginResponse(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("origin status=%d want=%d body=%s", response.Code, want, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" ||
		strings.Contains(response.Header().Get("Content-Security-Policy"), "unsafe-eval") {
		t.Fatalf("origin response security headers=%v", response.Header())
	}
	if want == http.StatusForbidden {
		var envelope common.APIResponse
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode origin response: %v", err)
		}
		if envelope.Code != constant.CodeOriginForbidden || envelope.RequestID == "" {
			t.Fatalf("origin envelope=%#v", envelope)
		}
	}
}

func testA45TrustedProxyBoundary(t *testing.T) {
	allowed := []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}
	newEngine := func(trusted []string) *gin.Engine {
		engine := gin.New()
		if err := engine.SetTrustedProxies(trusted); err != nil {
			t.Fatalf("set trusted proxies: %v", err)
		}
		engine.Use(middleware.RequestID(), middleware.AllowCIDRs(allowed))
		engine.GET("/guarded", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		return engine
	}

	trustedEngine := newEngine([]string{"10.0.0.0/8"})
	untrusted := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	untrusted.RemoteAddr = "198.51.100.77:3456"
	untrusted.Header.Set("X-Forwarded-For", "203.0.113.9")
	untrusted.Header.Set("X-Real-IP", "203.0.113.10")
	untrustedResponse := httptest.NewRecorder()
	trustedEngine.ServeHTTP(untrustedResponse, untrusted)
	if untrustedResponse.Code != http.StatusForbidden {
		t.Fatalf("untrusted peer forged forwarded IP status=%d", untrustedResponse.Code)
	}

	trusted := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	trusted.RemoteAddr = "10.9.8.7:3456"
	trusted.Header.Set("X-Forwarded-For", "203.0.113.9, 10.1.2.3")
	trustedResponse := httptest.NewRecorder()
	trustedEngine.ServeHTTP(trustedResponse, trusted)
	if trustedResponse.Code != http.StatusNoContent {
		t.Fatalf("trusted proxy chain status=%d body=%s", trustedResponse.Code, trustedResponse.Body.String())
	}

	noProxyEngine := newEngine(nil)
	noProxy := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	noProxy.RemoteAddr = "10.9.8.7:3456"
	noProxy.Header.Set("X-Forwarded-For", "203.0.113.9")
	noProxyResponse := httptest.NewRecorder()
	noProxyEngine.ServeHTTP(noProxyResponse, noProxy)
	if noProxyResponse.Code != http.StatusForbidden {
		t.Fatalf("forwarded IP was trusted with an empty proxy list: %d", noProxyResponse.Code)
	}
}

func testA45UpstreamBoundary(t *testing.T) {
	privateAddress := a45PrivateIPv4(t)
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen A45 TLS upstream: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	var upstreamHits atomic.Int32
	var redirectTargetHits atomic.Int32
	var leakedAuthorization atomic.Bool
	var redirectLocation atomic.Value
	redirectLocation.Store("")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamHits.Add(1)
		writer.Header().Set("Connection", "close")
		if request.Header.Get("Authorization") != "" {
			leakedAuthorization.Store(true)
		}
		if request.URL.Path == "/redirect-target" {
			redirectTargetHits.Add(1)
		}
		if location := redirectLocation.Load().(string); location != "" && request.URL.Path == "/api/status" {
			writer.Header().Set("Location", location)
			writer.WriteHeader(http.StatusFound)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"message":"","data":{"version":"test-version","system_name":"A45","quota_per_unit":1,"usd_exchange_rate":1,"enable_data_export":true}}`))
	}))
	if err := server.Listener.Close(); err != nil {
		_ = listener.Close()
		t.Fatalf("close default A45 listener: %v", err)
	}
	server.Listener = listener
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	certificate := server.Certificate()
	if certificate == nil || len(certificate.DNSNames) == 0 {
		t.Fatal("A45 TLS fixture has no DNS SAN")
	}
	hostname := certificate.DNSNames[0]
	baseURL := fmt.Sprintf("https://%s:%d", hostname, port)
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	caPath := filepath.Join(t.TempDir(), "a45-ca.pem")
	if err := os.WriteFile(caPath, certificatePEM, 0o600); err != nil {
		t.Fatalf("write A45 CA: %v", err)
	}

	dns := newA45DNSFixture(t, hostname, privateAddress)
	previousResolver := net.DefaultResolver
	net.DefaultResolver = dns.resolver()
	t.Cleanup(func() { net.DefaultResolver = previousResolver })

	var proxyHits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		proxyHits.Add(1)
		http.Error(writer, "proxy must not be used", http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("ALL_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	exactPrivate := netip.PrefixFrom(privateAddress, privateAddress.BitLen())
	newClient := func(options service.NewAPIClientOptions) *service.NewAPIClient {
		client, createErr := service.NewNewAPIClient(options)
		if createErr != nil {
			t.Fatalf("create A45 upstream client: %v", createErr)
		}
		t.Cleanup(client.CloseIdleConnections)
		return client
	}
	callStatus := func(client *service.NewAPIClient, requestID string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, callErr := client.Status(ctx, requestID)
		return callErr
	}
	baseOptions := service.NewAPIClientOptions{
		BaseURL: baseURL, AllowedHostSuffixes: []string{hostname},
		AllowedCIDRs: []netip.Prefix{exactPrivate}, CAFile: caPath,
	}

	dns.phase.Store(a45DNSAllowed)
	untrustedTLS := baseOptions
	untrustedTLS.CAFile = ""
	if err := callStatus(newClient(untrustedTLS), "a45_tls_untrusted"); !errors.Is(err, service.ErrUpstreamUnavailable) {
		t.Fatalf("untrusted A45 TLS certificate error=%v", err)
	}
	if upstreamHits.Load() != 0 {
		t.Fatal("untrusted TLS request reached the HTTP handler")
	}

	privateDenied := baseOptions
	privateDenied.AllowedCIDRs = nil
	if err := callStatus(newClient(privateDenied), "a45_private_denied"); !errors.Is(err, service.ErrUpstreamAddressForbidden) {
		t.Fatalf("private address without explicit CIDR error=%v", err)
	}
	if upstreamHits.Load() != 0 {
		t.Fatal("private address without CIDR reached the upstream")
	}

	client := newClient(baseOptions)
	dns.phase.Store(a45DNSMixed)
	if err := callStatus(client, "a45_dns_mixed"); !errors.Is(err, service.ErrUpstreamAddressForbidden) {
		t.Fatalf("mixed safe/loopback DNS answer error=%v", err)
	}
	if upstreamHits.Load() != 0 {
		t.Fatal("client dialed before validating every DNS answer")
	}

	dns.phase.Store(a45DNSAllowed)
	if err := callStatus(client, "a45_dns_allowed"); err != nil {
		t.Fatalf("allowed pinned TLS upstream: %v", err)
	}
	if upstreamHits.Load() != 1 || leakedAuthorization.Load() || proxyHits.Load() != 0 {
		t.Fatalf("allowed upstream hits=%d auth_leak=%t proxy_hits=%d", upstreamHits.Load(), leakedAuthorization.Load(), proxyHits.Load())
	}

	redirectLocation.Store(fmt.Sprintf("http://%s:%d/redirect-target", hostname, port))
	if err := callStatus(client, "a45_redirect_downgrade"); !errors.Is(err, service.ErrUpstreamAddressForbidden) {
		t.Fatalf("TLS downgrade redirect error=%v", err)
	}
	if redirectTargetHits.Load() != 0 {
		t.Fatal("blocked downgrade redirect reached its target")
	}
	redirectLocation.Store("")

	queriesBeforeRebind := dns.aQueries.Load()
	dns.phase.Store(a45DNSLoopback)
	if err := callStatus(client, "a45_dns_rebound"); !errors.Is(err, service.ErrUpstreamAddressForbidden) {
		t.Fatalf("DNS rebinding error=%v", err)
	}
	if dns.aQueries.Load() <= queriesBeforeRebind || upstreamHits.Load() != 2 {
		t.Fatalf("DNS was not re-resolved or rebound request connected: queries=%d/%d hits=%d",
			queriesBeforeRebind, dns.aQueries.Load(), upstreamHits.Load())
	}

	loopbackClient := newClient(service.NewAPIClientOptions{
		BaseURL:      fmt.Sprintf("https://127.0.0.1:%d", port),
		AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, CAFile: caPath,
	})
	if err := callStatus(loopbackClient, "a45_loopback_explicit"); !errors.Is(err, service.ErrUpstreamAddressForbidden) {
		t.Fatalf("explicit loopback CIDR error=%v", err)
	}

	dns.phase.Store(a45DNSAllowed)
	hitsBeforeCredential := upstreamHits.Load()
	queriesBeforeCredential := dns.aQueries.Load()
	const oldToken = "a45-old-token-never-send"
	credentialOptions := baseOptions
	credentialOptions.AccessToken = oldToken
	credentialOptions.RootUserID = 1
	credentialOptions.CredentialOrigin = fmt.Sprintf("https://old.%s:%d", hostname, port)
	credentialClient := newClient(credentialOptions)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, credentialErr := credentialClient.Self(ctx, "a45_credential_origin")
	cancel()
	if !errors.Is(credentialErr, service.ErrUpstreamCredentialOriginMismatch) {
		t.Fatalf("credential origin mismatch error=%v", credentialErr)
	}
	if upstreamHits.Load() != hitsBeforeCredential || dns.aQueries.Load() != queriesBeforeCredential ||
		strings.Contains(credentialErr.Error(), oldToken) || strings.Contains(credentialErr.Error(), "old.") {
		t.Fatalf("credential guard sent/resolved/leaked: hits=%d/%d queries=%d/%d error=%v",
			hitsBeforeCredential, upstreamHits.Load(), queriesBeforeCredential, dns.aQueries.Load(), credentialErr)
	}
	if leakedAuthorization.Load() || proxyHits.Load() != 0 {
		t.Fatal("A45 upstream boundary leaked credentials or used an environment proxy")
	}
	dns.assertHealthy(t)
}

func testA45SensitiveResponseAndLogs(t *testing.T) {
	const secret = "a45-sensitive-value-never-log"
	var applicationLog bytes.Buffer
	var ginLog bytes.Buffer
	previousLogWriter := log.Writer()
	previousGinWriter := gin.DefaultErrorWriter
	log.SetOutput(&applicationLog)
	gin.DefaultErrorWriter = &ginLog
	t.Cleanup(func() {
		log.SetOutput(previousLogWriter)
		gin.DefaultErrorWriter = previousGinWriter
	})

	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.AccessLog(), middleware.Recovery(), middleware.SecurityHeaders())
	engine.GET("/panic", func(c *gin.Context) { panic(secret) })
	request := httptest.NewRequest(http.MethodGet, "/panic?access_token="+secret, nil)
	request.Header.Set("Authorization", secret)
	request.Header.Set(middleware.RequestIDHeader, "a45_log_redaction")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("A45 panic response status=%d", response.Code)
	}
	var envelope common.APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode A45 panic envelope: %v", err)
	}
	if envelope.Code != constant.CodeInternalError || envelope.RequestID != "a45_log_redaction" {
		t.Fatalf("A45 panic envelope=%#v", envelope)
	}
	combined := applicationLog.String() + ginLog.String() + response.Body.String()
	for _, forbidden := range []string{secret, "access_token", "Authorization"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("A45 response/log leaked %q: %s", forbidden, combined)
		}
	}
	if !strings.Contains(applicationLog.String(), "panic recovered request_id=a45_log_redaction method=GET route=/panic") ||
		!strings.Contains(applicationLog.String(), "http request request_id=a45_log_redaction method=GET route=/panic status=500") ||
		ginLog.Len() != 0 {
		t.Fatalf("A45 safe logging contract failed: app=%q gin=%q", applicationLog.String(), ginLog.String())
	}
}

func a45PrivateIPv4(t *testing.T) netip.Addr {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("list A45 interfaces: %v", err)
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, raw := range addresses {
			prefix, parseErr := netip.ParsePrefix(raw.String())
			if parseErr != nil {
				continue
			}
			address := prefix.Addr().Unmap()
			if address.Is4() && address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() {
				return address
			}
		}
	}
	t.Fatal("A45 requires an isolated network with a private non-loopback IPv4 address")
	return netip.Addr{}
}

const (
	a45DNSMixed int32 = iota
	a45DNSAllowed
	a45DNSLoopback
)

type a45DNSFixture struct {
	connection net.PacketConn
	expected   string
	allowed    netip.Addr
	phase      atomic.Int32
	aQueries   atomic.Int32
	errors     chan error
	done       chan struct{}
}

func newA45DNSFixture(t *testing.T, expected string, allowed netip.Addr) *a45DNSFixture {
	t.Helper()
	connection, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen A45 DNS: %v", err)
	}
	fixture := &a45DNSFixture{
		connection: connection, expected: strings.TrimSuffix(strings.ToLower(expected), "."), allowed: allowed,
		errors: make(chan error, 1), done: make(chan struct{}),
	}
	go fixture.serve()
	t.Cleanup(func() {
		_ = fixture.connection.Close()
		<-fixture.done
	})
	return fixture
}

func (fixture *a45DNSFixture) resolver() *net.Resolver {
	return &net.Resolver{
		PreferGo:     true,
		StrictErrors: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "udp4", fixture.connection.LocalAddr().String())
		},
	}
}

func (fixture *a45DNSFixture) serve() {
	defer close(fixture.done)
	buffer := make([]byte, 1232)
	for {
		length, peer, err := fixture.connection.ReadFrom(buffer)
		if err != nil {
			return
		}
		var parser dnsmessage.Parser
		header, err := parser.Start(buffer[:length])
		if err != nil {
			fixture.report(err)
			continue
		}
		question, err := parser.Question()
		if err != nil {
			fixture.report(err)
			continue
		}
		builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
			ID: header.ID, Response: true, Authoritative: true,
			RecursionDesired: header.RecursionDesired, RecursionAvailable: true,
		})
		builder.EnableCompression()
		if err := builder.StartQuestions(); err != nil {
			fixture.report(err)
			continue
		}
		if err := builder.Question(question); err != nil {
			fixture.report(err)
			continue
		}
		if err := builder.StartAnswers(); err != nil {
			fixture.report(err)
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(question.Name.String()), ".")
		if name == fixture.expected && question.Class == dnsmessage.ClassINET && question.Type == dnsmessage.TypeA {
			fixture.aQueries.Add(1)
			for _, address := range fixture.currentAddresses() {
				resource := dnsmessage.ResourceHeader{
					Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 0,
				}
				if err := builder.AResource(resource, dnsmessage.AResource{A: address.As4()}); err != nil {
					fixture.report(err)
					continue
				}
			}
		}
		message, err := builder.Finish()
		if err != nil {
			fixture.report(err)
			continue
		}
		if _, err := fixture.connection.WriteTo(message, peer); err != nil {
			fixture.report(err)
		}
	}
}

func (fixture *a45DNSFixture) currentAddresses() []netip.Addr {
	switch fixture.phase.Load() {
	case a45DNSAllowed:
		return []netip.Addr{fixture.allowed}
	case a45DNSLoopback:
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	default:
		return []netip.Addr{fixture.allowed, netip.MustParseAddr("127.0.0.1")}
	}
}

func (fixture *a45DNSFixture) report(err error) {
	select {
	case fixture.errors <- err:
	default:
	}
}

func (fixture *a45DNSFixture) assertHealthy(t *testing.T) {
	t.Helper()
	select {
	case err := <-fixture.errors:
		t.Fatalf("A45 DNS fixture error: %v", err)
	default:
	}
}
