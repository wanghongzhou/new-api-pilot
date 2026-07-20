import Decimal from 'decimal.js'

import {
  isDecimalString,
  isIdString,
  isMetricString,
  parseDecimalString,
  parseIdString,
  parseMetricString,
  type DecimalString,
  type IdString,
  type MetricString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import type { ChannelInventoryState } from './types'

export interface ChannelInventorySearch {
  page: number
  pageSize: number
  siteIds: IdString[]
  keyword: string
  types: number[]
  statuses: number[]
  groups: string[]
  tags: string[]
  states: ChannelInventoryState[]
  minBalance?: DecimalString
  maxBalance?: DecimalString
  minResponseTime?: MetricString
  maxResponseTime?: MetricString
  start: number
  end: number
  exportId?: IdString
}

type SearchInput = Omit<
  Partial<ChannelInventorySearch>,
  | 'exportId'
  | 'groups'
  | 'maxBalance'
  | 'maxResponseTime'
  | 'minBalance'
  | 'minResponseTime'
  | 'siteIds'
  | 'states'
  | 'tags'
> & {
  exportId?: string
  groups?: readonly string[]
  maxBalance?: string
  maxResponseTime?: string
  minBalance?: string
  minResponseTime?: string
  siteIds?: readonly string[]
  states?: readonly string[]
  tags?: readonly string[]
}

function stableNumbers(values: readonly number[] | undefined, maximum: number) {
  return [...new Set(values ?? [])]
    .filter(
      (value) => Number.isInteger(value) && value >= 0 && value <= maximum
    )
    .sort((left, right) => left - right)
}

function stableStrings(values: readonly string[] | undefined, limit: number) {
  return [...new Set((values ?? []).map((value) => value.trim()))]
    .filter((value) => value !== '' && [...value].length <= limit)
    .sort((left, right) => left.localeCompare(right, 'zh-CN'))
}

function defaultRange(now = dayjs().tz(BEIJING_TIMEZONE)) {
  const end = now.startOf('hour')
  return { end: end.unix(), start: end.subtract(30, 'day').unix() }
}

export function buildChannelInventorySearch(
  raw: SearchInput,
  now = dayjs().tz(BEIJING_TIMEZONE)
): ChannelInventorySearch {
  const defaults = defaultRange(now)
  const requestedStart = raw.start ?? defaults.start
  const requestedEnd = raw.end ?? defaults.end
  const validRange =
    Number.isInteger(requestedStart) &&
    Number.isInteger(requestedEnd) &&
    requestedStart > 0 &&
    requestedStart % 3600 === 0 &&
    requestedEnd % 3600 === 0 &&
    requestedEnd > requestedStart &&
    requestedEnd <= fromUnixSeconds(requestedStart).add(366, 'day').unix()
  const minBalance =
    typeof raw.minBalance === 'string' && isDecimalString(raw.minBalance)
      ? parseDecimalString(raw.minBalance)
      : undefined
  const maxBalance =
    typeof raw.maxBalance === 'string' && isDecimalString(raw.maxBalance)
      ? parseDecimalString(raw.maxBalance)
      : undefined
  const balancesOrdered =
    minBalance == null ||
    maxBalance == null ||
    new Decimal(minBalance).lessThanOrEqualTo(maxBalance)
  const minResponseTime =
    typeof raw.minResponseTime === 'string' &&
    isMetricString(raw.minResponseTime) &&
    !raw.minResponseTime.startsWith('-')
      ? parseMetricString(raw.minResponseTime)
      : undefined
  const maxResponseTime =
    typeof raw.maxResponseTime === 'string' &&
    isMetricString(raw.maxResponseTime) &&
    !raw.maxResponseTime.startsWith('-')
      ? parseMetricString(raw.maxResponseTime)
      : undefined
  const responseOrdered =
    minResponseTime == null ||
    maxResponseTime == null ||
    BigInt(minResponseTime) <= BigInt(maxResponseTime)
  return {
    end: validRange ? requestedEnd : defaults.end,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    groups: stableStrings(raw.groups, 128),
    keyword:
      typeof raw.keyword === 'string' && [...raw.keyword.trim()].length <= 255
        ? raw.keyword.trim()
        : '',
    maxBalance: balancesOrdered ? maxBalance : undefined,
    maxResponseTime: responseOrdered ? maxResponseTime : undefined,
    minBalance: balancesOrdered ? minBalance : undefined,
    minResponseTime: responseOrdered ? minResponseTime : undefined,
    page:
      Number.isInteger(raw.page) && Number(raw.page) > 0 ? Number(raw.page) : 1,
    pageSize:
      Number.isInteger(raw.pageSize) &&
      Number(raw.pageSize) > 0 &&
      Number(raw.pageSize) <= 100
        ? Number(raw.pageSize)
        : 20,
    siteIds: [...new Set(raw.siteIds ?? [])]
      .filter(isIdString)
      .map(parseIdString)
      .sort((left, right) => left.localeCompare(right)),
    start: validRange ? requestedStart : defaults.start,
    states: [...new Set(raw.states ?? [])]
      .filter(
        (state): state is ChannelInventoryState =>
          state === 'normal' || state === 'missing'
      )
      .sort((left, right) => left.localeCompare(right)),
    statuses: stableNumbers(raw.statuses, 3),
    tags: stableStrings(raw.tags, 128),
    types: stableNumbers(raw.types, 10_000),
  }
}
