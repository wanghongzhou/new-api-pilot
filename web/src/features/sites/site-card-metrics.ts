import type { SitePerformanceModel } from './types'

const resourceGradientStops = [
  { hue: 145, value: 0 },
  { hue: 105, value: 55 },
  { hue: 80, value: 75 },
  { hue: 50, value: 90 },
  { hue: 25, value: 100 },
] as const

export function isSitePerformanceReady(
  dataStatus: 'ready' | 'unavailable'
): boolean {
  return dataStatus === 'ready'
}

export interface SitePerformanceDashboardSummary {
  avgLatencyMs: number | null
  successRate: number | null
  throughput: number | null
}

function simplePerformanceAverage(
  models: SitePerformanceModel[],
  metric: 'success_rate' | 'avg_latency_ms' | 'avg_tps',
  isValid: (value: number) => boolean
): number | null {
  const values = models
    .map((model) => model[metric])
    .filter((value) => Number.isFinite(value) && isValid(value))
  if (values.length === 0) return null
  return values.reduce((total, value) => total + value, 0) / values.length
}

export function sitePerformanceDashboardSummary(
  models: SitePerformanceModel[] | null | undefined
): SitePerformanceDashboardSummary {
  const rows = models ?? []
  const avgLatencyMs = simplePerformanceAverage(
    rows,
    'avg_latency_ms',
    (value) => value > 0
  )
  return {
    avgLatencyMs: avgLatencyMs == null ? null : Math.round(avgLatencyMs),
    successRate: simplePerformanceAverage(rows, 'success_rate', () => true),
    throughput: simplePerformanceAverage(rows, 'avg_tps', (value) => value > 0),
  }
}

export function formatPerformanceLatency(
  valueMs: number | null,
  unavailableLabel: string
): string {
  if (valueMs == null || !Number.isFinite(valueMs) || valueMs <= 0) {
    return unavailableLabel
  }
  if (valueMs >= 1000) return `${(valueMs / 1000).toFixed(2)}s`
  return `${Math.round(valueMs)}ms`
}

export function formatPerformanceThroughput(
  value: number | null,
  unavailableLabel: string
): string {
  if (value == null || !Number.isFinite(value) || value <= 0) {
    return unavailableLabel
  }
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K t/s`
  return `${value.toFixed(value < 10 ? 2 : 1)} t/s`
}

export function formatPerformanceSuccessRate(
  value: number | null,
  unavailableLabel: string
): string {
  return value == null || !Number.isFinite(value)
    ? unavailableLabel
    : `${value.toFixed(2)}%`
}

export function formatAverageRate(value: string | null): string {
  if (value == null) return '0.00'
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed.toFixed(2) : '0.00'
}

function formatHue(value: number) {
  return String(Number(value.toFixed(2)))
}

export function formatLatencySeconds(valueMs: number): string {
  if (!Number.isFinite(valueMs)) return '0'
  return String(Number((valueMs / 1000).toFixed(2)))
}

export function formatPercentValue(
  value: number | null,
  unavailableLabel: string
): string {
  return value == null || !Number.isFinite(value)
    ? unavailableLabel
    : `${value.toFixed(1)}%`
}

export function formatInstanceAvailability(
  online: number | null,
  total: number | null,
  unavailableLabel: string
): string {
  return online == null || total == null
    ? unavailableLabel
    : `${online}/${total}`
}

export function siteResourceColor(value: number | null): string | undefined {
  if (value == null || !Number.isFinite(value)) return undefined
  const bounded = Math.max(0, Math.min(100, value))
  for (let index = 0; index < resourceGradientStops.length - 1; index += 1) {
    const lower = resourceGradientStops[index]
    const upper = resourceGradientStops[index + 1]
    if (bounded <= upper.value) {
      const progress = (bounded - lower.value) / (upper.value - lower.value)
      const hue = lower.hue + (upper.hue - lower.hue) * progress
      return `oklch(var(--resource-metric-lightness) var(--resource-metric-chroma) ${formatHue(hue)})`
    }
  }
  return `oklch(var(--resource-metric-lightness) var(--resource-metric-chroma) 25)`
}
