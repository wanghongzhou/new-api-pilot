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
    tab: raw.tab === 'groups' ? 'groups' : 'pricing',
  }
}
