import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { subscriptionPlanSearchSchema } from './schema'
import {
  buildSubscriptionPlanSearch,
  changeSubscriptionPlanTab,
  serializeSubscriptionPlanSearch,
} from './search'

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
    expect(search.tab).toBe('plans')
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

  test('keeps the default route URL empty', () => {
    expect(subscriptionPlanSearchSchema.parse({})).toEqual({})
    expect(
      serializeSubscriptionPlanSearch(buildSubscriptionPlanSearch({}))
    ).toEqual({
      enabled: undefined,
      exportId: undefined,
      keyword: undefined,
      page: undefined,
      pageSize: undefined,
      siteIds: undefined,
      states: undefined,
      tab: undefined,
    })
  })

  test('keeps site analysis filter-free and serializes only the tab', () => {
    expect(changeSubscriptionPlanTab('site-analysis')).toMatchObject({
      enabled: undefined,
      keyword: '',
      page: 1,
      pageSize: 20,
      siteIds: [],
      states: [],
      tab: 'site-analysis',
    })
    expect(
      serializeSubscriptionPlanSearch(
        buildSubscriptionPlanSearch({
          enabled: false,
          keyword: 'plan',
          page: 3,
          siteIds: ['1'],
          states: ['missing'],
          tab: 'site-analysis',
        })
      )
    ).toEqual({
      enabled: undefined,
      exportId: undefined,
      keyword: undefined,
      page: undefined,
      pageSize: undefined,
      siteIds: undefined,
      states: undefined,
      tab: 'site-analysis',
    })
  })
})
