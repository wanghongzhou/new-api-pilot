import Decimal from 'decimal.js'

import { fromUnixSeconds } from '@/lib/dayjs'

import type { StatisticsGranularity } from './types'

export function formatStatisticsDecimal(value: string | null): string | null {
  if (value == null) return null
  const decimal = new Decimal(value)
  return decimal.toFixed(decimal.decimalPlaces())
}

export function formatStatisticsRangeLabel(
  start: number,
  end: number,
  granularity: StatisticsGranularity
): string {
  let format = 'YYYY-MM-DD'
  if (granularity === 'hour') format = 'YYYY-MM-DD HH:mm'
  else if (granularity === 'month') format = 'YYYY-MM'
  else if (granularity === 'year') format = 'YYYY'
  return `${fromUnixSeconds(start).format(format)} ~ ${fromUnixSeconds(end).format(format)}`
}
