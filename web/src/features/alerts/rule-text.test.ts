import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import {
  alertMessageForDisplay,
  formatAlertCurrentValue,
  formatAlertThreshold,
} from './rule-text'

describe('alert threshold text', () => {
  test.each([
    ['85.00', '85'],
    ['72.50', '72.5'],
    ['0.99', '0.99'],
  ])('formats %s with at most two decimal places', (value, expected) => {
    expect(formatAlertThreshold(value)).toBe(expected)
  })

  test('keeps an unexpected value visible instead of throwing', () => {
    expect(formatAlertThreshold('unavailable')).toBe('unavailable')
  })
})

test('formats alert message value evidence without mutating the source', () => {
  const message = {
    code: 'ALERT_CPU_HIGH' as const,
    params: {
      site_id: '1' as never,
      target_name: 'node-a',
      target_type: 'instance',
      threshold: '85.0000000000',
      value: '92.1267890000',
    },
    technical_detail: '',
  }
  const formatted = alertMessageForDisplay(message)
  const formattedParams = formatted.params as Record<string, unknown>

  expect(formattedParams.value).toBe('92.13')
  expect(formattedParams.threshold).toBe('85')
  expect(message.params.value).toBe('92.1267890000')
})

describe('alert current value text', () => {
  test.each([
    ['92.5000000000', '92.5'],
    ['0.1267890000', '0.13'],
    ['1.0000000000', '1'],
  ])(
    'formats %s for display without changing stored precision',
    (value, expected) => {
      expect(formatAlertCurrentValue(value)).toBe(expected)
    }
  )

  test('keeps an unexpected current value visible instead of throwing', () => {
    expect(formatAlertCurrentValue('unavailable')).toBe('unavailable')
  })
})

test('uses remote account balance wording for the empty account rule', async () => {
  const locale = JSON.parse(
    await readFile(
      new URL('../../i18n/locales/zh-CN.json', import.meta.url),
      'utf8'
    )
  ) as Record<string, string>

  expect(locale['alerts.rule.account_quota_empty']).toBe('远程账户余额为空')
  expect(locale['alerts.ruleDescription.account_quota_empty']).toContain(
    '远程账户的当前余额'
  )
  expect(locale.ALERT_ACCOUNT_QUOTA_EMPTY).toContain('没有可用余额')
})
