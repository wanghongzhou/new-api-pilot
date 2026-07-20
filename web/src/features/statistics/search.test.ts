import { describe, expect, test } from 'bun:test'

import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import { buildStatisticsSearch } from './search'

const now = dayjs.tz('2026-07-13 12:34:56', BEIJING_TIMEZONE)

describe('statistics URL range normalization', () => {
  test('keeps aligned Beijing bucket boundaries', () => {
    const search = buildStatisticsSearch(
      {
        end: dayjs.tz('2026-07-13 12:00', BEIJING_TIMEZONE).unix(),
        granularity: 'hour',
        start: dayjs.tz('2026-07-12 12:00', BEIJING_TIMEZONE).unix(),
      },
      now
    )
    expect(search.end - search.start).toBe(24 * 3600)
  })

  test('rejects unaligned hour, day, month, and year deep links', () => {
    for (const granularity of ['hour', 'day', 'month', 'year'] as const) {
      const defaults = buildStatisticsSearch({ granularity }, now)
      const search = buildStatisticsSearch(
        {
          end: defaults.end + 1,
          granularity,
          start: defaults.start + 1,
        },
        now
      )
      expect(search.start).toBe(defaults.start)
      expect(search.end).toBe(defaults.end)
    }
  })

  test('enforces 31-day, 2-year, and 20-year page limits', () => {
    const cases = [
      ['hour', dayjs.tz('2026-01-01', BEIJING_TIMEZONE), 32, 'day'],
      ['day', dayjs.tz('2023-01-01', BEIJING_TIMEZONE), 3, 'year'],
      ['month', dayjs.tz('2000-01-01', BEIJING_TIMEZONE), 21, 'year'],
    ] as const
    for (const [granularity, start, amount, unit] of cases) {
      const defaults = buildStatisticsSearch({ granularity }, now)
      const search = buildStatisticsSearch(
        {
          end: start.add(amount, unit).unix(),
          granularity,
          start: start.unix(),
        },
        now
      )
      expect(search.start).toBe(defaults.start)
      expect(search.end).toBe(defaults.end)
    }
  })

  test('allows an unlimited aligned year range', () => {
    const start = dayjs.tz('1971-01-01', BEIJING_TIMEZONE)
    const end = dayjs.tz('2027-01-01', BEIJING_TIMEZONE)
    const search = buildStatisticsSearch(
      { end: end.unix(), granularity: 'year', start: start.unix() },
      now
    )
    expect(search.start).toBe(start.unix())
    expect(search.end).toBe(end.unix())
  })

  test('normalizes bigint ID and option filters without Number coercion', () => {
    const search = buildStatisticsSearch(
      {
        accountIds: ['9007199254740997', '9007199254740997'],
        channelKeys: ['9007199254740993:0'],
        customerIds: ['9007199254740995'],
        models: ['超长中文模型名称', '超长中文模型名称'],
        nodeNames: ['', 'Node-A', 'Node-A'],
        siteIds: ['9007199254740993'],
        tokenKeys: ['9007199254740993:0', '9007199254740993:0'],
        useGroups: ['', 'vip', 'vip'],
      },
      now
    )
    expect(search.siteIds.join(',')).toBe('9007199254740993')
    expect(search.customerIds.join(',')).toBe('9007199254740995')
    expect(search.accountIds.join(',')).toBe('9007199254740997')
    expect(search.models).toEqual(['超长中文模型名称'])
    expect(search.channelKeys).toEqual(['9007199254740993:0'])
    expect(search.useGroups).toEqual(['', 'vip'])
    expect(search.tokenKeys).toEqual(['9007199254740993:0'])
    expect(search.nodeNames).toEqual(['', 'Node-A'])
  })
})
