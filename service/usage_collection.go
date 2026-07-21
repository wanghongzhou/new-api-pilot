package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"

	"new-api-pilot/common"
	"new-api-pilot/constant"
	"new-api-pilot/dto"
	"new-api-pilot/model"
)

type UsageCollectionService struct {
	sites      *model.SiteRepository
	clients    SiteClientFactory
	cipher     *common.Cipher
	clock      common.Clock
	postCommit PostCommitNotifier
}

type UsageCollectionServiceOptions struct {
	Repository    *model.SiteRepository
	ClientFactory SiteClientFactory
	Cipher        *common.Cipher
	Clock         common.Clock
	PostCommit    PostCommitNotifier
}

type UsageCollectionRequest struct {
	Run       model.CollectionRun
	Window    model.CollectionRunWindow
	RequestID string
}

type UsageCollectionFailure struct {
	Code   string
	Params []byte
	Cause  error
}

type UsageCollectionResult struct {
	Commit          model.UsageAggregationCommit
	Planned         model.UsageWindowMutationResult
	Failure         *UsageCollectionFailure
	FlowRows        int64
	DataRows        int64
	SourceRequestID string
}

func NewUsageCollectionService(options UsageCollectionServiceOptions) (*UsageCollectionService, error) {
	if options.Repository == nil || options.ClientFactory == nil || options.Cipher == nil || options.Clock == nil {
		return nil, errors.New("usage collection dependencies are required")
	}
	return &UsageCollectionService{
		sites: options.Repository, clients: options.ClientFactory, cipher: options.Cipher, clock: options.Clock,
		postCommit: options.PostCommit,
	}, nil
}

func (service *UsageCollectionService) CollectHour(
	ctx context.Context,
	request UsageCollectionRequest,
) (UsageCollectionResult, error) {
	if service == nil || request.Run.ID <= 0 || request.Run.SiteID == nil || request.Window.ID <= 0 ||
		request.Window.RunID != request.Run.ID || request.Window.SiteID != *request.Run.SiteID ||
		request.Window.HourTS <= 0 || request.Window.HourTS%3600 != 0 || request.Window.HourTS > math.MaxInt64-3600 ||
		!isUsageCollectionTaskType(request.Run.TaskType) || request.Run.StartTimestamp == nil || request.Run.EndTimestamp == nil ||
		request.Window.HourTS < *request.Run.StartTimestamp || request.Window.HourTS >= *request.Run.EndTimestamp ||
		request.Run.Status != model.CollectionTaskStatusRunning || request.Window.Status != model.CollectionTaskStatusRunning ||
		request.Run.LastRequestID != request.RequestID || request.RequestID == "" {
		return UsageCollectionResult{}, model.ErrCollectionRunContract
	}
	now := service.clock.Now().Unix()
	if now <= 0 || request.Window.HourTS+3600 > now {
		return UsageCollectionResult{}, model.ErrCollectionRunContract
	}
	site, client, err := service.authenticatedUsageClient(ctx, *request.Run.SiteID, request.Run.SiteConfigVersion)
	if err != nil {
		return UsageCollectionResult{}, err
	}
	defer client.CloseIdleConnections()
	if *site.StatisticsStartAt <= 0 || *site.StatisticsStartAt%3600 != 0 ||
		*site.StatisticsStartAt >= request.Window.HourTS+3600 {
		return UsageCollectionResult{}, model.ErrSiteRunConfigChanged
	}

	flow, flowErr := client.FlowHour(ctx, request.RequestID, request.Window.HourTS)
	data, dataErr := client.DataHour(ctx, request.RequestID, request.Window.HourTS)
	if authFailure := usageAuthorizationFailure(flowErr, dataErr); authFailure != nil {
		if err := service.expireUsageAuthorization(ctx, site.ID, request.Run.SiteConfigVersion); err != nil {
			return UsageCollectionResult{}, err
		}
		return UsageCollectionResult{}, authFailure
	}
	fetchedRows := int64(len(flow)) + int64(len(data))
	baseFailureRequest := model.FailedUsageWindowRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: site.ID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now, FetchedRows: fetchedRows,
	}
	if flowErr != nil || dataErr != nil {
		cause := firstError(flowErr, dataErr)
		failureCode := usageFailureCode(cause)
		failureParams := usageFailureParams(failureCode, cause, site.ID, request.Window.HourTS)
		failureParams = usageDiagnosticFailureParams(failureParams, cause, "fetch")
		logUsageCollectionFailure(request, "fetch", flowErr, dataErr, cause)
		reasonCode, reasonParams := usageWindowFailureReason(cause, false, site.ID, request.Window.HourTS)
		baseFailureRequest.ReasonCode = reasonCode
		baseFailureRequest.ReasonParams = reasonParams
		mutation, mutationErr := model.NewFailedUsageWindowMutation(baseFailureRequest)
		if mutationErr != nil {
			return UsageCollectionResult{}, mutationErr
		}
		commit, aggregationErr := bindUsageAggregationCommit(request, now, nil, mutation)
		if aggregationErr != nil {
			return UsageCollectionResult{}, aggregationErr
		}
		return UsageCollectionResult{
			Commit:   commit,
			Failure:  &UsageCollectionFailure{Code: failureCode, Params: failureParams, Cause: cause},
			FlowRows: int64(len(flow)), DataRows: int64(len(data)), SourceRequestID: request.RequestID,
		}, nil
	}

	facts, factErr := usageFactsFromFlow(flow)
	consistencyErr := ValidateFlowDataConsistency(flow, data)
	if consistencyErr == nil {
		consistencyErr = validateWholeHourUsageTotals(flow, data)
	}
	if factErr != nil || consistencyErr != nil {
		cause := firstError(factErr, consistencyErr)
		failureCode := usageFailureCode(cause)
		dataMismatch := factErr == nil && errors.Is(consistencyErr, ErrUpstreamDataMismatch)
		if dataMismatch {
			failureCode = string(constant.MessageDataValidationMismatch)
		}
		failureParams := usageFailureParams(failureCode, cause, site.ID, request.Window.HourTS)
		failureParams = usageDiagnosticFailureParams(failureParams, cause, "validate")
		logUsageCollectionFailure(request, "validate", factErr, consistencyErr, cause)
		reasonCode, reasonParams := usageWindowFailureReason(cause, dataMismatch, site.ID, request.Window.HourTS)
		baseFailureRequest.ReasonCode = reasonCode
		baseFailureRequest.ReasonParams = reasonParams
		baseFailureRequest.DataMismatch = dataMismatch
		mutation, mutationErr := model.NewFailedUsageWindowMutation(baseFailureRequest)
		if mutationErr != nil {
			return UsageCollectionResult{}, mutationErr
		}
		aggregationFacts := facts
		if factErr != nil {
			aggregationFacts = nil
		}
		commit, aggregationErr := bindUsageAggregationCommit(request, now, aggregationFacts, mutation)
		if aggregationErr != nil {
			return UsageCollectionResult{}, aggregationErr
		}
		return UsageCollectionResult{
			Commit:   commit,
			Failure:  &UsageCollectionFailure{Code: failureCode, Params: failureParams, Cause: cause},
			FlowRows: int64(len(flow)), DataRows: int64(len(data)), SourceRequestID: request.RequestID,
		}, nil
	}

	mutation, planned, err := model.NewCompleteUsageWindowMutation(model.CompleteUsageWindowRequest{
		RunID: request.Run.ID, WindowID: request.Window.ID, SiteID: site.ID,
		ExpectedConfigVersion: request.Run.SiteConfigVersion, HourTS: request.Window.HourTS,
		AttemptCount: request.Window.AttemptCount, RequestID: request.RequestID, Now: now,
		FetchedRows: fetchedRows, Validation: request.Run.TaskType == constant.TaskTypeUsageValidation, Facts: facts,
	})
	if err != nil {
		return UsageCollectionResult{}, err
	}
	commit, err := bindUsageAggregationCommit(request, now, facts, mutation)
	if err != nil {
		return UsageCollectionResult{}, err
	}
	return UsageCollectionResult{
		Commit:  commit,
		Planned: planned, FlowRows: int64(len(flow)), DataRows: int64(len(data)),
		SourceRequestID: request.RequestID,
	}, nil
}

func (service *UsageCollectionService) expireUsageAuthorization(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
) error {
	if service == nil {
		return model.ErrCollectionRunContract
	}
	return expireSiteAuthorization(
		ctx, service.sites, service.clock, service.postCommit, siteID, expectedConfigVersion,
	)
}

func usageAuthorizationFailure(first, second error) error {
	for _, candidate := range []error{first, second} {
		if upstreamAuthorizationFailure(candidate) {
			return candidate
		}
	}
	return nil
}

func (service *UsageCollectionService) authenticatedUsageClient(
	ctx context.Context,
	siteID int64,
	expectedConfigVersion int,
) (model.Site, SiteUpstreamClient, error) {
	site, err := service.sites.FindByID(ctx, siteID)
	if err != nil {
		return model.Site{}, nil, err
	}
	if site.ConfigVersion != expectedConfigVersion || site.ManagementStatus != constant.SiteManagementActive ||
		site.AuthStatus != constant.SiteAuthAuthorized || site.StatisticsEndAt != nil || !site.DataExportEnabled ||
		site.StatisticsStartAt == nil || site.RootUserID == nil || site.AccessTokenEncrypted == nil {
		return model.Site{}, nil, model.ErrSiteRunConfigChanged
	}
	plaintext, err := service.cipher.Decrypt(*site.AccessTokenEncrypted, siteTokenAAD(site.ID))
	if err != nil {
		if expireErr := service.expireUsageAuthorization(ctx, site.ID, expectedConfigVersion); expireErr != nil {
			return model.Site{}, nil, expireErr
		}
		return model.Site{}, nil, ErrUpstreamAuthExpired
	}
	client, err := service.clients.NewAuthenticated(site.BaseURL, site.BaseURL, string(plaintext), *site.RootUserID)
	if err != nil {
		if upstreamAuthorizationFailure(err) {
			if expireErr := service.expireUsageAuthorization(ctx, site.ID, expectedConfigVersion); expireErr != nil {
				return model.Site{}, nil, expireErr
			}
		}
		return model.Site{}, nil, err
	}
	return site, client, nil
}

func usageFactsFromFlow(flow []dto.UpstreamFlowRow) ([]model.UsageFactInput, error) {
	facts := make([]model.UsageFactInput, len(flow))
	for index, row := range flow {
		if row.UserID <= 0 || row.ChannelID < 0 || row.TokenID < 0 || row.RequestCount < 0 || row.Quota < 0 || row.TokenUsed < 0 ||
			!validUpstreamString(row.Username, 0, 255) || !validUpstreamString(row.ModelName, 0, 255) ||
			!validUpstreamString(row.UseGroup, 0, 128) || !validUpstreamString(row.TokenName, 0, 255) ||
			!validUpstreamString(row.NodeName, 0, 128) {
			return nil, newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		facts[index] = model.UsageFactInput{
			RemoteUserID: row.UserID, UsernameSnapshot: row.Username, ModelName: row.ModelName,
			ChannelID: row.ChannelID, UseGroup: row.UseGroup, TokenID: row.TokenID, TokenName: row.TokenName,
			NodeName: row.NodeName, RequestCount: row.RequestCount, Quota: row.Quota, TokenUsed: row.TokenUsed,
		}
	}
	return facts, nil
}

func validateWholeHourUsageTotals(flow []dto.UpstreamFlowRow, data []dto.UpstreamDataRow) error {
	var flowTotals, dataTotals metricTotals
	for _, row := range flow {
		if !addMetricTotals(&flowTotals, row.RequestCount, row.Quota, row.TokenUsed) {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
	}
	for _, row := range data {
		if !addMetricTotals(&dataTotals, row.RequestCount, row.Quota, row.TokenUsed) {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
	}
	if flowTotals != dataTotals {
		return newUpstreamRequestError(UpstreamErrorDataMismatch)
	}
	return nil
}

func addMetricTotals(total *metricTotals, requestCount, quota, tokenUsed int64) bool {
	var ok bool
	if total.RequestCount, ok = checkedAddInt64(total.RequestCount, requestCount); !ok {
		return false
	}
	if total.Quota, ok = checkedAddInt64(total.Quota, quota); !ok {
		return false
	}
	if total.TokenUsed, ok = checkedAddInt64(total.TokenUsed, tokenUsed); !ok {
		return false
	}
	return true
}

func usageFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrUpstreamResponseInvalid), errors.Is(err, ErrUpstreamEnvelopeInvalid):
		return string(constant.MessageUpstreamResponseInvalid)
	case errors.Is(err, ErrUpstreamResponseTooLarge):
		return string(constant.MessageUpstreamResponseTooLarge)
	case errors.Is(err, ErrUpstreamRemote):
		return constant.CodeUpstreamError
	default:
		return constant.CodeUpstreamUnavailable
	}
}

func isUsageCollectionTaskType(taskType string) bool {
	return taskType == constant.TaskTypeUsageHour || taskType == constant.TaskTypeUsageBackfill ||
		taskType == constant.TaskTypeUsageValidation
}

func usageWindowFailureReason(err error, dataMismatch bool, siteID, hourTS int64) (string, []byte) {
	code := string(constant.MessageDataUpstreamUnavailable)
	switch {
	case dataMismatch:
		code = string(constant.MessageDataValidationMismatch)
	case errors.Is(err, ErrUpstreamResponseInvalid), errors.Is(err, ErrUpstreamEnvelopeInvalid):
		code = string(constant.MessageUpstreamResponseInvalid)
	case errors.Is(err, ErrUpstreamResponseTooLarge):
		code = string(constant.MessageUpstreamResponseTooLarge)
	}
	return code, usageFailureParams(code, err, siteID, hourTS)
}

func usageFailureParams(code string, err error, siteID, hourTS int64) []byte {
	params := map[string]any{"site_id": fmt.Sprintf("%d", siteID)}
	switch code {
	case string(constant.MessageUpstreamResponseInvalid):
	case string(constant.MessageUpstreamResponseTooLarge):
		responseBytes, limitBytes := int64(UpstreamMaxResponseBytes+1), UpstreamMaxResponseBytes
		var requestError *UpstreamRequestError
		if errors.As(err, &requestError) && requestError.ResponseBytes > 0 && requestError.LimitBytes > 0 {
			responseBytes = requestError.ResponseBytes
			limitBytes = requestError.LimitBytes
		}
		params["response_bytes"] = fmt.Sprintf("%d", responseBytes)
		params["limit_bytes"] = fmt.Sprintf("%d", limitBytes)
	default:
		params["start_timestamp"] = hourTS
		params["end_timestamp"] = hourTS + 3600
	}
	encoded, marshalErr := common.Marshal(params)
	if marshalErr != nil {
		return nil
	}
	return encoded
}

func usageDiagnosticFailureParams(base []byte, err error, phase string) []byte {
	params := map[string]any{}
	if json.Unmarshal(base, &params) != nil {
		return base
	}
	params["failure_phase"] = phase
	var requestError *UpstreamRequestError
	if errors.As(err, &requestError) {
		params["upstream_error_kind"] = string(requestError.Kind)
		if requestError.Detail != "" {
			params["upstream_error_detail"] = requestError.Detail
		}
		if requestError.Method != "" {
			params["method"] = requestError.Method
		}
		if requestError.Endpoint != "" {
			params["endpoint"] = requestError.Endpoint
		}
		if requestError.StatusCode > 0 {
			params["status_code"] = requestError.StatusCode
		}
		if requestError.ContentType != "" {
			params["content_type"] = requestError.ContentType
		}
		if requestError.PayloadBytes > 0 {
			params["payload_bytes"] = requestError.PayloadBytes
		}
	}
	encoded, marshalErr := common.Marshal(params)
	if marshalErr != nil {
		return base
	}
	return encoded
}

func logUsageCollectionFailure(request UsageCollectionRequest, phase string, first, second, cause error) {
	log.Printf("usage collection failed run_id=%d window_id=%d site_id=%d hour_ts=%d attempt=%d phase=%s first_error_type=%T second_error_type=%T cause_type=%T", request.Run.ID, request.Window.ID, request.Window.SiteID, request.Window.HourTS, request.Window.AttemptCount, phase, first, second, cause)
	var requestError *UpstreamRequestError
	if errors.As(cause, &requestError) {
		log.Printf("usage collection upstream detail run_id=%d window_id=%d request_id=%s method=%s endpoint=%s status=%d content_type=%q payload_bytes=%d kind=%s detail=%s", request.Run.ID, request.Window.ID, request.RequestID, requestError.Method, requestError.Endpoint, requestError.StatusCode, requestError.ContentType, requestError.PayloadBytes, requestError.Kind, requestError.Detail)
	}
}
