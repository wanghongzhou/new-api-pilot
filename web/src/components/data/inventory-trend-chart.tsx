import { VChart, type ILineChartSpec } from '@visactor/react-vchart'
import { useMemo } from 'react'

import { useTheme } from '@/context/theme-provider'

import {
  buildInventoryTrendChartValues,
  hasRenderableInventoryTrendValues,
  shouldShowInventoryTrendPoints,
  type InventoryTrendChartPoint,
  type InventoryTrendSeries,
} from './inventory-trend-chart-data'

export function InventoryTrendChart({
  ariaLabel,
  description,
  emptyText,
  points,
  series,
}: {
  ariaLabel: string
  description: string
  emptyText: string
  points: InventoryTrendChartPoint[]
  series: InventoryTrendSeries[]
}) {
  const { resolvedTheme } = useTheme()
  const values = useMemo(
    () => buildInventoryTrendChartValues(points, series),
    [points, series]
  )
  const showPoints = shouldShowInventoryTrendPoints(points.length)
  const spec = useMemo<ILineChartSpec>(
    () => ({
      axes: [
        { orient: 'bottom', type: 'band' },
        { orient: 'left', type: 'linear' },
      ],
      data: [{ id: 'inventory-trend', values }],
      invalidType: 'break',
      legends: { orient: 'bottom', visible: true },
      line: { style: { curveType: 'stepAfter', lineWidth: 2 } },
      point: { style: { size: 6 }, visible: showPoints },
      seriesField: 'series',
      theme: resolvedTheme === 'dark' ? 'dark' : 'light',
      tooltip: { activeType: 'dimension', visible: true },
      type: 'line',
      xField: 'label',
      yField: 'value',
    }),
    [resolvedTheme, showPoints, values]
  )

  if (!hasRenderableInventoryTrendValues(values)) {
    return (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {emptyText}
      </p>
    )
  }
  return (
    <figure className='grid min-h-80 min-w-0 gap-2 lg:min-h-0 lg:flex-1'>
      <div
        aria-label={ariaLabel}
        className='h-full min-h-80 w-full min-w-0 overflow-hidden'
        role='img'
      >
        <VChart spec={spec} />
      </div>
      <figcaption className='text-muted-foreground text-xs'>
        {description}
      </figcaption>
    </figure>
  )
}
