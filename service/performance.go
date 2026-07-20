package service

import (
	"fmt"
	"math"
	"strconv"

	"new-api-pilot/dto"
)

const sitePerformanceDataReady = "ready"

func validatePerformanceSummary(summary dto.UpstreamPerformanceSummary) error {
	var totalRequests int64
	seenModels := make(map[string]struct{}, len(summary.Models))
	for _, model := range summary.Models {
		if !validUpstreamString(model.ModelName, 1, 255) || model.RequestCount < 0 ||
			!validPerformanceNumber(model.SuccessRate) || !validPerformanceNumber(model.AvgLatencyMS) ||
			!validPerformanceNumber(model.AvgTPS) {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		if _, exists := seenModels[model.ModelName]; exists || model.RequestCount > (1<<63-1)-totalRequests {
			return newUpstreamRequestError(UpstreamErrorResponseInvalid)
		}
		seenModels[model.ModelName] = struct{}{}
		totalRequests += model.RequestCount
	}
	return nil
}

func validPerformanceNumber(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func sitePerformanceSummary(hours int, sampledAt int64, upstream dto.UpstreamPerformanceSummary) dto.SitePerformanceSummary {
	result := dto.SitePerformanceSummary{
		Hours: hours, SampledAt: &sampledAt, DataStatus: sitePerformanceDataReady,
		RequestCount: "0", Models: make([]dto.SitePerformanceModel, 0, len(upstream.Models)),
	}
	var totalRequests int64
	var weightedSuccessRate, weightedLatency, weightedTPS float64
	for _, model := range upstream.Models {
		result.Models = append(result.Models, dto.SitePerformanceModel{
			ModelName: model.ModelName, RequestCount: strconv.FormatInt(model.RequestCount, 10),
			SuccessRate: model.SuccessRate, AvgLatencyMS: model.AvgLatencyMS, AvgTPS: model.AvgTPS,
		})
		totalRequests += model.RequestCount
		weight := float64(model.RequestCount)
		weightedSuccessRate += model.SuccessRate * weight
		weightedLatency += model.AvgLatencyMS * weight
		weightedTPS += model.AvgTPS * weight
	}
	result.RequestCount = strconv.FormatInt(totalRequests, 10)
	if totalRequests > 0 {
		weight := float64(totalRequests)
		result.SuccessRate = weightedSuccessRate / weight
		result.AvgLatencyMS = weightedLatency / weight
		result.AvgTPS = weightedTPS / weight
	}
	return result
}

func unavailableSitePerformanceSummary(hours int) dto.SitePerformanceSummary {
	return dto.SitePerformanceSummary{
		Hours: hours, DataStatus: "unavailable", RequestCount: "0", Models: []dto.SitePerformanceModel{},
	}
}

func performanceCacheRequestID(siteID int64) string {
	return fmt.Sprintf("site-performance-cache-%d", siteID)
}
