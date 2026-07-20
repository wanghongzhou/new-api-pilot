import type {
  DataStatus,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export type UserInventoryState =
  | 'normal'
  | 'missing'
  | 'deleted'
  | 'identity_mismatch'

export interface UserInventoryItem {
  id: IdString
  site_id: IdString
  site_name: string
  remote_user_id: IdString
  remote_created_at: Timestamp
  username: string
  display_name: string
  role: number
  status: number
  group: string
  quota: MetricString
  used_quota: MetricString
  balance: MetricString
  request_count: MetricString
  last_login_at: Timestamp
  remote_state: UserInventoryState
  missing_count: number
  first_seen_at: Timestamp
  last_seen_at: Timestamp | null
  account_id: IdString | null
}

export interface UserInventoryPage {
  items: UserInventoryItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
}

export interface UserInventoryMetric {
  user_count: MetricString
  new_user_count: MetricString
  active_user_count: MetricString
  quota: MetricString
  used_quota: MetricString
  balance: MetricString
  request_count: MetricString
}

export interface UserInventoryTrendPoint extends UserInventoryMetric {
  bucket_start: Timestamp
  bucket_end: Timestamp
  data_status: DataStatus
}

export interface UserInventoryBreakdown extends UserInventoryMetric {
  dimension_id: string
  dimension_name: string
  site_id: string
}

export interface UserInventorySiteBreakdown extends UserInventoryMetric {
  site_id: IdString
  site_name: string
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface UserInventoryStatisticsResponse {
  summary: UserInventoryMetric
  trend: UserInventoryTrendPoint[]
  role_breakdown: UserInventoryBreakdown[]
  status_breakdown: UserInventoryBreakdown[]
  group_breakdown: UserInventoryBreakdown[]
  site_breakdown: UserInventorySiteBreakdown[]
  data_status: DataStatus
}

export interface UserInventoryQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  keyword?: string
  remote_user_id?: IdString
  roles?: number[]
  statuses?: number[]
  groups?: string[]
  states?: UserInventoryState[]
  min_balance?: MetricString
  max_balance?: MetricString
}

export interface UserInventoryStatisticsQueryParams {
  start_timestamp: Timestamp
  end_timestamp: Timestamp
  site_ids?: IdString[]
  roles?: number[]
  statuses?: number[]
  groups?: string[]
}
