import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  NonNegativeIdString,
  Timestamp,
} from '@/lib/api-types'

export type UpstreamTaskStatus =
  | 'NOT_START'
  | 'SUBMITTED'
  | 'QUEUED'
  | 'IN_PROGRESS'
  | 'FAILURE'
  | 'SUCCESS'
  | 'UNKNOWN'

export interface UpstreamTaskItem {
  id: IdString
  site_id: IdString
  site_name: string
  remote_id: IdString
  created_at: Timestamp
  updated_at: Timestamp
  task_id: string
  platform: string
  user_id: NonNegativeIdString
  group: string
  channel_id: NonNegativeIdString
  quota: MetricString
  action: string
  status: UpstreamTaskStatus
  submit_time: Timestamp
  start_time: Timestamp
  finish_time: Timestamp
  progress: string
  properties: { model: string }
  first_seen_at: Timestamp
  last_seen_at: Timestamp
}

export interface UpstreamTaskPage {
  items: UpstreamTaskItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface UpstreamTaskMetric {
  total: MetricString
  queued: MetricString
  running: MetricString
  success: MetricString
  failure: MetricString
  success_rate: DecimalString | null
  avg_queue_seconds: DecimalString | null
  avg_run_seconds: DecimalString | null
  avg_total_seconds: DecimalString | null
}

export interface UpstreamTaskBreakdown extends UpstreamTaskMetric {
  dimension_id: string
  dimension_name: string
  site_id: NonNegativeIdString
  site_name: string
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface UpstreamTaskStatisticsResponse {
  summary: UpstreamTaskMetric
  status_breakdown: UpstreamTaskBreakdown[]
  platform_breakdown: UpstreamTaskBreakdown[]
  action_breakdown: UpstreamTaskBreakdown[]
  model_breakdown: UpstreamTaskBreakdown[]
  site_breakdown: UpstreamTaskBreakdown[]
  data_status: DataStatus
}

export interface UpstreamTaskQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  remote_id?: IdString
  remote_user_id?: NonNegativeIdString
  remote_channel_id?: NonNegativeIdString
  task_id?: string
  platforms?: string[]
  groups?: string[]
  actions?: string[]
  statuses?: UpstreamTaskStatus[]
  models?: string[]
  start_timestamp?: Timestamp
  end_timestamp?: Timestamp
}
