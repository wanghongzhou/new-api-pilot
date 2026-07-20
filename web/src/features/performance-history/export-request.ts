import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { PerformanceHistorySearch } from './search'

export function buildPerformanceHistoryExportRequest(
  format: 'csv' | 'xlsx',
  search: PerformanceHistorySearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: search.end,
      granularity: 'hour',
      model_names: search.models,
      node_names: [],
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: search.start,
      token_keys: [],
      use_groups: search.groups,
    },
    format,
    statistics_type: 'performance_history',
  }
}
