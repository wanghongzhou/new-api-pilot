import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { SubscriptionPlanSearch } from './search'

export function buildSubscriptionPlanExportRequest(
  format: 'csv' | 'xlsx',
  search: SubscriptionPlanSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: 0,
      granularity: 'hour',
      inventory_states: search.states,
      keyword: search.keyword,
      model_names: [],
      node_names: [],
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: 0,
      subscription_plan_enabled: search.enabled,
      token_keys: [],
      use_groups: [],
    },
    format,
    statistics_type: 'subscription_plans',
  }
}
