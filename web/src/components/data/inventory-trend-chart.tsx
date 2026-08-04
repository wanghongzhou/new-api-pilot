import type { ILineChartSpec } from '@visactor/react-vchart'
import { lazy, Suspense, useMemo } from 'react'

import { useTheme } from '@/context/theme-provider'
import { fromUnixSeconds } from '@/lib/dayjs'

const LazyVChart = lazy(() =>
  import('@visactor/react-vchart').then((module) => ({
    default: module.VChart,
  }))
)

export interface InventoryTrendSeries {
  key: string
  label: string
}

export interface InventoryTrendChartPoint {
  bucketStart: number
  values: Record<string, string>
}

export function InventoryTrendChart({
  ariaLabel,
  emptyText,
  points,
  series,
}: {
  ariaLabel: string
  emptyText: string
  points: InventoryTrendChartPoint[]
  series: InventoryTrendSeries[]
}) {
  const { resolvedTheme } = useTheme()
  const values = useMemo(
    () =>
      points.flatMap((point) =>
        series.map((item) => {
          const rawValue = point.values[item.key] ?? '0'
          return {
            label: fromUnixSeconds(point.bucketStart).format('MM-DD HH:mm'),
            rawValue,
            series: item.label,
            value: Number(rawValue),
          }
        })
      ),
    [points, series]
  )
  const spec = useMemo<ILineChartSpec>(
    () => ({
      axes: [
        { orient: 'bottom', type: 'band' },
        { orient: 'left', type: 'linear' },
      ],
      data: [{ id: 'inventory-trend', values }],
      legends: { orient: 'bottom', visible: true },
      line: { style: { lineWidth: 2 } },
      point: { style: { size: 6 }, visible: true },
      seriesField: 'series',
      theme: resolvedTheme === 'dark' ? 'dark' : 'light',
      tooltip: { activeType: 'dimension', visible: true },
      type: 'line',
      xField: 'label',
      yField: 'value',
    }),
    [resolvedTheme, values]
  )

  if (points.length === 0) {
    return (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {emptyText}
      </p>
    )
  }
  return (
    <figure className='grid min-h-0 min-w-0 flex-1 gap-2'>
      <div
        aria-label={ariaLabel}
        className='h-full min-h-80 w-full min-w-0 overflow-hidden'
        role='img'
      >
        <Suspense
          fallback={
            <div className='bg-muted h-full animate-pulse rounded-md' />
          }
        >
          <LazyVChart spec={spec} />
        </Suspense>
      </div>
    </figure>
  )
}
