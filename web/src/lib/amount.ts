import Decimal from 'decimal.js'

import type { MetricString, RateInfo } from './api-types'

Decimal.set({
  precision: 50,
  rounding: Decimal.ROUND_HALF_UP,
  toExpNeg: -30,
  toExpPos: 40,
})

export const DEFAULT_AMOUNT_FRACTION_DIGITS = 2
export const PRECISE_AMOUNT_FRACTION_DIGITS = 6

export type AmountStatus =
  | 'available'
  | 'quota_unavailable'
  | 'rate_unavailable'
  | 'partial_rate_unavailable'

export interface QuotaAmount {
  quota: Decimal | null
  amountUsd: Decimal | null
  amountCny: Decimal | null
  rateSource: RateInfo['source']
  status: AmountStatus
}

export interface SiteQuotaInput {
  siteId: string
  quota: MetricString | string | null
  rate: RateInfo
}

export interface CrossSiteQuotaAmount extends QuotaAmount {
  sites: ReadonlyArray<SiteQuotaInput & { amount: QuotaAmount }>
}

function parseDecimal(value: string | null): Decimal | null {
  if (value == null || value.trim() === '') return null
  try {
    const parsed = new Decimal(value)
    return parsed.isFinite() ? parsed : null
  } catch {
    return null
  }
}

function parsePositiveDecimal(value: string | null): Decimal | null {
  const parsed = parseDecimal(value)
  return parsed?.isPositive() ? parsed : null
}

export function calculateQuotaAmount(
  quotaValue: MetricString | string | null,
  rate: RateInfo
): QuotaAmount {
  const quota = parseDecimal(quotaValue)
  if (quota == null) {
    return {
      quota: null,
      amountUsd: null,
      amountCny: null,
      rateSource: rate.source,
      status: 'quota_unavailable',
    }
  }

  const quotaPerUnit = parsePositiveDecimal(rate.quota_per_unit)
  const usdExchangeRate = parsePositiveDecimal(rate.usd_exchange_rate)
  if (
    rate.source === 'unavailable' ||
    quotaPerUnit == null ||
    usdExchangeRate == null
  ) {
    return {
      quota,
      amountUsd: null,
      amountCny: null,
      rateSource: 'unavailable',
      status: 'rate_unavailable',
    }
  }

  const amountUsd = quota.div(quotaPerUnit)
  return {
    quota,
    amountUsd,
    amountCny: amountUsd.mul(usdExchangeRate),
    rateSource: rate.source,
    status: 'available',
  }
}

export function calculateCrossSiteQuotaAmount(
  inputs: readonly SiteQuotaInput[]
): CrossSiteQuotaAmount {
  const sites = inputs.map((input) => ({
    ...input,
    amount: calculateQuotaAmount(input.quota, input.rate),
  }))

  let quota = new Decimal(0)
  let amountUsd = new Decimal(0)
  let amountCny = new Decimal(0)
  let quotaUnavailable = false
  let nonZeroRateUnavailable = false

  for (const site of sites) {
    const amount = site.amount
    if (amount.quota == null) {
      quotaUnavailable = true
      continue
    }

    quota = quota.add(amount.quota)
    if (amount.amountUsd != null && amount.amountCny != null) {
      amountUsd = amountUsd.add(amount.amountUsd)
      amountCny = amountCny.add(amount.amountCny)
    } else if (!amount.quota.isZero()) {
      nonZeroRateUnavailable = true
    }
  }

  if (quotaUnavailable) {
    return {
      quota: null,
      amountUsd: null,
      amountCny: null,
      rateSource: 'unavailable',
      status: 'quota_unavailable',
      sites,
    }
  }

  if (nonZeroRateUnavailable) {
    return {
      quota,
      amountUsd: null,
      amountCny: null,
      rateSource: 'unavailable',
      status: 'partial_rate_unavailable',
      sites,
    }
  }

  const usedFallback = sites.some(
    ({ amount }) => amount.rateSource === 'fallback'
  )
  return {
    quota,
    amountUsd,
    amountCny,
    rateSource: usedFallback ? 'fallback' : 'site',
    status: 'available',
    sites,
  }
}

export function formatDecimal(
  value: Decimal | null,
  fractionDigits = DEFAULT_AMOUNT_FRACTION_DIGITS
): string | null {
  return value?.toFixed(fractionDigits) ?? null
}

export function decimalToChartNumber(value: Decimal | null): number | null {
  if (value == null) return null
  const numericValue = value.toNumber()
  return Number.isFinite(numericValue) ? numericValue : null
}
