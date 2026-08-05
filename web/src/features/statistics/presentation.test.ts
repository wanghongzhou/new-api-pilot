import { describe, expect, test } from 'bun:test'

import {
  formatStatisticsDecimal,
  formatStatisticsRangeLabel,
} from './presentation'

describe('statistics presentation', () => {
  test.each([
    ['500000.0000000000', '500000'],
    ['6.8200000000', '6.82'],
    ['0.0000000001', '0.0000000001'],
  ])('removes meaningless decimal zeroes from %s', (value, expected) => {
    expect(formatStatisticsDecimal(value)).toBe(expected)
  })

  test('uses a readable label while keeping datetime-local values internal', () => {
    expect(
      formatStatisticsRangeLabel(1_725_285_600, 1_725_289_200, 'hour')
    ).toBe('2024-09-02 22:00 ~ 2024-09-02 23:00')
  })
})
