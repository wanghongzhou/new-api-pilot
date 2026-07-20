import { describe, expect, test } from 'bun:test'

import { parseDecimalString, parseMetricString } from '@/lib/api-types'

import { trustedWeightedSummary } from './presentation'
import type { PerformanceHistoryStatisticsResponse } from './types'

function statistics(
  aggregation_status: 'complete' | 'unavailable'
): PerformanceHistoryStatisticsResponse {
  return {
    aggregation_status,
    data_status: 'complete',
    site_breakdown: [],
    summary: {
      avg_latency_ms: parseDecimalString('123.4560000000'),
      avg_tps: parseDecimalString('9.5000000000'),
      avg_ttft_ms: parseDecimalString('45.6000000000'),
      request_count: parseMetricString('9007199254740993'),
      success_rate: parseDecimalString('0.9900000000'),
    },
    trend: [],
  }
}

describe('performance history aggregation boundary', () => {
  test('uses the backend weighted summary only when counters are complete', () => {
    expect(trustedWeightedSummary(statistics('complete'))).toEqual(
      statistics('complete').summary
    )
  })

  test('never exposes averages as a cross-site summary when counters are missing', () => {
    expect(trustedWeightedSummary(statistics('unavailable'))).toEqual({
      avg_latency_ms: null,
      avg_tps: null,
      avg_ttft_ms: null,
      request_count: null,
      success_rate: null,
    })
  })
})
