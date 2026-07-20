import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  PerformanceHistoryPage,
  PerformanceHistoryQueryParams,
  PerformanceHistoryStatisticsResponse,
} from './types'
function params(v: PerformanceHistoryQueryParams) {
  const p = new URLSearchParams()
  p.set('p', String(v.p))
  p.set('page_size', String(v.page_size))
  p.set('start_timestamp', String(v.start_timestamp))
  p.set('end_timestamp', String(v.end_timestamp))
  for (const x of v.site_ids ?? []) p.append('site_ids', x)
  for (const x of v.model_names ?? []) p.append('model_names', x)
  for (const x of v.groups ?? []) p.append('groups', x)
  return p
}
export function listPerformanceHistory(v: PerformanceHistoryQueryParams) {
  return requestApiData<PerformanceHistoryPage>({
    method: 'get',
    url: '/api/performance-history',
    params: params(v),
  })
}
export function listSitePerformanceHistory(
  id: IdString,
  v: PerformanceHistoryQueryParams
) {
  return requestApiData<PerformanceHistoryPage>({
    method: 'get',
    url: `/api/sites/${id}/performance-history`,
    params: params({ ...v, site_ids: undefined }),
  })
}
export function getPerformanceHistoryStatistics(
  v: PerformanceHistoryQueryParams
) {
  return requestApiData<PerformanceHistoryStatisticsResponse>({
    method: 'get',
    url: '/api/performance-history/statistics',
    params: params(v),
  })
}
export function getSitePerformanceHistoryStatistics(
  id: IdString,
  v: PerformanceHistoryQueryParams
) {
  return requestApiData<PerformanceHistoryStatisticsResponse>({
    method: 'get',
    url: `/api/sites/${id}/performance-history/statistics`,
    params: params({ ...v, site_ids: undefined }),
  })
}
