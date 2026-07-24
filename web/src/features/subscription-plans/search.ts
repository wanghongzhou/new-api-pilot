import { isIdString, parseIdString } from '@/lib/api-types'

import type { SubscriptionPlanState, SubscriptionPlanTab } from './types'

export interface SubscriptionPlanSearch {
  tab: SubscriptionPlanTab
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
    tab: raw.tab === 'site-analysis' ? 'site-analysis' : 'plans',
  }
}

export function changeSubscriptionPlanTab(
  tab: SubscriptionPlanTab
): Partial<SubscriptionPlanSearch> {
  return tab === 'site-analysis'
    ? {
        enabled: undefined,
        exportId: undefined,
        keyword: '',
        page: 1,
        pageSize: 20,
        siteIds: [],
        states: [],
        tab,
      }
    : { page: 1, tab }
}

export function hasSubscriptionPlanAnalysisFilters(
  search: SubscriptionPlanSearch
) {
  return (
    search.enabled != null ||
    search.keyword !== '' ||
    search.page !== 1 ||
    search.pageSize !== 20 ||
    search.siteIds.length > 0 ||
    search.states.length > 0
  )
}

export function serializeSubscriptionPlanSearch(
  search: SubscriptionPlanSearch
) {
  const analysis = search.tab === 'site-analysis'
  return {
    enabled: analysis ? undefined : search.enabled,
    exportId: search.exportId,
    keyword: !analysis && search.keyword ? search.keyword : undefined,
    page: !analysis && search.page !== 1 ? search.page : undefined,
    pageSize: !analysis && search.pageSize !== 20 ? search.pageSize : undefined,
    siteIds:
      !analysis && search.siteIds.length > 0 ? search.siteIds : undefined,
    states: !analysis && search.states.length > 0 ? search.states : undefined,
    tab: search.tab === 'plans' ? undefined : search.tab,
  }
}
