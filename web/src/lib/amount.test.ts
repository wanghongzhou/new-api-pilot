import { describe, expect, test } from 'bun:test'

import {
  calculateCrossSiteQuotaAmount,
  calculateQuotaAmount,
  formatDecimal,
} from './amount'
import type { RateInfo } from './api-types'

const siteRate: RateInfo = {
  quota_per_unit: '500000',
  usd_exchange_rate: '7.3',
  source: 'site',
  updated_at: 1_720_003_230,
}

describe('quota amount calculations', () => {
  test('uses Decimal for exact site conversion', () => {
    const result = calculateQuotaAmount('64000000', siteRate)
    expect(formatDecimal(result.amountUsd, 6)).toBe('128.000000')
    expect(formatDecimal(result.amountCny, 6)).toBe('934.400000')
  })

  test('invalidates a cross-site total when a non-zero rate is unavailable', () => {
    const result = calculateCrossSiteQuotaAmount([
      { siteId: '1', quota: '500000', rate: siteRate },
      {
        siteId: '2',
        quota: '1',
        rate: {
          quota_per_unit: null,
          usd_exchange_rate: null,
          source: 'unavailable',
          updated_at: null,
        },
      },
    ])
    expect(result.status).toBe('partial_rate_unavailable')
    expect(result.amountUsd).toBeNull()
    expect(result.amountCny).toBeNull()
    expect(result.sites[0]?.amount.amountUsd?.toFixed(0)).toBe('1')
  })

  test('allows zero-quota sites to have an unavailable rate', () => {
    const result = calculateCrossSiteQuotaAmount([
      {
        siteId: '1',
        quota: '0',
        rate: {
          quota_per_unit: null,
          usd_exchange_rate: null,
          source: 'unavailable',
          updated_at: null,
        },
      },
    ])
    expect(result.status).toBe('available')
    expect(result.amountUsd?.toFixed()).toBe('0')
  })
})
