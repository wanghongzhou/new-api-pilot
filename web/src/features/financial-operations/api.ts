import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  FinanceInventoryPage,
  FinanceInventoryQueryParams,
  FinanceStatisticsResponse,
  RedemptionInventoryItem,
  TopupInventoryItem,
} from './types'

function params(values: FinanceInventoryQueryParams) {
  const result = new URLSearchParams()
  result.set('p', String(values.p))
  result.set('page_size', String(values.page_size))
  if (values.remote_id) result.set('remote_id', values.remote_id)
  if (values.remote_user_id) result.set('remote_user_id', values.remote_user_id)
  if (values.start_timestamp) {
    result.set('start_timestamp', String(values.start_timestamp))
  }
  if (values.end_timestamp) {
    result.set('end_timestamp', String(values.end_timestamp))
  }
  if (values.keyword) result.set('keyword', values.keyword)
  for (const value of values.site_ids ?? []) result.append('site_ids', value)
  for (const value of values.statuses ?? []) result.append('statuses', value)
  for (const value of values.providers ?? []) result.append('providers', value)
  for (const value of values.methods ?? []) result.append('methods', value)
  for (const value of values.states ?? []) result.append('states', value)
  return result
}

function request<T>(url: string, values: FinanceInventoryQueryParams) {
  return requestApiData<T>({ method: 'get', params: params(values), url })
}

export function listTopups(values: FinanceInventoryQueryParams) {
  return request<FinanceInventoryPage<TopupInventoryItem>>(
    '/api/topups',
    values
  )
}

export function getTopupStatistics(values: FinanceInventoryQueryParams) {
  return request<FinanceStatisticsResponse>('/api/topups/statistics', values)
}

export function listSiteTopups(
  siteId: IdString,
  values: FinanceInventoryQueryParams
) {
  return request<FinanceInventoryPage<TopupInventoryItem>>(
    `/api/sites/${siteId}/topups`,
    { ...values, site_ids: undefined }
  )
}

export function getSiteTopupStatistics(
  siteId: IdString,
  values: FinanceInventoryQueryParams
) {
  return request<FinanceStatisticsResponse>(
    `/api/sites/${siteId}/topups/statistics`,
    { ...values, site_ids: undefined }
  )
}

export function listRedemptions(values: FinanceInventoryQueryParams) {
  return request<FinanceInventoryPage<RedemptionInventoryItem>>(
    '/api/redemptions',
    values
  )
}

export function getRedemptionStatistics(values: FinanceInventoryQueryParams) {
  return request<FinanceStatisticsResponse>(
    '/api/redemptions/statistics',
    values
  )
}

export function listSiteRedemptions(
  siteId: IdString,
  values: FinanceInventoryQueryParams
) {
  return request<FinanceInventoryPage<RedemptionInventoryItem>>(
    `/api/sites/${siteId}/redemptions`,
    { ...values, site_ids: undefined }
  )
}

export function getSiteRedemptionStatistics(
  siteId: IdString,
  values: FinanceInventoryQueryParams
) {
  return request<FinanceStatisticsResponse>(
    `/api/sites/${siteId}/redemptions/statistics`,
    { ...values, site_ids: undefined }
  )
}
