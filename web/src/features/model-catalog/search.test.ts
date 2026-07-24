import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { modelCatalogSearchSchema } from './schema'
import {
  buildModelCatalogQueryParams,
  buildModelCatalogSearch,
  changeModelCatalogTab,
  serializeModelCatalogSearch,
} from './search'

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

  test('builds view-specific API parameters', () => {
    const search = buildModelCatalogSearch({
      keyword: 'safe-model',
      siteIds: [parseIdString('9007199254740993')],
      statuses: [1],
      syncOfficial: [0],
      vendorId: parseNonNegativeIdString('8'),
    })
    expect(buildModelCatalogQueryParams(search, 'catalog')).toMatchObject({
      statuses: [1],
      sync_official: [0],
      vendor_id: parseNonNegativeIdString('8'),
    })
    expect(buildModelCatalogQueryParams(search, 'coverage')).toMatchObject({
      statuses: [1],
      sync_official: [0],
      vendor_id: parseNonNegativeIdString('8'),
    })
    expect(buildModelCatalogQueryParams(search, 'missing')).toEqual({
      keyword: 'safe-model',
      p: 1,
      page_size: 20,
      site_ids: [parseIdString('9007199254740993')],
    })
  })

  test('clears incompatible URL filters when switching to missing', () => {
    expect(changeModelCatalogTab('missing')).toEqual({
      page: 1,
      statuses: [],
      syncOfficial: [],
      tab: 'missing',
      vendorId: undefined,
    })
    expect(changeModelCatalogTab('coverage')).toEqual({
      keyword: '',
      page: 1,
      pageSize: 20,
      siteIds: [],
      statuses: [],
      syncOfficial: [],
      tab: 'coverage',
      vendorId: undefined,
    })
  })

  test('keeps the default route URL empty and serializes only active state', () => {
    expect(modelCatalogSearchSchema.parse({})).toEqual({})
    expect(serializeModelCatalogSearch(buildModelCatalogSearch({}))).toEqual({
      exportId: undefined,
      keyword: undefined,
      page: undefined,
      pageSize: undefined,
      siteIds: undefined,
      statuses: undefined,
      syncOfficial: undefined,
      tab: undefined,
      vendorId: undefined,
    })
  })

  test('coverage clears filters and pagination because it is one analysis result', () => {
    expect(changeModelCatalogTab('coverage')).toEqual({
      keyword: '',
      page: 1,
      pageSize: 20,
      siteIds: [],
      statuses: [],
      syncOfficial: [],
      tab: 'coverage',
      vendorId: undefined,
    })
  })
})
