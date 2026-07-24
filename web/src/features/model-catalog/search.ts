import {
  isIdString,
  isNonNegativeIdString,
  parseIdString,
  parseNonNegativeIdString,
} from '@/lib/api-types'

import type {
  ModelBinaryState,
  ModelCatalogQueryParams,
  ModelCatalogTab,
} from './types'

export interface ModelCatalogSearch {
  tab: ModelCatalogTab
  page: number
  pageSize: number
  siteIds: ReturnType<typeof parseIdString>[]
  keyword: string
  vendorId?: ReturnType<typeof parseNonNegativeIdString>
  statuses: ModelBinaryState[]
  syncOfficial: ModelBinaryState[]
  exportId?: ReturnType<typeof parseIdString>
}

type SearchInput = Omit<
  Partial<ModelCatalogSearch>,
  'exportId' | 'siteIds' | 'statuses' | 'syncOfficial' | 'vendorId'
> & {
  exportId?: string
  siteIds?: readonly string[]
  statuses?: readonly number[]
  syncOfficial?: readonly number[]
  vendorId?: string
}

function binaryValues(values: readonly number[] | undefined) {
  return [...new Set(values ?? [])]
    .filter((value): value is ModelBinaryState => value === 0 || value === 1)
    .sort((left, right) => left - right)
}

export function buildModelCatalogSearch(raw: SearchInput): ModelCatalogSearch {
  const keyword = typeof raw.keyword === 'string' ? raw.keyword.trim() : ''
  return {
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
    statuses: binaryValues(raw.statuses),
    syncOfficial: binaryValues(raw.syncOfficial),
    tab: raw.tab === 'coverage' || raw.tab === 'missing' ? raw.tab : 'catalog',
    vendorId:
      typeof raw.vendorId === 'string' && isNonNegativeIdString(raw.vendorId)
        ? parseNonNegativeIdString(raw.vendorId)
        : undefined,
  }
}

export function buildModelCatalogQueryParams(
  search: ModelCatalogSearch,
  tab: ModelCatalogTab = search.tab
): ModelCatalogQueryParams {
  const common = {
    keyword: search.keyword || undefined,
    p: search.page,
    page_size: search.pageSize,
    site_ids: search.siteIds,
  }
  if (tab === 'missing') return common
  return {
    ...common,
    statuses: search.statuses,
    sync_official: search.syncOfficial,
    vendor_id: search.vendorId,
  }
}

export function changeModelCatalogTab(
  tab: ModelCatalogTab
): Partial<ModelCatalogSearch> {
  if (tab === 'coverage') {
    return {
      keyword: '',
      page: 1,
      pageSize: 20,
      siteIds: [],
      statuses: [],
      syncOfficial: [],
      tab,
      vendorId: undefined,
    }
  }
  return tab === 'missing'
    ? {
        page: 1,
        statuses: [],
        syncOfficial: [],
        tab,
        vendorId: undefined,
      }
    : { page: 1, tab }
}

export function hasMissingIncompatibleFilters(search: ModelCatalogSearch) {
  return (
    search.vendorId != null ||
    search.statuses.length > 0 ||
    search.syncOfficial.length > 0
  )
}

export function serializeModelCatalogSearch(search: ModelCatalogSearch) {
  return {
    exportId: search.exportId,
    keyword: search.keyword || undefined,
    page: search.page === 1 ? undefined : search.page,
    pageSize: search.pageSize === 20 ? undefined : search.pageSize,
    siteIds: search.siteIds.length > 0 ? search.siteIds : undefined,
    statuses: search.statuses.length > 0 ? search.statuses : undefined,
    syncOfficial:
      search.syncOfficial.length > 0 ? search.syncOfficial : undefined,
    tab: search.tab === 'catalog' ? undefined : search.tab,
    vendorId: search.vendorId,
  }
}
