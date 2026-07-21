import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  SystemTaskPage,
  SystemTaskQueryParams,
  SystemTaskStatistics,
} from './types'

function params(values: SystemTaskQueryParams, forcedSite = false) {
  const result = new URLSearchParams()
  result.set('p', String(values.p))
  result.set('page_size', String(values.page_size))
  if (!forcedSite) {
    for (const value of values.site_ids ?? []) result.append('site_ids', value)
  }
  for (const value of values.types ?? []) result.append('types', value)
  for (const value of values.statuses ?? []) result.append('statuses', value)
  if (values.error_present != null) {
    result.set('error_present', String(values.error_present))
  }
  if (values.created_start != null) {
    result.set('created_start', String(values.created_start))
  }
  if (values.created_end != null) {
    result.set('created_end', String(values.created_end))
  }
  return result
}

function requestTasks<T>(
  values: SystemTaskQueryParams,
  siteId?: IdString,
  statistics = false
) {
  return requestApiData<T>({
    method: 'get',
    params: params(values, siteId != null),
    url: `${siteId ? `/api/sites/${siteId}` : '/api'}/system-tasks${statistics ? '/statistics' : ''}`,
  })
}

export const listSystemTasks = (values: SystemTaskQueryParams) =>
  requestTasks<SystemTaskPage>(values)
export const listSiteSystemTasks = (
  siteId: IdString,
  values: SystemTaskQueryParams
) => requestTasks<SystemTaskPage>(values, siteId)
export const getSystemTaskStatistics = (values: SystemTaskQueryParams) =>
  requestTasks<SystemTaskStatistics>(values, undefined, true)
export const getSiteSystemTaskStatistics = (
  siteId: IdString,
  values: SystemTaskQueryParams
) => requestTasks<SystemTaskStatistics>(values, siteId, true)
