import Decimal from 'decimal.js'

import { parseDecimalString, type DecimalString } from '@/lib/api-types'

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

export function millisecondsToSeconds(
  value: string | null
): DecimalString | null {
  return value == null
    ? null
    : parseDecimalString(new Decimal(value).dividedBy(1000).toFixed())
}

export function successRateToPercent(value: string | null): string | null {
  return value == null ? null : `${new Decimal(value).times(100).toFixed()}%`
}

export function trustedWeightedSummary(
  statistics: PerformanceHistoryStatisticsResponse
): PerformanceWeightedMetric {
  return statistics.aggregation_status === 'complete'
    ? statistics.summary
    : unavailableSummary
}
