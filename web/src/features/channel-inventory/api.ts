import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  ChannelInventoryPage,
  ChannelInventoryQueryParams,
  ChannelInventoryStatisticsQueryParams,
  ChannelInventoryStatisticsResponse,
} from './types'
function append(
  p: URLSearchParams,
  k: string,
  v: readonly (number | string)[] | undefined
) {
  for (const x of v ?? []) p.append(k, String(x))
}
function listParams(v: ChannelInventoryQueryParams) {
  const p = new URLSearchParams()
  p.set('p', String(v.p))
  p.set('page_size', String(v.page_size))
  append(p, 'site_ids', v.site_ids)
  append(p, 'types', v.types)
  append(p, 'statuses', v.statuses)
  append(p, 'groups', v.groups)
  append(p, 'tags', v.tags)
  append(p, 'states', v.states)
  if (v.keyword) p.set('keyword', v.keyword)
  if (v.min_balance) p.set('min_balance', v.min_balance)
  if (v.max_balance) p.set('max_balance', v.max_balance)
  if (v.min_response_time_ms)
    p.set('min_response_time_ms', v.min_response_time_ms)
  if (v.max_response_time_ms)
    p.set('max_response_time_ms', v.max_response_time_ms)
  return p
}
function statParams(v: ChannelInventoryStatisticsQueryParams) {
  const p = new URLSearchParams()
  p.set('start_timestamp', String(v.start_timestamp))
  p.set('end_timestamp', String(v.end_timestamp))
  append(p, 'site_ids', v.site_ids)
  append(p, 'types', v.types)
  append(p, 'statuses', v.statuses)
  append(p, 'groups', v.groups)
  append(p, 'tags', v.tags)
  return p
}
export function listChannelInventory(v: ChannelInventoryQueryParams) {
  return requestApiData<ChannelInventoryPage>({
    method: 'get',
    url: '/api/channel-inventory',
    params: listParams(v),
  })
}
export function listSiteChannelInventory(
  id: IdString,
  v: ChannelInventoryQueryParams
) {
  return requestApiData<ChannelInventoryPage>({
    method: 'get',
    url: `/api/sites/${id}/channel-inventory`,
    params: listParams({ ...v, site_ids: undefined }),
  })
}
export function getChannelInventoryStatistics(
  v: ChannelInventoryStatisticsQueryParams
) {
  return requestApiData<ChannelInventoryStatisticsResponse>({
    method: 'get',
    url: '/api/channel-inventory/statistics',
    params: statParams(v),
  })
}
export function getSiteChannelInventoryStatistics(
  id: IdString,
  v: ChannelInventoryStatisticsQueryParams
) {
  return requestApiData<ChannelInventoryStatisticsResponse>({
    method: 'get',
    url: `/api/sites/${id}/channel-inventory/statistics`,
    params: statParams({ ...v, site_ids: undefined }),
  })
}
