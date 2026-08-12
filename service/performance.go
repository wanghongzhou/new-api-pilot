package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"new-api-pilot/dto"
)

const sitePerformanceDataReady = "ready"

type upstreamPerformanceSummaryResponse struct {
	Summary dto.UpstreamPerformanceSummary
}

func (response *upstreamPerformanceSummaryResponse) decodeUpstreamResponse(payload []byte) error {
	var envelope struct {
		Success *bool           `json:"success"`
		Message *string         `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := validateStrictJSONFor(payload, envelope); err != nil {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorResponseInvalid, "invalid_envelope_json")
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorEnvelopeInvalid, "invalid_envelope_json")
	}
	if envelope.Success == nil {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorEnvelopeInvalid, "missing_success")
	}
	if !*envelope.Success {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorEnvelopeInvalid, "success_false")
	}
	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorEnvelopeInvalid, "missing_data")
	}
	if err := validateStrictJSONFor(envelope.Data, dto.UpstreamPerformanceSummary{}); err != nil {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorResponseInvalid, "invalid_data_json")
	}
	if err := json.Unmarshal(envelope.Data, &response.Summary); err != nil {
		return newUpstreamRequestErrorWithDetail(UpstreamErrorResponseInvalid, "invalid_data_schema")
	}
	return nil
}

func validatePerformanceSummary(summary dto.UpstreamPerformanceSummary) error {
	seenModels := make(map[string]struct{}, len(summary.Models))
	for _, model := range summary.Models {
		if !validUpstreamString(model.ModelName, 1, 255) || model.SuccessRate > 100 ||
			!validPerformanceNumber(model.SuccessRate) || !validPerformanceNumber(model.AvgLatencyMS) ||
			!validPerformanceNumber(model.AvgTPS) {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if _, exists := seenModels[model.ModelName]; exists {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		seenModels[model.ModelName] = struct{}{}
	}
	return nil
}

func validPerformanceNumber(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func sitePerformanceSummary(hours int, sampledAt int64, upstream dto.UpstreamPerformanceSummary) dto.SitePerformanceSummary {
	result := dto.SitePerformanceSummary{
		Hours: hours, SampledAt: &sampledAt, DataStatus: sitePerformanceDataReady,
		Models: make([]dto.SitePerformanceModel, 0, len(upstream.Models)),
	}
	for _, model := range upstream.Models {
		result.Models = append(result.Models, dto.SitePerformanceModel{
			ModelName: model.ModelName, SuccessRate: model.SuccessRate,
			AvgLatencyMS: model.AvgLatencyMS, AvgTPS: model.AvgTPS,
		})
	}
	return result
}

func unavailableSitePerformanceSummary(hours int) dto.SitePerformanceSummary {
	return dto.SitePerformanceSummary{
		Hours: hours, DataStatus: "unavailable", Models: []dto.SitePerformanceModel{},
	}
}

func performanceCacheRequestID(siteID int64) string {
	return fmt.Sprintf("site-performance-cache-%d", siteID)
}
