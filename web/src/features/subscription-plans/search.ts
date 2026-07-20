import { isIdString, parseIdString } from '@/lib/api-types'

import type { SubscriptionPlanState } from './types'

export interface SubscriptionPlanSearch {
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  states: SubscriptionPlanState[]
  enabled?: boolean
  keyword: string
  exportId?: ReturnType<typeof parseIdString>
}

type SearchInput = Omit<
  Partial<SubscriptionPlanSearch>,
  'exportId' | 'siteIds' | 'states'
> & {
  exportId?: string
  siteIds?: readonly string[]
  states?: readonly string[]
}

export function buildSubscriptionPlanSearch(
  raw: SearchInput
): SubscriptionPlanSearch {
  const keyword = typeof raw.keyword === 'string' ? raw.keyword.trim() : ''
  return {
    enabled: typeof raw.enabled === 'boolean' ? raw.enabled : undefined,
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    keyword: new TextEncoder().encode(keyword).length <= 128 ? keyword : '',
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
    states: [...new Set(raw.states ?? [])]
      .filter(
        (value): value is SubscriptionPlanState =>
          value === 'normal' || value === 'missing'
      )
      .sort(),
  }
}
