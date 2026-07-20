import { isIdString, parseIdString } from '@/lib/api-types'
import {
  BEIJING_TIMEZONE,
  dayjs,
  fromUnixSeconds,
  isBeijingBucketAligned,
} from '@/lib/dayjs'

import type { StatisticsGranularity, StatisticsSearch } from './types'

type StatisticsSearchInput = Omit<
  Partial<StatisticsSearch>,
  | 'accountIds'
  | 'channelKeys'
  | 'customerIds'
  | 'models'
  | 'nodeNames'
  | 'siteIds'
  | 'tokenKeys'
  | 'useGroups'
> & {
  accountIds?: readonly string[]
  channelKeys?: readonly string[]
  customerIds?: readonly string[]
  models?: readonly string[]
  nodeNames?: readonly string[]
  siteIds?: readonly string[]
  tokenKeys?: readonly string[]
  useGroups?: readonly string[]
}

function stableStrings(values: readonly string[] | undefined): string[] {
  return [...new Set(values ?? [])].sort((left, right) =>
    left.localeCompare(right, 'zh-CN')
  )
}

function stableIds(values: readonly string[] | undefined) {
  return stableStrings(values).filter(isIdString).map(parseIdString)
}

export function defaultStatisticsRange(
  granularity: StatisticsGranularity,
  now = dayjs().tz(BEIJING_TIMEZONE)
): Pick<StatisticsSearch, 'end' | 'start'> {
  if (granularity === 'hour') {
    const end = now.startOf('hour')
    return { end: end.unix(), start: end.subtract(24, 'hour').unix() }
  }
  if (granularity === 'day') {
    const end = now.startOf('day').add(1, 'day')
    return { end: end.unix(), start: end.subtract(30, 'day').unix() }
  }
  if (granularity === 'month') {
    const end = now.startOf('month').add(1, 'month')
    return { end: end.unix(), start: end.subtract(12, 'month').unix() }
  }
  const end = now.startOf('year').add(1, 'year')
  const start = dayjs.tz('1971-01-01', 'YYYY-MM-DD', BEIJING_TIMEZONE)
  return { end: end.unix(), start: start.unix() }
}

export function buildStatisticsSearch(
  raw: StatisticsSearchInput,
  now = dayjs().tz(BEIJING_TIMEZONE)
): StatisticsSearch {
  const granularity = raw.granularity ?? 'hour'
  const defaults = defaultStatisticsRange(granularity, now)
  const explicitRange = raw.start != null && raw.end != null
  const requestedStart = raw.start ?? defaults.start
  const requestedEnd = raw.end ?? defaults.end
  const aligned =
    isBeijingBucketAligned(requestedStart, granularity) &&
    isBeijingBucketAligned(requestedEnd, granularity)
  const ordered = requestedEnd > requestedStart
  const startValue = fromUnixSeconds(requestedStart)
  let withinMaximum = true
  if (granularity === 'hour') {
    withinMaximum = requestedEnd <= startValue.add(31, 'day').unix()
  } else if (granularity === 'day') {
    withinMaximum = requestedEnd <= startValue.add(2, 'year').unix()
  } else if (granularity === 'month') {
    withinMaximum = requestedEnd <= startValue.add(20, 'year').unix()
  }
  const validRange = explicitRange && aligned && ordered && withinMaximum
  const start = validRange ? requestedStart : defaults.start
  const end = validRange ? requestedEnd : defaults.end
  return {
    accountIds: stableIds(raw.accountIds),
    channelKeys: stableStrings(raw.channelKeys).filter((value) =>
      /^[1-9]\d*:(?:0|[1-9]\d*)$/.test(value)
    ),
    customerIds: stableIds(raw.customerIds),
    display: raw.display ?? 'quota',
    end,
    granularity,
    metric: raw.metric ?? 'request_count',
    models: stableStrings(raw.models).filter(
      (value) => value.length > 0 && [...value].length <= 255
    ),
    nodeNames: stableStrings(raw.nodeNames).filter(
      (value) => [...value].length <= 128
    ),
    order: raw.order ?? 'asc',
    page: raw.page ?? 1,
    pageSize: raw.pageSize ?? 20,
    sort: raw.sort ?? 'bucket_start',
    siteIds: stableIds(raw.siteIds),
    start,
    tokenKeys: stableStrings(raw.tokenKeys).filter((value) =>
      /^[1-9]\d*:(?:0|[1-9]\d*)$/.test(value)
    ),
    useGroups: stableStrings(raw.useGroups).filter(
      (value) => [...value].length <= 128
    ),
    view: raw.view ?? 'chart',
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? raw.exportId
        : undefined,
  }
}
