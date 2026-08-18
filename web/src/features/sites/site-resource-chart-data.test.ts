import { describe, expect, test } from 'bun:test'

import {
  formatSiteResourceValue,
  hasRenderableSiteResourceValues,
  siteResourceValue,
} from './site-resource-chart-data'
import type { ResourcePoint } from './types'

function point(overrides: Partial<ResourcePoint> = {}): ResourcePoint {
  return {
    bucket_end: 120,
    bucket_start: 60,
    cpu_avg_percent: null,
    cpu_max_percent: null,
    data_status: 'missing',
    disk_last_used_percent: null,
    disk_max_used_percent: null,
    expected_sample_count: 1,
    health_status: 'unavailable',
    instance_count: null,
    memory_avg_percent: null,
    memory_max_percent: null,
    online_instance_count: null,
    sample_count: 0,
    ...overrides,
  } as ResourcePoint
}

describe('site resource chart data', () => {
  test('keeps missing and non-finite resource metrics as gaps', () => {
    expect(siteResourceValue(point(), 'cpu', 'avg')).toBeNull()
    expect(
      siteResourceValue(point({ cpu_avg_percent: Number.NaN }), 'cpu', 'avg')
    ).toBeNull()
  })

  test('treats an observed zero as renderable data', () => {
    const value = siteResourceValue(
      point({ cpu_avg_percent: 0, data_status: 'complete', sample_count: 1 }),
      'cpu',
      'avg'
    )

    expect(value).toBe(0)
    expect(
      hasRenderableSiteResourceValues([{ health: 'ok', time: '12:00', value }])
    ).toBe(true)
  })

  test('reports an explicit empty chart when every selected metric is null', () => {
    expect(
      hasRenderableSiteResourceValues([
        { health: 'unavailable', time: '12:00', value: null },
        { health: 'unavailable', time: '12:01', value: null },
      ])
    ).toBe(false)
  })

  test('labels missing table values without disguising observed zero', () => {
    expect(formatSiteResourceValue(null, '缺失')).toBe('缺失')
    expect(formatSiteResourceValue(Number.NaN, '缺失')).toBe('缺失')
    expect(formatSiteResourceValue(0, '缺失')).toBe('0.0%')
  })
})
