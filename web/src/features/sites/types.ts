import type {
  DataStatus,
  IdString,
  ListQuery,
  MetricString,
  PageData,
  RateInfo,
  Timestamp,
} from '@/lib/api-types'
import type { AnyMessageRef } from '@/lib/message-ref'

export type SiteManagementStatus = 'active' | 'disabled'
export type SiteOnlineStatus = 'unknown' | 'online' | 'offline'
export type SiteAuthStatus = 'unauthorized' | 'authorized' | 'expired'
export type SiteStatisticsStatus =
  | 'pending_config'
  | 'backfilling'
  | 'ready'
  | 'partial'
  | 'error'
  | 'paused'
export type SiteHealthStatus = 'ok' | 'warning' | 'critical' | 'unavailable'
export type SiteCapabilityKey =
  | 'status_contract'
  | 'self_identity'
  | 'root_identity'
  | 'first_user_proof'
  | 'user_pagination'
  | 'channel_pagination'
  | 'data_export_enabled'
  | 'flow_contract'
  | 'data_contract'
  | 'flow_data_consistency'
  | 'instance_contract'
  | 'realtime_contract'
export type CollectionTaskType =
  | 'site_probe'
  | 'realtime_stat'
  | 'resource_snapshot'
  | 'performance_sync'
  | 'topup_sync'
  | 'redemption_sync'
  | 'upstream_task_sync'
  | 'model_meta_sync'
  | 'plan_sync'
  | 'pricing_group_sync'
  | 'system_task_sync'
  | 'user_sync'
  | 'channel_sync'
  | 'log_sync'
  | 'usage_hour'
  | 'usage_backfill'
  | 'usage_validation'
  | 'account_rebuild'
  | 'customer_rebuild'

export type FastCollectionTaskType =
  | 'site_probe'
  | 'realtime_stat'
  | 'resource_snapshot'

export interface UsageSummary {
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  avg_rpm: MetricString | null
  avg_tpm: MetricString | null
  as_of: Timestamp | null
  data_status: DataStatus
  is_final?: boolean
}

export interface SitePerformanceModel {
  model_name: string
  request_count: MetricString
  success_rate: number
  avg_latency_ms: number
  avg_tps: number
}

export interface SitePerformanceSummary {
  hours: number
  sampled_at: Timestamp | null
  data_status: DataStatus
  request_count: MetricString
  success_rate: number
  avg_latency_ms: number
  avg_tps: number
  models: SitePerformanceModel[]
}

export interface MissingRange {
  site_id: IdString
  status: DataStatus
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  reason: AnyMessageRef
}

export interface Completeness {
  data_status: DataStatus
  complete_site_count: number
  expected_site_count: number
  unit_type: 'site_hour' | 'hour' | 'customer_site_hour'
  complete_unit_count: number
  expected_unit_count: number
  completeness_rate: number
  missing_site_ids: IdString[]
  missing_ranges: MissingRange[]
  missing_range_total: number
  missing_ranges_truncated: boolean
  last_verified_at: Timestamp | null
}

export interface BackfillSummary {
  status: 'none' | 'pending' | 'running' | 'failed'
  progress: number
  total_windows: number
  completed_windows: number
  failed_windows: number
  start_timestamp: Timestamp | null
  end_timestamp: Timestamp | null
  latest_error: AnyMessageRef | null
  run_id: IdString | null
}

export interface SiteListItem {
  id: IdString
  name: string
  base_url: string
  management_status: SiteManagementStatus
  online_status: SiteOnlineStatus
  auth_status: SiteAuthStatus
  statistics_status: SiteStatisticsStatus
  health_status: SiteHealthStatus
  version: string | null
  system_name: string | null
  data_export_enabled: boolean | null
  rate: RateInfo
  realtime: {
    rpm: MetricString | null
    tpm: MetricString | null
    updated_at: Timestamp | null
    expired: boolean
  }
  resource: {
    instance_count: number | null
    online_instance_count: number | null
    cpu_max_percent: number | null
    memory_max_percent: number | null
    disk_max_used_percent: number | null
    updated_at: Timestamp | null
    data_status: DataStatus
  }
  today: UsageSummary
  performance: SitePerformanceSummary
  completeness_rate: number
  disabled_at: Timestamp | null
  updated_at: Timestamp
}

export interface SiteDetail extends SiteListItem {
  remark: string
  config_version: number
  root_user_id: IdString | null
  root_created_at: Timestamp | null
  statistics_start_at: Timestamp | null
  statistics_start_source: 'root_created_at' | null
  statistics_end_at: Timestamp | null
  monitoring_start_at: Timestamp | null
  last_probe_at: Timestamp | null
  last_probe_success_at: Timestamp | null
  backfill: BackfillSummary
  completeness: Completeness
}

export interface SiteCapabilityResult {
  key: SiteCapabilityKey
  status: 'passed' | 'failed' | 'skipped'
  message: AnyMessageRef
}

export interface SiteAuthorizationResult {
  root_user_id: IdString
  version: string | null
  system_name: string | null
  data_export_enabled: boolean | null
  first_user_proof: {
    snapshot_total: number
    min_user_id: IdString
    earliest_created_at: Timestamp
    passed: boolean
  }
  capabilities: SiteCapabilityResult[]
  flow_data_validation: 'passed' | 'failed' | 'skipped'
  root_created_at: Timestamp
  statistics_start_at: Timestamp
  backfill_run_id: IdString | null
}

export interface SiteProbeResult {
  probe_success: boolean
  online_status: SiteOnlineStatus
  contract_status: 'compatible' | 'incompatible' | 'unavailable'
  version: string | null
  system_name: string | null
  data_export_enabled: boolean | null
  probed_at: Timestamp
}

export interface SitePublicIdentity {
  base_url: string
  system_name: string
  version: string
}

export interface SiteBaseUrlPreflightResult {
  normalized_base_url: string
  change_type: 'none' | 'path' | 'origin'
  old_public: SitePublicIdentity
  candidate_public: SitePublicIdentity
  contract_status: 'compatible' | 'incompatible'
  preflight_token: string
  expires_at: Timestamp
}

export interface SiteCreateRequest {
  name: string
  base_url: string
  remark?: string
}

export interface SiteUpdateRequest extends SiteCreateRequest {
  base_url_preflight_token?: string
  confirm_same_site?: boolean
}

export type SiteAuthorizeRequest =
  | {
      mode: 'existing_token'
      root_user_id: IdString
      access_token: string
    }
  | {
      mode: 'login_generate_token'
      username: string
      password: string
      confirm_token_rotation: true
    }

export interface SiteBackfillRequest {
  start_timestamp?: Timestamp
  end_timestamp?: Timestamp
  only_missing?: boolean
}

export interface CollectionRunItem {
  id: IdString
  site_id: IdString | null
  site_config_version: number
  task_type: CollectionTaskType
  target_type: 'site' | 'account' | 'customer'
  target_id: IdString
  trigger_type: 'schedule' | 'manual' | 'recovery' | 'dependency'
  start_timestamp: Timestamp | null
  end_timestamp: Timestamp | null
  status: 'pending' | 'running' | 'success' | 'failed'
  priority: number
  progress: number
  windows_initialized: boolean
  total_windows: number
  completed_windows: number
  failed_windows: number
  created_request_id: string
  last_request_id: string
  fetched_rows: MetricString
  written_rows: MetricString
  retry_count: number
  error: AnyMessageRef | null
  next_attempt_at: Timestamp | null
  started_at: Timestamp | null
  finished_at: Timestamp | null
  created_at: Timestamp
  deduplicated: boolean
}

export interface CollectionRunWindowItem {
  id: IdString
  run_id: IdString
  site_id: IdString
  hour_ts: Timestamp
  status: 'pending' | 'running' | 'success' | 'failed' | 'unavailable'
  fact_status: 'pending' | 'complete' | 'missing' | 'unavailable'
  fetched_rows: MetricString
  written_rows: MetricString
  attempt_count: number
  next_retry_at: Timestamp | null
  verified_at: Timestamp | null
  error: AnyMessageRef | null
  started_at: Timestamp | null
  finished_at: Timestamp | null
  updated_at: Timestamp
}

export interface FastTaskHistoryItem {
  site_id: IdString
  task_type: FastCollectionTaskType
  started_at: Timestamp
  finished_at: Timestamp
  status: 'running' | 'success' | 'failed'
  duration_ms: number
  error?: string
  request_id: string
}

export interface FastTaskHistoryListParams extends ListQuery {
  site_id: IdString
  task_type: FastCollectionTaskType
  status?: FastTaskHistoryItem['status'] | ''
  offset?: number
  limit?: number
}

export interface SiteInstanceItem {
  site_id: IdString
  node_name: string
  hostname: string
  is_master: boolean
  runtime_version: string
  goos: string
  goarch: string
  upstream_status: 'online' | 'stale' | 'unknown'
  upstream_stale_after_seconds: number | null
  current_status: 'online' | 'stale' | 'offline' | 'unknown'
  effective_stale_after_seconds: number
  cpu_percent: number | null
  memory_percent: number | null
  disk_used_percent: number | null
  disk_total_bytes: MetricString | null
  disk_used_bytes: MetricString | null
  sampled_at: Timestamp | null
  data_status: DataStatus
  first_seen_at: Timestamp
  started_at: Timestamp | null
  last_seen_at: Timestamp | null
  last_synced_at: Timestamp
}

export interface ResourcePoint {
  bucket_start: Timestamp
  bucket_end: Timestamp
  cpu_max_percent: number | null
  cpu_avg_percent: number | null
  memory_max_percent: number | null
  memory_avg_percent: number | null
  disk_max_used_percent: number | null
  disk_last_used_percent: number | null
  instance_count: number | null
  online_instance_count: number | null
  sample_count: number
  expected_sample_count: number
  health_status: SiteHealthStatus
  data_status: DataStatus
}

export interface SiteResourceResponse {
  site_id: IdString
  node_name: string | null
  granularity: 'minute' | 'hour' | 'day'
  summary: ResourcePoint | null
  trend: ResourcePoint[]
}

export interface SiteSearch {
  page: number
  pageSize: number
  filter: string
  management: SiteManagementStatus[]
  online: SiteOnlineStatus[]
  auth: SiteAuthStatus[]
  statistics: SiteStatisticsStatus[]
  health: SiteHealthStatus[]
  view: 'card' | 'table'
  sort: 'priority' | 'name' | 'today_quota' | 'updated_at'
  order: 'asc' | 'desc'
}

export interface SiteListParams extends ListQuery {
  management_status?: SiteManagementStatus[]
  online_status?: SiteOnlineStatus[]
  auth_status?: SiteAuthStatus[]
  statistics_status?: SiteStatisticsStatus[]
  health_status?: SiteHealthStatus[]
}

export interface CollectionRunListParams extends ListQuery {
  task_type?: CollectionTaskType
  status?: CollectionRunItem['status']
}

export interface CollectionRunWindowListParams extends ListQuery {
  status?: CollectionRunWindowItem['status']
}

export interface SiteResourceQuery {
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  granularity: SiteResourceResponse['granularity']
  node_name?: string
}

export type SitePage = PageData<SiteListItem>
export type CollectionRunPage = PageData<CollectionRunItem>
export type CollectionRunWindowPage = PageData<CollectionRunWindowItem>
export interface FastTaskHistoryPage {
  items: FastTaskHistoryItem[]
  offset: number
  limit: number
  total: number
  has_more: boolean
}
