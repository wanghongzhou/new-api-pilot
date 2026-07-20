import { describe, expect, test } from 'bun:test'

import {
  alignToBeijingHour,
  formatBeijingTimestamp,
  isBeijingBucketAligned,
  parseBeijingNaturalDay,
} from './dayjs'

describe('Beijing time helpers', () => {
  test('converts a natural day to a left-closed, right-open range', () => {
    const range = parseBeijingNaturalDay('2026-07-13')
    expect(range).toEqual({
      startTimestamp: 1_783_872_000,
      endTimestamp: 1_783_958_400,
    })
  })

  test('aligns and formats Unix seconds in Asia/Shanghai', () => {
    const timestamp = 1_783_877_445
    const aligned = alignToBeijingHour(timestamp)
    expect(isBeijingBucketAligned(aligned, 'hour')).toBeTrue()
    expect(formatBeijingTimestamp(aligned)).toBe('2026-07-13 01:00')
  })
})
