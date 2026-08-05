import type { DataStatus } from '@/lib/api-types'
import { fromUnixSeconds } from '@/lib/dayjs'

export interface InventoryTrendSeries {
  key: string
  label: string
}

export interface InventoryTrendChartPoint {
  bucketStart: number
  dataStatus: DataStatus
  values: Record<string, string>
}

export interface InventoryTrendChartValue {
  dataStatus: DataStatus
  label: string
  rawValue: string | null
  series: string
  value: number | null
}

const renderableStatuses = new Set<DataStatus>(['complete', 'partial'])

export function buildInventoryTrendChartValues(
  points: InventoryTrendChartPoint[],
  series: InventoryTrendSeries[]
): InventoryTrendChartValue[] {
  return points.flatMap((point) =>
    series.map((item) => {
      const rawValue = point.values[item.key]
      const numericValue =
        rawValue === undefined || rawValue.trim() === ''
          ? Number.NaN
          : Number(rawValue)
      const value =
        renderableStatuses.has(point.dataStatus) &&
        Number.isFinite(numericValue)
          ? numericValue
          : null

      return {
        dataStatus: point.dataStatus,
        label: fromUnixSeconds(point.bucketStart).format('MM-DD HH:mm'),
        rawValue: value === null ? null : rawValue,
        series: item.label,
        value,
      }
    })
  )
}

export function shouldShowInventoryTrendPoints(pointCount: number) {
  return pointCount <= 48
}
