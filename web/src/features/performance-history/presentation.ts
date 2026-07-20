import type {
  PerformanceHistoryStatisticsResponse,
  PerformanceWeightedMetric,
} from './types'

const unavailableSummary: PerformanceWeightedMetric = {
  avg_latency_ms: null,
  avg_tps: null,
  avg_ttft_ms: null,
  request_count: null,
  success_rate: null,
}

export function trustedWeightedSummary(
  statistics: PerformanceHistoryStatisticsResponse
): PerformanceWeightedMetric {
  return statistics.aggregation_status === 'complete'
    ? statistics.summary
    : unavailableSummary
}
