import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { buildModelCatalogSearch } from './search'

describe('model catalog URL search', () => {
  test('preserves bigint-safe filters and canonical tab state', () => {
    const search = buildModelCatalogSearch({
      keyword: ' safe-model ',
      siteIds: [parseIdString('9007199254740993')],
      statuses: [1, 0, 1],
      syncOfficial: [1],
      tab: 'missing',
      vendorId: parseNonNegativeIdString('0'),
    })
    expect(search.keyword).toBe('safe-model')
    expect(search.siteIds).toEqual([parseIdString('9007199254740993')])
    expect(search.statuses).toEqual([0, 1])
    expect(search.syncOfficial).toEqual([1])
    expect(search.vendorId).toBe(parseNonNegativeIdString('0'))
    expect(search.tab).toBe('missing')
  })

  test('fails closed for unsupported filters and tab values', () => {
    const search = buildModelCatalogSearch({
      statuses: [2],
      syncOfficial: [-1],
      tab: 'other' as 'catalog',
      vendorId: '-1',
    })
    expect(search.statuses).toEqual([])
    expect(search.syncOfficial).toEqual([])
    expect(search.vendorId).toBeUndefined()
    expect(search.tab).toBe('catalog')
  })
})
