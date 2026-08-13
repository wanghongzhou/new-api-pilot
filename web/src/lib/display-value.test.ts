import { describe, expect, test } from 'bun:test'

import { appLanguage, resources } from '@/i18n/config'

import {
  EMPTY_DISPLAY_VALUE,
  EMPTY_NUMERIC_DISPLAY_VALUE,
  formatDisplayValue,
  formatDecimalDisplayValue,
  formatNumericDisplayValue,
  formatMetricDisplayValue,
} from './display-value'

describe('formatDisplayValue', () => {
  test('uses one placeholder for missing scalar values', () => {
    expect(formatDisplayValue(null)).toBe(EMPTY_DISPLAY_VALUE)
    expect(formatDisplayValue(undefined)).toBe(EMPTY_DISPLAY_VALUE)
    expect(formatDisplayValue('')).toBe(EMPTY_DISPLAY_VALUE)
    expect(formatDisplayValue('   ')).toBe(EMPTY_DISPLAY_VALUE)
  })

  test('preserves meaningful falsy values', () => {
    expect(formatDisplayValue(0)).toBe('0')
    expect(formatDisplayValue('0')).toBe('0')
    expect(formatDisplayValue(false)).toBe('false')
  })

  test('keeps generic translated placeholders aligned with the contract', () => {
    const translations = resources[appLanguage].translation
    expect(translations['data.unavailableValue']).toBe(
      EMPTY_NUMERIC_DISPLAY_VALUE
    )
    expect(translations['alerts.value.unavailable']).toBe(EMPTY_DISPLAY_VALUE)
    expect(translations['statistics.metric.active_users_unavailable']).toBe(
      EMPTY_NUMERIC_DISPLAY_VALUE
    )
    expect(translations['data.unavailable']).toBe(EMPTY_DISPLAY_VALUE)
  })
})

describe('formatNumericDisplayValue', () => {
  test('uses zero for missing numeric values', () => {
    expect(formatNumericDisplayValue(null)).toBe(EMPTY_NUMERIC_DISPLAY_VALUE)
    expect(formatNumericDisplayValue(undefined)).toBe(
      EMPTY_NUMERIC_DISPLAY_VALUE
    )
    expect(formatNumericDisplayValue('')).toBe(EMPTY_NUMERIC_DISPLAY_VALUE)
  })

  test('preserves provided numeric values', () => {
    expect(formatNumericDisplayValue(0)).toBe('0')
    expect(formatNumericDisplayValue(12)).toBe('12')
    expect(formatNumericDisplayValue('42')).toBe('42')
  })
})

describe('formatDecimalDisplayValue', () => {
  test.each([
    ['1000000.0000000000', '1,000,000'],
    ['1000.5000000000', '1,000.5'],
    ['0.0000000001', '0.0000000001'],
    ['0', '0'],
  ])('formats %s without losing meaningful precision', (value, expected) => {
    expect(formatDecimalDisplayValue(value)).toBe(expected)
  })

  test('uses zero for invalid or absent decimals', () => {
    expect(formatDecimalDisplayValue(null)).toBe(EMPTY_NUMERIC_DISPLAY_VALUE)
    expect(formatDecimalDisplayValue('not-a-number')).toBe(
      EMPTY_NUMERIC_DISPLAY_VALUE
    )
  })

  test('rounds display-only decimals without retaining storage scale', () => {
    expect(formatDecimalDisplayValue('3042.7560000000', 2)).toBe('3,042.76')
    expect(formatDecimalDisplayValue('0.0000000000', 4)).toBe('0')
  })
})

describe('formatMetricDisplayValue', () => {
  test('groups bigint strings without converting through number', () => {
    expect(formatMetricDisplayValue('9007199254740993')).toBe(
      '9,007,199,254,740,993'
    )
    expect(formatMetricDisplayValue('0')).toBe('0')
  })

  test('keeps invalid input visible and renders missing input as zero', () => {
    expect(formatMetricDisplayValue('not-a-metric')).toBe('not-a-metric')
    expect(formatMetricDisplayValue(null)).toBe(EMPTY_NUMERIC_DISPLAY_VALUE)
  })
})
