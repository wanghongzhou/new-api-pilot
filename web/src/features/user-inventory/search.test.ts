import { describe, expect, test } from 'bun:test'

import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import { buildUserInventorySearch } from './search'

const now = dayjs.tz('2026-07-17 12:34:56', BEIJING_TIMEZONE)

describe('user inventory URL normalization', () => {
  test('preserves bigint identity, signed balances and authoritative enums', () => {
    const search = buildUserInventorySearch(
      {
        end: dayjs.tz('2026-07-17 12:00', BEIJING_TIMEZONE).unix(),
        exportId: '9007199254740999',
        groups: [' vip ', 'vip'],
        keyword: ' alice ',
        maxBalance: '9223372036854775807',
        minBalance: '-9223372036854775808',
        roles: [100, 1, 999],
        remoteUserId: '9007199254740997',
        siteIds: ['9007199254740993', '9007199254740993'],
        start: dayjs.tz('2026-07-16 12:00', BEIJING_TIMEZONE).unix(),
        states: ['identity_mismatch', 'missing', 'invalid'],
        statuses: [2, 1, 9],
      },
      now
    )
    expect(search.siteIds.join(',')).toBe('9007199254740993')
    expect(search.groups).toEqual(['vip'])
    expect(search.roles).toEqual([1, 100])
    expect(String(search.remoteUserId)).toBe('9007199254740997')
    expect(search.statuses).toEqual([1, 2])
    expect(search.states).toEqual(['identity_mismatch', 'missing'])
    expect(String(search.minBalance)).toBe('-9223372036854775808')
    expect(String(search.maxBalance)).toBe('9223372036854775807')
    expect(String(search.exportId)).toBe('9007199254740999')
  })

  test('fails closed for unaligned, oversized and reversed ranges', () => {
    const defaults = buildUserInventorySearch({}, now)
    const invalid = buildUserInventorySearch(
      { end: defaults.end + 1, start: defaults.start - 367 * 24 * 3600 },
      now
    )
    expect(invalid.start).toBe(defaults.start)
    expect(invalid.end).toBe(defaults.end)
  })
})
