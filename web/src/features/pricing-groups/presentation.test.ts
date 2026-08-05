import { describe, expect, test } from 'bun:test'

import {
  hiddenPricingValueCount,
  keyedPricingValues,
  PRICING_DISCLOSURE_ITEM_LIMIT,
  PRICING_DISCLOSURE_TEXT_LIMIT,
  visiblePricingText,
  visiblePricingValues,
} from './presentation'

describe('pricing presentation disclosure', () => {
  test('bounds extreme lists without mutating their exact values', () => {
    const values = Array.from(
      { length: 50 },
      (_, index) => `${'very-long-model-'.repeat(20)}${index}`
    )

    expect(visiblePricingValues(values, false)).toEqual(
      values.slice(0, PRICING_DISCLOSURE_ITEM_LIMIT)
    )
    expect(hiddenPricingValueCount(values, false)).toBe(
      values.length - PRICING_DISCLOSURE_ITEM_LIMIT
    )
    expect(visiblePricingValues(values, true)).toEqual(values)
    expect(hiddenPricingValueCount(values, true)).toBe(0)
  })

  test('collapses very long audit text and restores the exact source text', () => {
    const value = `https://icons.invalid/${'segment/'.repeat(100)}icon.svg`
    const collapsed = visiblePricingText(value, false)

    expect(collapsed.endsWith('…')).toBe(true)
    expect(collapsed.length).toBe(PRICING_DISCLOSURE_TEXT_LIMIT + 1)
    expect(visiblePricingText(value, true)).toBe(value)
    expect(visiblePricingText('short', false)).toBe('short')
  })

  test('keeps duplicate badge values with stable data-derived keys', () => {
    expect(keyedPricingValues(['vip', 'vip', 'default'])).toEqual([
      { key: 'vip\u00001', value: 'vip' },
      { key: 'vip\u00002', value: 'vip' },
      { key: 'default\u00001', value: 'default' },
    ])
  })
})
