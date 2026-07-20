import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  UserInventoryPage,
  UserInventoryQueryParams,
  UserInventoryStatisticsQueryParams,
  UserInventoryStatisticsResponse,
} from './types'

function appendValues(
  params: URLSearchParams,
  key: string,
  values: readonly (number | string)[] | undefined
) {
  for (const value of values ?? []) params.append(key, String(value))
}

function inventoryParams(values: UserInventoryQueryParams) {
  const params = new URLSearchParams()
  params.set('p', String(values.p))
  params.set('page_size', String(values.page_size))
  appendValues(params, 'site_ids', values.site_ids)
  appendValues(params, 'roles', values.roles)
  appendValues(params, 'statuses', values.statuses)
  appendValues(params, 'groups', values.groups)
  appendValues(params, 'states', values.states)
  if (values.keyword) params.set('keyword', values.keyword)
  if (values.remote_user_id) params.set('remote_user_id', values.remote_user_id)
  if (values.min_balance) params.set('min_balance', values.min_balance)
  if (values.max_balance) params.set('max_balance', values.max_balance)
  return params
}

function statisticsParams(values: UserInventoryStatisticsQueryParams) {
  const params = new URLSearchParams()
  params.set('start_timestamp', String(values.start_timestamp))
  params.set('end_timestamp', String(values.end_timestamp))
  appendValues(params, 'site_ids', values.site_ids)
  appendValues(params, 'roles', values.roles)
  appendValues(params, 'statuses', values.statuses)
  appendValues(params, 'groups', values.groups)
  return params
}

export function listUserInventory(
  values: UserInventoryQueryParams
): Promise<UserInventoryPage> {
  return requestApiData<UserInventoryPage>({
    method: 'get',
    params: inventoryParams(values),
    url: '/api/user-inventory',
  })
}

export function listSiteUserInventory(
  siteId: IdString,
  values: UserInventoryQueryParams
): Promise<UserInventoryPage> {
  return requestApiData<UserInventoryPage>({
    method: 'get',
    params: inventoryParams({ ...values, site_ids: undefined }),
    url: `/api/sites/${siteId}/user-inventory`,
  })
}

export function getUserInventoryStatistics(
  values: UserInventoryStatisticsQueryParams
): Promise<UserInventoryStatisticsResponse> {
  return requestApiData<UserInventoryStatisticsResponse>({
    method: 'get',
    params: statisticsParams(values),
    url: '/api/user-inventory/statistics',
  })
}

export function getSiteUserInventoryStatistics(
  siteId: IdString,
  values: UserInventoryStatisticsQueryParams
): Promise<UserInventoryStatisticsResponse> {
  return requestApiData<UserInventoryStatisticsResponse>({
    method: 'get',
    params: statisticsParams({ ...values, site_ids: undefined }),
    url: `/api/sites/${siteId}/user-inventory/statistics`,
  })
}
