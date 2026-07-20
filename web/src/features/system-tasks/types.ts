import type {
  DataStatus,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export const systemTaskTypes = [
  'log_cleanup',
  'channel_test',
  'model_update',
  'midjourney_poll',
  'async_task_poll',
] as const
export const systemTaskStatuses = [
  'pending',
  'running',
  'succeeded',
  'failed',
] as const

export type SystemTaskType = (typeof systemTaskTypes)[number]
export type SystemTaskStatus = (typeof systemTaskStatuses)[number]
export type SystemTaskErrorCode =
  | 'UPSTREAM_SYSTEM_TASK_FAILED'
  | 'UPSTREAM_SYSTEM_TASK_LEASE_EXPIRED'
  | 'UPSTREAM_SYSTEM_TASK_INVALID_RESPONSE'

export interface SystemTaskProgress {
  total: MetricString | null
  processed: MetricString | null
  progress: MetricString | null
  remaining: MetricString | null
}

interface LogCleanupResult {
  deleted_count: MetricString | null
}
interface ChannelTestResult {
  tested: MetricString | null
  succeeded: MetricString | null
  failed: MetricString | null
  disabled: MetricString | null
  enabled: MetricString | null
}
interface ModelUpdateResult {
  checked_channels: MetricString | null
  changed_channels: MetricString | null
  detected_add_models: MetricString | null
  detected_remove_models: MetricString | null
  failed_channels: MetricString | null
  auto_added_models: MetricString | null
}
interface MidjourneyPollResult {
  unfinished_tasks: MetricString | null
  channels_scanned: MetricString | null
  null_tasks_failed: MetricString | null
}
interface AsyncTaskPollResult {
  unfinished_tasks: MetricString | null
  platforms_scanned: MetricString | null
  null_tasks_failed: MetricString | null
}

interface SystemTaskItemBase<TType extends SystemTaskType, TResult> {
  id: IdString
  site_id: IdString
  remote_id: IdString
  site_name: string
  task_id: string
  type: TType
  status: SystemTaskStatus
  error_present: boolean
  error_code: SystemTaskErrorCode | ''
  progress: SystemTaskProgress | null
  result: TResult | null
  remote_created_at: Timestamp
  remote_updated_at: Timestamp
  collected_at: Timestamp
  data_status: DataStatus
}

export type SystemTaskItem =
  | SystemTaskItemBase<'log_cleanup', LogCleanupResult>
  | SystemTaskItemBase<'channel_test', ChannelTestResult>
  | SystemTaskItemBase<'model_update', ModelUpdateResult>
  | SystemTaskItemBase<'midjourney_poll', MidjourneyPollResult>
  | SystemTaskItemBase<'async_task_poll', AsyncTaskPollResult>

export interface SystemTaskPage {
  items: SystemTaskItem[]
  total: MetricString
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
  truncated: boolean
  truncation_reason: 'source_limit' | 'id_gap' | null
  source_limit: MetricString
  observed_count: MetricString
}

export interface SystemTaskMetric {
  total: MetricString
  active: MetricString
  succeeded: MetricString
  failed: MetricString
  error_present: MetricString
}

export interface SystemTaskBreakdown extends SystemTaskMetric {
  dimension_id: string
  dimension_name: string
  site_id: string
  site_name: string
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface SystemTaskStatistics {
  summary: SystemTaskMetric
  type_breakdown: SystemTaskBreakdown[]
  status_breakdown: SystemTaskBreakdown[]
  site_breakdown: SystemTaskBreakdown[]
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface SystemTaskQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  types?: SystemTaskType[]
  statuses?: SystemTaskStatus[]
  error_present?: boolean
  created_start?: Timestamp
  created_end?: Timestamp
}
