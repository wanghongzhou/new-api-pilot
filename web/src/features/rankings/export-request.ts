import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { RankingSearch } from './search'

export function buildRankingExportRequest(
  format: 'csv' | 'xlsx',
  search: RankingSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: 0,
      granularity: 'hour',
      model_names: [],
      node_names: [],
      ranking_period: search.period,
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: 0,
      token_keys: [],
      use_groups: [],
    },
    format,
    statistics_type:
      search.tab === 'models' ? 'model_rankings' : 'vendor_rankings',
  }
}
