package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

const (
	UpstreamMaxResponseBytes int64 = 64 << 20
	NewAPIClientUserAgent          = "new-api-pilot/0.1.0"
	maximumUpstreamRedirects       = 3
)

var ErrUpstreamAddressForbidden = errors.New("upstream address is forbidden")

type upstreamResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type upstreamContextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type upstreamTransportDependencies struct {
	resolver upstreamResolver
	dialer   upstreamContextDialer
}

type upstreamNetworkPolicy struct {
	hostSuffixes []string
	allowedCIDRs []netip.Prefix
	resolver     upstreamResolver
}

type safeUpstreamDialer struct {
	policy  *upstreamNetworkPolicy
	dialer  upstreamContextDialer
	timeout time.Duration
}

func NormalizeUpstreamBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("upstream base URL is invalid")
	}
	if parsed.Opaque != "" || parsed.Host == "" {
		return "", errors.New("upstream base URL must be absolute")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("upstream base URL must use http or https")
	}
	if parsed.User != nil {
		return "", errors.New("upstream base URL must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("upstream base URL must not contain a query")
	}
	if parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", errors.New("upstream base URL must not contain a fragment")
	}

	hostname, err := normalizeUpstreamHostname(parsed.Hostname())
	if err != nil {
		return "", err
	}
	port, err := normalizeUpstreamPort(parsed.Scheme, parsed.Port())
	if err != nil {
		return "", err
	}
	if address, parseErr := netip.ParseAddr(hostname); parseErr == nil && address.Is6() {
		parsed.Host = "[" + address.String() + "]"
	} else {
		parsed.Host = hostname
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(hostname, port)
	}

	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		decoded, decodeErr := url.PathUnescape(segment)
		if decodeErr != nil {
			return "", errors.New("upstream base URL path is invalid")
		}
		if decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\") {
			return "", errors.New("upstream base URL path must not contain dot segments")
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func normalizeUpstreamHostname(raw string) (string, error) {
	hostname := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", errors.New("upstream base URL has an invalid host")
	}
	if address, err := netip.ParseAddr(hostname); err == nil {
		if address.Is4In6() {
			return "", fmt.Errorf("%w: IPv4-mapped IPv6 addresses are not allowed", ErrUpstreamAddressForbidden)
		}
		return address.String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(hostname)
	if err != nil || ascii == "" || strings.ContainsAny(ascii, "/:@") {
		return "", errors.New("upstream base URL has an invalid host")
	}
	return strings.ToLower(ascii), nil
}

func normalizeUpstreamPort(scheme, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("upstream base URL has an invalid port")
	}
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		return "", nil
	}
	return strconv.Itoa(port), nil
}

func newUpstreamNetworkPolicy(hostSuffixes []string, allowedCIDRs []netip.Prefix, resolver upstreamResolver) (*upstreamNetworkPolicy, error) {
	normalizedSuffixes := make([]string, 0, len(hostSuffixes))
	seenSuffixes := make(map[string]struct{}, len(hostSuffixes))
	for _, raw := range hostSuffixes {
		suffix, err := normalizeUpstreamHostname(strings.TrimPrefix(strings.TrimSpace(raw), "*."))
		if err != nil {
			return nil, fmt.Errorf("normalize upstream host suffix: %w", err)
		}
		if _, err := netip.ParseAddr(suffix); err == nil {
			return nil, errors.New("upstream host suffix must be a DNS name")
		}
		if _, exists := seenSuffixes[suffix]; !exists {
			seenSuffixes[suffix] = struct{}{}
			normalizedSuffixes = append(normalizedSuffixes, suffix)
		}
	}
	normalizedCIDRs := make([]netip.Prefix, 0, len(allowedCIDRs))
	seenCIDRs := make(map[netip.Prefix]struct{}, len(allowedCIDRs))
	for _, prefix := range allowedCIDRs {
		if !prefix.IsValid() {
			return nil, errors.New("upstream allowlist contains an invalid CIDR")
		}
		prefix = prefix.Masked()
		if _, exists := seenCIDRs[prefix]; !exists {
			seenCIDRs[prefix] = struct{}{}
			normalizedCIDRs = append(normalizedCIDRs, prefix)
		}
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &upstreamNetworkPolicy{hostSuffixes: normalizedSuffixes, allowedCIDRs: normalizedCIDRs, resolver: resolver}, nil
}

func (policy *upstreamNetworkPolicy) validateHost(host string) error {
	host, err := normalizeUpstreamHostname(host)
	if err != nil {
		return err
	}
	if len(policy.hostSuffixes) == 0 {
		return nil
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil
	}
	for _, suffix := range policy.hostSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return nil
		}
	}
	return fmt.Errorf("%w: host does not match the configured suffixes", ErrUpstreamAddressForbidden)
}

func (policy *upstreamNetworkPolicy) resolveAndValidate(ctx context.Context, host string) ([]netip.Addr, error) {
	if err := policy.validateHost(host); err != nil {
		return nil, err
	}
	normalizedHost, err := normalizeUpstreamHostname(host)
	if err != nil {
		return nil, err
	}
	addresses := make([]netip.Addr, 0)
	literalHost := false
	if literal, parseErr := netip.ParseAddr(normalizedHost); parseErr == nil {
		literalHost = true
		addresses = append(addresses, literal)
	} else {
		resolved, resolveErr := policy.resolver.LookupNetIP(ctx, "ip", normalizedHost)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve upstream host: %w", resolveErr)
		}
		addresses = append(addresses, resolved...)
	}
	if len(addresses) == 0 {
		return nil, errors.New("upstream host resolved to no addresses")
	}
	result := make([]netip.Addr, 0, len(addresses))
	seen := make(map[netip.Addr]struct{}, len(addresses))
	for _, address := range addresses {
		if err := policy.validateResolvedAddress(address, literalHost); err != nil {
			return nil, err
		}
		if _, exists := seen[address]; !exists {
			seen[address] = struct{}{}
			result = append(result, address)
		}
	}
	return result, nil
}

func (policy *upstreamNetworkPolicy) validateAddress(address netip.Addr) error {
	return policy.validateResolvedAddress(address, len(policy.allowedCIDRs) > 0)
}

func (policy *upstreamNetworkPolicy) validateResolvedAddress(address netip.Addr, literalHost bool) error {
	if !address.IsValid() || address.Is4In6() {
		return fmt.Errorf("%w: invalid or IPv4-mapped address", ErrUpstreamAddressForbidden)
	}
	if address.IsUnspecified() || address.IsLoopback() || address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() || address.IsMulticast() || isReservedUpstreamAddress(address) ||
		isForbiddenTransitionAddress(address) {
		return fmt.Errorf("%w: special-use address", ErrUpstreamAddressForbidden)
	}
	allowed := false
	for _, prefix := range policy.allowedCIDRs {
		if prefix.Contains(address) {
			allowed = true
			break
		}
	}
	if literalHost && !allowed {
		return fmt.Errorf("%w: address does not match the configured CIDRs", ErrUpstreamAddressForbidden)
	}
	if address.IsPrivate() && !allowed {
		return fmt.Errorf("%w: private address requires an explicit CIDR", ErrUpstreamAddressForbidden)
	}
	return nil
}

func isForbiddenTransitionAddress(address netip.Addr) bool {
	for _, prefix := range forbiddenUpstreamTransitionPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	if address.Is6() {
		bytes := address.As16()
		if (bytes[8] == 0x00 || bytes[8] == 0x02) && bytes[9] == 0x00 && bytes[10] == 0x5e && bytes[11] == 0xfe {
			return true
		}
	}
	return false
}

func isReservedUpstreamAddress(address netip.Addr) bool {
	for _, prefix := range reservedUpstreamPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var reservedUpstreamPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.192/32"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
}

var forbiddenUpstreamTransitionPrefixes = []netip.Prefix{
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::ffff:0:0:0/96"),
}

func (dialer safeUpstreamDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if dialer.timeout <= 0 {
		return nil, errors.New("upstream connect timeout is invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, dialer.timeout)
	defer cancel()
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("%w: unsupported network", ErrUpstreamAddressForbidden)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dial address", ErrUpstreamAddressForbidden)
	}
	if _, err := normalizeUpstreamPort("tcp", port); err != nil {
		return nil, err
	}
	addresses, err := dialer.policy.resolveAndValidate(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastError error
	for _, resolved := range addresses {
		if network == "tcp4" && !resolved.Is4() {
			continue
		}
		if network == "tcp6" && !resolved.Is6() {
			continue
		}
		connection, dialErr := dialer.dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastError = dialErr
	}
	if lastError == nil {
		lastError = errors.New("upstream host has no address for the requested network")
	}
	return nil, lastError
}

func loadUpstreamRootCAs(path string) (*x509.CertPool, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read upstream CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(contents) {
		return nil, errors.New("upstream CA file contains no valid certificates")
	}
	return pool, nil
}

func newSafeUpstreamHTTPClient(
	baseURL string,
	hostSuffixes []string,
	allowedCIDRs []netip.Prefix,
	caFile string,
	connectTimeout time.Duration,
	headerTimeout time.Duration,
	maxIdleConns int,
	maxIdleConnsPerHost int,
	dependencies upstreamTransportDependencies,
) (*http.Client, *http.Transport, error) {
	policy, err := newUpstreamNetworkPolicy(hostSuffixes, allowedCIDRs, dependencies.resolver)
	if err != nil {
		return nil, nil, err
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse normalized upstream base URL: %w", err)
	}
	if err := policy.validateHost(parsedBase.Hostname()); err != nil {
		return nil, nil, err
	}
	if connectTimeout <= 0 || headerTimeout <= 0 {
		return nil, nil, errors.New("upstream connect and response header timeouts must be positive")
	}
	if maxIdleConns <= 0 || maxIdleConnsPerHost <= 0 || maxIdleConnsPerHost > maxIdleConns {
		return nil, nil, errors.New("upstream idle connection limits are invalid")
	}
	rootCAs, err := loadUpstreamRootCAs(caFile)
	if err != nil {
		return nil, nil, err
	}
	baseDialer := dependencies.dialer
	if baseDialer == nil {
		baseDialer = &net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}
	}
	pinnedDialer := safeUpstreamDialer{policy: policy, dialer: baseDialer, timeout: connectTimeout}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           pinnedDialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   connectTimeout,
		ResponseHeaderTimeout: headerTimeout,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: rootCAs},
	}
	baseOrigin := parsedBase.Scheme + "://" + parsedBase.Host
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maximumUpstreamRedirects {
				return fmt.Errorf("%w: redirect limit exceeded", ErrUpstreamAddressForbidden)
			}
			origin, err := normalizedRequestOrigin(request.URL)
			if err != nil {
				return err
			}
			if origin != baseOrigin {
				return fmt.Errorf("%w: redirect changed the upstream origin", ErrUpstreamAddressForbidden)
			}
			preventAutomaticRequestReplay(request)
			return policy.validateHost(request.URL.Hostname())
		},
	}
	return client, transport, nil
}

type nonReplayableEmptyBody struct{}

func (nonReplayableEmptyBody) Read([]byte) (int, error) { return 0, io.EOF }
func (nonReplayableEmptyBody) Close() error             { return nil }

func preventAutomaticRequestReplay(request *http.Request) {
	if request == nil || request.Method != http.MethodGet || (request.Body != nil && request.Body != http.NoBody) {
		return
	}
	request.Body = nonReplayableEmptyBody{}
	request.ContentLength = 0
	request.GetBody = nil
}

func normalizedRequestOrigin(value *url.URL) (string, error) {
	if value == nil || value.User != nil || value.Fragment != "" || value.RawFragment != "" {
		return "", fmt.Errorf("%w: redirect URL is invalid", ErrUpstreamAddressForbidden)
	}
	normalized, err := NormalizeUpstreamBaseURL(value.Scheme + "://" + value.Host)
	if err != nil {
		return "", fmt.Errorf("%w: redirect URL is invalid", ErrUpstreamAddressForbidden)
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("%w: redirect URL is invalid", ErrUpstreamAddressForbidden)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}
