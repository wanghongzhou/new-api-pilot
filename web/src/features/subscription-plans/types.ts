import type {
  DataStatus,
  DecimalString,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'

export type SubscriptionPlanState = 'normal' | 'missing'
export type SubscriptionPlanTab = 'plans' | 'site-analysis'
export type SubscriptionDurationUnit =
  | 'year'
  | 'month'
  | 'day'
  | 'hour'
  | 'custom'
export type SubscriptionResetPeriod =
  | 'never'
  | 'daily'
  | 'weekly'
  | 'monthly'
  | 'custom'

export interface SubscriptionPlanItem {
  id: IdString
  site_id: IdString
  remote_id: IdString
  site_name: string
  title: string
  subtitle: string
  price_amount: DecimalString
  currency: string
  duration_unit: SubscriptionDurationUnit
  duration_value: number
  custom_seconds: MetricString
  enabled: boolean
  sort_order: number
  total_amount: MetricString
  quota_reset_period: SubscriptionResetPeriod
  quota_reset_custom_seconds: MetricString
  created_at: Timestamp
  updated_at: Timestamp
  remote_state: SubscriptionPlanState
  missing_count: number
  data_status: DataStatus
}

export interface SubscriptionPlanPage {
  items: SubscriptionPlanItem[]
  total: number
  page: number
  page_size: number
  data_status: DataStatus
}

export interface SubscriptionPlanBreakdown {
  site_id: IdString
  site_name: string
  total: MetricString
  enabled: MetricString
  disabled: MetricString
  missing: MetricString
  data_status: DataStatus
  as_of: Timestamp | null
}

export interface SubscriptionPlanStatistics {
  total: MetricString
  enabled: MetricString
  disabled: MetricString
  missing: MetricString
  data_status: DataStatus
  site_breakdown: SubscriptionPlanBreakdown[]
}

export interface SubscriptionPlanQueryParams {
  p: number
  page_size: number
  site_ids?: IdString[]
  states?: SubscriptionPlanState[]
  enabled?: boolean
  keyword?: string
}
