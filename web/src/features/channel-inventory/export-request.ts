import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { ChannelInventorySearch } from './search'

export function buildChannelInventoryExportRequest(
  format: 'csv' | 'xlsx',
  search: ChannelInventorySearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      channel_states: search.states,
      channel_statuses: search.statuses,
      channel_tags: search.tags,
      channel_types: search.types,
      customer_ids: [],
      end_timestamp: search.end,
      granularity: 'hour',
      keyword: search.keyword || undefined,
      max_balance: search.maxBalance,
      max_response_time_ms: search.maxResponseTime,
      min_balance: search.minBalance,
      min_response_time_ms: search.minResponseTime,
      model_names: [],
      node_names: [],
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: search.start,
      token_keys: [],
      use_groups: search.groups,
    },
    format,
    statistics_type: 'channel_inventory',
  }
}
