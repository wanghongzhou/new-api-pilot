import type { Completeness } from '@/features/sites/types'
import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  NonNegativeIdString,
  PageData,
  RateInfo,
  Timestamp,
} from '@/lib/api-types'
import type { AnyMessageRef } from '@/lib/message-ref'

export type StatisticsScope =
  | 'global'
  | 'site'
  | 'customer'
  | 'account'
  | 'model'
  | 'channel'
  | 'group'
  | 'token'
  | 'node'
export type StatisticsExportScope =
  | StatisticsScope
  | 'logs'
  | 'user_inventory'
  | 'channel_inventory'
  | 'performance_history'
  | 'topup_inventory'
  | 'redemption_inventory'
  | 'upstream_tasks'
  | 'model_catalog'
  | 'model_rankings'
  | 'vendor_rankings'
  | 'subscription_plans'
  | 'pricing_catalog'
  | 'group_catalog'
  | 'system_tasks'
export type StatisticsGranularity = 'hour' | 'day' | 'month' | 'year'
export type StatisticsMetric =
  | 'request_count'
  | 'quota'
  | 'token_used'
  | 'active_users'
export type StatisticsDisplay = 'quota' | 'usd' | 'cny'
export type StatisticsView = 'chart' | 'table'
export type StatisticsSort = StatisticsMetric | 'name' | 'bucket_start'
export type StatisticsExportFormat = 'xlsx' | 'csv'
export type StatisticsExportSort = Exclude<StatisticsSort, 'bucket_start'>
export type StatisticsExportStatus =
  | 'pending'
  | 'running'
  | 'success'
  | 'failed'
  | 'expired'
export type StatisticsExportListSort =
  | 'created_at'
  | 'finished_at'
  | 'status'
  | 'file_size'

export interface SiteQuotaBreakdown {
  site_id: IdString
  site_name: string
  quota: MetricString | null
  quota_per_unit: string | null
  usd_exchange_rate: string | null
  rate_source: 'site' | 'fallback' | 'unavailable'
  rate_updated_at: Timestamp | null
  data_status: DataStatus
}

export interface TrendPoint {
  bucket_start: Timestamp
  bucket_end: Timestamp
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  data_status: DataStatus
  is_final: boolean
  as_of: Timestamp | null
  complete_site_count: number
  expected_site_count: number
  site_breakdown: SiteQuotaBreakdown[]
  reason: AnyMessageRef | null
}

export interface StatisticsBreakdownBase {
  dimension_id: string
  dimension_name: string
  site_id: IdString | null
  site_name: string | null
  bucket_start: Timestamp
  bucket_end: Timestamp
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  data_status: DataStatus
  is_final: boolean
  as_of: Timestamp | null
  site_breakdown: SiteQuotaBreakdown[]
  completeness_rate: number
}

export interface GlobalStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'global'
  complete_site_count: number
  expected_site_count: number
}

export interface SiteStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'site'
  site_id: IdString
  site_name: string
  management_status: string
  online_status: string
  auth_status: string
  statistics_status: string
  health_status: string
  rate: RateInfo
}

export interface CustomerStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'customer'
  site_id: null
  site_name: null
  account_count: number
  site_count: number
}

export interface AccountStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'account'
  site_id: IdString
  site_name: string
  customer_id: IdString
  customer_name: string
  remote_user_id: IdString
}

export interface ModelStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'model'
  model_name: string
}

export interface ChannelStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'channel'
  remote_channel_id: string
  remote_missing: boolean
}

export interface GroupStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'group'
  use_group: string
}

export interface TokenStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'token'
  token_id: IdString
  token_name: string
}

export interface NodeStatisticsBreakdown extends StatisticsBreakdownBase {
  dimension_type: 'node'
  node_name: string
}

export interface StatisticsBreakdownByScope {
  global: GlobalStatisticsBreakdown
  site: SiteStatisticsBreakdown
  customer: CustomerStatisticsBreakdown
  account: AccountStatisticsBreakdown
  model: ModelStatisticsBreakdown
  channel: ChannelStatisticsBreakdown
  group: GroupStatisticsBreakdown
  token: TokenStatisticsBreakdown
  node: NodeStatisticsBreakdown
}

export interface StatisticsResponse<
  TBreakdown extends StatisticsBreakdownBase = StatisticsBreakdownBase,
> {
  scope: StatisticsScope
  granularity: StatisticsGranularity
  range: {
    start_timestamp: Timestamp
    end_timestamp: Timestamp
    timezone: 'Asia/Shanghai'
    as_of: Timestamp | null
  }
  summary: {
    request_count: MetricString | null
    quota: MetricString | null
    token_used: MetricString | null
    active_users: MetricString | null
    data_status: DataStatus
    is_partial: boolean
  }
  trend: TrendPoint[]
  breakdown: PageData<TBreakdown>
  site_breakdown: SiteQuotaBreakdown[]
  completeness: Completeness
}

export interface EntityStatisticsParams {
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  granularity: StatisticsGranularity
  p: number
  page_size: number
  sort_by: StatisticsSort
  sort_order: 'asc' | 'desc'
}

export interface StatisticsQueryParams extends EntityStatisticsParams {
  site_ids?: IdString[]
  customer_ids?: IdString[]
  account_ids?: IdString[]
  model_names?: string[]
  channel_keys?: string[]
  use_groups?: string[]
  token_keys?: string[]
  node_names?: string[]
}

export interface StatisticsSearch {
  start: Timestamp
  end: Timestamp
  granularity: StatisticsGranularity
  metric: StatisticsMetric
  display: StatisticsDisplay
  view: StatisticsView
  page: number
  pageSize: number
  sort: StatisticsSort
  order: 'asc' | 'desc'
  siteIds: IdString[]
  customerIds: IdString[]
  accountIds: IdString[]
  models: string[]
  channelKeys: string[]
  useGroups: string[]
  tokenKeys: string[]
  nodeNames: string[]
  exportId?: IdString
}

export interface StatisticsOptionParams {
  keyword?: string
  site_ids?: IdString[]
  p?: number
  page_size?: number
}

export interface ModelOption {
  key: string
  site_id: IdString
  site_name: string
  model_name: string
}

export interface ChannelOption {
  key: string
  site_id: IdString
  site_name: string
  remote_channel_id: string
  name: string
  remote_missing: boolean
}

export type ModelOptionPage = PageData<ModelOption>
export type ChannelOptionPage = PageData<ChannelOption>

export interface GroupOption {
  key: string
  site_id: IdString
  site_name: string
  use_group: string
}

export interface TokenOption {
  key: string
  site_id: IdString
  site_name: string
  token_id: IdString
  token_name: string
}

export interface NodeOption {
  key: string
  site_id: IdString
  site_name: string
  node_name: string
}

export type GroupOptionPage = PageData<GroupOption>
export type TokenOptionPage = PageData<TokenOption>
export type NodeOptionPage = PageData<NodeOption>

export interface StatisticsExportFilters {
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  granularity: StatisticsGranularity
  site_ids: IdString[]
  customer_ids: IdString[]
  account_ids: IdString[]
  model_names: string[]
  channel_keys: string[]
  use_groups: string[]
  token_keys: string[]
  node_names: string[]
  log_type?: number
  username?: string
  token_name?: string
  channel_id?: string
  request_id?: string
  upstream_request_id?: string
  inventory_roles?: number[]
  inventory_statuses?: number[]
  inventory_states?: string[]
  subscription_plan_enabled?: boolean
  pricing_group?: string
  types?: string[]
  statuses?: string[]
  error_present?: boolean
  created_start?: Timestamp
  created_end?: Timestamp
  keyword?: string
  min_balance?: MetricString | DecimalString
  max_balance?: MetricString | DecimalString
  remote_user_id?: IdString | NonNegativeIdString
  channel_types?: number[]
  channel_statuses?: number[]
  channel_tags?: string[]
  channel_states?: string[]
  min_response_time_ms?: MetricString
  max_response_time_ms?: MetricString
  finance_statuses?: string[]
  finance_providers?: string[]
  finance_methods?: string[]
  finance_states?: string[]
  remote_id?: IdString
  remote_channel_id?: NonNegativeIdString
  task_id?: string
  task_platforms?: string[]
  task_actions?: string[]
  task_statuses?: string[]
  task_models?: string[]
  model_vendor_id?: NonNegativeIdString
  model_statuses?: number[]
  model_sync_official?: number[]
  ranking_period?: 'today' | 'week' | 'month' | 'year'
  sort_by: StatisticsExportSort
  sort_order: 'asc' | 'desc'
}

export interface StatisticsExportCreateRequest {
  format: StatisticsExportFormat
  statistics_type: StatisticsExportScope
  filters: StatisticsExportFilters
}

export interface StatisticsExportJobItem {
  id: IdString
  format: StatisticsExportFormat
  statistics_type: StatisticsExportScope
  filters: StatisticsExportFilters
  status: StatisticsExportStatus
  progress: number
  file_name: string
  file_size: MetricString
  row_count: MetricString
  error: AnyMessageRef | null
  data_snapshot_at: Timestamp | null
  expires_at: Timestamp | null
  created_at: Timestamp
  started_at: Timestamp | null
  finished_at: Timestamp | null
  deduplicated: boolean
}

export interface StatisticsExportListParams {
  p: number
  page_size: number
  status?: StatisticsExportStatus[]
  format?: StatisticsExportFormat
  statistics_type?: StatisticsExportScope
  sort_by: StatisticsExportListSort
  sort_order: 'asc' | 'desc'
}

export interface StatisticsExportSearch {
  exportId?: IdString
  format?: StatisticsExportFormat
  order: 'asc' | 'desc'
  page: number
  pageSize: number
  scope?: StatisticsExportScope
  sort: StatisticsExportListSort
  status: StatisticsExportStatus[]
}

export interface StatisticsExportDownload {
  blob: Blob
  fileName: string
}
