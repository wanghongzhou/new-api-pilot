import { describe, expect, test } from 'bun:test'

import { parseMetricString } from '@/lib/api-types'

import { buildTrendChartModel } from './chart-data'
import type { TrendPoint } from './types'

function point(
  start: number,
  quota: string | null,
  overrides: Partial<TrendPoint> = {}
): TrendPoint {
  return {
    active_users: quota == null ? null : parseMetricString('1'),
    as_of: start + 1800,
    bucket_end: start + 3600,
    bucket_start: start,
    complete_site_count: 1,
    data_status: quota == null ? 'missing' : 'complete',
    expected_site_count: 1,
    is_final: true,
    quota: quota == null ? null : parseMetricString(quota),
    reason: null,
    request_count: quota == null ? null : parseMetricString(quota),
    site_breakdown:
      quota == null
        ? []
        : [
            {
              data_status: 'complete',
              quota: parseMetricString(quota),
              quota_per_unit: '500000',
              rate_source: 'site',
              rate_updated_at: start + 1800,
              site_id: '1' as never,
              site_name: '华东站点',
              usd_exchange_rate: '7.3',
            },
          ],
    token_used: quota == null ? null : parseMetricString(quota),
    ...overrides,
  }
}

describe('statistics trend chart model', () => {
  test('offsets unsafe bigint values without changing exact tooltip values', () => {
    const model = buildTrendChartModel(
      [
        point(1_783_789_200, '9223372036854775806'),
        point(1_783_792_800, '9223372036854775807'),
      ],
      'quota',
      'quota',
      'hour'
    )
    expect(model.baseline).toBe('9223372036854775806')
    expect(model.values.map((item) => item.chartValue)).toEqual([0, 1])
    expect(model.values.map((item) => item.rawValue)).toEqual([
      '9223372036854775806',
      '9223372036854775807',
    ])
  })

  test('keeps per-bucket rates and precise site amounts', () => {
    const model = buildTrendChartModel(
      [point(1_783_789_200, '500001')],
      'quota',
      'usd',
      'hour'
    )
    expect(model.values[0]?.exactValue).toBe('1.000002')
    expect(model.values[0]?.siteAmounts[0]).toMatchObject({
      amountCny: '7.300015',
      amountUsd: '1.000002',
      quota: '500001',
      quotaPerUnit: '500000',
      usdExchangeRate: '7.3',
    })
  })

  test('marks partial points and leaves null values as chart breaks', () => {
    const model = buildTrendChartModel(
      [
        point(1_783_789_200, '1', {
          data_status: 'partial',
          is_final: false,
        }),
        point(1_783_792_800, null),
      ],
      'quota',
      'quota',
      'hour'
    )
    expect(model.values[0]).toMatchObject({ partial: true, chartValue: 1 })
    expect(model.values[1]).toMatchObject({
      chartValue: null,
      exactValue: null,
      rawValue: null,
    })
  })
})
