import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildSubscriptionPlanSearch } from './search'

describe('subscription plan URL search', () => {
  test('preserves bigint sites, enabled false, safe keyword and states', () => {
    const search = buildSubscriptionPlanSearch({
      enabled: false,
      keyword: ' Plan ',
      page: 3,
      pageSize: 50,
      siteIds: ['9007199254740995', '9007199254740993'],
      states: ['missing', 'normal', 'missing'],
    })

    expect(search.enabled).toBe(false)
    expect(search.keyword).toBe('Plan')
    expect(search.siteIds).toEqual([
      parseIdString('9007199254740993'),
      parseIdString('9007199254740995'),
    ])
    expect(search.states).toEqual(['missing', 'normal'])
    expect(search.page).toBe(3)
    expect(search.pageSize).toBe(50)
  })

  test('fails closed for invalid ids, states, pagination and long keyword', () => {
    const search = buildSubscriptionPlanSearch({
      keyword: '订'.repeat(129),
      page: 0,
      pageSize: 101,
      siteIds: ['0', '-1'],
      states: ['deleted'],
    })

    expect(search.keyword).toBe('')
    expect(search.page).toBe(1)
    expect(search.pageSize).toBe(20)
    expect(search.siteIds).toEqual([])
    expect(search.states).toEqual([])
  })
})
