import { fromUnixSeconds } from '@/lib/dayjs'

import { millisecondsToSeconds } from './presentation'
import type { PerformanceHistoryItem } from './types'

export interface PerformanceTrendValue {
  bucket: string
  rawValue: string | null
  series: string
  value: number | null
}

function finiteMetric(value: string | null): number | null {
  if (value == null || value.trim() === '') return null
  const numeric = Number(value)
  return Number.isFinite(numeric) ? numeric : null
}

function secondsMetric(value: string | null): {
  rawValue: string | null
  value: number | null
} {
  try {
    const rawValue = millisecondsToSeconds(value)
    return { rawValue, value: finiteMetric(rawValue) }
  } catch {
    return { rawValue: null, value: null }
  }
}

export function buildPerformanceTrendValues(
  items: PerformanceHistoryItem[],
  latencyLabel: string,
  ttftLabel: string
): PerformanceTrendValue[] {
  return items.flatMap((item) => {
    const identity = `${item.site_name} · ${item.model_name} / ${item.group || '-'}`
    const bucket = fromUnixSeconds(item.bucket_start).format('MM-DD HH:mm')
    const latency = secondsMetric(item.avg_latency_ms)
    const ttft = secondsMetric(item.avg_ttft_ms)

    return [
      {
        bucket,
        rawValue: latency.rawValue,
        series: `${latencyLabel} · ${identity}`,
        value: latency.value,
      },
      {
        bucket,
        rawValue: ttft.rawValue,
        series: `${ttftLabel} · ${identity}`,
        value: ttft.value,
      },
    ]
  })
}

export function hasRenderablePerformanceTrendValues(
  values: PerformanceTrendValue[]
): boolean {
  return values.some((item) => item.value != null)
}
