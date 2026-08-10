import type {
  DataStatus,
  IdString,
  MetricString,
  NonNegativeIdString,
  Timestamp,
  RateInfo,
} from '@/lib/api-types'

export type LogDataStatus = DataStatus | 'disabled'
export type LogType = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7

export interface LogItem {
  id: IdString
  site_id: IdString
  site_name: string
  created_at: Timestamp
  type: LogType
  remote_user_id: NonNegativeIdString
  username: string
  model_name: string
  token_id: NonNegativeIdString
  token_name: string
  channel_id: NonNegativeIdString
  group: string
  request_id: string
  upstream_request_id: string
  quota: MetricString
  prompt_tokens: MetricString
  completion_tokens: MetricString
  cache_read_tokens: MetricString
  cache_creation_tokens: MetricString
  cache_creation_tokens_5m: MetricString
  cache_creation_tokens_1h: MetricString
  use_time_seconds: MetricString
  is_stream: boolean
  first_response_time_ms: MetricString | null
  stream_status: string
  stream_end_reason: string
  stream_error_count: MetricString
  content: string
  ip: string
  rate: RateInfo
}

export interface LogStatSiteBreakdown {
  site_id: IdString
  site_name: string
  quota: MetricString
  rate: RateInfo
}

export interface LogStatResponse {
  quota: MetricString
  rpm: MetricString
  tpm: MetricString
  site_breakdown: LogStatSiteBreakdown[]
}

export interface LogResponse {
  items: LogItem[]
  total: number
  page: number
  page_size: number
  data_status: LogDataStatus
  as_of: Timestamp | null
}

export interface LogQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  type?: LogType
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  username?: string
  model_name?: string
  token_name?: string
  channel_id?: NonNegativeIdString
  group?: string
  request_id?: string
  upstream_request_id?: string
}

export interface LogSearch {
  page: number
  pageSize: number
  siteIds: IdString[]
  type?: LogType
  start: Timestamp
  end: Timestamp
  username: string
  modelName: string
  tokenName: string
  channelId?: NonNegativeIdString
  group: string
  requestId: string
  upstreamRequestId: string
  exportId?: IdString
}
