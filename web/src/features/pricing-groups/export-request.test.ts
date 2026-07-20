import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildPricingGroupExportRequest } from './export-request'
import { buildPricingGroupSearch } from './search'

describe('pricing/group export contract', () => {
  test('freezes safe filters and tab-specific export type', () => {
    const search = buildPricingGroupSearch({
      group: 'vip',
      keyword: 'gpt',
      siteIds: ['9007199254740993'],
      states: ['missing'],
      tab: 'pricing',
    })
    const pricing = buildPricingGroupExportRequest('xlsx', search)
    expect(pricing.statistics_type).toBe('pricing_catalog')
    expect(pricing.filters).toMatchObject({
      inventory_states: ['missing'],
      keyword: 'gpt',
      pricing_group: 'vip',
      site_ids: [parseIdString('9007199254740993')],
    })
    const groups = buildPricingGroupExportRequest('csv', {
      ...search,
      tab: 'groups',
    })
    expect(groups.statistics_type).toBe('group_catalog')
  })

  test('forced site replaces global site filters', () => {
    const request = buildPricingGroupExportRequest(
      'csv',
      buildPricingGroupSearch({ siteIds: ['9'] }),
      parseIdString('9007199254740993')
    )
    expect(request.filters.site_ids).toEqual([
      parseIdString('9007199254740993'),
    ])
  })
})
