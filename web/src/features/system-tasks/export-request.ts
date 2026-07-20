import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { SystemTaskSearch } from './search'

export function buildSystemTaskExportRequest(
  format: 'csv' | 'xlsx',
  search: SystemTaskSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      created_end: search.createdEnd ?? 0,
      created_start: search.createdStart ?? 0,
      customer_ids: [],
      end_timestamp: 0,
      granularity: 'hour',
      model_names: [],
      node_names: [],
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: 0,
      error_present: search.errorPresent,
      statuses: search.statuses,
      types: search.types,
      token_keys: [],
      use_groups: [],
    },
    format,
    statistics_type: 'system_tasks',
  }
}
