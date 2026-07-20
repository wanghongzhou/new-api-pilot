import type { SubscriptionPlanQueryParams } from './types'

export const subscriptionPlanKeys = {
  all: ['subscription-plans'] as const,
  global: (kind: 'list' | 'statistics', params: SubscriptionPlanQueryParams) =>
    [...subscriptionPlanKeys.all, 'global', kind, params] as const,
  site: (
    siteId: string,
    kind: 'list' | 'statistics',
    params: SubscriptionPlanQueryParams
  ) => [...subscriptionPlanKeys.all, 'site', siteId, kind, params] as const,
}
