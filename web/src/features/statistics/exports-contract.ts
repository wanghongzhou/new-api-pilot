import type {
  StatisticsExportListParams,
  StatisticsExportSearch,
} from './types'

export function exportListParams(
  search: StatisticsExportSearch
): StatisticsExportListParams {
  return {
    format: search.format,
    p: search.page,
    page_size: search.pageSize,
    sort_by: search.sort,
    sort_order: search.order,
    statistics_type: search.scope,
    status: search.status.length > 0 ? search.status : undefined,
  }
}

export function hasExportFilters(search: StatisticsExportSearch): boolean {
  return Boolean(search.status.length || search.format || search.scope)
}

export function exportUrlSearch(
  search: StatisticsExportSearch
): Partial<StatisticsExportSearch> {
  return {
    exportId: search.exportId,
    format: search.format,
    order: search.order === 'desc' ? undefined : search.order,
    page: search.page === 1 ? undefined : search.page,
    pageSize: search.pageSize === 20 ? undefined : search.pageSize,
    scope: search.scope,
    sort: search.sort === 'created_at' ? undefined : search.sort,
    status: search.status.length > 0 ? search.status : undefined,
  }
}
