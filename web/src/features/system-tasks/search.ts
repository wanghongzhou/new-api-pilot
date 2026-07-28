import { isIdString, parseIdString } from '@/lib/api-types'

import {
  systemTaskStatuses,
  systemTaskTypes,
  type SystemTaskStatus,
  type SystemTaskType,
} from './types'

export interface SystemTaskSearch {
  tab: 'list' | 'types' | 'statuses' | 'sites'
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  types: SystemTaskType[]
  statuses: SystemTaskStatus[]
  errorPresent?: boolean
  createdStart?: number
  createdEnd?: number
  exportId?: ReturnType<typeof parseIdString>
}

type SearchInput = Omit<
  Partial<SystemTaskSearch>,
  'exportId' | 'siteIds' | 'statuses' | 'types'
> & {
  exportId?: string
  siteIds?: readonly string[]
  statuses?: readonly string[]
  types?: readonly string[]
  tab?: string
}

function optionalTimestamp(value: unknown) {
  return Number.isSafeInteger(value) && Number(value) > 0
    ? Number(value)
    : undefined
}

export function buildSystemTaskSearch(raw: SearchInput): SystemTaskSearch {
  let createdStart = optionalTimestamp(raw.createdStart)
  let createdEnd = optionalTimestamp(raw.createdEnd)
  if (
    createdStart != null &&
    createdEnd != null &&
    createdEnd <= createdStart
  ) {
    createdStart = undefined
    createdEnd = undefined
  }
  return {
    createdEnd,
    createdStart,
    errorPresent:
      typeof raw.errorPresent === 'boolean' ? raw.errorPresent : undefined,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
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
      .sort((a, b) => a.localeCompare(b)),
    statuses: systemTaskStatuses.filter((value) =>
      raw.statuses?.includes(value)
    ),
    tab:
      raw.tab === 'types' || raw.tab === 'statuses' || raw.tab === 'sites'
        ? raw.tab
        : 'list',
    types: systemTaskTypes.filter((value) => raw.types?.includes(value)),
  }
}

export function changeSystemTaskTab(
  tab: SystemTaskSearch['tab']
): Partial<SystemTaskSearch> {
  if (tab === 'list') return { page: 1, tab }
  return {
    createdEnd: undefined,
    createdStart: undefined,
    errorPresent: undefined,
    page: 1,
    pageSize: 20,
    siteIds: [],
    statuses: [],
    tab,
    types: [],
  }
}
