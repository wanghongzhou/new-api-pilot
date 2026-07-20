import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildPricingGroupSearch } from './search'

describe('pricing/group URL search', () => {
  test('preserves bigint sites and normalizes tab, text and states', () => {
    const search = buildPricingGroupSearch({
      group: ' vip ',
      keyword: ' gpt ',
      page: 3,
      pageSize: 50,
      siteIds: ['9007199254740995', '9007199254740993'],
      states: ['missing', 'normal', 'missing'],
      tab: 'groups',
    })
    expect(search.group).toBe('vip')
    expect(search.keyword).toBe('gpt')
    expect(search.siteIds).toEqual([
      parseIdString('9007199254740993'),
      parseIdString('9007199254740995'),
    ])
    expect(search.states).toEqual(['missing', 'normal'])
    expect(search.tab).toBe('groups')
    expect(search.page).toBe(3)
  })

  test('fails closed for invalid ids, state, pagination and long text', () => {
    const search = buildPricingGroupSearch({
      group: '组'.repeat(129),
      keyword: '价'.repeat(256),
      page: 0,
      pageSize: 101,
      siteIds: ['0', '-1'],
      states: ['deleted'],
    })
    expect(search).toMatchObject({
      group: '',
      keyword: '',
      page: 1,
      pageSize: 20,
      siteIds: [],
      states: [],
      tab: 'pricing',
    })
  })
})
