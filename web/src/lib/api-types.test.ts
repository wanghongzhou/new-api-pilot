import { describe, expect, test } from 'bun:test'

import { isPricingDecimalString, parsePricingDecimalString } from './api-types'

describe('pricing decimal strings', () => {
  test('preserves the backend pricing precision contract', () => {
    const value = '12345678901234567890.123456789012345678'
    expect(isPricingDecimalString(value)).toBe(true)
    expect(String(parsePricingDecimalString(value))).toBe(value)
  })

  test('rejects negative, exponent, and over-precision values', () => {
    for (const value of ['-1', '1e-8', '01', '1.1234567890123456789']) {
      expect(isPricingDecimalString(value)).toBe(false)
      expect(() => parsePricingDecimalString(value)).toThrow(TypeError)
    }
  })
})
