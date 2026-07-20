import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'

import type { UpstreamTaskStatus } from './types'

export interface UpstreamTaskSearch {
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  remoteId?: ReturnType<typeof parseIdString>
  remoteUserId?: ReturnType<typeof parseNonNegativeIdString>
  remoteChannelId?: ReturnType<typeof parseNonNegativeIdString>
  taskId: string
  platforms: string[]
  groups: string[]
  actions: string[]
  statuses: UpstreamTaskStatus[]
  models: string[]
  start?: number
  end?: number
  exportId?: ReturnType<typeof parseIdString>
}

type SearchInput = Omit<
  Partial<UpstreamTaskSearch>,
  | 'actions'
  | 'exportId'
  | 'groups'
  | 'models'
  | 'platforms'
  | 'remoteChannelId'
  | 'remoteId'
  | 'remoteUserId'
  | 'siteIds'
  | 'statuses'
> & {
  actions?: readonly string[]
  exportId?: string
  groups?: readonly string[]
  models?: readonly string[]
  platforms?: readonly string[]
  remoteChannelId?: string
  remoteId?: string
  remoteUserId?: string
  siteIds?: readonly string[]
  statuses?: readonly string[]
}

export const upstreamTaskStatuses: readonly UpstreamTaskStatus[] = [
  'NOT_START',
  'SUBMITTED',
  'QUEUED',
  'IN_PROGRESS',
  'FAILURE',
  'SUCCESS',
  'UNKNOWN',
]
const statusSet = new Set<string>(upstreamTaskStatuses)

function stableStrings(values: readonly string[] | undefined, bytes: number) {
  return [...new Set((values ?? []).map((value) => value.trim()))]
    .filter(
      (value) => value !== '' && new TextEncoder().encode(value).length <= bytes
    )
    .sort((left, right) => left.localeCompare(right, 'zh-CN'))
}

function optionalTimestamp(value: unknown) {
  return Number.isSafeInteger(value) && Number(value) > 0
    ? Number(value)
    : undefined
}

export function buildUpstreamTaskSearch(raw: SearchInput): UpstreamTaskSearch {
  let start = optionalTimestamp(raw.start)
  let end = optionalTimestamp(raw.end)
  if (start != null && end != null && end <= start) {
    start = undefined
    end = undefined
  }
  const taskId = typeof raw.taskId === 'string' ? raw.taskId.trim() : ''
  return {
    actions: stableStrings(raw.actions, 40),
    end,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    groups: stableStrings(raw.groups, 50),
    models: stableStrings(raw.models, 255),
    page:
      Number.isInteger(raw.page) && Number(raw.page) > 0 ? Number(raw.page) : 1,
    pageSize:
      Number.isInteger(raw.pageSize) &&
      Number(raw.pageSize) > 0 &&
      Number(raw.pageSize) <= 100
        ? Number(raw.pageSize)
        : 20,
    platforms: stableStrings(raw.platforms, 30),
    remoteChannelId:
      typeof raw.remoteChannelId === 'string' &&
      isNonNegativeIdString(raw.remoteChannelId)
        ? parseNonNegativeIdString(raw.remoteChannelId)
        : undefined,
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
    start,
    statuses: [...new Set(raw.statuses ?? [])]
      .filter((status): status is UpstreamTaskStatus => statusSet.has(status))
      .sort(
        (left, right) =>
          upstreamTaskStatuses.indexOf(left) -
          upstreamTaskStatuses.indexOf(right)
      ),
    taskId: new TextEncoder().encode(taskId).length <= 191 ? taskId : '',
  }
}
