import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  UpstreamTaskPage,
  UpstreamTaskQueryParams,
  UpstreamTaskStatisticsResponse,
} from './types'

function appendValues(
  params: URLSearchParams,
  key: string,
  values: readonly string[] | undefined
) {
  for (const value of values ?? []) params.append(key, value)
}

function taskParams(values: UpstreamTaskQueryParams) {
  const params = new URLSearchParams()
  params.set('p', String(values.p))
  params.set('page_size', String(values.page_size))
  appendValues(params, 'site_ids', values.site_ids)
  appendValues(params, 'platforms', values.platforms)
  appendValues(params, 'groups', values.groups)
  appendValues(params, 'actions', values.actions)
  appendValues(params, 'statuses', values.statuses)
  appendValues(params, 'models', values.models)
  if (values.remote_id) params.set('remote_id', values.remote_id)
  if (values.remote_user_id != null)
    params.set('remote_user_id', values.remote_user_id)
  if (values.remote_channel_id != null)
    params.set('remote_channel_id', values.remote_channel_id)
  if (values.task_id) params.set('task_id', values.task_id)
  if (values.start_timestamp != null)
    params.set('start_timestamp', String(values.start_timestamp))
  if (values.end_timestamp != null)
    params.set('end_timestamp', String(values.end_timestamp))
  return params
}

export function listUpstreamTasks(values: UpstreamTaskQueryParams) {
  return requestApiData<UpstreamTaskPage>({
    method: 'get',
    params: taskParams(values),
    url: '/api/upstream-tasks',
  })
}

export function listSiteUpstreamTasks(
  siteId: IdString,
  values: UpstreamTaskQueryParams
) {
  return requestApiData<UpstreamTaskPage>({
    method: 'get',
    params: taskParams({ ...values, site_ids: undefined }),
    url: `/api/sites/${siteId}/upstream-tasks`,
  })
}

export function getUpstreamTaskStatistics(values: UpstreamTaskQueryParams) {
  return requestApiData<UpstreamTaskStatisticsResponse>({
    method: 'get',
    params: taskParams({ ...values, p: 1, page_size: 1 }),
    url: '/api/upstream-tasks/statistics',
  })
}

export function getSiteUpstreamTaskStatistics(
  siteId: IdString,
  values: UpstreamTaskQueryParams
) {
  return requestApiData<UpstreamTaskStatisticsResponse>({
    method: 'get',
    params: taskParams({ ...values, p: 1, page_size: 1, site_ids: undefined }),
    url: `/api/sites/${siteId}/upstream-tasks/statistics`,
  })
}
