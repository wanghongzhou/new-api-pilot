import {
  isIdString,
  isMetricString,
  parseIdString,
  parseMetricString,
} from '@/lib/api-types'
import { BEIJING_TIMEZONE, dayjs, fromUnixSeconds } from '@/lib/dayjs'

import type { UserInventoryState } from './types'

export interface UserInventorySearch {
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  keyword: string
  remoteUserId?: ReturnType<typeof parseIdString>
  roles: number[]
  statuses: number[]
  groups: string[]
  states: UserInventoryState[]
  minBalance?: ReturnType<typeof parseMetricString>
  maxBalance?: ReturnType<typeof parseMetricString>
  start: number
  end: number
  exportId?: ReturnType<typeof parseIdString>
}

type SearchInput = Omit<
  Partial<UserInventorySearch>,
  | 'exportId'
  | 'groups'
  | 'maxBalance'
  | 'minBalance'
  | 'remoteUserId'
  | 'siteIds'
  | 'states'
> & {
  exportId?: string
  groups?: readonly string[]
  maxBalance?: string
  minBalance?: string
  remoteUserId?: string
  siteIds?: readonly string[]
  states?: readonly string[]
}

const validRoles = new Set([0, 1, 10, 100])
const validStatuses = new Set([1, 2])
const validStates = new Set<UserInventoryState>([
  'normal',
  'missing',
  'deleted',
  'identity_mismatch',
])

function stableNumbers(
  values: readonly number[] | undefined,
  valid: Set<number>
) {
  return [...new Set(values ?? [])]
    .filter((value) => Number.isInteger(value) && valid.has(value))
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

export function buildUserInventorySearch(
  raw: SearchInput,
  now = dayjs().tz(BEIJING_TIMEZONE)
): UserInventorySearch {
  const defaults = defaultRange(now)
  const requestedStart = raw.start ?? defaults.start
  const requestedEnd = raw.end ?? defaults.end
  const startValue = fromUnixSeconds(requestedStart)
  const validRange =
    Number.isInteger(requestedStart) &&
    Number.isInteger(requestedEnd) &&
    requestedStart > 0 &&
    requestedStart % 3600 === 0 &&
    requestedEnd % 3600 === 0 &&
    requestedEnd > requestedStart &&
    requestedEnd <= startValue.add(366, 'day').unix()
  const minBalance =
    typeof raw.minBalance === 'string' && isMetricString(raw.minBalance)
      ? parseMetricString(raw.minBalance)
      : undefined
  const maxBalance =
    typeof raw.maxBalance === 'string' && isMetricString(raw.maxBalance)
      ? parseMetricString(raw.maxBalance)
      : undefined
  const orderedBalances =
    minBalance == null ||
    maxBalance == null ||
    BigInt(minBalance) <= BigInt(maxBalance)
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
    maxBalance: orderedBalances ? maxBalance : undefined,
    minBalance: orderedBalances ? minBalance : undefined,
    page:
      Number.isInteger(raw.page) && Number(raw.page) > 0 ? Number(raw.page) : 1,
    pageSize:
      Number.isInteger(raw.pageSize) &&
      Number(raw.pageSize) > 0 &&
      Number(raw.pageSize) <= 100
        ? Number(raw.pageSize)
        : 20,
    roles: stableNumbers(raw.roles, validRoles),
    remoteUserId:
      typeof raw.remoteUserId === 'string' && isIdString(raw.remoteUserId)
        ? parseIdString(raw.remoteUserId)
        : undefined,
    siteIds: [...new Set(raw.siteIds ?? [])]
      .filter(isIdString)
      .map(parseIdString)
      .sort((left, right) => left.localeCompare(right)),
    start: validRange ? requestedStart : defaults.start,
    states: [...new Set(raw.states ?? [])]
      .filter((state): state is UserInventoryState =>
        validStates.has(state as UserInventoryState)
      )
      .sort((left, right) => left.localeCompare(right)),
    statuses: stableNumbers(raw.statuses, validStatuses),
  }
}
