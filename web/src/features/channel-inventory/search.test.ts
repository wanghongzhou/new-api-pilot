import { describe, expect, test } from 'bun:test'

import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import {
  buildChannelInventorySearch,
  changeChannelInventoryTab,
} from './search'

const now = dayjs.tz('2026-07-18 12:34:56', BEIJING_TIMEZONE)

describe('channel inventory URL normalization', () => {
  test('preserves exact decimals, bigint values and authoritative enums', () => {
    const search = buildChannelInventorySearch(
      {
        end: dayjs.tz('2026-07-18 12:00', BEIJING_TIMEZONE).unix(),
        exportId: '9007199254740999',
        groups: [' vip ', 'vip'],
        keyword: ' gpt ',
        maxBalance: '9007199254740993.1234567890',
        maxResponseTime: '9223372036854775807',
        minBalance: '-10.0000000001',
        minResponseTime: '0',
        siteIds: ['9007199254740993', '9007199254740993'],
        start: dayjs.tz('2026-07-17 12:00', BEIJING_TIMEZONE).unix(),
        states: ['missing', 'normal', 'invalid'],
        statuses: [3, 1, 99],
        tags: [' primary ', 'primary'],
        trendPage: 3,
        trendPageSize: 50,
        trendView: 'table',
        types: [8, 1, -1],
      },
      now
    )
    expect(search.siteIds.join(',')).toBe('9007199254740993')
    expect(search.groups).toEqual(['vip'])
    expect(search.tags).toEqual(['primary'])
    expect(search.types).toEqual([1, 8])
    expect(search.statuses).toEqual([1, 3])
    expect(search.states).toEqual(['missing', 'normal'])
    expect(String(search.minBalance)).toBe('-10.0000000001')
    expect(String(search.maxBalance)).toBe('9007199254740993.1234567890')
    expect(String(search.maxResponseTime)).toBe('9223372036854775807')
    expect(String(search.exportId)).toBe('9007199254740999')
    expect(search.trendPage).toBe(3)
    expect(search.trendPageSize).toBe(50)
    expect(search.trendView).toBe('table')
  })

  test('fails closed for reversed values and unaligned ranges', () => {
    const defaults = buildChannelInventorySearch({}, now)
    const invalid = buildChannelInventorySearch(
      {
        end: defaults.end + 1,
        maxBalance: '-2',
        maxResponseTime: '4',
        minBalance: '1',
        minResponseTime: '5',
        start: defaults.start,
      },
      now
    )
    expect(invalid.start).toBe(defaults.start)
    expect(invalid.end).toBe(defaults.end)
    expect(invalid.minBalance).toBeUndefined()
    expect(invalid.maxBalance).toBeUndefined()
    expect(invalid.minResponseTime).toBeUndefined()
    expect(invalid.maxResponseTime).toBeUndefined()
  })

  test('normalizes tabs and clears list-only filters for analysis views', () => {
    expect(buildChannelInventorySearch({ tab: 'sites' }, now).tab).toBe('sites')
    expect(
      buildChannelInventorySearch({ tab: 'invalid' as never }, now).tab
    ).toBe('list')
    expect(
      buildChannelInventorySearch({ trendView: 'invalid' as never }, now)
        .trendView
    ).toBe('chart')
    expect(changeChannelInventoryTab('trend')).toMatchObject({
      keyword: '',
      page: 1,
      states: [],
      tab: 'trend',
    })
  })
})
