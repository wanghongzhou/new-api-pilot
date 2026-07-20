import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { LogSearch } from './types'

export function buildLogExportRequest(
  format: 'csv' | 'xlsx',
  search: LogSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_id: search.channelId,
      channel_keys: [],
      customer_ids: [],
      end_timestamp: search.end,
      granularity: 'hour',
      log_type: search.type,
      model_names: search.modelName ? [search.modelName] : [],
      node_names: [],
      request_id: search.requestId || undefined,
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'desc',
      start_timestamp: search.start,
      token_keys: [],
      token_name: search.tokenName || undefined,
      upstream_request_id: search.upstreamRequestId || undefined,
      use_groups: search.group ? [search.group] : [],
      username: search.username || undefined,
    },
    format,
    statistics_type: 'logs',
  }
}
