import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type { LogQueryParams, LogResponse } from './types'

function logSearchParams(values: LogQueryParams) {
  const params = new URLSearchParams()
  params.set('p', String(values.p))
  params.set('page_size', String(values.page_size))
  params.set('start_timestamp', String(values.start_timestamp))
  params.set('end_timestamp', String(values.end_timestamp))
  for (const siteId of values.site_ids ?? []) params.append('site_ids', siteId)
  if (values.type != null) params.set('type', String(values.type))
  if (values.username) params.set('username', values.username)
  if (values.model_name) params.set('model_name', values.model_name)
  if (values.token_name) params.set('token_name', values.token_name)
  if (values.channel_id) params.set('channel_id', values.channel_id)
  if (values.group) params.set('group', values.group)
  if (values.request_id) params.set('request_id', values.request_id)
  if (values.upstream_request_id) {
    params.set('upstream_request_id', values.upstream_request_id)
  }
  return params
}

export function listLogs(params: LogQueryParams): Promise<LogResponse> {
  return requestApiData<LogResponse>({
    method: 'get',
    params: logSearchParams(params),
    url: '/api/logs',
  })
}

export function listSiteLogs(
  siteId: IdString,
  params: LogQueryParams
): Promise<LogResponse> {
  return requestApiData<LogResponse>({
    method: 'get',
    params: logSearchParams({ ...params, site_ids: undefined }),
    url: `/api/sites/${siteId}/logs`,
  })
}
