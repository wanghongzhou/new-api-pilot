import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  PricingCatalogPage,
  PricingCatalogQueryParams,
  PricingCatalogStatistics,
  PricingGroupPage,
} from './types'

function params(
  values: PricingCatalogQueryParams,
  forcedSite = false,
  statistics = false
) {
  const result = new URLSearchParams()
  if (!forcedSite) {
    for (const siteId of values.site_ids ?? []) {
      result.append('site_ids', siteId)
    }
  }
  if (statistics) return result
  result.set('p', String(values.p))
  result.set('page_size', String(values.page_size))
  for (const state of values.states ?? []) result.append('states', state)
  if (values.keyword) result.set('keyword', values.keyword)
  if (values.group) result.set('group', values.group)
  if (values.billing_mode) result.set('billing_mode', values.billing_mode)
  return result
}

function requestCatalog<T>(
  resource: 'pricing-catalog' | 'group-catalog',
  values: PricingCatalogQueryParams,
  siteId?: IdString,
  statistics = false
) {
  return requestApiData<T>({
    method: 'get',
    params: params(
      resource === 'pricing-catalog'
        ? values
        : { ...values, billing_mode: undefined },
      siteId != null,
      statistics
    ),
    url: `${siteId ? `/api/sites/${siteId}` : '/api'}/${resource}${statistics ? '/statistics' : ''}`,
  })
}

export const listPricingCatalog = (values: PricingCatalogQueryParams) =>
  requestCatalog<PricingCatalogPage>('pricing-catalog', values)
export const listSitePricingCatalog = (
  siteId: IdString,
  values: PricingCatalogQueryParams
) => requestCatalog<PricingCatalogPage>('pricing-catalog', values, siteId)
export const listPricingGroups = (values: PricingCatalogQueryParams) =>
  requestCatalog<PricingGroupPage>('group-catalog', values)
export const listSitePricingGroups = (
  siteId: IdString,
  values: PricingCatalogQueryParams
) => requestCatalog<PricingGroupPage>('group-catalog', values, siteId)
export const getPricingCatalogStatistics = (
  values: PricingCatalogQueryParams
) =>
  requestCatalog<PricingCatalogStatistics>(
    'pricing-catalog',
    values,
    undefined,
    true
  )
export const getSitePricingCatalogStatistics = (
  siteId: IdString,
  values: PricingCatalogQueryParams
) =>
  requestCatalog<PricingCatalogStatistics>(
    'pricing-catalog',
    values,
    siteId,
    true
  )
