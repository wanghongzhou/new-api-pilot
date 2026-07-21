import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  SubscriptionPlanPage,
  SubscriptionPlanQueryParams,
  SubscriptionPlanStatistics,
} from './types'

function params(values: SubscriptionPlanQueryParams, forcedSite = false) {
  const params = new URLSearchParams()
  params.set('p', String(values.p))
  params.set('page_size', String(values.page_size))
  if (!forcedSite) {
    for (const siteId of values.site_ids ?? []) {
      params.append('site_ids', siteId)
    }
  }
  for (const state of values.states ?? []) params.append('states', state)
  if (values.enabled != null) params.set('enabled', String(values.enabled))
  if (values.keyword) params.set('keyword', values.keyword)
  return params
}

function requestPlans<T>(
  suffix: '' | '/statistics',
  values: SubscriptionPlanQueryParams,
  siteId?: IdString
) {
  return requestApiData<T>({
    method: 'get',
    params: params(values, siteId != null),
    url: siteId
      ? `/api/sites/${siteId}/subscription-plans${suffix}`
      : `/api/subscription-plans${suffix}`,
  })
}

export const listSubscriptionPlans = (values: SubscriptionPlanQueryParams) =>
  requestPlans<SubscriptionPlanPage>('', values)
export const listSiteSubscriptionPlans = (
  siteId: IdString,
  values: SubscriptionPlanQueryParams
) => requestPlans<SubscriptionPlanPage>('', values, siteId)
export const getSubscriptionPlanStatistics = (
  values: SubscriptionPlanQueryParams
) => requestPlans<SubscriptionPlanStatistics>('/statistics', values)
export const getSiteSubscriptionPlanStatistics = (
  siteId: IdString,
  values: SubscriptionPlanQueryParams
) => requestPlans<SubscriptionPlanStatistics>('/statistics', values, siteId)
