import { describe, expect, test } from 'bun:test'

import {
  buildPerformanceTrendValues,
  hasRenderablePerformanceTrendValues,
} from './performance-trend-chart-data'
import type { PerformanceHistoryItem } from './types'

function item(
  avgLatency: string | null,
  avgTtft: string | null
): PerformanceHistoryItem {
  return {
    avg_latency_ms: avgLatency,
    avg_ttft_ms: avgTtft,
    bucket_start: 1_754_352_000,
    group: 'default',
    model_name: 'gpt-test',
    site_name: 'site',
  } as PerformanceHistoryItem
}

describe('performance trend chart data', () => {
  test('preserves missing metrics instead of drawing them as zero', () => {
    const values = buildPerformanceTrendValues(
      [item(null, null)],
      'Latency',
      'TTFT'
    )

    expect(values.map((value) => value.value)).toEqual([null, null])
    expect(hasRenderablePerformanceTrendValues(values)).toBe(false)
  })

  test('renders real finite values including an observed zero', () => {
    const values = buildPerformanceTrendValues(
      [item('1250', '0')],
      'Latency',
      'TTFT'
    )

    expect(values.map((value) => value.value)).toEqual([1.25, 0])
    expect(hasRenderablePerformanceTrendValues(values)).toBe(true)
  })

  test('turns malformed and non-finite runtime values into gaps', () => {
    const values = buildPerformanceTrendValues(
      [item('not-a-number', 'Infinity')],
      'Latency',
      'TTFT'
    )

    expect(values.map((value) => value.value)).toEqual([null, null])
    expect(hasRenderablePerformanceTrendValues(values)).toBe(false)
  })
})
