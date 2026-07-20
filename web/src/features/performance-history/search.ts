import { isIdString, parseIdString, type IdString } from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

export interface PerformanceHistorySearch {
  page: number
  pageSize: number
  siteIds: IdString[]
  models: string[]
  groups: string[]
  hours: 24 | 168 | 720
  start: number
  end: number
  exportId?: IdString
}

type SearchInput = Omit<
  Partial<PerformanceHistorySearch>,
  'exportId' | 'groups' | 'models' | 'siteIds'
> & {
  exportId?: string
  groups?: readonly string[]
  models?: readonly string[]
  siteIds?: readonly string[]
}

function stableStrings(values: readonly string[] | undefined, limit: number) {
  return [...new Set((values ?? []).map((value) => value.trim()))]
    .filter((value) => value !== '' && [...value].length <= limit)
    .sort((left, right) => left.localeCompare(right, 'zh-CN'))
}

function defaultRange(
  hours: 24 | 168 | 720,
  now = dayjs().tz(BEIJING_TIMEZONE)
) {
  const end = now.startOf('hour')
  return { end: end.unix(), start: end.subtract(hours, 'hour').unix() }
}

export function buildPerformanceHistorySearch(
  raw: SearchInput,
  now = dayjs().tz(BEIJING_TIMEZONE)
): PerformanceHistorySearch {
  const hours =
    raw.hours === 24 || raw.hours === 168 || raw.hours === 720 ? raw.hours : 24
  const defaults = defaultRange(hours, now)
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

  return {
    end: validRange ? requestedEnd : defaults.end,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    groups: stableStrings(raw.groups, 255),
    hours,
    models: stableStrings(raw.models, 255),
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
  }
}

export function performanceRangeForHours(
  hours: 24 | 168 | 720,
  end = dayjs().tz(BEIJING_TIMEZONE).startOf('hour').unix()
) {
  return { end, hours, page: 1, start: end - hours * 3600 }
}
