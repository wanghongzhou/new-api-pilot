import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildRankingSearch } from './search'

describe('local rankings URL search', () => {
  test('preserves canonical period, tab, and bigint-safe site ids', () => {
    const search = buildRankingSearch({
      period: 'year',
      page: 3,
      pageSize: 50,
      siteIds: ['9007199254740995', '9007199254740993', '9007199254740995'],
      tab: 'vendors',
      view: 'history',
    })

    expect(search.period).toBe('year')
    expect(search.page).toBe(3)
    expect(search.pageSize).toBe(50)
    expect(search.tab).toBe('vendors')
    expect(search.view).toBe('history')
    expect(search.siteIds).toEqual([
      parseIdString('9007199254740993'),
      parseIdString('9007199254740995'),
    ])
  })

  test('defaults unsupported values and rejects invalid ids', () => {
    const search = buildRankingSearch({
      period: 'quarter',
      page: 0,
      pageSize: 17,
      siteIds: ['0', '-1', '1.5', 'safe'],
      tab: 'channels',
      view: 'everything',
    })

    expect(search.period).toBe('month')
    expect(search.page).toBe(1)
    expect(search.pageSize).toBe(20)
    expect(search.tab).toBe('models')
    expect(search.view).toBe('ranking')
    expect(search.siteIds).toEqual([])
  })
})
