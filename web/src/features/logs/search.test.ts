import { describe, expect, test } from 'bun:test'

import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import { buildLogSearch } from './search'

const now = dayjs.tz('2026-07-17 12:34:56', BEIJING_TIMEZONE)

describe('log URL search normalization', () => {
  test('preserves bigint-safe filters and the complete query identity', () => {
    const search = buildLogSearch(
      {
        channelId: '9007199254740995',
        end: 1_784_262_400,
        exportId: '9007199254740997',
        group: ' vip ',
        modelName: 'gpt-4.1',
        page: 3,
        pageSize: 50,
        requestId: 'req-local',
        siteIds: ['9007199254740993', '9007199254740993'],
        start: 1_784_176_000,
        tokenName: 'production',
        type: 5,
        upstreamRequestId: 'req-upstream',
        username: 'alice',
      },
      now
    )
    expect(search.siteIds.join(',')).toBe('9007199254740993')
    expect(String(search.channelId)).toBe('9007199254740995')
    expect(String(search.exportId)).toBe('9007199254740997')
    expect(search.group).toBe('vip')
    expect(search.type).toBe(5)
    expect(search.page).toBe(3)
  })

  test('fails closed to a 24 hour range for invalid or oversized deep links', () => {
    const defaults = buildLogSearch({}, now)
    const invalid = buildLogSearch(
      { end: 1_800_000_000, start: 1, type: 9 },
      now
    )
    expect(invalid.start).toBe(defaults.start)
    expect(invalid.end).toBe(defaults.end)
    expect(invalid.type).toBeUndefined()
    expect(defaults.end - defaults.start).toBe(24 * 3600)
  })
})
