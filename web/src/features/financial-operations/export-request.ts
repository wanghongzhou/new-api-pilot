import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { FinancialOperationsSearch } from './search'

export function buildFinancialOperationsExportRequest(
  format: 'csv' | 'xlsx',
  search: FinancialOperationsSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: search.end,
      finance_methods: search.methods,
      finance_providers: search.providers,
      finance_states: search.states,
      finance_statuses: search.statuses,
      granularity: 'hour',
      keyword: search.keyword || undefined,
      model_names: [],
      node_names: [],
      remote_id: search.remoteId,
      remote_user_id: search.remoteUserId,
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: search.start,
      token_keys: [],
      use_groups: [],
    },
    format,
    statistics_type:
      search.tab === 'topups' ? 'topup_inventory' : 'redemption_inventory',
  }
}
