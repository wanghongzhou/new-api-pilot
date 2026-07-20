import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildRankingSearch } from './search'

describe('local rankings URL search', () => {
  test('preserves canonical period, tab, and bigint-safe site ids', () => {
    const search = buildRankingSearch({
      period: 'year',
      siteIds: ['9007199254740995', '9007199254740993', '9007199254740995'],
      tab: 'vendors',
    })

    expect(search.period).toBe('year')
    expect(search.tab).toBe('vendors')
    expect(search.siteIds).toEqual([
      parseIdString('9007199254740993'),
      parseIdString('9007199254740995'),
    ])
  })

  test('defaults unsupported values and rejects invalid ids', () => {
    const search = buildRankingSearch({
      period: 'quarter',
      siteIds: ['0', '-1', '1.5', 'safe'],
      tab: 'channels',
    })

    expect(search.period).toBe('today')
    expect(search.tab).toBe('models')
    expect(search.siteIds).toEqual([])
  })
})
