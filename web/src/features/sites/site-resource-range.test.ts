import { describe, expect, test } from 'bun:test'

import { dayjs, BEIJING_TIMEZONE } from '@/lib/dayjs'

import {
  alignSiteResourceTimestamp,
  defaultSiteResourceRange,
} from './site-resource-range'

const now = dayjs.tz('2026-08-11 12:57:28', BEIJING_TIMEZONE).unix()

describe('site resource ranges', () => {
  test('uses the last 24 closed hours worth of minute buckets', () => {
    const range = defaultSiteResourceRange('minute', now)
    expect(range.end).toBe(
      dayjs.tz('2026-08-11 12:57:00', BEIJING_TIMEZONE).unix()
    )
    expect(range.end - range.start).toBe(24 * 60 * 60)
  })

  test('uses seven days ending on a closed Beijing hour', () => {
    const range = defaultSiteResourceRange('hour', now)
    expect(range.end).toBe(
      dayjs.tz('2026-08-11 12:00:00', BEIJING_TIMEZONE).unix()
    )
    expect(range.end - range.start).toBe(7 * 24 * 60 * 60)
  })

  test('uses a 30-day window aligned to Beijing day boundaries', () => {
    const range = defaultSiteResourceRange('day', now)
    expect(range.end).toBe(
      dayjs.tz('2026-08-11 00:00:00', BEIJING_TIMEZONE).unix()
    )
    expect(range.start).toBe(
      dayjs.tz('2026-07-12 00:00:00', BEIJING_TIMEZONE).unix()
    )
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
