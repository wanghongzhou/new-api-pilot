package service

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoginMutationNeverFollowsRedirect(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var loginHits atomic.Int32
			var targetHits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/user/login":
					loginHits.Add(1)
					writer.Header().Set("Location", "/password-target")
					writer.WriteHeader(status)
				case "/password-target":
					targetHits.Add(1)
					_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			client := testClientForServer(t, server, false, testClientSettings{})
			_, _, err := client.LoginAndGenerateAccessToken(context.Background(), "login-redirect", "root", "secret-password")
			if !errors.Is(err, ErrUpstreamResponseInvalid) {
				t.Fatalf("login redirect classified as %v", err)
			}
			if loginHits.Load() != 1 || targetHits.Load() != 0 {
				t.Fatalf("login=%d target=%d; password request was replayed", loginHits.Load(), targetHits.Load())
			}
		})
	}
}

func TestTokenMutationRedirectIsUnknownAndNeverFollowed(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var loginHits atomic.Int32
			var tokenHits atomic.Int32
			var targetHits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/user/login":
					loginHits.Add(1)
					http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
					_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
				case "/api/user/token":
					tokenHits.Add(1)
					writer.Header().Set("Location", "/token-target")
					writer.WriteHeader(status)
				case "/token-target":
					targetHits.Add(1)
					_, _ = writer.Write([]byte(`{"success":true,"message":"","data":"unexpected-token"}`))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			client := testClientForServer(t, server, false, testClientSettings{})
			_, token, err := client.LoginAndGenerateAccessToken(context.Background(), "token-redirect", "root", "secret-password")
			if !errors.Is(err, ErrUpstreamTokenRotationResultUnknown) || token != "" {
				t.Fatalf("token redirect classified as token=%q err=%v", token, err)
			}
			if loginHits.Load() != 1 || tokenHits.Load() != 1 || targetHits.Load() != 0 {
				t.Fatalf("login=%d token=%d target=%d", loginHits.Load(), tokenHits.Load(), targetHits.Load())
			}
		})
	}
}

func TestTokenMutationUncertainSuccessfulResponse(t *testing.T) {
	var tokenHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		case "/api/user/token":
			tokenHits.Add(1)
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := testClientForServer(t, server, false, testClientSettings{})
	_, _, err := client.LoginAndGenerateAccessToken(context.Background(), "token-invalid-response", "root", "secret-password")
	if !errors.Is(err, ErrUpstreamTokenRotationResultUnknown) || tokenHits.Load() != 1 {
		t.Fatalf("invalid successful response classified as %v with %d token hits", err, tokenHits.Load())
	}
}

func TestTokenMutationInvalidTokenValueIsUnknown(t *testing.T) {
	var tokenHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		case "/api/user/token":
			tokenHits.Add(1)
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":""}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := testClientForServer(t, server, false, testClientSettings{})
	_, _, err := client.LoginAndGenerateAccessToken(context.Background(), "token-invalid-value", "root", "secret-password")
	if !errors.Is(err, ErrUpstreamTokenRotationResultUnknown) || tokenHits.Load() != 1 {
		t.Fatalf("invalid token value classified as %v with %d token hits", err, tokenHits.Load())
	}
}

func TestMutationTransportUsesFreshHTTP1ConnectionsAndSingleAttempt(t *testing.T) {
	var tokenHits atomic.Int32
	var mu sync.Mutex
	remoteAddresses := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		remoteAddresses[request.URL.Path] = request.RemoteAddr
		mu.Unlock()
		switch request.URL.Path {
		case "/api/user/login":
			if request.ProtoMajor != 1 {
				t.Errorf("login used HTTP/%d", request.ProtoMajor)
			}
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		case "/api/user/token":
			tokenHits.Add(1)
			if request.ProtoMajor != 1 {
				t.Errorf("token used HTTP/%d", request.ProtoMajor)
			}
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := testClientForServer(t, server, false, testClientSettings{})
	_, _, err := client.LoginAndGenerateAccessToken(context.Background(), "mutation-fresh", "root", "secret-password")
	if !errors.Is(err, ErrUpstreamTokenRotationResultUnknown) || tokenHits.Load() != 1 {
		t.Fatalf("disconnect classified as %v with %d token hits", err, tokenHits.Load())
	}
	mu.Lock()
	loginRemote := remoteAddresses["/api/user/login"]
	tokenRemote := remoteAddresses["/api/user/token"]
	mu.Unlock()
	if loginRemote == "" || tokenRemote == "" || loginRemote == tokenRemote {
		t.Fatalf("mutation requests reused a connection: login=%q token=%q", loginRemote, tokenRemote)
	}
}

func TestTokenMutationPreSendSSRFRejectionIsNotUnknown(t *testing.T) {
	var tokenHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		case "/api/user/token":
			tokenHits.Add(1)
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":"unexpected"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &sequenceUpstreamResolver{responses: [][]netip.Addr{
		{netip.MustParseAddr("192.0.2.10")},
		{netip.MustParseAddr("64:ff9b::7f00:1")},
	}}
	client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, "fixture.example", "http"), parsed.Host,
		resolver, redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{})
	_, _, err = client.LoginAndGenerateAccessToken(context.Background(), "mutation-presend", "root", "secret-password")
	if !errors.Is(err, ErrUpstreamAddressForbidden) || errors.Is(err, ErrUpstreamTokenRotationResultUnknown) {
		t.Fatalf("pre-send rejection classified as %v", err)
	}
	if tokenHits.Load() != 0 {
		t.Fatalf("SSRF-rejected token endpoint was hit %d times", tokenHits.Load())
	}
}

func TestMutationTransportDisablesHTTP2ALPN(t *testing.T) {
	var regularProtocol atomic.Int32
	var loginProtocol atomic.Int32
	var tokenProtocol atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/status":
			regularProtocol.Store(int32(request.ProtoMajor))
			_, _ = writer.Write([]byte(testStatusEnvelope))
		case "/api/user/login":
			loginProtocol.Store(int32(request.ProtoMajor))
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "temporary", Path: "/api"})
			_, _ = writer.Write([]byte(upstreamSelfEnvelope()))
		case "/api/user/token":
			tokenProtocol.Store(int32(request.ProtoMajor))
			_, _ = writer.Write([]byte(`{"success":true,"message":"","data":"rotated-token"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	certificate := server.Certificate()
	if len(certificate.DNSNames) == 0 {
		t.Fatal("httptest certificate has no DNS SAN")
	}
	caPath := filepath.Join(t.TempDir(), "mutation-ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	hostname := certificate.DNSNames[0]
	client := newTestUpstreamClient(t, logicalTestBaseURL(t, server, hostname, "https"), parsed.Host,
		staticUpstreamResolver{addresses: []netip.Addr{netip.MustParseAddr("192.0.2.10")}},
		redirectingUpstreamDialer{target: parsed.Host}, testClientSettings{caFile: caPath})
	if _, err := client.Status(context.Background(), "regular-http2"); err != nil {
		t.Fatalf("regular HTTP/2 request failed: %v", err)
	}
	_, token, err := client.LoginAndGenerateAccessToken(context.Background(), "mutation-http1", "root", "secret-password")
	if err != nil || token != "rotated-token" {
		t.Fatalf("mutation request failed: token=%q err=%v", token, err)
	}
	if regularProtocol.Load() != 2 || loginProtocol.Load() != 1 || tokenProtocol.Load() != 1 {
		t.Fatalf("regular HTTP/%d, mutation login HTTP/%d token HTTP/%d", regularProtocol.Load(), loginProtocol.Load(), tokenProtocol.Load())
	}
}
