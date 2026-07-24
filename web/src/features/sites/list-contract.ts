import type { SiteListParams, SiteSearch } from './types'

export function siteListParams(search: SiteSearch): SiteListParams {
  return {
    auth_status: search.auth.length > 0 ? search.auth : undefined,
    health_status: search.health.length > 0 ? search.health : undefined,
    keyword: search.filter || undefined,
    management_status:
      search.management.length > 0 ? search.management : undefined,
    online_status: search.online.length > 0 ? search.online : undefined,
    p: search.page === 1 ? undefined : search.page,
    page_size: search.pageSize === 20 ? undefined : search.pageSize,
    sort_by: search.sort === 'priority' ? undefined : search.sort,
    sort_order: search.order === 'desc' ? undefined : search.order,
    statistics_status:
      search.statistics.length > 0 ? search.statistics : undefined,
  }
}
