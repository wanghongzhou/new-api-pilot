import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
  type IdString,
  type NonNegativeIdString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import type { FinanceRemoteState, FinancialOperationTab } from './types'

export interface FinancialOperationsSearch {
  tab: FinancialOperationTab
  page: number
  pageSize: number
  siteIds: IdString[]
  remoteId?: IdString
  remoteUserId?: NonNegativeIdString
  statuses: string[]
  providers: string[]
  methods: string[]
  states: FinanceRemoteState[]
  keyword: string
  start: number
  end: number
  exportId?: IdString
}

type SearchInput = Omit<
  Partial<FinancialOperationsSearch>,
  | 'exportId'
  | 'methods'
  | 'providers'
  | 'remoteId'
  | 'remoteUserId'
  | 'siteIds'
  | 'states'
  | 'statuses'
> & {
  exportId?: string
  methods?: readonly string[]
  providers?: readonly string[]
  remoteId?: string
  remoteUserId?: string
  siteIds?: readonly string[]
  states?: readonly string[]
  statuses?: readonly string[]
}

function stableStrings(values: readonly string[] | undefined, limit = 255) {
  return [...new Set((values ?? []).map((value) => value.trim()))]
    .filter((value) => value !== '' && [...value].length <= limit)
    .sort((left, right) => left.localeCompare(right, 'zh-CN'))
}

function defaultRange(now = dayjs().tz(BEIJING_TIMEZONE)) {
  const end = now.startOf('hour')
  return { end: end.unix(), start: end.subtract(30, 'day').unix() }
}

export function buildFinancialOperationsSearch(
  raw: SearchInput,
  now = dayjs().tz(BEIJING_TIMEZONE)
): FinancialOperationsSearch {
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
  return {
    end: validRange ? requestedEnd : defaults.end,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    keyword:
      typeof raw.keyword === 'string' && [...raw.keyword.trim()].length <= 255
        ? raw.keyword.trim()
        : '',
    methods: stableStrings(raw.methods),
    page:
      Number.isInteger(raw.page) && Number(raw.page) > 0 ? Number(raw.page) : 1,
    pageSize:
      Number.isInteger(raw.pageSize) &&
      Number(raw.pageSize) > 0 &&
      Number(raw.pageSize) <= 100
        ? Number(raw.pageSize)
        : 20,
    providers: stableStrings(raw.providers),
    remoteId:
      typeof raw.remoteId === 'string' && isIdString(raw.remoteId)
        ? parseIdString(raw.remoteId)
        : undefined,
    remoteUserId:
      typeof raw.remoteUserId === 'string' &&
      isNonNegativeIdString(raw.remoteUserId)
        ? parseNonNegativeIdString(raw.remoteUserId)
        : undefined,
    siteIds: [...new Set(raw.siteIds ?? [])]
      .filter(isIdString)
      .map(parseIdString)
      .sort((left, right) => left.localeCompare(right)),
    start: validRange ? requestedStart : defaults.start,
    states: [...new Set(raw.states ?? [])]
      .filter(
        (state): state is FinanceRemoteState =>
          state === 'normal' || state === 'missing'
      )
      .sort((left, right) => left.localeCompare(right)),
    statuses: stableStrings(raw.statuses),
    tab: raw.tab === 'redemptions' ? 'redemptions' : 'topups',
  }
}
