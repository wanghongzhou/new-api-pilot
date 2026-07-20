import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { PricingGroupSearch } from './search'

export function buildPricingGroupExportRequest(
  format: 'csv' | 'xlsx',
  search: PricingGroupSearch,
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
      pricing_group: search.group,
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: 0,
      token_keys: [],
      use_groups: [],
    },
    format,
    statistics_type:
      search.tab === 'pricing' ? 'pricing_catalog' : 'group_catalog',
  }
}
