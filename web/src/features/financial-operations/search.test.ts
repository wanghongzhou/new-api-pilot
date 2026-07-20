import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'
import { dayjs } from '@/lib/dayjs'

import { buildFinancialOperationsSearch } from './search'

describe('financial operations URL search', () => {
  test('keeps branded ids exact and normalizes all frozen filters', () => {
    const search = buildFinancialOperationsSearch(
      {
        methods: [' stripe ', 'stripe'],
        providers: ['wechat', 'stripe'],
        remoteId: parseIdString('9007199254740993'),
        remoteUserId: parseNonNegativeIdString('0'),
        siteIds: [parseIdString('9007199254740995')],
        states: ['missing', 'normal'],
        statuses: ['success', 'pending'],
        tab: 'redemptions',
      },
      dayjs.tz('2026-07-18 12:34:00', 'Asia/Shanghai')
    )
    expect(search.remoteId).toBe(parseIdString('9007199254740993'))
    expect(search.remoteUserId).toBe(parseNonNegativeIdString('0'))
    expect(search.siteIds).toEqual([parseIdString('9007199254740995')])
    expect(search.methods).toEqual(['stripe'])
    expect(search.states).toEqual(['missing', 'normal'])
    expect(search.tab).toBe('redemptions')
  })

  test('fails closed to an aligned 30-day range', () => {
    const now = dayjs.tz('2026-07-18 12:34:00', 'Asia/Shanghai')
    const search = buildFinancialOperationsSearch({ end: 10, start: 20 }, now)
    expect(search.end - search.start).toBe(30 * 24 * 3600)
    expect(search.start % 3600).toBe(0)
  })
})
