package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"new-api-pilot/dto"
)

const (
	UpstreamConnectTimeout        = 5 * time.Second
	UpstreamResponseHeaderTimeout = 15 * time.Second
	UpstreamRequestTimeout        = 30 * time.Second
	UpstreamExportTimeout         = 120 * time.Second
	UpstreamMaxIdleConnections    = 100
	UpstreamMaxIdlePerHost        = 10
	performanceDetailParallelism  = 8
	upstreamPageSize              = 100
)

type NewAPIClientOptions struct {
	BaseURL              string
	CredentialOrigin     string
	AccessToken          string
	RootUserID           int64
	AllowedHostSuffixes  []string
	AllowedCIDRs         []netip.Prefix
	AllowPrivateNetworks bool
	CAFile               string
	ConnectTimeout       time.Duration
	HeaderTimeout        time.Duration
	RequestTimeout       time.Duration
	ExportTimeout        time.Duration
	Metrics              UpstreamMetricsRecorder
	Governor             UpstreamGovernor
}

type newAPIClientDependencies struct {
	transport              upstreamTransportDependencies
	now                    func() time.Time
	allowNonDesignTimeouts bool
	maxResponseBytes       int64
}

type NewAPIClient struct {
	baseURL          *url.URL
	baseOrigin       string
	credentialOrigin string
	accessToken      string
	rootUserID       int64
	requestTimeout   time.Duration
	exportTimeout    time.Duration
	httpClient       *http.Client
	transport        *http.Transport
	now              func() time.Time
	maxResponseBytes int64
	metrics          UpstreamMetricsRecorder
	governor         UpstreamGovernor
}

// The upstream application does not attach cache headers to its management
// APIs. Keep a very small process-wide read cache for endpoints whose source
// is process memory or a small, slowly-changing catalogue. The key includes
// the origin and credential fingerprint so data cannot cross sites/users.
const upstreamReadCacheMaxEntries = 1024

type upstreamReadCacheEntry struct {
	expiresAt time.Time
	payload   []byte
}

type upstreamReadCacheLock struct {
	mutex sync.Mutex
	users int
}

var upstreamReadCache = struct {
	sync.Mutex
	entries map[string]upstreamReadCacheEntry
	locks   map[string]*upstreamReadCacheLock
}{entries: make(map[string]upstreamReadCacheEntry), locks: make(map[string]*upstreamReadCacheLock)}

func upstreamCacheTTL(endpoint string) time.Duration {
	switch endpoint {
	case "/api/option/", "/api/group/":
		return 5 * time.Minute
	case "/api/pricing", "/api/subscription/admin/plans":
		return 60 * time.Second
	default:
		return 0
	}
}

func (client *NewAPIClient) upstreamCacheKey(method, endpoint string, query url.Values, authMode upstreamAuthMode) string {
	credential := "public"
	if authMode == upstreamAuthManagement {
		sum := sha256.Sum256([]byte(client.accessToken))
		credential = strconv.FormatInt(client.rootUserID, 10) + ":" + fmt.Sprintf("%x", sum[:])
	}
	return client.baseOrigin + "|" + strconv.Itoa(int(authMode)) + "|" + credential + "|" + method + "|" + endpoint + "?" + query.Encode()
}

func acquireUpstreamCacheLock(key string, now time.Time) func() {
	upstreamReadCache.Lock()
	for cachedKey, entry := range upstreamReadCache.entries {
		if !now.Before(entry.expiresAt) {
			delete(upstreamReadCache.entries, cachedKey)
		}
	}
	lock := upstreamReadCache.locks[key]
	if lock == nil {
		lock = &upstreamReadCacheLock{}
		upstreamReadCache.locks[key] = lock
	}
	lock.users++
	upstreamReadCache.Unlock()
	lock.mutex.Lock()
	return func() {
		lock.mutex.Unlock()
		upstreamReadCache.Lock()
		lock.users--
		if lock.users == 0 && upstreamReadCache.locks[key] == lock {
			delete(upstreamReadCache.locks, key)
		}
		upstreamReadCache.Unlock()
	}
}

func storeUpstreamReadCache(key string, entry upstreamReadCacheEntry) {
	upstreamReadCache.Lock()
	defer upstreamReadCache.Unlock()
	if len(upstreamReadCache.entries) >= upstreamReadCacheMaxEntries {
		oldestKey := ""
		var oldestExpiry time.Time
		for cachedKey, cached := range upstreamReadCache.entries {
			if oldestKey == "" || cached.expiresAt.Before(oldestExpiry) {
				oldestKey, oldestExpiry = cachedKey, cached.expiresAt
			}
		}
		delete(upstreamReadCache.entries, oldestKey)
	}
	upstreamReadCache.entries[key] = entry
}

func NewNewAPIClient(options NewAPIClientOptions) (*NewAPIClient, error) {
	return newNewAPIClient(options, newAPIClientDependencies{})
}

func newNewAPIClient(options NewAPIClientOptions, dependencies newAPIClientDependencies) (*NewAPIClient, error) {
	normalizedBaseURL, err := NormalizeUpstreamBaseURL(options.BaseURL)
	if err != nil {
		return nil, err
	}
	parsedBaseURL, err := url.Parse(normalizedBaseURL)
	if err != nil {
		return nil, errors.New("normalized upstream base URL is invalid")
	}
	baseOrigin, err := normalizedRequestOrigin(parsedBaseURL)
	if err != nil {
		return nil, errors.New("normalized upstream base URL has an invalid origin")
	}

	connectTimeout := defaultDuration(options.ConnectTimeout, UpstreamConnectTimeout)
	headerTimeout := defaultDuration(options.HeaderTimeout, UpstreamResponseHeaderTimeout)
	requestTimeout := defaultDuration(options.RequestTimeout, UpstreamRequestTimeout)
	exportTimeout := defaultDuration(options.ExportTimeout, UpstreamExportTimeout)
	if connectTimeout <= 0 || headerTimeout <= 0 || requestTimeout <= 0 || exportTimeout <= 0 {
		return nil, errors.New("upstream timeouts must be positive")
	}

	credentialOrigin, err := validateUpstreamCredentials(options)
	if err != nil {
		return nil, err
	}
	httpClient, transport, err := newSafeUpstreamHTTPClient(
		normalizedBaseURL,
		options.AllowedHostSuffixes,
		options.AllowedCIDRs,
		options.AllowPrivateNetworks,
		options.CAFile,
		connectTimeout,
		headerTimeout,
		UpstreamMaxIdleConnections,
		UpstreamMaxIdlePerHost,
		dependencies.transport,
	)
	if err != nil {
		return nil, err
	}
	now := dependencies.now
	if now == nil {
		now = time.Now
	}
	maxResponseBytes := dependencies.maxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = UpstreamMaxResponseBytes
	}
	if maxResponseBytes <= 0 || maxResponseBytes > UpstreamMaxResponseBytes {
		return nil, errors.New("upstream response limit is invalid")
	}
	return &NewAPIClient{
		baseURL:          parsedBaseURL,
		baseOrigin:       baseOrigin,
		credentialOrigin: credentialOrigin,
		accessToken:      options.AccessToken,
		rootUserID:       options.RootUserID,
		requestTimeout:   requestTimeout,
		exportTimeout:    exportTimeout,
		httpClient:       httpClient,
		transport:        transport,
		now:              now,
		maxResponseBytes: maxResponseBytes,
		metrics:          options.Metrics,
		governor:         options.Governor,
	}, nil
}

func defaultDuration(value, fallback time.Duration) time.Duration {
	if value == 0 {
		return fallback
	}
	return value
}

func validateUpstreamCredentials(options NewAPIClientOptions) (string, error) {
	hasToken := options.AccessToken != ""
	hasRoot := options.RootUserID != 0
	if hasToken != hasRoot {
		return "", errors.New("upstream access token and root user ID must be configured together")
	}
	if !hasToken {
		if options.CredentialOrigin != "" {
			return "", errors.New("upstream credential origin requires credentials")
		}
		return "", nil
	}
	if options.RootUserID <= 0 {
		return "", errors.New("upstream root user ID must be positive")
	}
	if !validAccessToken(options.AccessToken) {
		return "", errors.New("upstream access token is invalid")
	}
	if strings.TrimSpace(options.CredentialOrigin) == "" {
		return "", errors.New("upstream credential origin is required")
	}
	normalized, err := NormalizeUpstreamBaseURL(options.CredentialOrigin)
	if err != nil {
		return "", errors.New("upstream credential origin is invalid")
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", errors.New("upstream credential origin is invalid")
	}
	origin, err := normalizedRequestOrigin(parsed)
	if err != nil {
		return "", errors.New("upstream credential origin is invalid")
	}
	return origin, nil
}

func validAccessToken(token string) bool {
	if !utf8.ValidString(token) || utf8.RuneCountInString(token) < 1 || utf8.RuneCountInString(token) > 4096 {
		return false
	}
	for index := 0; index < len(token); index++ {
		if token[index] < 0x21 || token[index] == 0x7f {
			return false
		}
	}
	return true
}

func (client *NewAPIClient) CloseIdleConnections() {
	if client != nil && client.transport != nil {
		client.transport.CloseIdleConnections()
	}
}

func (client *NewAPIClient) Status(ctx context.Context, requestID string) (dto.UpstreamStatus, error) {
	var wire upstreamStatusWire
	if _, err := client.get(ctx, client.httpClient, "/api/status", nil, requestID, upstreamAuthPublic, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamStatus{}, err
	}
	status, err := client.validateStatus(wire)
	if err != nil {
		return dto.UpstreamStatus{}, err
	}
	if !status.DataExportEnabled {
		return status, newUpstreamRequestError(UpstreamErrorExportDisabled)
	}
	return status, nil
}

func (client *NewAPIClient) Self(ctx context.Context, requestID string) (dto.UpstreamIdentity, error) {
	var wire upstreamIdentityWire
	if _, err := client.get(ctx, client.httpClient, "/api/user/self", nil, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamIdentity{}, err
	}
	identity, err := validateUpstreamIdentity(wire)
	if err != nil {
		return dto.UpstreamIdentity{}, err
	}
	if identity.ID != client.rootUserID || identity.Role != 100 || identity.Status != 1 {
		return dto.UpstreamIdentity{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	return identity, nil
}

func (client *NewAPIClient) GetUser(ctx context.Context, requestID string, userID int64) (dto.UpstreamUser, error) {
	if userID <= 0 {
		return dto.UpstreamUser{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	var wire upstreamUserWire
	endpoint := "/api/user/" + strconv.FormatInt(userID, 10)
	if _, err := client.get(ctx, client.httpClient, endpoint, nil, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		var requestError *UpstreamRequestError
		if errors.As(err, &requestError) && requestError.StatusCode == http.StatusNotFound {
			return dto.UpstreamUser{}, ErrUpstreamUserNotFound
		}
		return dto.UpstreamUser{}, err
	}
	user, err := client.validateUser(wire)
	if err != nil {
		return dto.UpstreamUser{}, err
	}
	if user.ID != userID {
		return dto.UpstreamUser{}, &UpstreamUserIdentityConflictError{ExpectedID: userID, ActualID: user.ID}
	}
	return user, nil
}

func (client *NewAPIClient) ListUsersPage(ctx context.Context, requestID string, page int) (dto.UpstreamUserPage, error) {
	result, _, err := client.listUsersPage(ctx, requestID, "/api/user/", nil, page)
	return result, err
}

func (client *NewAPIClient) SearchUsers(ctx context.Context, requestID, keyword string, page int) (dto.UpstreamUserPage, error) {
	if !validUpstreamString(keyword, 1, 255) {
		return dto.UpstreamUserPage{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	query := url.Values{"keyword": []string{keyword}}
	result, _, err := client.listUsersPage(ctx, requestID, "/api/user/search", query, page)
	return result, err
}

func (client *NewAPIClient) listUsersPage(ctx context.Context, requestID, endpoint string, query url.Values, page int) (dto.UpstreamUserPage, int64, error) {
	if page <= 0 {
		return dto.UpstreamUserPage{}, 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	if query == nil {
		query = make(url.Values)
	} else {
		query = cloneURLValues(query)
	}
	query.Set("p", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(upstreamPageSize))
	query.Set("sort_by", "id")
	query.Set("sort_order", "desc")
	var wire upstreamUserPageWire
	payloadSize, err := client.get(ctx, client.httpClient, endpoint, query, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamUserPage{}, 0, err
	}
	result, err := client.validateUserPage(wire, page)
	if err != nil {
		return dto.UpstreamUserPage{}, 0, err
	}
	return result, payloadSize, nil
}

func (client *NewAPIClient) SnapshotUsers(ctx context.Context, requestID string) (dto.UpstreamUserSnapshot, error) {
	first, size, err := client.listUsersPage(ctx, requestID, "/api/user/", nil, 1)
	if err != nil {
		return dto.UpstreamUserSnapshot{}, err
	}
	if first.Total <= 0 || len(first.Items) == 0 {
		return dto.UpstreamUserSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	if first.Total > 100000 {
		return dto.UpstreamUserSnapshot{}, newUpstreamResponseTooLargeError(first.Total, 100000)
	}
	maximumPages, ok := protectedPageLimit(first.Total)
	if !ok {
		return dto.UpstreamUserSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	items := make([]dto.UpstreamUser, 0, minInt64ToInt(first.Total))
	seen := make(map[int64]struct{}, minInt64ToInt(first.Total))
	if err := appendUniqueUsers(&items, seen, first.Items, first.Total); err != nil {
		return dto.UpstreamUserSnapshot{}, err
	}
	totalPayload := size
	for page := 2; int64(len(items)) < first.Total; page++ {
		if int64(page) > maximumPages {
			return dto.UpstreamUserSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		next, pageSize, err := client.listUsersPage(ctx, requestID, "/api/user/", nil, page)
		if err != nil {
			return dto.UpstreamUserSnapshot{}, err
		}
		if next.Total != first.Total || len(next.Items) == 0 {
			return dto.UpstreamUserSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if totalPayload > client.maxResponseBytes-pageSize {
			return dto.UpstreamUserSnapshot{}, newUpstreamResponseTooLargeError(totalPayload+pageSize, client.maxResponseBytes)
		}
		totalPayload += pageSize
		if err := appendUniqueUsers(&items, seen, next.Items, first.Total); err != nil {
			return dto.UpstreamUserSnapshot{}, err
		}
	}
	if int64(len(items)) != first.Total {
		return dto.UpstreamUserSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	fence, _, err := client.listUsersPage(ctx, requestID+"_fence", "/api/user/", nil, 1)
	if err != nil {
		return dto.UpstreamUserSnapshot{}, err
	}
	if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		return dto.UpstreamUserSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	return dto.UpstreamUserSnapshot{Total: first.Total, Items: items}, nil
}

func appendUniqueUsers(destination *[]dto.UpstreamUser, seen map[int64]struct{}, items []dto.UpstreamUser, expected int64) error {
	for _, item := range items {
		if _, duplicate := seen[item.ID]; duplicate || len(*destination) > 0 && item.ID >= (*destination)[len(*destination)-1].ID {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		seen[item.ID] = struct{}{}
		*destination = append(*destination, item)
		if int64(len(*destination)) > expected {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
	}
	return nil
}

func (client *NewAPIClient) ListChannelsPage(ctx context.Context, requestID string, page int) (dto.UpstreamChannelPage, error) {
	result, _, err := client.listChannelsPage(ctx, requestID, page)
	return result, err
}

func (client *NewAPIClient) listChannelsPage(ctx context.Context, requestID string, page int) (dto.UpstreamChannelPage, int64, error) {
	if page <= 0 {
		return dto.UpstreamChannelPage{}, 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	query := url.Values{
		"p":         []string{strconv.Itoa(page)},
		"page_size": []string{strconv.Itoa(upstreamPageSize)},
		"id_sort":   []string{"true"},
	}
	var wire upstreamChannelPageWire
	payloadSize, err := client.get(ctx, client.httpClient, "/api/channel/", query, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamChannelPage{}, 0, err
	}
	result, err := validateChannelPage(wire, page)
	if err != nil {
		return dto.UpstreamChannelPage{}, 0, err
	}
	return result, payloadSize, nil
}

func (client *NewAPIClient) SnapshotChannels(ctx context.Context, requestID string) (dto.UpstreamChannelSnapshot, error) {
	first, size, err := client.listChannelsPage(ctx, requestID, 1)
	if err != nil {
		return dto.UpstreamChannelSnapshot{}, err
	}
	if first.Total == 0 {
		fence, _, fenceErr := client.listChannelsPage(ctx, requestID+"_fence", 1)
		if fenceErr != nil {
			return dto.UpstreamChannelSnapshot{}, fenceErr
		}
		if fence.Total != 0 || len(fence.Items) != 0 {
			return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		return dto.UpstreamChannelSnapshot{Items: []dto.UpstreamChannel{}}, nil
	}
	if len(first.Items) == 0 {
		return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	if first.Total > 100000 {
		return dto.UpstreamChannelSnapshot{}, newUpstreamResponseTooLargeError(first.Total, 100000)
	}
	maximumPages, ok := protectedPageLimit(first.Total)
	if !ok {
		return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	items := make([]dto.UpstreamChannel, 0, minInt64ToInt(first.Total))
	seen := make(map[int64]struct{}, minInt64ToInt(first.Total))
	if err := appendUniqueChannels(&items, seen, first.Items, first.Total); err != nil {
		return dto.UpstreamChannelSnapshot{}, err
	}
	totalPayload := size
	for page := 2; int64(len(items)) < first.Total; page++ {
		if int64(page) > maximumPages {
			return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		next, pageSize, err := client.listChannelsPage(ctx, requestID, page)
		if err != nil {
			return dto.UpstreamChannelSnapshot{}, err
		}
		if next.Total != first.Total || len(next.Items) == 0 {
			return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if totalPayload > client.maxResponseBytes-pageSize {
			return dto.UpstreamChannelSnapshot{}, newUpstreamResponseTooLargeError(totalPayload+pageSize, client.maxResponseBytes)
		}
		totalPayload += pageSize
		if err := appendUniqueChannels(&items, seen, next.Items, first.Total); err != nil {
			return dto.UpstreamChannelSnapshot{}, err
		}
	}
	if int64(len(items)) != first.Total {
		return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	fence, _, err := client.listChannelsPage(ctx, requestID+"_fence", 1)
	if err != nil {
		return dto.UpstreamChannelSnapshot{}, err
	}
	if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		return dto.UpstreamChannelSnapshot{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	return dto.UpstreamChannelSnapshot{Total: first.Total, Items: items}, nil
}

func appendUniqueChannels(destination *[]dto.UpstreamChannel, seen map[int64]struct{}, items []dto.UpstreamChannel, expected int64) error {
	for _, item := range items {
		if _, duplicate := seen[item.ID]; duplicate || len(*destination) > 0 && item.ID >= (*destination)[len(*destination)-1].ID {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		seen[item.ID] = struct{}{}
		*destination = append(*destination, item)
		if int64(len(*destination)) > expected {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
	}
	return nil
}

func (client *NewAPIClient) listTopupsPage(ctx context.Context, requestID string, page int) (dto.UpstreamTopupPage, int64, error) {
	if page <= 0 {
		return dto.UpstreamTopupPage{}, 0, invalidUpstreamResponse()
	}
	query := url.Values{"p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(upstreamPageSize)}}
	var wire upstreamTopupPageWire
	size, err := client.get(ctx, client.httpClient, "/api/user/topup", query, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamTopupPage{}, 0, err
	}
	result, err := validateTopupPage(wire, page)
	return result, size, err
}
func (client *NewAPIClient) SnapshotTopups(ctx context.Context, requestID string) (dto.UpstreamTopupSnapshot, error) {
	first, size, err := client.listTopupsPage(ctx, requestID+"_first", 1)
	if err != nil {
		return dto.UpstreamTopupSnapshot{}, err
	}
	return client.collectTopups(ctx, requestID, first, size)
}

func (client *NewAPIClient) SnapshotTopupsIncremental(ctx context.Context, requestID string, knownMaxID, knownTotal, oldestPendingID int64) (dto.UpstreamTopupSnapshot, error) {
	first, size, err := client.listTopupsPage(ctx, requestID+"_first", 1)
	if err != nil {
		return dto.UpstreamTopupSnapshot{}, err
	}
	if knownMaxID <= 0 || knownTotal <= 0 || first.Total < knownTotal {
		return client.collectTopups(ctx, requestID, first, size)
	}
	if first.Total > 100000 || first.Total == 0 || len(first.Items) == 0 {
		return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamTopup, 0, len(first.Items))
	seen := map[int64]struct{}{}
	previous := int64(^uint64(0) >> 1)
	reachedKnown := false
	reachedPending := oldestPendingID <= 0
	page, totalSize := 1, size
	current := first
	for {
		if current.Total != first.Total || len(current.Items) == 0 {
			return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
		}
		for _, item := range current.Items {
			if _, ok := seen[item.ID]; ok || item.ID >= previous {
				return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
			}
			seen[item.ID] = struct{}{}
			previous = item.ID
			items = append(items, item)
			if item.ID <= knownMaxID {
				reachedKnown = true
			}
			if oldestPendingID > 0 && item.ID <= oldestPendingID {
				reachedPending = true
			}
		}
		if reachedKnown && reachedPending || int64(len(items)) >= first.Total {
			break
		}
		page++
		next, pageSize, pageErr := client.listTopupsPage(ctx, requestID+"_page_"+strconv.Itoa(page), page)
		if pageErr != nil || totalSize > client.maxResponseBytes-pageSize {
			if pageErr != nil {
				return dto.UpstreamTopupSnapshot{}, pageErr
			}
			return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
		}
		totalSize += pageSize
		current = next
	}
	if (!reachedKnown || !reachedPending) && int64(len(items)) != first.Total {
		return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
	}
	fence, _, err := client.listTopupsPage(ctx, requestID+"_fence", 1)
	if err != nil || fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		if err != nil {
			return dto.UpstreamTopupSnapshot{}, err
		}
		return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
	}
	return dto.UpstreamTopupSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items, Incremental: reachedKnown && reachedPending}, nil
}
func (client *NewAPIClient) collectTopups(ctx context.Context, requestID string, first dto.UpstreamTopupPage, size int64) (dto.UpstreamTopupSnapshot, error) {
	if first.Total > 100000 {
		return dto.UpstreamTopupSnapshot{}, newUpstreamResponseTooLargeError(first.Total, 100000)
	}
	if first.Total == 0 {
		fence, _, err := client.listTopupsPage(ctx, requestID+"_fence", 1)
		if err != nil || fence.Total != 0 || len(fence.Items) != 0 {
			if err != nil {
				return dto.UpstreamTopupSnapshot{}, err
			}
			return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
		}
		return dto.UpstreamTopupSnapshot{Items: []dto.UpstreamTopup{}}, nil
	}
	if len(first.Items) == 0 {
		return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
	}
	items := append([]dto.UpstreamTopup{}, first.Items...)
	seen := map[int64]struct{}{}
	previous := int64(^uint64(0) >> 1)
	for _, item := range items {
		if _, ok := seen[item.ID]; ok || item.ID >= previous {
			return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
		}
		seen[item.ID] = struct{}{}
		previous = item.ID
	}
	totalSize := size
	for page := 2; int64(len(items)) < first.Total; page++ {
		next, pageSize, err := client.listTopupsPage(ctx, requestID+"_page_"+strconv.Itoa(page), page)
		if err != nil {
			return dto.UpstreamTopupSnapshot{}, err
		}
		if next.Total != first.Total || len(next.Items) == 0 || totalSize > client.maxResponseBytes-pageSize {
			return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
		}
		totalSize += pageSize
		for _, item := range next.Items {
			if _, ok := seen[item.ID]; ok || item.ID >= previous {
				return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
			}
			seen[item.ID] = struct{}{}
			previous = item.ID
			items = append(items, item)
		}
	}
	if int64(len(items)) != first.Total {
		return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
	}
	fence, _, err := client.listTopupsPage(ctx, requestID+"_fence", 1)
	if err != nil {
		return dto.UpstreamTopupSnapshot{}, err
	}
	if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		return dto.UpstreamTopupSnapshot{}, invalidUpstreamResponse()
	}
	return dto.UpstreamTopupSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items}, nil
}

func (client *NewAPIClient) listRedemptionsPage(ctx context.Context, requestID string, page int) (dto.UpstreamRedemptionPage, int64, error) {
	if page <= 0 {
		return dto.UpstreamRedemptionPage{}, 0, invalidUpstreamResponse()
	}
	query := url.Values{"p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(upstreamPageSize)}}
	var wire upstreamRedemptionPageWire
	size, err := client.get(ctx, client.httpClient, "/api/redemption/", query, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamRedemptionPage{}, 0, err
	}
	result, err := validateRedemptionPage(wire, page)
	return result, size, err
}
func (client *NewAPIClient) SnapshotRedemptions(ctx context.Context, requestID string) (dto.UpstreamRedemptionSnapshot, error) {
	first, size, err := client.listRedemptionsPage(ctx, requestID+"_first", 1)
	if err != nil {
		return dto.UpstreamRedemptionSnapshot{}, err
	}
	return client.collectRedemptions(ctx, requestID, first, size)
}

func (client *NewAPIClient) collectRedemptions(ctx context.Context, requestID string, first dto.UpstreamRedemptionPage, size int64) (dto.UpstreamRedemptionSnapshot, error) {
	if first.Total > 100000 {
		return dto.UpstreamRedemptionSnapshot{}, newUpstreamResponseTooLargeError(first.Total, 100000)
	}
	if first.Total == 0 {
		fence, _, err := client.listRedemptionsPage(ctx, requestID+"_fence", 1)
		if err != nil || fence.Total != 0 || len(fence.Items) != 0 {
			if err != nil {
				return dto.UpstreamRedemptionSnapshot{}, err
			}
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		return dto.UpstreamRedemptionSnapshot{Items: []dto.UpstreamRedemption{}}, nil
	}
	if len(first.Items) == 0 {
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	items := append([]dto.UpstreamRedemption{}, first.Items...)
	seen := map[int64]struct{}{}
	previous := int64(^uint64(0) >> 1)
	for _, item := range items {
		if _, ok := seen[item.ID]; ok || item.ID >= previous {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		seen[item.ID] = struct{}{}
		previous = item.ID
	}
	totalSize := size
	for page := 2; int64(len(items)) < first.Total; page++ {
		next, pageSize, err := client.listRedemptionsPage(ctx, requestID+"_page_"+strconv.Itoa(page), page)
		if err != nil {
			return dto.UpstreamRedemptionSnapshot{}, err
		}
		if next.Total != first.Total || len(next.Items) == 0 || totalSize > client.maxResponseBytes-pageSize {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		totalSize += pageSize
		for _, item := range next.Items {
			if _, ok := seen[item.ID]; ok || item.ID >= previous {
				return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
			}
			seen[item.ID] = struct{}{}
			previous = item.ID
			items = append(items, item)
		}
	}
	if int64(len(items)) != first.Total {
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	fence, _, err := client.listRedemptionsPage(ctx, requestID+"_fence", 1)
	if err != nil {
		return dto.UpstreamRedemptionSnapshot{}, err
	}
	if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	return dto.UpstreamRedemptionSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items}, nil
}

func (client *NewAPIClient) SnapshotRedemptionsIncremental(ctx context.Context, requestID string, knownMaxID, knownTotal int64, enabledIDs []int64) (dto.UpstreamRedemptionSnapshot, error) {
	first, size, err := client.listRedemptionsPage(ctx, requestID+"_first", 1)
	if err != nil {
		return dto.UpstreamRedemptionSnapshot{}, err
	}
	if knownMaxID <= 0 || knownTotal <= 0 || first.Total < knownTotal {
		return client.collectRedemptions(ctx, requestID, first, size)
	}
	if len(enabledIDs) > 100000 {
		return dto.UpstreamRedemptionSnapshot{}, newUpstreamResponseTooLargeError(int64(len(enabledIDs)), 100000)
	}
	if first.Total > 100000 || first.Total == 0 || len(first.Items) == 0 {
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	items := make([]dto.UpstreamRedemption, 0, len(first.Items))
	seen := map[int64]struct{}{}
	previous := int64(^uint64(0) >> 1)
	reached := false
	page, totalSize := 1, size
	current := first
	for {
		if current.Total != first.Total || len(current.Items) == 0 {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		for _, item := range current.Items {
			if _, ok := seen[item.ID]; ok || item.ID >= previous {
				return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
			}
			seen[item.ID] = struct{}{}
			previous = item.ID
			items = append(items, item)
			if item.ID <= knownMaxID {
				reached = true
			}
		}
		if reached || int64(len(items)) >= first.Total {
			break
		}
		page++
		next, pageSize, pageErr := client.listRedemptionsPage(ctx, requestID+"_page_"+strconv.Itoa(page), page)
		if pageErr != nil || totalSize > client.maxResponseBytes-pageSize {
			if pageErr != nil {
				return dto.UpstreamRedemptionSnapshot{}, pageErr
			}
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		totalSize += pageSize
		current = next
	}
	if !reached && int64(len(items)) != first.Total {
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	if int64(len(items)) == first.Total {
		fence, _, fenceErr := client.listRedemptionsPage(ctx, requestID+"_fence", 1)
		if fenceErr != nil {
			return dto.UpstreamRedemptionSnapshot{}, fenceErr
		}
		if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		return dto.UpstreamRedemptionSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items}, nil
	}
	unseenEnabledIDs := make([]int64, 0, len(enabledIDs))
	unseenEnabled := make(map[int64]struct{}, len(enabledIDs))
	for _, id := range enabledIDs {
		if id <= 0 {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		if _, exists := seen[id]; exists {
			continue
		}
		if _, duplicate := unseenEnabled[id]; duplicate {
			continue
		}
		unseenEnabled[id] = struct{}{}
		unseenEnabledIDs = append(unseenEnabledIDs, id)
	}
	totalPages := int((first.Total + upstreamPageSize - 1) / upstreamPageSize)
	remainingPages := totalPages - page
	if remainingPages <= 0 {
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	if len(unseenEnabledIDs) > remainingPages {
		for int64(len(items)) < first.Total {
			page++
			next, pageSize, pageErr := client.listRedemptionsPage(ctx, requestID+"_page_"+strconv.Itoa(page), page)
			if pageErr != nil {
				return dto.UpstreamRedemptionSnapshot{}, pageErr
			}
			if next.Total != first.Total || len(next.Items) == 0 || totalSize > client.maxResponseBytes-pageSize {
				return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
			}
			totalSize += pageSize
			for _, item := range next.Items {
				if _, duplicate := seen[item.ID]; duplicate || item.ID >= previous {
					return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
				}
				seen[item.ID] = struct{}{}
				previous = item.ID
				items = append(items, item)
			}
		}
		if int64(len(items)) != first.Total {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		fence, _, fenceErr := client.listRedemptionsPage(ctx, requestID+"_fence", 1)
		if fenceErr != nil {
			return dto.UpstreamRedemptionSnapshot{}, fenceErr
		}
		if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		return dto.UpstreamRedemptionSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items}, nil
	}
	for index, id := range unseenEnabledIDs {
		item, detailErr := client.redemptionByID(ctx, requestID+"_enabled_"+strconv.Itoa(index), id)
		if detailErr != nil {
			return client.collectRedemptions(ctx, requestID+"_fallback", first, size)
		}
		if _, duplicate := seen[item.ID]; duplicate || item.ID != id {
			return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
		}
		seen[item.ID] = struct{}{}
		items = append(items, item)
	}
	fence, _, err := client.listRedemptionsPage(ctx, requestID+"_fence", 1)
	if err != nil || fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		if err != nil {
			return dto.UpstreamRedemptionSnapshot{}, err
		}
		return dto.UpstreamRedemptionSnapshot{}, invalidUpstreamResponse()
	}
	return dto.UpstreamRedemptionSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items, Incremental: reached}, nil
}

func (client *NewAPIClient) redemptionByID(ctx context.Context, requestID string, id int64) (dto.UpstreamRedemption, error) {
	if id <= 0 {
		return dto.UpstreamRedemption{}, invalidUpstreamResponse()
	}
	var wire upstreamRedemptionWire
	endpoint := "/api/redemption/" + strconv.FormatInt(id, 10)
	if _, err := client.get(ctx, client.httpClient, endpoint, nil, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamRedemption{}, err
	}
	pageNumber, pageSize, total := 1, upstreamPageSize, int64(1)
	wires := []upstreamRedemptionWire{wire}
	page, err := validateRedemptionPage(upstreamRedemptionPageWire{
		Page: &pageNumber, PageSize: &pageSize, Total: &total, Items: &wires,
	}, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != id {
		if err != nil {
			return dto.UpstreamRedemption{}, err
		}
		return dto.UpstreamRedemption{}, invalidUpstreamResponse()
	}
	return page.Items[0], nil
}

func (client *NewAPIClient) listTasksPage(ctx context.Context, requestID string, page int, query url.Values) (dto.UpstreamTaskPage, int64, error) {
	if page <= 0 {
		return dto.UpstreamTaskPage{}, 0, invalidUpstreamResponse()
	}
	values := url.Values{"p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(upstreamPageSize)}}
	for k, v := range query {
		values[k] = append([]string(nil), v...)
	}
	var wire upstreamTaskPageWire
	size, err := client.get(ctx, client.httpClient, "/api/task/", values, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamTaskPage{}, 0, err
	}
	result, err := validateTaskPage(wire, page)
	return result, size, err
}
func (client *NewAPIClient) listModelMetaPage(ctx context.Context, requestID string, page int) (dto.UpstreamModelMetaPage, int64, error) {
	var wire upstreamModelMetaPageWire
	size, err := client.get(ctx, client.httpClient, "/api/models/", url.Values{"p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(upstreamPageSize)}}, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamModelMetaPage{}, 0, err
	}
	result, err := validateModelMetaPage(wire, page)
	return result, size, err
}
func (client *NewAPIClient) SnapshotModelMeta(ctx context.Context, requestID string) (dto.UpstreamModelMetaSnapshot, error) {
	first, size, err := client.listModelMetaPage(ctx, requestID+"_first", 1)
	if err != nil {
		return dto.UpstreamModelMetaSnapshot{}, err
	}
	if first.Total > 100000 {
		return dto.UpstreamModelMetaSnapshot{}, newUpstreamResponseTooLargeError(first.Total, 100000)
	}
	if first.Total == 0 {
		f, _, e := client.listModelMetaPage(ctx, requestID+"_fence", 1)
		if e != nil || f.Total != 0 {
			return dto.UpstreamModelMetaSnapshot{}, invalidUpstreamResponse()
		}
		return dto.UpstreamModelMetaSnapshot{}, nil
	}
	if len(first.Items) == 0 {
		return dto.UpstreamModelMetaSnapshot{}, invalidUpstreamResponse()
	}
	items := append([]dto.UpstreamModelMeta{}, first.Items...)
	totalSize := size
	for page := 2; int64(len(items)) < first.Total; page++ {
		next, n, err := client.listModelMetaPage(ctx, requestID+"_page_"+strconv.Itoa(page), page)
		if err != nil {
			return dto.UpstreamModelMetaSnapshot{}, err
		}
		if next.Total != first.Total || len(next.Items) == 0 || totalSize > client.maxResponseBytes-n || next.Items[0].ID >= items[len(items)-1].ID {
			return dto.UpstreamModelMetaSnapshot{}, invalidUpstreamResponse()
		}
		totalSize += n
		items = append(items, next.Items...)
	}
	if int64(len(items)) != first.Total {
		return dto.UpstreamModelMetaSnapshot{}, invalidUpstreamResponse()
	}
	f, _, err := client.listModelMetaPage(ctx, requestID+"_fence", 1)
	if err != nil || f.Total != first.Total || len(f.Items) == 0 || f.Items[0].ID != first.Items[0].ID {
		return dto.UpstreamModelMetaSnapshot{}, invalidUpstreamResponse()
	}
	return dto.UpstreamModelMetaSnapshot{Total: first.Total, MaxID: first.Items[0].ID, Items: items}, nil
}
func (client *NewAPIClient) SnapshotSubscriptionPlans(ctx context.Context, requestID string) (dto.UpstreamSubscriptionPlanSnapshot, error) {
	var wire []upstreamSubscriptionPlanDTO
	_, err := client.get(ctx, client.httpClient, "/api/subscription/admin/plans", nil, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false)
	if err != nil {
		return dto.UpstreamSubscriptionPlanSnapshot{}, err
	}
	return validateSubscriptionPlans(wire)
}

func (client *NewAPIClient) SnapshotPricingCatalog(ctx context.Context, requestID string) (dto.UpstreamPricingSnapshot, error) {
	groups, err := client.SnapshotPricingGroups(ctx, requestID+"_groups")
	if err != nil {
		return dto.UpstreamPricingSnapshot{}, err
	}
	pricing, err := client.SnapshotPricing(ctx, requestID+"_pricing")
	if err != nil {
		return dto.UpstreamPricingSnapshot{}, err
	}
	configuration, err := client.SnapshotPricingConfiguration(ctx, requestID+"_configuration")
	if err != nil {
		return dto.UpstreamPricingSnapshot{}, err
	}
	return mergePricingConfiguration(groups, pricing, configuration), nil
}

func (client *NewAPIClient) SnapshotPricingConfiguration(ctx context.Context, requestID string) (dto.UpstreamPricingConfiguration, error) {
	var options upstreamOptionsResponseWire
	if _, err := client.get(ctx, client.httpClient, "/api/option/", nil, requestID, upstreamAuthManagement, client.requestTimeout, &options, false); err != nil {
		return dto.UpstreamPricingConfiguration{}, err
	}
	return validatePricingConfiguration(*options.Data)
}

func (client *NewAPIClient) SnapshotPricingGroups(ctx context.Context, requestID string) (dto.UpstreamPricingGroupSnapshot, error) {
	var groupNames []string
	if _, err := client.get(ctx, client.httpClient, "/api/group/", nil, requestID, upstreamAuthManagement, client.requestTimeout, &groupNames, false); err != nil {
		return dto.UpstreamPricingGroupSnapshot{}, err
	}
	return validatePricingGroups(groupNames)
}

func (client *NewAPIClient) SnapshotPricing(ctx context.Context, requestID string) (dto.UpstreamPricingOnlySnapshot, error) {
	var pricing upstreamPricingResponseWire
	if _, err := client.get(ctx, client.httpClient, "/api/pricing", nil, requestID, upstreamAuthManagement, client.requestTimeout, &pricing, false); err != nil {
		return dto.UpstreamPricingOnlySnapshot{}, err
	}
	return validatePricing(pricing)
}
func (client *NewAPIClient) collectTaskQuery(ctx context.Context, requestID string, query url.Values) ([]dto.UpstreamTask, error) {
	first, size, err := client.listTasksPage(ctx, requestID+"_first", 1, query)
	if err != nil {
		return nil, err
	}
	if first.Total > 100000 {
		return nil, newUpstreamResponseTooLargeError(first.Total, 100000)
	}
	if first.Total == 0 {
		fence, _, err := client.listTasksPage(ctx, requestID+"_fence", 1, query)
		if err != nil {
			return nil, err
		}
		if fence.Total != 0 || len(fence.Items) != 0 {
			return nil, invalidUpstreamResponse()
		}
		return []dto.UpstreamTask{}, nil
	}
	if len(first.Items) == 0 {
		return nil, invalidUpstreamResponse()
	}
	items := append([]dto.UpstreamTask{}, first.Items...)
	previous := items[len(items)-1].ID
	totalSize := size
	for page := 2; int64(len(items)) < first.Total; page++ {
		next, pageSize, err := client.listTasksPage(ctx, requestID+"_page_"+strconv.Itoa(page), page, query)
		if err != nil {
			return nil, err
		}
		if next.Total != first.Total || len(next.Items) == 0 || totalSize > client.maxResponseBytes-pageSize || next.Items[0].ID >= previous {
			return nil, invalidUpstreamResponse()
		}
		totalSize += pageSize
		items = append(items, next.Items...)
		previous = next.Items[len(next.Items)-1].ID
	}
	if int64(len(items)) != first.Total {
		return nil, invalidUpstreamResponse()
	}
	fence, _, err := client.listTasksPage(ctx, requestID+"_fence", 1, query)
	if err != nil {
		return nil, err
	}
	if fence.Total != first.Total || len(fence.Items) == 0 || fence.Items[0].ID != first.Items[0].ID {
		return nil, invalidUpstreamResponse()
	}
	return items, nil
}
func (client *NewAPIClient) SnapshotUpstreamTasks(ctx context.Context, requestID string, start, end int64, unfinishedTaskIDs []string) (dto.UpstreamTaskSnapshot, error) {
	if start <= 0 || end <= start || len(unfinishedTaskIDs) > 100000 {
		return dto.UpstreamTaskSnapshot{}, invalidUpstreamResponse()
	}
	window, err := client.collectTaskQuery(ctx, requestID+"_window", url.Values{"start_timestamp": {strconv.FormatInt(start, 10)}, "end_timestamp": {strconv.FormatInt(end, 10)}})
	if err != nil {
		return dto.UpstreamTaskSnapshot{}, err
	}
	byID := make(map[int64]dto.UpstreamTask, len(window))
	for _, item := range window {
		byID[item.ID] = item
	}
	seenTaskIDs := map[string]struct{}{}
	for index, taskID := range unfinishedTaskIDs {
		if !validUpstreamString(taskID, 1, 191) {
			return dto.UpstreamTaskSnapshot{}, invalidUpstreamResponse()
		}
		if _, duplicate := seenTaskIDs[taskID]; duplicate {
			continue
		}
		seenTaskIDs[taskID] = struct{}{}
		rows, err := client.collectTaskQuery(ctx, requestID+"_unfinished_"+strconv.Itoa(index), url.Values{"task_id": {taskID}})
		if err != nil {
			return dto.UpstreamTaskSnapshot{}, err
		}
		for _, item := range rows {
			existing, ok := byID[item.ID]
			if !ok || item.UpdatedAt > existing.UpdatedAt {
				byID[item.ID] = item
			}
		}
		if len(byID) > 100000 {
			return dto.UpstreamTaskSnapshot{}, newUpstreamResponseTooLargeError(int64(len(byID)), 100000)
		}
	}
	items := make([]dto.UpstreamTask, 0, len(byID))
	for _, item := range byID {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return dto.UpstreamTaskSnapshot{Items: items}, nil
}

func protectedPageLimit(total int64) (int64, bool) {
	if total < 0 {
		return 0, false
	}
	pages := total / upstreamPageSize
	if total%upstreamPageSize != 0 {
		pages++
	}
	if pages > int64(^uint(0)>>1)-2 {
		return 0, false
	}
	return pages + 2, true
}

func minInt64ToInt(value int64) int {
	const maximumInitialCapacity = 1024
	if value > maximumInitialCapacity {
		return maximumInitialCapacity
	}
	if value < 0 {
		return 0
	}
	return int(value)
}

func (client *NewAPIClient) FlowHour(ctx context.Context, requestID string, hourStart int64) ([]dto.UpstreamFlowRow, error) {
	query, err := hourQuery(hourStart)
	if err != nil {
		return nil, err
	}
	var wire []upstreamFlowRowWire
	if _, err := client.get(ctx, client.httpClient, "/api/data/flow", query, requestID, upstreamAuthManagement, client.exportTimeout, &wire, false); err != nil {
		return nil, err
	}
	rows, err := validateAndAggregateFlowRows(wire)
	return rows, annotateUpstreamRequestError(err, http.MethodGet, "/api/data/flow", "", 0, 0)
}

func (client *NewAPIClient) LogPage(ctx context.Context, requestID string, start, end int64, page int) (dto.UpstreamLogPage, error) {
	if start <= 0 || end < start || page <= 0 {
		return dto.UpstreamLogPage{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	query := url.Values{
		"p":               []string{strconv.Itoa(page)},
		"page_size":       []string{strconv.Itoa(upstreamPageSize)},
		"start_timestamp": []string{strconv.FormatInt(start, 10)},
		"end_timestamp":   []string{strconv.FormatInt(end, 10)},
	}
	var wire upstreamLogPageWire
	if _, err := client.get(ctx, client.httpClient, "/api/log/", query, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamLogPage{}, err
	}
	return validateLogPage(wire, page)
}

func (client *NewAPIClient) DataHour(ctx context.Context, requestID string, hourStart int64) ([]dto.UpstreamDataRow, error) {
	query, err := hourQuery(hourStart)
	if err != nil {
		return nil, err
	}
	var wire []upstreamDataRowWire
	if _, err := client.get(ctx, client.httpClient, "/api/data", query, requestID, upstreamAuthManagement, client.exportTimeout, &wire, false); err != nil {
		return nil, err
	}
	rows, err := validateAndAggregateDataRows(wire, hourStart)
	return rows, annotateUpstreamRequestError(err, http.MethodGet, "/api/data", "", 0, 0)
}

func hourQuery(hourStart int64) (url.Values, error) {
	if hourStart <= 0 || hourStart%3600 != 0 || hourStart > int64(^uint64(0)>>1)-3599 {
		return nil, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	return url.Values{
		"start_timestamp": []string{strconv.FormatInt(hourStart, 10)},
		"end_timestamp":   []string{strconv.FormatInt(hourStart+3599, 10)},
	}, nil
}

func ValidateFlowDataConsistency(flow []dto.UpstreamFlowRow, data []dto.UpstreamDataRow) error {
	flowTotals := make(map[string]metricTotals)
	for _, row := range flow {
		if row.UserID <= 0 || row.ChannelID < 0 || row.RequestCount < 0 || row.Quota < 0 || row.TokenUsed < 0 ||
			!validUpstreamString(row.Username, 0, 255) || !validUpstreamString(row.ModelName, 0, 255) {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		totals := flowTotals[row.ModelName]
		var ok bool
		if totals.RequestCount, ok = checkedAddInt64(totals.RequestCount, row.RequestCount); !ok {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if totals.Quota, ok = checkedAddInt64(totals.Quota, row.Quota); !ok {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if totals.TokenUsed, ok = checkedAddInt64(totals.TokenUsed, row.TokenUsed); !ok {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		flowTotals[row.ModelName] = totals
	}
	dataTotals := make(map[string]metricTotals)
	for _, row := range data {
		if row.CreatedAt <= 0 || row.RequestCount < 0 || row.Quota < 0 || row.TokenUsed < 0 ||
			!validUpstreamString(row.ModelName, 0, 255) {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		totals := dataTotals[row.ModelName]
		var ok bool
		if totals.RequestCount, ok = checkedAddInt64(totals.RequestCount, row.RequestCount); !ok {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if totals.Quota, ok = checkedAddInt64(totals.Quota, row.Quota); !ok {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if totals.TokenUsed, ok = checkedAddInt64(totals.TokenUsed, row.TokenUsed); !ok {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		dataTotals[row.ModelName] = totals
	}
	if len(flowTotals) != len(dataTotals) {
		return newUpstreamRequestError(UpstreamErrorDataMismatch)
	}
	for modelName, expected := range dataTotals {
		if actual, exists := flowTotals[modelName]; !exists || actual != expected {
			return newUpstreamRequestError(UpstreamErrorDataMismatch)
		}
	}
	return nil
}

func (client *NewAPIClient) Instances(ctx context.Context, requestID string) ([]dto.UpstreamInstance, error) {
	var wire []upstreamInstanceWire
	if _, err := client.get(ctx, client.httpClient, "/api/system-info/instances", nil, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return nil, err
	}
	return client.validateInstances(wire)
}

func (client *NewAPIClient) LogStat(ctx context.Context, requestID string) (dto.UpstreamLogStat, error) {
	// The upstream quota aggregation is unbounded without explicit timestamps,
	// even though RPM/TPM independently cover only the last minute.
	now := client.now().Unix()
	if now <= 60 {
		return dto.UpstreamLogStat{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	query := url.Values{
		"start_timestamp": {strconv.FormatInt(now-60, 10)},
		"end_timestamp":   {strconv.FormatInt(now, 10)},
	}
	var wire upstreamLogStatWire
	if _, err := client.get(ctx, client.httpClient, "/api/log/stat", query, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamLogStat{}, err
	}
	return validateLogStat(wire)
}

func (client *NewAPIClient) PerformanceSummary(ctx context.Context, requestID string, hours int) (dto.UpstreamPerformanceSummary, error) {
	if hours < 1 || hours > 24*30 {
		return dto.UpstreamPerformanceSummary{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	var response upstreamPerformanceSummaryResponse
	if _, err := client.get(ctx, client.httpClient, "/api/perf-metrics/summary", url.Values{"hours": []string{strconv.Itoa(hours)}}, requestID, upstreamAuthManagement, client.requestTimeout, &response, false); err != nil {
		return dto.UpstreamPerformanceSummary{}, err
	}
	if err := validatePerformanceSummary(response.Summary); err != nil {
		return dto.UpstreamPerformanceSummary{}, err
	}
	return response.Summary, nil
}

func (client *NewAPIClient) PerformanceHistory(ctx context.Context, requestID string, hours int) (dto.UpstreamPerformanceHistory, error) {
	if hours < 1 || hours > 720 {
		return dto.UpstreamPerformanceHistory{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	summary, err := client.PerformanceSummary(ctx, requestID+"_summary", hours)
	if err != nil {
		return dto.UpstreamPerformanceHistory{}, err
	}
	if len(summary.Models) > 1000 {
		return dto.UpstreamPerformanceHistory{}, newUpstreamResponseTooLargeError(int64(len(summary.Models)), 1000)
	}
	models, counterReady, err := client.performanceModelHistories(ctx, requestID, hours, summary.Models)
	if err != nil {
		return dto.UpstreamPerformanceHistory{}, err
	}
	return dto.UpstreamPerformanceHistory{Models: models, CounterReady: counterReady && len(models) > 0}, nil
}

// PerformanceHistoryIncremental keeps the summary/model contract intact but
// refreshes only the short rolling window supplied by the scheduler. The
// upstream API does not expose a reliable since-cursor, so older local facts
// are retained by the repository and only the moving buckets are replaced.
func (client *NewAPIClient) PerformanceHistoryIncremental(ctx context.Context, requestID string, hours int, knownModels []string) (dto.UpstreamPerformanceHistory, error) {
	if hours < 1 || hours > 720 {
		return dto.UpstreamPerformanceHistory{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	summary, err := client.PerformanceSummary(ctx, requestID+"_summary", hours)
	if err != nil {
		return dto.UpstreamPerformanceHistory{}, err
	}
	// knownModels is deliberately advisory: the summary remains authoritative
	// for additions/removals, while the local set supplies a stable request
	// order for models that are still present.
	models := make([]dto.UpstreamPerformanceModel, 0, len(summary.Models))
	byName := make(map[string]dto.UpstreamPerformanceModel, len(summary.Models))
	for _, item := range summary.Models {
		byName[item.ModelName] = item
	}
	// Preserve the local order for stable model sets, then append newly
	// discovered models from the authoritative summary.
	for _, name := range knownModels {
		if item, ok := byName[name]; ok {
			models = append(models, item)
			delete(byName, name)
		}
	}
	for _, item := range summary.Models {
		if _, ok := byName[item.ModelName]; ok {
			models = append(models, item)
			delete(byName, item.ModelName)
		}
	}
	if len(models) > 1000 {
		return dto.UpstreamPerformanceHistory{}, newUpstreamResponseTooLargeError(int64(len(models)), 1000)
	}
	histories, counterReady, err := client.performanceModelHistories(ctx, requestID, hours, models)
	if err != nil {
		return dto.UpstreamPerformanceHistory{}, err
	}
	out := dto.UpstreamPerformanceHistory{Models: histories, CounterReady: len(histories) > 0 && counterReady}
	return out, nil
}

func (client *NewAPIClient) performanceModelHistories(
	ctx context.Context,
	requestID string,
	hours int,
	models []dto.UpstreamPerformanceModel,
) ([]dto.UpstreamPerformanceModelHistory, bool, error) {
	seen := make(map[string]struct{}, len(models))
	for _, item := range models {
		if _, duplicate := seen[item.ModelName]; duplicate {
			return nil, false, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		seen[item.ModelName] = struct{}{}
	}
	if len(models) == 0 {
		return []dto.UpstreamPerformanceModelHistory{}, false, nil
	}
	type result struct {
		model dto.UpstreamPerformanceModelHistory
		ready bool
		err   error
	}
	results := make([]result, len(models))
	jobs := make(chan int)
	workers := performanceDetailParallelism
	if workers > len(models) {
		workers = len(models)
	}
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				item := models[index]
				var wire upstreamPerformanceHistoryWire
				if _, err := client.get(ctx, client.httpClient, "/api/perf-metrics", url.Values{
					"model": {item.ModelName}, "hours": {strconv.Itoa(hours)},
				}, fmt.Sprintf("%s_model_%d", requestID, index+1), upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
					results[index].err = err
					continue
				}
				results[index].model, results[index].ready, results[index].err =
					validatePerformanceHistory(wire, item.ModelName, client.now().Unix())
			}
		}()
	}
	for index := range models {
		jobs <- index
	}
	close(jobs)
	wait.Wait()

	histories := make([]dto.UpstreamPerformanceModelHistory, len(results))
	counterReady := true
	for index, item := range results {
		if item.err != nil {
			return nil, false, item.err
		}
		histories[index] = item.model
		counterReady = counterReady && item.ready
	}
	return histories, counterReady, nil
}

func (client *NewAPIClient) LoginAndGenerateAccessToken(ctx context.Context, requestID, username, password string) (dto.UpstreamIdentity, string, error) {
	if !validUpstreamString(username, 1, 128) || !validUpstreamString(password, 1, 1024) {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorUnavailable)
	}
	sessionClient, sessionTransport := client.newMutationSessionClient(jar)
	defer sessionTransport.CloseIdleConnections()
	body, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password})
	if err != nil {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	var loginWire upstreamLoginWire
	if _, err := client.do(ctx, sessionClient, http.MethodPost, "/api/user/login", nil, requestID, upstreamAuthPublic, client.requestTimeout, bytes.NewReader(body), "application/json", &loginWire, false); err != nil {
		var requestError *UpstreamRequestError
		if errors.As(err, &requestError) && requestError.Kind == UpstreamErrorEnvelopeInvalid && requestError.Detail == "success_false" {
			return dto.UpstreamIdentity{}, "", ErrUpstreamLoginRejected
		}
		return dto.UpstreamIdentity{}, "", err
	}
	identityWire := loginWire.upstreamIdentityWire
	sessionAuth := upstreamSessionAuth{}
	if loginWire.User != nil {
		if loginWire.AccessToken == nil || loginWire.TokenType == nil || loginWire.AccessExpiresAt == nil ||
			*loginWire.TokenType != "Bearer" || !validAccessToken(*loginWire.AccessToken) ||
			*loginWire.AccessExpiresAt <= client.now().Unix() {
			return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		identityWire = *loginWire.User
		sessionAuth.bearerToken = *loginWire.AccessToken
	} else if loginWire.AccessToken != nil || loginWire.TokenType != nil || loginWire.AccessExpiresAt != nil {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	identity, err := validateUpstreamIdentity(identityWire)
	if err != nil || identity.Role != 100 || identity.Status != 1 {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	sessionAuth.userID = identity.ID
	var token string
	if _, err := client.get(ctx, sessionClient, "/api/user/token", nil, requestID, upstreamAuthSession, client.requestTimeout, &token, true, sessionAuth); err != nil {
		return dto.UpstreamIdentity{}, "", err
	}
	if !validAccessToken(token) {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
	}
	return identity, token, nil
}

func (client *NewAPIClient) newMutationSessionClient(jar http.CookieJar) (*http.Client, *http.Transport) {
	transport := client.transport.Clone()
	transport.DisableKeepAlives = true
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	tlsConfig := transport.TLSClientConfig.Clone()
	tlsConfig.NextProtos = []string{"http/1.1"}
	transport.TLSClientConfig = tlsConfig
	return &http.Client{
		Transport: transport,
		Jar:       jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, transport
}

type upstreamAuthMode int

type upstreamSessionAuth struct {
	userID      int64
	bearerToken string
}

type upstreamResponseDecoder interface {
	decodeUpstreamResponse([]byte) error
}

const (
	upstreamAuthPublic upstreamAuthMode = iota
	upstreamAuthManagement
	upstreamAuthSession
)

func (client *NewAPIClient) get(
	ctx context.Context,
	httpClient *http.Client,
	endpoint string,
	query url.Values,
	requestID string,
	authMode upstreamAuthMode,
	timeout time.Duration,
	destination any,
	tokenMutation bool,
	sessionAuth ...upstreamSessionAuth,
) (int64, error) {
	return client.do(ctx, httpClient, http.MethodGet, endpoint, query, requestID, authMode, timeout, nil, "", destination, tokenMutation, sessionAuth...)
}

func (client *NewAPIClient) do(
	ctx context.Context,
	httpClient *http.Client,
	method string,
	endpoint string,
	query url.Values,
	requestID string,
	authMode upstreamAuthMode,
	timeout time.Duration,
	body io.Reader,
	contentType string,
	destination any,
	tokenMutation bool,
	sessionAuth ...upstreamSessionAuth,
) (payloadSize int64, resultErr error) {
	startedAt := time.Now()
	responseStatus := 0
	responseContentType := ""
	if client != nil {
		defer func() {
			if resultErr != nil {
				var requestError *UpstreamRequestError
				if errors.As(resultErr, &requestError) {
					log.Printf("upstream request failed request_id=%s method=%s endpoint=%s status=%d content_type=%q payload_bytes=%d error_kind=%s detail=%s", requestID, method, endpoint, requestError.StatusCode, requestError.ContentType, requestError.PayloadBytes, requestError.Kind, requestError.Detail)
				} else {
					log.Printf("upstream request failed request_id=%s method=%s endpoint=%s status=%d error_type=%T", requestID, method, endpoint, responseStatus, resultErr)
				}
			}
		}()
	}
	if client != nil && client.metrics != nil {
		operation := upstreamMetricOperation(method, endpoint)
		defer func() {
			recordServiceMetric(func() {
				client.metrics.ObserveUpstream(operation, upstreamMetricResult(resultErr), time.Since(startedAt))
			})
		}()
	}
	if client == nil || httpClient == nil || !validRequestID(requestID) {
		return 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	if authMode == upstreamAuthManagement {
		if client.accessToken == "" || client.rootUserID <= 0 || client.credentialOrigin != client.baseOrigin {
			return 0, newUpstreamRequestError(UpstreamErrorCredentialOriginMismatch)
		}
	}
	requestURL := *client.baseURL
	requestURL.Path = strings.TrimRight(client.baseURL.Path, "/") + endpoint
	requestURL.RawPath = ""
	requestURL.RawQuery = query.Encode()
	cacheTTL := time.Duration(0)
	cacheKey := ""
	var releaseCacheLock func()
	if method == http.MethodGet && !tokenMutation {
		cacheTTL = upstreamCacheTTL(endpoint)
		if cacheTTL > 0 {
			cacheKey = client.upstreamCacheKey(method, endpoint, query, authMode)
			releaseCacheLock = acquireUpstreamCacheLock(cacheKey, client.now())
			defer releaseCacheLock()
			upstreamReadCache.Lock()
			entry, ok := upstreamReadCache.entries[cacheKey]
			upstreamReadCache.Unlock()
			if ok && client.now().Before(entry.expiresAt) {
				var decodeErr error
				if decoder, decoderOK := destination.(upstreamResponseDecoder); decoderOK {
					decodeErr = decoder.decodeUpstreamResponse(entry.payload)
				} else {
					decodeErr = decodeUpstreamEnvelope(entry.payload, destination)
				}
				if decodeErr == nil {
					return int64(len(entry.payload)), nil
				}
				upstreamReadCache.Lock()
				delete(upstreamReadCache.entries, cacheKey)
				upstreamReadCache.Unlock()
			}
		}
	}
	if client.governor != nil {
		release, acquireErr := client.governor.Acquire(ctx, client.baseOrigin, upstreamRequestClassFromContext(ctx))
		if acquireErr != nil {
			return 0, annotateUpstreamRequestError(newUpstreamRequestError(UpstreamErrorUnavailable), method, endpoint, "", 0, 0)
		}
		defer release()
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, method, requestURL.String(), body)
	if err != nil {
		return 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	preventAutomaticRequestReplay(request)
	isLoginMutation := method == http.MethodPost && endpoint == "/api/user/login"
	if isLoginMutation || tokenMutation {
		request.Close = true
		request.GetBody = nil
	}
	var requestSent atomic.Bool
	if tokenMutation {
		trace := &httptrace.ClientTrace{
			WroteHeaders: func() {
				requestSent.Store(true)
			},
			WroteRequest: func(httptrace.WroteRequestInfo) {
				requestSent.Store(true)
			},
		}
		request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	}
	request.Header.Set("User-Agent", NewAPIClientUserAgent)
	request.Header.Set("X-Request-ID", requestID)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	switch authMode {
	case upstreamAuthManagement:
		request.Header.Set("Authorization", client.accessToken)
		request.Header.Set("New-Api-User", strconv.FormatInt(client.rootUserID, 10))
	case upstreamAuthSession:
		if len(sessionAuth) != 1 || sessionAuth[0].userID <= 0 {
			return 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		request.Header.Set("New-Api-User", strconv.FormatInt(sessionAuth[0].userID, 10))
		if sessionAuth[0].bearerToken != "" {
			if !validAccessToken(sessionAuth[0].bearerToken) {
				return 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
			}
			request.Header.Set("Authorization", "Bearer "+sessionAuth[0].bearerToken)
		}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if tokenMutation && requestSent.Load() {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		if errors.Is(err, ErrUpstreamAddressForbidden) {
			reason := upstreamAddressRejectionReason(err)
			return 0, newUpstreamRequestErrorWithDetail(UpstreamErrorAddressForbidden, reason)
		}
		return 0, annotateUpstreamRequestError(newUpstreamRequestError(UpstreamErrorUnavailable), method, endpoint, "", 0, 0)
	}
	defer response.Body.Close()
	responseStatus = response.StatusCode
	responseContentType = response.Header.Get("Content-Type")
	if tokenMutation && response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		drainUpstreamResponse(response.Body)
		return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
	}
	retryAfter, hasRetryAfter := parseRetryAfter(response.Header.Get("Retry-After"), client.now())
	if response.StatusCode == http.StatusTooManyRequests && client.governor != nil {
		client.governor.ObserveRateLimit(client.baseOrigin, retryAfter, hasRetryAfter)
	}
	if statusErr := classifyUpstreamHTTPStatus(response.StatusCode, retryAfter, hasRetryAfter); statusErr != nil {
		drainUpstreamResponse(response.Body)
		return 0, annotateUpstreamRequestError(statusErr, method, endpoint, responseContentType, responseStatus, 0)
	}
	if response.ContentLength > client.maxResponseBytes {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, annotateUpstreamRequestError(newUpstreamResponseTooLargeError(response.ContentLength, client.maxResponseBytes), method, endpoint, responseContentType, responseStatus, response.ContentLength)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
	if err != nil {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, annotateUpstreamRequestError(newUpstreamRequestError(UpstreamErrorUnavailable), method, endpoint, responseContentType, responseStatus, 0)
	}
	if int64(len(payload)) > client.maxResponseBytes {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, annotateUpstreamRequestError(newUpstreamResponseTooLargeError(int64(len(payload)), client.maxResponseBytes), method, endpoint, responseContentType, responseStatus, int64(len(payload)))
	}
	payloadSize = int64(len(payload))
	var decodeErr error
	if decoder, ok := destination.(upstreamResponseDecoder); ok {
		decodeErr = decoder.decodeUpstreamResponse(payload)
	} else {
		decodeErr = decodeUpstreamEnvelope(payload, destination)
	}
	if decodeErr != nil {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, annotateUpstreamRequestError(decodeErr, method, endpoint, responseContentType, responseStatus, payloadSize)
	}
	if cacheTTL > 0 {
		payloadCopy := append([]byte(nil), payload...)
		storeUpstreamReadCache(cacheKey, upstreamReadCacheEntry{expiresAt: client.now().Add(cacheTTL), payload: payloadCopy})
	}
	return int64(len(payload)), nil
}

func validRequestID(requestID string) bool {
	if len(requestID) < 1 || len(requestID) > 64 {
		return false
	}
	for _, character := range requestID {
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func drainUpstreamResponse(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64<<10))
}

func parseRetryAfter(raw string, now time.Time) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	var delay time.Duration
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > int64(time.Hour/time.Second) {
			return time.Hour, true
		}
		delay = time.Duration(seconds) * time.Second
	} else if target, err := http.ParseTime(raw); err == nil {
		delay = target.Sub(now)
		if delay <= 0 {
			return 0, true
		}
	} else {
		return 0, false
	}
	if delay > time.Hour {
		return time.Hour, true
	}
	return delay, true
}

func cloneURLValues(source url.Values) url.Values {
	result := make(url.Values, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func checkedAddInt64(left, right int64) (int64, bool) {
	if right > 0 && left > int64(^uint64(0)>>1)-right {
		return 0, false
	}
	if right < 0 && left < -int64(^uint64(0)>>1)-1-right {
		return 0, false
	}
	return left + right, true
}

type metricTotals struct {
	RequestCount int64
	Quota        int64
	TokenUsed    int64
}
