import type { IdString } from '@/lib/api-types'

import type {
  StatisticsExportCreateRequest,
  StatisticsExportFormat,
  StatisticsExportSort,
  StatisticsSearch,
  StatisticsScope,
} from './types'

function exportSort(search: StatisticsSearch): StatisticsExportSort {
  return search.sort === 'bucket_start' ? 'name' : search.sort
}

export function buildEntityExportRequest(
  scope: 'site' | 'customer' | 'account',
  id: IdString,
  format: StatisticsExportFormat,
  search: StatisticsSearch
): StatisticsExportCreateRequest {
  const filters = buildStatisticsExportRequest(scope, format, search).filters
  filters.account_ids = scope === 'account' ? [id] : []
  filters.customer_ids = scope === 'customer' ? [id] : []
  filters.site_ids = scope === 'site' ? [id] : []
  return {
    filters,
    format,
    statistics_type: scope,
  }
}

export function buildStatisticsExportRequest(
  scope: StatisticsScope,
  format: StatisticsExportFormat,
  search: StatisticsSearch
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: search.accountIds,
      channel_keys: search.channelKeys,
      customer_ids: search.customerIds,
      end_timestamp: search.end,
      granularity: search.granularity,
      model_names: search.models,
      node_names: search.nodeNames,
      site_ids: search.siteIds,
      sort_by: exportSort(search),
      sort_order: search.order,
      start_timestamp: search.start,
      token_keys: search.tokenKeys,
      use_groups: search.useGroups,
    },
    format,
    statistics_type: scope,
  }
}
