import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export type ChannelInventoryState = 'normal' | 'missing'
export interface ChannelInventoryItem {
  id: IdString
  site_id: IdString
  site_name: string
  remote_channel_id: IdString
  name: string
  type: number
  status: number
  test_time: Timestamp
  response_time_ms: MetricString
  balance: DecimalString
  balance_updated_at: Timestamp
  models: string
  group: string
  used_quota: MetricString
  priority: MetricString
  weight: MetricString
  auto_ban: number
  tag: string
  remote_state: ChannelInventoryState
  missing_count: number
  first_seen_at: Timestamp
  last_seen_at: Timestamp | null
}
export interface ChannelInventoryPage {
  items: ChannelInventoryItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
  as_of: Timestamp | null
}
export interface ChannelInventoryMetric {
  channel_count: MetricString
  available_count: MetricString
  unavailable_count: MetricString
  missing_count: MetricString
  balance_total: DecimalString
  used_quota: MetricString
  response_time_avg_ms: DecimalString
  response_time_max_ms: MetricString
  availability_rate: DecimalString
}
export interface ChannelInventoryTrendPoint extends ChannelInventoryMetric {
  bucket_start: Timestamp
  bucket_end: Timestamp
  data_status: DataStatus
}
export interface ChannelInventoryBreakdown extends ChannelInventoryMetric {
  dimension_id: string
  dimension_name: string
  site_id: string
  site_name: string
  data_status: DataStatus
  as_of: Timestamp | null
}
export interface ChannelInventoryStatisticsResponse {
  summary: ChannelInventoryMetric
  trend: ChannelInventoryTrendPoint[]
  type_breakdown: ChannelInventoryBreakdown[]
  status_breakdown: ChannelInventoryBreakdown[]
  group_breakdown: ChannelInventoryBreakdown[]
  tag_breakdown: ChannelInventoryBreakdown[]
  site_breakdown: ChannelInventoryBreakdown[]
  data_status: DataStatus
}
export interface ChannelInventoryQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  keyword?: string
  types?: number[]
  statuses?: number[]
  groups?: string[]
  tags?: string[]
  states?: ChannelInventoryState[]
  min_balance?: DecimalString
  max_balance?: DecimalString
  min_response_time_ms?: MetricString
  max_response_time_ms?: MetricString
}
export interface ChannelInventoryStatisticsQueryParams {
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  site_ids?: IdString[]
  types?: number[]
  statuses?: number[]
  groups?: string[]
  tags?: string[]
}
