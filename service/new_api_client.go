package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	upstreamPageSize              = 100
)

type NewAPIClientOptions struct {
	BaseURL             string
	CredentialOrigin    string
	AccessToken         string
	RootUserID          int64
	AllowedHostSuffixes []string
	AllowedCIDRs        []netip.Prefix
	CAFile              string
	ConnectTimeout      time.Duration
	HeaderTimeout       time.Duration
	RequestTimeout      time.Duration
	ExportTimeout       time.Duration
	Metrics             UpstreamMetricsRecorder
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
	if !dependencies.allowNonDesignTimeouts && (connectTimeout != UpstreamConnectTimeout ||
		headerTimeout != UpstreamResponseHeaderTimeout || requestTimeout != UpstreamRequestTimeout ||
		exportTimeout != UpstreamExportTimeout) {
		return nil, errors.New("upstream timeouts must match the fixed design values")
	}
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
	return dto.UpstreamUserSnapshot{Total: first.Total, Items: items}, nil
}

func appendUniqueUsers(destination *[]dto.UpstreamUser, seen map[int64]struct{}, items []dto.UpstreamUser, expected int64) error {
	for _, item := range items {
		if _, duplicate := seen[item.ID]; duplicate {
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
	return dto.UpstreamChannelSnapshot{Total: first.Total, Items: items}, nil
}

func appendUniqueChannels(destination *[]dto.UpstreamChannel, seen map[int64]struct{}, items []dto.UpstreamChannel, expected int64) error {
	for _, item := range items {
		if _, duplicate := seen[item.ID]; duplicate {
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
	enrichment := make(map[string]dto.UpstreamPricingGroup, len(pricing.Groups))
	for _, group := range pricing.Groups {
		enrichment[group.Name] = group
	}
	for index := range groups.Groups {
		if enriched, exists := enrichment[groups.Groups[index].Name]; exists {
			groups.Groups[index] = enriched
		}
	}
	return dto.UpstreamPricingSnapshot{PricingVersion: pricing.PricingVersion, Items: pricing.Items, Groups: groups.Groups}, nil
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
	return validateAndAggregateFlowRows(wire)
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
	return validateAndAggregateDataRows(wire, hourStart)
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
	var wire upstreamLogStatWire
	if _, err := client.get(ctx, client.httpClient, "/api/log/stat", nil, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamLogStat{}, err
	}
	return validateLogStat(wire)
}

func (client *NewAPIClient) PerformanceSummary(ctx context.Context, requestID string, hours int) (dto.UpstreamPerformanceSummary, error) {
	if hours < 1 || hours > 24*30 {
		return dto.UpstreamPerformanceSummary{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	var wire dto.UpstreamPerformanceSummary
	if _, err := client.get(ctx, client.httpClient, "/api/perf-metrics/summary", url.Values{"hours": []string{strconv.Itoa(hours)}}, requestID, upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
		return dto.UpstreamPerformanceSummary{}, err
	}
	if err := validatePerformanceSummary(wire); err != nil {
		return dto.UpstreamPerformanceSummary{}, err
	}
	return wire, nil
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
	models := make([]dto.UpstreamPerformanceModelHistory, 0, len(summary.Models))
	counterReady := true
	seen := map[string]struct{}{}
	for index, item := range summary.Models {
		if _, ok := seen[item.ModelName]; ok {
			return dto.UpstreamPerformanceHistory{}, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		seen[item.ModelName] = struct{}{}
		var wire upstreamPerformanceHistoryWire
		if _, err := client.get(ctx, client.httpClient, "/api/perf-metrics", url.Values{"model": []string{item.ModelName}, "hours": []string{strconv.Itoa(hours)}}, fmt.Sprintf("%s_model_%d", requestID, index+1), upstreamAuthManagement, client.requestTimeout, &wire, false); err != nil {
			return dto.UpstreamPerformanceHistory{}, err
		}
		model, ready, err := validatePerformanceHistory(wire, item.ModelName, client.now().Unix())
		if err != nil {
			return dto.UpstreamPerformanceHistory{}, err
		}
		counterReady = counterReady && ready
		models = append(models, model)
	}
	return dto.UpstreamPerformanceHistory{Models: models, CounterReady: counterReady && len(models) > 0}, nil
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
	var loginWire upstreamIdentityWire
	if _, err := client.do(ctx, sessionClient, http.MethodPost, "/api/user/login", nil, requestID, upstreamAuthPublic, client.requestTimeout, bytes.NewReader(body), "application/json", &loginWire, false); err != nil {
		return dto.UpstreamIdentity{}, "", err
	}
	identity, err := validateUpstreamIdentity(loginWire)
	if err != nil || identity.Role != 100 || identity.Status != 1 {
		return dto.UpstreamIdentity{}, "", newUpstreamRequestError(UpstreamErrorResponseInvalid)
	}
	var token string
	if _, err := client.get(ctx, sessionClient, "/api/user/token", nil, requestID, upstreamAuthSession, client.requestTimeout, &token, true, identity.ID); err != nil {
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
	sessionUserID ...int64,
) (int64, error) {
	var sessionID int64
	if len(sessionUserID) > 0 {
		sessionID = sessionUserID[0]
	}
	return client.do(ctx, httpClient, http.MethodGet, endpoint, query, requestID, authMode, timeout, nil, "", destination, tokenMutation, sessionID)
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
	sessionUserID ...int64,
) (payloadSize int64, resultErr error) {
	startedAt := time.Now()
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
		if len(sessionUserID) != 1 || sessionUserID[0] <= 0 {
			return 0, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		request.Header.Set("New-Api-User", strconv.FormatInt(sessionUserID[0], 10))
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if tokenMutation && requestSent.Load() {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		if errors.Is(err, ErrUpstreamAddressForbidden) {
			return 0, newUpstreamRequestError(UpstreamErrorAddressForbidden)
		}
		return 0, newUpstreamRequestError(UpstreamErrorUnavailable)
	}
	defer response.Body.Close()
	if tokenMutation && response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		drainUpstreamResponse(response.Body)
		return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
	}
	retryAfter, hasRetryAfter := parseRetryAfter(response.Header.Get("Retry-After"), client.now())
	if statusErr := classifyUpstreamHTTPStatus(response.StatusCode, retryAfter, hasRetryAfter); statusErr != nil {
		drainUpstreamResponse(response.Body)
		return 0, statusErr
	}
	if response.ContentLength > client.maxResponseBytes {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, newUpstreamResponseTooLargeError(response.ContentLength, client.maxResponseBytes)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
	if err != nil {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, newUpstreamRequestError(UpstreamErrorUnavailable)
	}
	if int64(len(payload)) > client.maxResponseBytes {
		if tokenMutation {
			return 0, newUpstreamRequestError(UpstreamErrorTokenRotationResultUnknown)
		}
		return 0, newUpstreamResponseTooLargeError(int64(len(payload)), client.maxResponseBytes)
	}
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
		return 0, decodeErr
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
