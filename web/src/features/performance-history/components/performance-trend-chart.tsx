import { VChart, type ILineChartSpec } from '@visactor/react-vchart'
import { useMemo } from 'react'

import { useTheme } from '@/context/theme-provider'

import {
  buildPerformanceTrendValues,
  hasRenderablePerformanceTrendValues,
} from '../performance-trend-chart-data'
import type { PerformanceHistoryItem } from '../types'

export function PerformanceTrendChart({
  ariaLabel,
  emptyText,
  items,
  latencyLabel,
  ttftLabel,
}: {
  ariaLabel: string
  emptyText: string
  items: PerformanceHistoryItem[]
  latencyLabel: string
  ttftLabel: string
}) {
  const { resolvedTheme } = useTheme()
  const values = useMemo(
    () => buildPerformanceTrendValues(items, latencyLabel, ttftLabel),
    [items, latencyLabel, ttftLabel]
  )
  const spec = useMemo<ILineChartSpec>(
    () => ({
      axes: [
        { orient: 'bottom', type: 'band' },
        { orient: 'left', type: 'linear' },
      ],
      data: [{ id: 'performance-trend', values }],
      invalidType: 'break',
      legends: { orient: 'bottom', visible: true },
      line: { style: { lineWidth: 2 } },
      point: { style: { size: 5 }, visible: true },
      seriesField: 'series',
      theme: resolvedTheme === 'dark' ? 'dark' : 'light',
      tooltip: { activeType: 'dimension', visible: true },
      type: 'line',
      xField: 'bucket',
      yField: 'value',
    }),
    [resolvedTheme, values]
  )

  if (!hasRenderablePerformanceTrendValues(values)) {
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
        className='h-full min-h-96 w-full min-w-0 overflow-hidden'
        role='img'
      >
        <VChart spec={spec} />
      </div>
    </figure>
  )
}
