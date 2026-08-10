import { describe, expect, test } from 'bun:test'

import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import {
  buildLogQuickRange,
  buildLogSearch,
  getLogQuickRange,
  mergeLogSearch,
} from './search'

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

  test('builds Beijing quick ranges and detects frozen custom ranges', () => {
    expect(buildLogQuickRange('today', now)).toEqual({
      end: dayjs.tz('2026-07-17 12:00:00', BEIJING_TIMEZONE).unix(),
      start: dayjs.tz('2026-07-17 00:00:00', BEIJING_TIMEZONE).unix(),
    })
    expect(
      buildLogQuickRange('24h', now).end - buildLogQuickRange('24h', now).start
    ).toBe(24 * 3600)
    expect(
      buildLogQuickRange('7d', now).end - buildLogQuickRange('7d', now).start
    ).toBe(7 * 24 * 3600)
    expect(
      buildLogQuickRange('14d', now).end - buildLogQuickRange('14d', now).start
    ).toBe(14 * 24 * 3600)
    expect(getLogQuickRange(buildLogQuickRange('7d', now))).toBe('7d')
    expect(
      getLogQuickRange({
        end: now.unix(),
        start: now.subtract(2, 'day').unix(),
      })
    ).toBe('custom')
  })

  test('removes cleared optional filters instead of reviving URL values', () => {
    const merged = mergeLogSearch(
      buildLogSearch({ channelId: '1', modelName: 'gpt-5', page: 4 }, now),
      { channelId: undefined, page: 1 }
    )
    expect(merged).not.toHaveProperty('channelId')
    expect(merged.modelName).toBe('gpt-5')
    expect(merged.page).toBe(1)
  })
})
