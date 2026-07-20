import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildPerformanceHistoryExportRequest } from './export-request'
import { buildPerformanceHistorySearch } from './search'

describe('performance history export request', () => {
  test('exports the same model/group/time/site filters without client aggregation', () => {
    const request = buildPerformanceHistoryExportRequest(
      'csv',
      buildPerformanceHistorySearch({
        end: 1_784_348_800,
        groups: ['vip'],
        models: ['gpt-5'],
        siteIds: [parseIdString('9007199254740993')],
        start: 1_784_262_400,
      })
    )
    expect(request.statistics_type).toBe('performance_history')
    expect(request.filters.site_ids).toEqual([
      parseIdString('9007199254740993'),
    ])
    expect(request.filters.model_names).toEqual(['gpt-5'])
    expect(request.filters.use_groups).toEqual(['vip'])
  })
})
