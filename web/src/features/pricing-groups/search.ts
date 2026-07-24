import { isIdString, parseIdString } from '@/lib/api-types'

import type { PricingCatalogState, PricingCatalogTab } from './types'

export interface PricingGroupSearch {
  tab: PricingCatalogTab
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  states: PricingCatalogState[]
  keyword: string
  group: string
  exportId?: ReturnType<typeof parseIdString>
}

type SearchInput = Omit<
  Partial<PricingGroupSearch>,
  'exportId' | 'siteIds' | 'states'
> & {
  exportId?: string
  siteIds?: readonly string[]
  states?: readonly string[]
}

function safeText(value: unknown, maxBytes: number) {
  const text = typeof value === 'string' ? value.trim() : ''
  return new TextEncoder().encode(text).length <= maxBytes ? text : ''
}

export function buildPricingGroupSearch(raw: SearchInput): PricingGroupSearch {
  const validTabs: PricingCatalogTab[] = [
    'pricing',
    'groups',
    'site-analysis',
    'vendor-analysis',
    'group-model-analysis',
    'group-availability-analysis',
  ]
  return {
    exportId:
      typeof raw.exportId === 'string' && isIdString(raw.exportId)
        ? parseIdString(raw.exportId)
        : undefined,
    group: safeText(raw.group, 128),
    keyword: safeText(raw.keyword, 255),
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
        (value): value is PricingCatalogState =>
          value === 'normal' || value === 'missing'
      )
      .sort(),
    tab:
      typeof raw.tab === 'string' &&
      validTabs.includes(raw.tab as PricingCatalogTab)
        ? (raw.tab as PricingCatalogTab)
        : 'pricing',
  }
}

export function isPricingAnalysisTab(tab: PricingCatalogTab) {
  return tab !== 'pricing' && tab !== 'groups'
}

export function changePricingGroupTab(
  tab: PricingCatalogTab
): Partial<PricingGroupSearch> {
  if (isPricingAnalysisTab(tab)) {
    return {
      exportId: undefined,
      group: '',
      keyword: '',
      page: 1,
      pageSize: 20,
      siteIds: [],
      states: [],
      tab,
    }
  }
  return tab === 'groups' ? { group: '', page: 1, tab } : { page: 1, tab }
}

export function hasPricingAnalysisFilters(search: PricingGroupSearch) {
  return (
    search.group !== '' ||
    search.keyword !== '' ||
    search.page !== 1 ||
    search.pageSize !== 20 ||
    search.siteIds.length > 0 ||
    search.states.length > 0
  )
}

export function serializePricingGroupSearch(search: PricingGroupSearch) {
  const analysis = isPricingAnalysisTab(search.tab)
  return {
    exportId: search.exportId,
    group: !analysis && search.group ? search.group : undefined,
    keyword: !analysis && search.keyword ? search.keyword : undefined,
    page: !analysis && search.page !== 1 ? search.page : undefined,
    pageSize: !analysis && search.pageSize !== 20 ? search.pageSize : undefined,
    siteIds:
      !analysis && search.siteIds.length > 0 ? search.siteIds : undefined,
    states: !analysis && search.states.length > 0 ? search.states : undefined,
    tab: search.tab === 'pricing' ? undefined : search.tab,
  }
}
