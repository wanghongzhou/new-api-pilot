import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { ModelCatalogSearch } from './search'

export function buildModelCatalogExportRequest(
  format: 'csv' | 'xlsx',
  search: ModelCatalogSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  const supportsCatalogFilters = search.tab !== 'missing'
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: 0,
      granularity: 'hour',
      keyword: search.keyword || undefined,
      model_names: [],
      model_statuses: supportsCatalogFilters ? search.statuses : [],
      model_sync_official: supportsCatalogFilters ? search.syncOfficial : [],
      model_vendor_id: supportsCatalogFilters ? search.vendorId : undefined,
      node_names: [],
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: 0,
      token_keys: [],
      use_groups: [],
    },
    format,
    statistics_type: 'model_catalog',
  }
}
