import { describe, expect, test } from 'bun:test'

import { dayjs, BEIJING_TIMEZONE } from '@/lib/dayjs'

import {
  alignSiteResourceTimestamp,
  defaultSiteResourceRange,
  siteResourceRangeExceedsLimit,
  siteResourceRangeLimitEnd,
  siteResourceRangeLimitStart,
} from './site-resource-range'

const now = dayjs.tz('2026-08-11 12:57:28', BEIJING_TIMEZONE).unix()

describe('site resource ranges', () => {
  test('uses the last 24 hours of closed minute buckets', () => {
    const range = defaultSiteResourceRange('minute', now)
    expect(range.end).toBe(
      dayjs.tz('2026-08-11 12:57:00', BEIJING_TIMEZONE).unix()
    )
    expect(range.end - range.start).toBe(24 * 60 * 60)
  })

  test('uses the last seven days of closed Beijing hours', () => {
    const range = defaultSiteResourceRange('hour', now)
    expect(range.end).toBe(
      dayjs.tz('2026-08-11 12:00:00', BEIJING_TIMEZONE).unix()
    )
    expect(range.end - range.start).toBe(7 * 24 * 60 * 60)
  })

  test('uses the previous calendar month aligned to Beijing day boundaries', () => {
    const range = defaultSiteResourceRange('day', now)
    expect(range.end).toBe(
      dayjs.tz('2026-08-11 00:00:00', BEIJING_TIMEZONE).unix()
    )
    expect(range.start).toBe(
      dayjs.tz('2026-07-11 00:00:00', BEIJING_TIMEZONE).unix()
    )
  })

  test('uses calendar-aware limits for manual ranges', () => {
    const leapMonthStart = dayjs
      .tz('2024-01-29 00:00:00', BEIJING_TIMEZONE)
      .unix()
    const limitEnd = dayjs.tz('2024-02-29 00:00:00', BEIJING_TIMEZONE).unix()
    expect(siteResourceRangeLimitEnd(leapMonthStart, 'day')).toBe(limitEnd)
    expect(siteResourceRangeLimitStart(limitEnd, 'day')).toBe(leapMonthStart)
    expect(siteResourceRangeExceedsLimit(leapMonthStart, limitEnd, 'day')).toBe(
      false
    )
    expect(
      siteResourceRangeExceedsLimit(
        leapMonthStart,
        limitEnd + 24 * 60 * 60,
        'day'
      )
    ).toBe(true)
    expect(
      siteResourceRangeLimitStart(
        dayjs.tz('2024-03-31 00:00:00', BEIJING_TIMEZONE).unix(),
        'day'
      )
    ).toBe(dayjs.tz('2024-03-01 00:00:00', BEIJING_TIMEZONE).unix())
  })

  test('aligns manually entered timestamps to the selected granularity', () => {
    expect(alignSiteResourceTimestamp(now, 'hour')).toBe(
      dayjs.tz('2026-08-11 12:00:00', BEIJING_TIMEZONE).unix()
    )
    expect(alignSiteResourceTimestamp(now, 'day')).toBe(
      dayjs.tz('2026-08-11 00:00:00', BEIJING_TIMEZONE).unix()
    )
  })
})
