import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildRankingExportRequest } from './export-request'
import { buildRankingSearch } from './search'

describe('local rankings export request', () => {
  test('freezes global model scope and ranking period', () => {
    const request = buildRankingExportRequest(
      'xlsx',
      buildRankingSearch({
        period: 'year',
        siteIds: ['9007199254740993'],
        tab: 'models',
      })
    )

    expect(request.statistics_type).toBe('model_rankings')
    expect(request.filters.ranking_period).toBe('year')
    expect(request.filters.site_ids).toEqual([
      parseIdString('9007199254740993'),
    ])
  })

  test('forces site vendor scope and ignores global site ids', () => {
    const request = buildRankingExportRequest(
      'csv',
      buildRankingSearch({
        period: 'today',
        siteIds: ['9007199254740995'],
        tab: 'vendors',
      }),
      parseIdString('9007199254740993')
    )

    expect(request.statistics_type).toBe('vendor_rankings')
    expect(request.filters.ranking_period).toBe('today')
    expect(request.filters.site_ids).toEqual([
      parseIdString('9007199254740993'),
    ])
  })
})
