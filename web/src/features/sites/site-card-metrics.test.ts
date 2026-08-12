import { describe, expect, test } from 'bun:test'

import {
  formatAverageRate,
  formatInstanceAvailability,
  formatLatencySeconds,
  formatPerformanceLatency,
  formatPerformanceSuccessRate,
  formatPerformanceThroughput,
  formatPercentValue,
  isSitePerformanceReady,
  sitePerformanceDashboardSummary,
  siteResourceColor,
} from './site-card-metrics'

describe('site card average RPM and TPM formatting', () => {
  test('keeps exactly two decimal places', () => {
    expect(formatAverageRate('0.3382')).toBe('0.34')
    expect(formatAverageRate('39635.9910')).toBe('39635.99')
    expect(formatAverageRate('0')).toBe('0.00')
    expect(formatAverageRate(null)).toBe('0.00')
  })
})

describe('site performance availability', () => {
  test('treats a successfully sampled ready summary as available', () => {
    expect(isSitePerformanceReady('ready')).toBe(true)
  })

  test('keeps unavailable summaries out of the measured presentation', () => {
    expect(isSitePerformanceReady('unavailable')).toBe(false)
  })
})

describe('site latency formatting', () => {
  test.each([
    [0, '0'],
    [120, '0.12'],
    [1234, '1.23'],
    [10000, '10'],
  ])('formats %s milliseconds as seconds', (value, expected) => {
    expect(formatLatencySeconds(value)).toBe(expected)
  })
})

describe('new-api dashboard compatible performance summary', () => {
  const models = [
    {
      avg_latency_ms: 120,
      avg_tps: 8.25,
      model_name: 'small',
      success_rate: 98.5,
    },
    {
      avg_latency_ms: 240,
      avg_tps: 20,
      model_name: 'large',
      success_rate: 100,
    },
  ]

  test('uses the same simple-average rules as the new-api dashboard', () => {
    expect(sitePerformanceDashboardSummary(models)).toEqual({
      avgLatencyMs: 180,
      successRate: 99.25,
      throughput: 14.125,
    })
  })

  test('ignores zero latency and throughput but keeps zero success rates', () => {
    expect(
      sitePerformanceDashboardSummary([
        ...models,
        {
          avg_latency_ms: 0,
          avg_tps: 0,
          model_name: 'zero',
          success_rate: 0,
        },
      ])
    ).toEqual({
      avgLatencyMs: 180,
      successRate: 66.16666666666667,
      throughput: 14.125,
    })
  })

  test('formats values like the new-api performance dashboard', () => {
    expect(formatPerformanceSuccessRate(99.25, '-')).toBe('99.25%')
    expect(formatPerformanceLatency(180, '-')).toBe('180ms')
    expect(formatPerformanceLatency(12_345, '-')).toBe('12.35s')
    expect(formatPerformanceThroughput(8.25, '-')).toBe('8.25 t/s')
    expect(formatPerformanceThroughput(20, '-')).toBe('20.0 t/s')
    expect(formatPerformanceThroughput(0, '-')).toBe('-')
  })
})

describe('site card resource gradient', () => {
  test('uses capacity-oriented semantic anchors', () => {
    expect(siteResourceColor(null)).toBeUndefined()
    expect(siteResourceColor(0)).toContain(' 145)')
    expect(siteResourceColor(55)).toContain(' 105)')
    expect(siteResourceColor(75)).toContain(' 80)')
    expect(siteResourceColor(90)).toContain(' 50)')
    expect(siteResourceColor(100)).toContain(' 25)')
  })

  test('keeps low utilization green and interpolates every interval', () => {
    expect(siteResourceColor(27.5)).toContain(' 125)')
    expect(siteResourceColor(65)).toContain(' 92.5)')
    expect(siteResourceColor(82.5)).toContain(' 65)')
    expect(siteResourceColor(95)).toContain(' 37.5)')
  })

  test('clamps out-of-range percentages to the endpoints', () => {
    expect(siteResourceColor(-1)).toContain(' 145)')
    expect(siteResourceColor(101)).toContain(' 25)')
  })
})

describe('nullable site resource presentation', () => {
  test('keeps unavailable percentages distinct from a measured zero', () => {
    expect(formatPercentValue(null, '-')).toBe('-')
    expect(formatPercentValue(Number.NaN, '-')).toBe('-')
    expect(formatPercentValue(0, '-')).toBe('0.0%')
  })

  test('does not fabricate a partial instance ratio', () => {
    expect(formatInstanceAvailability(null, null, '-')).toBe('-')
    expect(formatInstanceAvailability(0, null, '-')).toBe('-')
    expect(formatInstanceAvailability(0, 0, '-')).toBe('0/0')
  })
})
