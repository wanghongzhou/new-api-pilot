import type { PricingCatalogQueryParams, PricingCatalogTab } from './types'

export const pricingGroupKeys = {
  all: ['pricing-groups'] as const,
  global: (
    kind: PricingCatalogTab | 'statistics',
    params: PricingCatalogQueryParams
  ) => [...pricingGroupKeys.all, 'global', kind, params] as const,
  site: (
    siteId: string,
    kind: PricingCatalogTab | 'statistics',
    params: PricingCatalogQueryParams
  ) => [...pricingGroupKeys.all, 'site', siteId, kind, params] as const,
}
