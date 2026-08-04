import type { ILineChartSpec } from '@visactor/react-vchart'
import { lazy, Suspense, useMemo } from 'react'

import { useTheme } from '@/context/theme-provider'
import { fromUnixSeconds } from '@/lib/dayjs'

import { millisecondsToSeconds } from '../presentation'
import type { PerformanceHistoryItem } from '../types'

const LazyVChart = lazy(() =>
  import('@visactor/react-vchart').then((module) => ({
    default: module.VChart,
  }))
)

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
    () =>
      items.flatMap((item) => {
        const identity = `${item.site_name} · ${item.model_name} / ${item.group || '-'}`
        const bucket = fromUnixSeconds(item.bucket_start).format('MM-DD HH:mm')
        const latency = millisecondsToSeconds(item.avg_latency_ms) ?? '0'
        const ttft = millisecondsToSeconds(item.avg_ttft_ms) ?? '0'
        return [
          {
            bucket,
            rawValue: latency,
            series: `${latencyLabel} · ${identity}`,
            value: Number(latency),
          },
          {
            bucket,
            rawValue: ttft,
            series: `${ttftLabel} · ${identity}`,
            value: Number(ttft),
          },
        ]
      }),
    [items, latencyLabel, ttftLabel]
  )
  const spec = useMemo<ILineChartSpec>(
    () => ({
      axes: [
        { orient: 'bottom', type: 'band' },
        { orient: 'left', type: 'linear' },
      ],
      data: [{ id: 'performance-trend', values }],
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

  if (items.length === 0) {
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
