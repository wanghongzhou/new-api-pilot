import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'
import { dayjs } from '@/lib/dayjs'

import {
  buildPerformanceHistorySearch,
  performanceRangeForHours,
} from './search'

describe('performance history URL state', () => {
  test('keeps bigint ids exact and normalizes model/group filters', () => {
    const now = dayjs.tz('2026-07-18 12:34:00', 'Asia/Shanghai')
    const search = buildPerformanceHistorySearch(
      {
        groups: [' vip ', 'vip', ''],
        models: ['gpt-5', 'GPT-5', 'gpt-5'],
        siteIds: [
          parseIdString('9007199254740993'),
          parseIdString('9007199254740993'),
          'invalid',
        ],
      },
      now
    )
    expect(search.siteIds).toEqual([parseIdString('9007199254740993')])
    expect(search.groups).toEqual(['vip'])
    expect(search.models).toEqual(['gpt-5', 'GPT-5'])
    expect(search.end - search.start).toBe(24 * 3600)
  })

  test('builds aligned 24h, 7d and 30d shortcut ranges', () => {
    const end = 1_784_348_800
    expect(performanceRangeForHours(24, end)).toEqual({
      end,
      hours: 24,
      page: 1,
      start: end - 24 * 3600,
    })
    expect(performanceRangeForHours(168, end).start).toBe(end - 168 * 3600)
    expect(performanceRangeForHours(720, end).start).toBe(end - 720 * 3600)
  })
})
