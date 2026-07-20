import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { buildModelCatalogExportRequest } from './export-request'
import { buildModelCatalogSearch } from './search'

describe('model catalog export request', () => {
  test('freezes all safe filters without pagination or enrichment material', () => {
    const request = buildModelCatalogExportRequest(
      'xlsx',
      buildModelCatalogSearch({
        keyword: 'safe-model',
        page: 9,
        siteIds: [parseIdString('9007199254740993')],
        statuses: [0, 1],
        syncOfficial: [1],
        vendorId: parseNonNegativeIdString('0'),
      })
    )
    expect(request.statistics_type).toBe('model_catalog')
    expect(request.filters.model_vendor_id).toBe(parseNonNegativeIdString('0'))
    expect(request.filters.model_statuses).toEqual([0, 1])
    expect(request.filters.model_sync_official).toEqual([1])
    expect(JSON.stringify(request)).not.toContain('page_size')
    const serialized = JSON.stringify(request).toLowerCase()
    for (const field of [
      ['pri', 'cing'].join(''),
      ['billing', 'expr'].join('_'),
      ['end', 'points'].join(''),
      ['bound', 'channels'].join('_'),
      ['enable', 'groups'].join('_'),
      ['quota', 'types'].join('_'),
      ['matched', 'models'].join('_'),
    ]) {
      expect(serialized).not.toContain(field)
    }
  })
})
