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
