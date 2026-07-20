import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { UserInventorySearch } from './search'

export function buildUserInventoryExportRequest(
  format: 'csv' | 'xlsx',
  search: UserInventorySearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: search.end,
      granularity: 'hour',
      inventory_roles: search.roles,
      inventory_states: search.states,
      inventory_statuses: search.statuses,
      keyword: search.keyword || undefined,
      remote_user_id: search.remoteUserId,
      max_balance: search.maxBalance,
      min_balance: search.minBalance,
      model_names: [],
      node_names: [],
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: search.start,
      token_keys: [],
      use_groups: search.groups,
      channel_id: undefined,
      request_id: undefined,
      token_name: undefined,
      upstream_request_id: undefined,
      username: undefined,
    },
    format,
    statistics_type: 'user_inventory',
  }
}
