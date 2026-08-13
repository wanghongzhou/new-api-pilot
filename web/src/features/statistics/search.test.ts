import { describe, expect, test } from 'bun:test'

import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import {
  buildScopeStatisticsSearch,
  buildStatisticsSearch,
  decodeStatisticsDimensionFilters,
  EMPTY_STATISTICS_DIMENSION_FILTER,
} from './search'

const now = dayjs.tz('2026-07-13 12:34:56', BEIJING_TIMEZONE)

describe('statistics URL range normalization', () => {
  test('defaults the operations entry to the latest 24 hourly buckets', () => {
    const search = buildStatisticsSearch({}, now)

    expect(search.granularity).toBe('hour')
    expect(search.end).toBe(
      dayjs.tz('2026-07-13 12:00', BEIJING_TIMEZONE).unix()
    )
    expect(search.end - search.start).toBe(24 * 3600)
  })

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

  test('defaults the year view to five visible calendar buckets', () => {
    const search = buildStatisticsSearch({ granularity: 'year' }, now)

    expect(search.start).toBe(dayjs.tz('2022-01-01', BEIJING_TIMEZONE).unix())
    expect(search.end).toBe(dayjs.tz('2027-01-01', BEIJING_TIMEZONE).unix())
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
    expect(search.useGroups).toEqual([EMPTY_STATISTICS_DIMENSION_FILTER, 'vip'])
    expect(search.tokenKeys).toEqual(['9007199254740993:0'])
    expect(search.nodeNames).toEqual([
      EMPTY_STATISTICS_DIMENSION_FILTER,
      'Node-A',
    ])
    expect(decodeStatisticsDimensionFilters(search.useGroups)).toEqual([
      '',
      'vip',
    ])
    expect(decodeStatisticsDimensionFilters(search.nodeNames)).toEqual([
      '',
      'Node-A',
    ])
  })
})

describe('statistics scope navigation', () => {
  test('preserves shared state and only the target scope filters', () => {
    const current = buildStatisticsSearch({
      accountIds: ['13'],
      channelKeys: ['11:12'],
      customerIds: ['10'],
      display: 'cny',
      end: 1_784_275_200,
      granularity: 'hour',
      metric: 'quota',
      models: ['gpt-5'],
      nodeNames: ['node-a'],
      order: 'desc',
      page: 8,
      pageSize: 50,
      siteIds: ['9'],
      sort: 'quota',
      start: 1_784_188_800,
      tokenKeys: ['11:14'],
      useGroups: ['vip'],
      view: 'table',
    })

    const account = buildScopeStatisticsSearch(current, 'account')
    expect(account).toMatchObject({
      accountIds: ['13'],
      customerIds: ['10'],
      display: 'cny',
      end: current.end,
      granularity: 'hour',
      metric: 'quota',
      order: 'desc',
      page: 1,
      pageSize: 50,
      siteIds: current.siteIds,
      sort: 'quota',
      start: current.start,
      view: 'table',
    })
    expect(account.models).toEqual([])
    expect(account.channelKeys).toEqual([])
    expect(account.useGroups).toEqual([])
    expect(account.tokenKeys).toEqual([])
    expect(account.nodeNames).toEqual([])

    const model = buildScopeStatisticsSearch(current, 'model')
    expect(model.models).toEqual(['gpt-5'])
    expect(model.customerIds).toEqual([])
    expect(model.accountIds).toEqual([])
    expect(model.siteIds).toEqual(current.siteIds)
    expect(model.start).toBe(current.start)
    expect(model.end).toBe(current.end)
  })
})
