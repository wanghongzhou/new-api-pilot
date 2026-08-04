import { describe, expect, test } from 'bun:test'

import { ratioToPercent } from './presentation'

describe('local ranking percentage presentation', () => {
  test.each([
    ['1.0000000000', '100%'],
    ['0.3333333333', '33.33333333%'],
    ['0.1250000000', '12.5%'],
    ['-0.2500000000', '-25%'],
    ['0', '0%'],
  ])('formats ratio %s as %s', (value, expected) => {
    expect(ratioToPercent(value)).toBe(expected)
  })

  test('preserves unavailable growth', () => {
    expect(ratioToPercent(null)).toBeNull()
  })
})
