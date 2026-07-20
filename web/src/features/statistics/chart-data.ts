import Decimal from 'decimal.js'

import {
  calculateCrossSiteQuotaAmount,
  type CrossSiteQuotaAmount,
} from '@/lib/amount'
import { formatBeijingTimestamp } from '@/lib/dayjs'

import type {
  SiteQuotaBreakdown,
  StatisticsDisplay,
  StatisticsGranularity,
  StatisticsMetric,
  TrendPoint,
} from './types'

export interface TrendChartSiteAmount {
  siteId: string
  siteName: string
  quota: string | null
  quotaPerUnit: string | null
  usdExchangeRate: string | null
  amountUsd: string | null
  amountCny: string | null
  dataStatus: SiteQuotaBreakdown['data_status']
  rateSource: SiteQuotaBreakdown['rate_source']
  rateUpdatedAt: number | null
}

export interface TrendChartDatum extends TrendPoint {
  chartValue: number | null
  exactValue: string | null
  label: string
  partial: boolean
  rawValue: string | null
  siteAmounts: TrendChartSiteAmount[]
}

export interface TrendChartModel {
  baseline: string
  scale: string
  values: TrendChartDatum[]
}

function crossSiteAmount(
  siteBreakdown: SiteQuotaBreakdown[]
): CrossSiteQuotaAmount {
  return calculateCrossSiteQuotaAmount(
    siteBreakdown.map((site) => ({
      quota: site.quota,
      rate: {
        quota_per_unit: site.quota_per_unit,
        source: site.rate_source,
        updated_at: site.rate_updated_at,
        usd_exchange_rate: site.usd_exchange_rate,
      },
      siteId: site.site_id,
    }))
  )
}

function parseDecimal(value: string | null): Decimal | null {
  if (value == null) return null
  try {
    const parsed = new Decimal(value)
    return parsed.isFinite() ? parsed : null
  } catch {
    return null
  }
}

function displayDecimal(
  point: TrendPoint,
  metric: StatisticsMetric,
  display: StatisticsDisplay
): Decimal | null {
  if (metric !== 'quota' || display === 'quota') {
    return parseDecimal(point[metric])
  }
  const amount = crossSiteAmount(point.site_breakdown)
  if (amount.status !== 'available') return null
  return display === 'usd' ? amount.amountUsd : amount.amountCny
}

function exactValue(
  value: Decimal | null,
  metric: StatisticsMetric,
  display: StatisticsDisplay
): string | null {
  if (value == null) return null
  return metric === 'quota' && display !== 'quota'
    ? value.toFixed(6)
    : value.toFixed()
}

function siteAmounts(point: TrendPoint): TrendChartSiteAmount[] {
  const amount = crossSiteAmount(point.site_breakdown)
  return point.site_breakdown.map((site, index) => ({
    amountCny: amount.sites[index]?.amount.amountCny?.toFixed(6) ?? null,
    amountUsd: amount.sites[index]?.amount.amountUsd?.toFixed(6) ?? null,
    dataStatus: site.data_status,
    quota: site.quota,
    quotaPerUnit: site.quota_per_unit,
    rateSource: site.rate_source,
    rateUpdatedAt: site.rate_updated_at,
    siteId: site.site_id,
    siteName: site.site_name,
    usdExchangeRate: site.usd_exchange_rate,
  }))
}

function scaleFor(values: Decimal[]): { baseline: Decimal; scale: Decimal } {
  if (values.length === 0) {
    return { baseline: new Decimal(0), scale: new Decimal(1) }
  }
  const unsafe = values.some((value) =>
    value.abs().greaterThan(Number.MAX_SAFE_INTEGER)
  )
  const baseline = unsafe ? Decimal.min(...values) : new Decimal(0)
  const maxOffset = Decimal.max(
    ...values.map((value) => value.minus(baseline).abs())
  )
  const exponent = maxOffset.isZero() ? 0 : Math.max(0, maxOffset.e - 12)
  return { baseline, scale: new Decimal(10).pow(exponent) }
}

export function buildTrendChartModel(
  points: TrendPoint[],
  metric: StatisticsMetric,
  display: StatisticsDisplay,
  granularity: StatisticsGranularity
): TrendChartModel {
  const decimalValues = points.map((point) =>
    displayDecimal(point, metric, display)
  )
  const { baseline, scale } = scaleFor(
    decimalValues.filter((value): value is Decimal => value != null)
  )
  return {
    baseline: baseline.toFixed(),
    scale: scale.toFixed(),
    values: points.map((point, index) => {
      const value = decimalValues[index] ?? null
      return {
        ...point,
        chartValue:
          value == null
            ? null
            : value
                .minus(baseline)
                .div(scale)
                .toSignificantDigits(15)
                .toNumber(),
        exactValue: exactValue(value, metric, display),
        label: formatBeijingTimestamp(point.bucket_start, granularity),
        partial: point.data_status === 'partial' || !point.is_final,
        rawValue: point[metric],
        siteAmounts: siteAmounts(point),
      }
    }),
  }
}
