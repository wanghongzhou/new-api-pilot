import { requestApiData } from '@/lib/api'
import type { IdString } from '@/lib/api-types'

import type {
  CatalogPage,
  PricingCatalogItem,
  PricingCatalogQueryParams,
  PricingCatalogStatistics,
  PricingGroupPage,
} from './types'

function params(values: PricingCatalogQueryParams, forcedSite = false) {
  const result = new URLSearchParams()
  result.set('p', String(values.p))
  result.set('page_size', String(values.page_size))
  if (!forcedSite) {
    for (const siteId of values.site_ids ?? [])
      result.append('site_ids', siteId)
  }
  for (const state of values.states ?? []) result.append('states', state)
  if (values.keyword) result.set('keyword', values.keyword)
  if (values.group) result.set('group', values.group)
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
    params: params(values, siteId != null),
    url: `${siteId ? `/api/sites/${siteId}` : '/api'}/${resource}${statistics ? '/statistics' : ''}`,
  })
}

export const listPricingCatalog = (values: PricingCatalogQueryParams) =>
  requestCatalog<CatalogPage<PricingCatalogItem>>('pricing-catalog', values)
export const listSitePricingCatalog = (
  siteId: IdString,
  values: PricingCatalogQueryParams
) =>
  requestCatalog<CatalogPage<PricingCatalogItem>>(
    'pricing-catalog',
    values,
    siteId
  )
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
