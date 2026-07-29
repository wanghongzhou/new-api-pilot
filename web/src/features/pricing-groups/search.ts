import { isIdString, parseIdString } from '@/lib/api-types'

import type {
  PricingBillingMode,
  PricingCatalogState,
  PricingCatalogTab,
} from './types'

export interface PricingGroupSearch {
  tab: PricingCatalogTab
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  states: PricingCatalogState[]
  keyword: string
  group: string
  billingMode?: PricingBillingMode
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
  const validTabs: PricingCatalogTab[] = ['groups', 'pricing']
  return {
    billingMode:
      raw.billingMode === 'token' ||
      raw.billingMode === 'fixed' ||
      raw.billingMode === 'tiered_expr'
        ? raw.billingMode
        : undefined,
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
        : 'groups',
  }
}

export function changePricingGroupTab(
  tab: PricingCatalogTab
): Partial<PricingGroupSearch> {
  return tab === 'groups'
    ? { billingMode: undefined, group: '', page: 1, tab }
    : { page: 1, tab }
}

export function serializePricingGroupSearch(search: PricingGroupSearch) {
  return {
    billingMode: search.tab === 'pricing' ? search.billingMode : undefined,
    exportId: search.exportId,
    group: search.group || undefined,
    keyword: search.keyword || undefined,
    page: search.page !== 1 ? search.page : undefined,
    pageSize: search.pageSize !== 20 ? search.pageSize : undefined,
    siteIds: search.siteIds.length > 0 ? search.siteIds : undefined,
    states: search.states.length > 0 ? search.states : undefined,
    tab: search.tab === 'groups' ? undefined : search.tab,
  }
}
