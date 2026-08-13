package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

const maximumLogWindowRows = 10000
const maximumLogBackfillWindowsPerRun = 96

type upstreamLogClient interface {
	LogPage(context.Context, string, int64, int64, int) (dto.UpstreamLogPage, error)
}

type UpstreamLogService struct {
	repository *model.UpstreamLogRepository
	sites      *model.SiteRepository
	clients    SiteClientFactory
	cipher     *common.Cipher
	clock      common.Clock
}

type UpstreamLogServiceOptions struct {
	Database       *gorm.DB
	SiteRepository *model.SiteRepository
	ClientFactory  SiteClientFactory
	Cipher         *common.Cipher
	Clock          common.Clock
}

func NewUpstreamLogService(options UpstreamLogServiceOptions) (*UpstreamLogService, error) {
	if options.Database == nil || options.SiteRepository == nil || options.ClientFactory == nil || options.Cipher == nil || options.Clock == nil {
		return nil, errors.New("upstream log service dependencies are required")
	}
	return &UpstreamLogService{repository: model.NewUpstreamLogRepository(options.Database), sites: options.SiteRepository,
		clients: options.ClientFactory, cipher: options.Cipher, clock: options.Clock}, nil
}

func (service *UpstreamLogService) CollectWindow(ctx context.Context, siteID int64, configVersion int, start, end int64, requestID string) error {
	_, _, err := service.collectWindow(ctx, siteID, configVersion, start, end, requestID)
	return err
}

func (service *UpstreamLogService) ExecuteScheduledLogTask(ctx context.Context, siteID int64, configVersion int, requestID string) (int64, int64, error) {
	if service == nil || service.clock == nil {
		return 0, 0, model.ErrCollectionRunContract
	}
	end := service.clock.Now().Unix()
	end -= end % 3600
	if end <= 2*3600 {
		return 0, 0, model.ErrCollectionRunContract
	}
	recentStart := end - 2*3600
	fetched, written, err := service.recoverScheduledGap(ctx, siteID, configVersion, recentStart)
	if err != nil {
		return fetched, written, err
	}
	recentFetched, recentWritten, err := service.collectWindow(ctx, siteID, configVersion, recentStart, end, requestID)
	fetched += recentFetched
	written += recentWritten
	if err != nil {
		return fetched, written, err
	}
	backfillFetched, backfillWritten, err := service.backfillHistory(ctx, siteID, configVersion, end)
	return fetched + backfillFetched, written + backfillWritten, err
}

func (service *UpstreamLogService) recoverScheduledGap(ctx context.Context, siteID int64, configVersion int, recentStart int64) (int64, int64, error) {
	state, err := service.repository.LoadState(ctx, siteID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if state.ConfigVersion != configVersion || state.WindowEnd <= 0 || state.WindowEnd >= recentStart {
		return 0, 0, nil
	}
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		return 0, 0, err
	}
	if site.ConfigVersion != configVersion {
		return 0, 0, model.ErrSiteRunConfigChanged
	}
	retentionDays, err := service.repository.LoadRetentionDays(ctx)
	if err != nil {
		return 0, 0, err
	}
	targetStart := recentStart - int64(retentionDays)*86400
	if site.StatisticsStartAt != nil && *site.StatisticsStartAt > targetStart {
		targetStart = *site.StatisticsStartAt
	}
	targetStart -= targetStart % 3600
	cursor := state.WindowEnd
	if cursor < targetStart {
		cursor = targetStart
	}
	var fetched, written int64
	for window := 0; cursor < recentStart && window < maximumLogBackfillWindowsPerRun; window++ {
		windowEnd := cursor + 24*3600
		if windowEnd > recentStart {
			windowEnd = recentStart
		}
		windowFetched, windowWritten, collectErr := service.collectWindow(
			ctx, siteID, configVersion, cursor, windowEnd,
			fmt.Sprintf("loggap-%d-%d-%d", siteID, recentStart, window+1),
		)
		fetched += windowFetched
		written += windowWritten
		if collectErr != nil {
			return fetched, written, collectErr
		}
		cursor = windowEnd
	}
	if cursor < recentStart {
		return fetched, written, ErrUpstreamResponseTooLarge
	}
	return fetched, written, nil
}

func (service *UpstreamLogService) backfillHistory(ctx context.Context, siteID int64, configVersion int, currentHour int64) (int64, int64, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		return 0, 0, err
	}
	if site.ConfigVersion != configVersion {
		return 0, 0, model.ErrSiteRunConfigChanged
	}
	state, err := service.repository.LoadState(ctx, siteID)
	if err != nil {
		return 0, 0, err
	}
	if state.BackfillCompletedAt != nil {
		return 0, 0, nil
	}
	retentionDays, err := service.repository.LoadRetentionDays(ctx)
	if err != nil {
		return 0, 0, err
	}
	targetStart := currentHour - int64(retentionDays)*86400
	if site.StatisticsStartAt != nil && *site.StatisticsStartAt > targetStart {
		targetStart = *site.StatisticsStartAt
	}
	targetStart -= targetStart % 3600
	cursor := currentHour - 2*3600
	if state.HistoryStartAt != nil && *state.HistoryStartAt < cursor {
		cursor = *state.HistoryStartAt
	} else if state.HistoryStartAt == nil && site.MonitoringStartAt != nil {
		cursor = *site.MonitoringStartAt - *site.MonitoringStartAt%3600
	}
	var fetched, written int64
	for window := 0; cursor > targetStart && window < maximumLogBackfillWindowsPerRun; window++ {
		start := cursor - 24*3600
		if start < targetStart {
			start = targetStart
		}
		windowFetched, windowWritten, collectErr := service.collectWindow(
			ctx, siteID, configVersion, start, cursor, fmt.Sprintf("logbf-%d-%d-%d", siteID, currentHour, window+1),
		)
		fetched += windowFetched
		written += windowWritten
		if collectErr != nil {
			return fetched, written, collectErr
		}
		cursor = start
	}
	if cursor <= targetStart {
		if err := service.repository.MarkBackfillCompleted(ctx, siteID, configVersion, targetStart, service.clock.Now().Unix()); err != nil {
			return fetched, written, err
		}
	}
	return fetched, written, nil
}

func (service *UpstreamLogService) collectWindow(ctx context.Context, siteID int64, configVersion int, start, end int64, requestID string) (fetched int64, written int64, resultErr error) {
	if service == nil || siteID <= 0 || configVersion <= 0 || start <= 0 || end <= start || end-start > 24*3600 || requestID == "" {
		return 0, 0, model.ErrCollectionRunContract
	}
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		return 0, 0, err
	}
	if site.ConfigVersion != configVersion || site.ManagementStatus != constant.SiteManagementActive || site.AuthStatus != constant.SiteAuthAuthorized || site.RootUserID == nil || site.AccessTokenEncrypted == nil {
		return 0, 0, model.ErrSiteRunConfigChanged
	}
	defer func() {
		if resultErr == nil || errors.Is(resultErr, model.ErrUpstreamLogFence) || errors.Is(resultErr, model.ErrSiteRunConfigChanged) {
			return
		}
		commitErr := service.repository.CommitWindow(ctx, siteID, configVersion, start, end, service.clock.Now().Unix(), nil,
			dto.LogCollectionUnavailable, upstreamLogFailureCode(resultErr), nil)
		if commitErr != nil && !errors.Is(commitErr, model.ErrUpstreamLogFence) {
			resultErr = errors.Join(resultErr, commitErr)
		}
	}()
	plaintext, err := service.cipher.Decrypt(*site.AccessTokenEncrypted, siteTokenAAD(site.ID))
	if err != nil {
		return 0, 0, ErrUpstreamAuthExpired
	}
	baseClient, err := service.clients.NewAuthenticated(site.BaseURL, site.BaseURL, string(plaintext), *site.RootUserID)
	if err != nil {
		return 0, 0, err
	}
	defer baseClient.CloseIdleConnections()
	client, ok := baseClient.(upstreamLogClient)
	if !ok {
		return 0, 0, errors.New("upstream log client is not supported")
	}

	page := 1
	var total int64 = -1
	seen := map[string]model.UpstreamLogFact{}
	for {
		result, pageErr := client.LogPage(ctx, requestID, start, end-1, page)
		if pageErr != nil {
			return fetched, int64(len(seen)), pageErr
		}
		if total < 0 {
			total = result.Total
		} else if total != result.Total {
			return fetched, int64(len(seen)), ErrUpstreamResponseInvalid
		}
		if total > maximumLogWindowRows {
			return fetched, int64(len(seen)), ErrUpstreamResponseTooLarge
		}
		fetched += int64(len(result.Items))
		for _, row := range result.Items {
			if row.CreatedAt < start || row.CreatedAt >= end {
				return fetched, int64(len(seen)), ErrUpstreamResponseInvalid
			}
			fact, key, factErr := canonicalUpstreamLogFact(row)
			if factErr != nil {
				return fetched, int64(len(seen)), factErr
			}
			seen[key] = fact
		}
		if int64(len(seen)) > total {
			return fetched, int64(len(seen)), ErrUpstreamResponseInvalid
		}
		if int64(len(seen)) == total {
			break
		}
		if len(result.Items) == 0 || page >= maximumLogWindowRows {
			return fetched, int64(len(seen)), ErrUpstreamResponseInvalid
		}
		page++
	}
	facts := make([]model.UpstreamLogFact, 0, len(seen))
	for _, fact := range seen {
		facts = append(facts, fact)
	}
	err = service.repository.CommitWindow(ctx, siteID, configVersion, start, end, service.clock.Now().Unix(), facts,
		dto.LogCollectionComplete, "", nil)
	return fetched, int64(len(facts)), err
}

func upstreamLogFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrUpstreamResponseInvalid), errors.Is(err, ErrUpstreamEnvelopeInvalid):
		return string(constant.MessageUpstreamResponseInvalid)
	case errors.Is(err, ErrUpstreamResponseTooLarge):
		return string(constant.MessageUpstreamResponseTooLarge)
	default:
		return constant.CodeUpstreamUnavailable
	}
}

func canonicalUpstreamLogFact(row dto.UpstreamLogRow) (model.UpstreamLogFact, string, error) {
	content := redactUpstreamLogContent(row.Content)
	canonical := struct {
		CreatedAt         int64  `json:"created_at"`
		Type              int    `json:"type"`
		UserID            int64  `json:"user_id"`
		Username          string `json:"username"`
		ModelName         string `json:"model_name"`
		TokenID           int64  `json:"token_id"`
		TokenName         string `json:"token_name"`
		ChannelID         int64  `json:"channel_id"`
		Group             string `json:"group"`
		RequestID         string `json:"request_id"`
		UpstreamRequestID string `json:"upstream_request_id"`
		Quota             int64  `json:"quota"`
		PromptTokens      int64  `json:"prompt_tokens"`
		CompletionTokens  int64  `json:"completion_tokens"`
		UseTime           int64  `json:"use_time"`
		Stream            bool   `json:"stream"`
		Content           string `json:"content"`
	}{row.CreatedAt, row.Type, row.UserID, row.Username, row.ModelName, row.TokenID, row.TokenName, row.ChannelID, row.UseGroup,
		row.RequestID, row.UpstreamRequestID, row.Quota, row.PromptTokens, row.CompletionTokens, row.UseTimeSeconds, row.IsStream, row.Content}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return model.UpstreamLogFact{}, "", ErrUpstreamResponseInvalid
	}
	digest := sha256.Sum256(payload)
	key := hex.EncodeToString(digest[:])
	return model.UpstreamLogFact{UpstreamLogKey: key, UpstreamLogID: row.ID, CreatedAt: row.CreatedAt, Type: row.Type,
		RemoteUserID: row.UserID, Username: row.Username, ModelName: row.ModelName, TokenID: row.TokenID, TokenName: row.TokenName,
		ChannelID: row.ChannelID, UseGroup: row.UseGroup, RequestID: row.RequestID, UpstreamRequestID: row.UpstreamRequestID,
		Quota: row.Quota, PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, UseTimeSeconds: row.UseTimeSeconds,
		CacheReadTokens: row.CacheReadTokens, CacheCreationTokens: row.CacheCreationTokens, CacheCreation5m: row.CacheCreation5m, CacheCreation1h: row.CacheCreation1h,
		IsStream: row.IsStream, FirstResponseTimeMs: row.FirstResponseTimeMs, StreamStatus: row.StreamStatus,
		StreamEndReason: row.StreamEndReason, StreamErrorCount: row.StreamErrorCount, ContentRedacted: content, IP: ""}, key, nil
}

func redactUpstreamLogContent(value string) string {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"password", "passwd", "authorization", "bearer ", "access_token", "api_key", "apikey", "webhook", "secret", "cookie", "private_key"} {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	if utf8.RuneCountInString(trimmed) <= 1024 {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:1024])
}

func (service *UpstreamLogService) Query(ctx context.Context, query dto.LogQuery) (dto.LogResponse, error) {
	query.Normalize()
	if service == nil || query.Validate() != nil {
		return dto.LogResponse{}, ErrStatisticsInvalid
	}
	rows, total, err := service.repository.Query(ctx, query)
	if err != nil {
		return dto.LogResponse{}, err
	}
	fallbackQuotaPerUnit, fallbackExchangeRate, err := service.repository.LoadFallbackRate(ctx)
	if err != nil {
		return dto.LogResponse{}, err
	}
	items := make([]dto.LogItem, 0, len(rows))
	for _, row := range rows {
		rate := resolveLogRate(row.QuotaPerUnit, row.USDExchangeRate, row.LastRateAt, fallbackQuotaPerUnit, fallbackExchangeRate)
		items = append(items, dto.LogItem{ID: dto.Int64String(row.ID), SiteID: dto.Int64String(row.SiteID), SiteName: row.SiteName,
			CreatedAt: row.CreatedAt, Type: row.Type, RemoteUserID: dto.Int64String(row.RemoteUserID), Username: row.Username,
			ModelName: row.ModelName, TokenID: dto.Int64String(row.TokenID), TokenName: row.TokenName, ChannelID: dto.Int64String(row.ChannelID),
			UseGroup: row.UseGroup, RequestID: row.RequestID, UpstreamRequestID: row.UpstreamRequestID, Quota: dto.Int64String(row.Quota),
			PromptTokens: dto.Int64String(row.PromptTokens), CompletionTokens: dto.Int64String(row.CompletionTokens),
			CacheReadTokens: dto.Int64String(row.CacheReadTokens), CacheCreationTokens: dto.Int64String(row.CacheCreationTokens),
			CacheCreation5m: dto.Int64String(row.CacheCreation5m), CacheCreation1h: dto.Int64String(row.CacheCreation1h),
			UseTimeSeconds: dto.Int64String(row.UseTimeSeconds), IsStream: row.IsStream,
			FirstResponseTimeMs: dto.OptionalInt64String(row.FirstResponseTimeMs), StreamStatus: row.StreamStatus,
			StreamEndReason: row.StreamEndReason, StreamErrorCount: dto.Int64String(row.StreamErrorCount),
			Content: row.ContentRedacted, IP: "", Rate: rate})
	}
	states, err := service.repository.LoadStates(ctx, query.SiteIDs)
	if err != nil {
		return dto.LogResponse{}, err
	}
	status, asOf := summarizeLogStates(states)
	return dto.LogResponse{Items: items, Total: dto.Int64String(total), Page: query.Page, PageSize: query.PageSize, DataStatus: status, AsOf: asOf}, nil
}

func (service *UpstreamLogService) Stats(ctx context.Context, query dto.LogQuery) (dto.LogStatResponse, error) {
	query.Normalize()
	if service == nil || query.Validate() != nil {
		return dto.LogStatResponse{}, ErrStatisticsInvalid
	}
	rows, rpm, tpm, err := service.repository.Stats(ctx, query, service.clock.Now().Unix())
	if err != nil {
		return dto.LogStatResponse{}, err
	}
	fallbackQuotaPerUnit, fallbackExchangeRate, err := service.repository.LoadFallbackRate(ctx)
	if err != nil {
		return dto.LogStatResponse{}, err
	}
	breakdown := make([]dto.LogStatSiteBreakdown, 0, len(rows))
	var quota int64
	for _, row := range rows {
		quota += row.Quota
		breakdown = append(breakdown, dto.LogStatSiteBreakdown{
			SiteID: dto.Int64String(row.SiteID), SiteName: row.SiteName, Quota: dto.Int64String(row.Quota),
			Rate: resolveLogRate(row.QuotaPerUnit, row.USDExchangeRate, row.LastRateAt, fallbackQuotaPerUnit, fallbackExchangeRate),
		})
	}
	return dto.LogStatResponse{Quota: dto.Int64String(quota), RPM: dto.Int64String(rpm), TPM: dto.Int64String(tpm), SiteBreakdown: breakdown}, nil
}

func resolveLogRate(quotaPerUnit, exchangeRate *string, lastRateAt *int64, fallbackQuotaPerUnit, fallbackExchangeRate string) dto.RateInfo {
	if lastRateAt != nil && statisticsPositiveDecimal(quotaPerUnit) && statisticsPositiveDecimal(exchangeRate) {
		return dto.RateInfo{QuotaPerUnit: quotaPerUnit, USDExchangeRate: exchangeRate, Source: "site", UpdatedAt: lastRateAt}
	}
	if lastRateAt == nil && statisticsPositiveDecimalString(fallbackQuotaPerUnit) && statisticsPositiveDecimalString(fallbackExchangeRate) {
		return dto.RateInfo{QuotaPerUnit: &fallbackQuotaPerUnit, USDExchangeRate: &fallbackExchangeRate, Source: "fallback"}
	}
	return dto.RateInfo{Source: "unavailable"}
}

func summarizeLogStates(states []model.UpstreamLogCollectionState) (string, *int64) {
	if len(states) == 0 {
		return dto.LogCollectionPending, nil
	}
	status := dto.LogCollectionComplete
	var asOf *int64
	for _, state := range states {
		if state.LastSuccessAt != nil && (asOf == nil || *state.LastSuccessAt < *asOf) {
			value := *state.LastSuccessAt
			asOf = &value
		}
		if state.Status == dto.LogCollectionUnavailable {
			status = dto.LogCollectionUnavailable
		} else if state.Status == dto.LogCollectionPartial && status != dto.LogCollectionUnavailable {
			status = dto.LogCollectionPartial
		} else if state.Status == dto.LogCollectionPending && status != dto.LogCollectionUnavailable && status != dto.LogCollectionPartial {
			status = dto.LogCollectionPending
		} else if state.Status == dto.LogCollectionDisabled && status == dto.LogCollectionComplete {
			status = dto.LogCollectionDisabled
		}
	}
	return status, asOf
}
