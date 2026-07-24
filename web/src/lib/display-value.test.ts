import { describe, expect, test } from 'bun:test'

import { appLanguage, resources } from '@/i18n/config'

import {
  EMPTY_DISPLAY_VALUE,
  EMPTY_NUMERIC_DISPLAY_VALUE,
  formatDisplayValue,
  formatNumericDisplayValue,
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
    expect(translations['data.unavailableValue']).toBe(EMPTY_DISPLAY_VALUE)
    expect(translations['alerts.value.unavailable']).toBe(EMPTY_DISPLAY_VALUE)
    expect(translations['statistics.metric.active_users_unavailable']).toBe(
      EMPTY_DISPLAY_VALUE
    )
    expect(translations['data.unavailable']).toBe('不可用')
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
