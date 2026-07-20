import type { StatisticsExportCreateRequest } from '@/features/statistics/types'
import type { IdString } from '@/lib/api-types'

import type { UpstreamTaskSearch } from './search'

export function buildUpstreamTaskExportRequest(
  format: 'csv' | 'xlsx',
  search: UpstreamTaskSearch,
  forcedSiteId?: IdString
): StatisticsExportCreateRequest {
  return {
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: search.end ?? 0,
      granularity: 'hour',
      model_names: [],
      node_names: [],
      remote_channel_id: search.remoteChannelId,
      remote_id: search.remoteId,
      remote_user_id: search.remoteUserId,
      site_ids: forcedSiteId ? [forcedSiteId] : search.siteIds,
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: search.start ?? 0,
      task_actions: search.actions,
      task_id: search.taskId || undefined,
      task_models: search.models,
      task_platforms: search.platforms,
      task_statuses: search.statuses,
      token_keys: [],
      use_groups: search.groups,
    },
    format,
    statistics_type: 'upstream_tasks',
  }
}
