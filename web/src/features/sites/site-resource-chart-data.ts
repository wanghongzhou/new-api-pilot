import type { ResourcePoint } from './types'

export interface SiteResourceChartValue {
  health: string
  time: string
  value: number | null
}

export function siteResourceValue(
  point: ResourcePoint,
  metric: 'cpu' | 'disk' | 'memory',
  aggregation: 'avg' | 'last' | 'max'
): number | null {
  let value: number | null
  if (metric === 'cpu') {
    value =
      aggregation === 'avg' ? point.cpu_avg_percent : point.cpu_max_percent
  } else if (metric === 'memory') {
    value =
      aggregation === 'avg'
        ? point.memory_avg_percent
        : point.memory_max_percent
  } else {
    value =
      aggregation === 'last'
        ? point.disk_last_used_percent
        : point.disk_max_used_percent
  }
  return value != null && Number.isFinite(value) ? value : null
}

export function hasRenderableSiteResourceValues(
  values: SiteResourceChartValue[]
): boolean {
  return values.some((item) => item.value != null)
}
