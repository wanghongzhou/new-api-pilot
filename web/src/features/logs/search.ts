import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs } from '@/lib/dayjs'

import type { LogSearch, LogType } from './types'

export const logQuickRanges = ['today', '24h', '7d', '14d'] as const
export type LogQuickRange = (typeof logQuickRanges)[number]

export function mergeLogSearch(
  current: LogSearch,
  changes: Partial<LogSearch>
) {
  const next = { ...current, ...changes }
  for (const key of Object.keys(changes) as (keyof LogSearch)[]) {
    if (changes[key] === undefined) delete next[key]
  }
  return next
}

type LogSearchInput = Omit<
  Partial<LogSearch>,
  'channelId' | 'exportId' | 'siteIds' | 'type'
> & {
  siteIds?: readonly string[]
  channelId?: string
  exportId?: string
  type?: number
}

export function buildLogQuickRange(
  range: LogQuickRange,
  now = dayjs().tz(BEIJING_TIMEZONE)
) {
  const end = now.startOf('hour')
  let start = end.subtract(24, 'hour')
  if (range === 'today') start = end.startOf('day')
  else if (range === '7d') start = end.subtract(7, 'day')
  else if (range === '14d') start = end.subtract(14, 'day')
  return { end: end.unix(), start: start.unix() }
}

export function getLogQuickRange(
  search: Pick<LogSearch, 'end' | 'start'>
): LogQuickRange | 'custom' {
  const duration = search.end - search.start
  const startOfEndDay = dayjs
    .unix(search.end)
    .tz(BEIJING_TIMEZONE)
    .startOf('day')
    .unix()
  if (search.start === startOfEndDay && duration < 24 * 3600) return 'today'
  if (duration === 24 * 3600) return '24h'
  if (duration === 7 * 24 * 3600) return '7d'
  if (duration === 14 * 24 * 3600) return '14d'
  return 'custom'
}

function defaultRange(now = dayjs().tz(BEIJING_TIMEZONE)) {
  return buildLogQuickRange('24h', now)
}

function bounded(value: unknown, limit: number): string {
  if (typeof value !== 'string') return ''
  const trimmed = value.trim()
  return [...trimmed].length <= limit ? trimmed : ''
}

export function buildLogSearch(
  raw: LogSearchInput,
  now = dayjs().tz(BEIJING_TIMEZONE)
): LogSearch {
  const defaults = defaultRange(now)
  const requestedStart = raw.start ?? defaults.start
  const requestedEnd = raw.end ?? defaults.end
  const validRange =
    Number.isInteger(requestedStart) &&
    Number.isInteger(requestedEnd) &&
    requestedStart > 0 &&
    requestedEnd > requestedStart &&
    requestedEnd - requestedStart <= 31 * 24 * 3600
  const type =
    Number.isInteger(raw.type) && Number(raw.type) >= 0 && Number(raw.type) <= 7
      ? (raw.type as LogType)
      : undefined
  const page =
    Number.isInteger(raw.page) && Number(raw.page) > 0 ? Number(raw.page) : 1
  const pageSize =
    Number.isInteger(raw.pageSize) &&
    Number(raw.pageSize) > 0 &&
    Number(raw.pageSize) <= 100
      ? Number(raw.pageSize)
      : 20
  return {
    channelId:
      typeof raw.channelId === 'string' && isNonNegativeIdString(raw.channelId)
        ? parseNonNegativeIdString(raw.channelId)
        : undefined,
    end: validRange ? requestedEnd : defaults.end,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    group: bounded(raw.group, 128),
    modelName: bounded(raw.modelName, 255),
    page,
    pageSize,
    requestId: bounded(raw.requestId, 64),
    siteIds: [...new Set(raw.siteIds ?? [])]
      .filter(isIdString)
      .map(parseIdString)
      .sort((left, right) => left.localeCompare(right)),
    start: validRange ? requestedStart : defaults.start,
    tokenName: bounded(raw.tokenName, 255),
    type,
    upstreamRequestId: bounded(raw.upstreamRequestId, 128),
    username: bounded(raw.username, 255),
  }
}
