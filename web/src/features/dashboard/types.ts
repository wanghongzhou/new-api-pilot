import type { Completeness } from '@/features/sites/types'
import type {
  DataStatus,
  IdString,
  MetricString,
  Timestamp,
} from '@/lib/api-types'
import type { AnyMessageRef } from '@/lib/message-ref'

import type { SiteQuotaBreakdown, TrendPoint } from '../statistics/types'

export interface DashboardUsageSummary {
  request_count: MetricString | null
  quota: MetricString | null
  token_used: MetricString | null
  active_users: MetricString | null
  as_of: Timestamp | null
  data_status: DataStatus
  is_partial: boolean
  is_final: boolean
  site_breakdown: SiteQuotaBreakdown[]
  reason: AnyMessageRef | null
}

export interface DashboardSummary {
  today: DashboardUsageSummary
  active_accounts_today: MetricString | null
  site_count: number
  online_site_count: number
  offline_site_count: number
  customer_count: number
  managed_account_count: number
  instance_count: number | null
  online_instance_count: number | null
  resource_complete_site_count: number
  resource_expected_site_count: number
  resource_stale_site_ids: IdString[]
  resource_data_status: DataStatus
  resource_as_of: Timestamp | null
  resource_reason: AnyMessageRef | null
  rpm: MetricString | null
  tpm: MetricString | null
  realtime_complete_site_count: number
  realtime_expected_site_count: number
  stale_site_ids: IdString[]
  realtime_data_status: DataStatus
  realtime_as_of: Timestamp | null
  realtime_reason: AnyMessageRef | null
}

export type DashboardTopType = 'site' | 'customer' | 'model' | 'channel'
export type DashboardTopMetric = 'request_count' | 'quota'

export interface DashboardRankingItem {
  dimension_type: DashboardTopType
  dimension_id: string
  dimension_name: string
  site_id: IdString | null
  value: MetricString | null
  data_status: DataStatus
  site_breakdown: SiteQuotaBreakdown[]
  as_of: Timestamp | null
  is_final: boolean
  reason: AnyMessageRef | null
}

export interface DashboardSiteHealthItem {
  site_id: IdString
  site_name: string
  management_status: string
  online_status: string
  auth_status: string
  statistics_status: string
  health_status: string
  updated_at: Timestamp
}

export interface DashboardAlertItem {
  id: IdString
  site_id: IdString | null
  site_name: string
  target_name: string
  level: 'critical' | 'warning' | 'info'
  status: string
  message: AnyMessageRef
  first_observed_at: Timestamp
  last_fired_at: Timestamp | null
}

export interface DashboardHealth {
  firing_alert_count: number
  critical_alert_count: number
  warning_alert_count: number
  auth_expired_site_ids: IdString[]
  statistics_not_ready_site_ids: IdString[]
  yesterday_validation_status: DataStatus
  completeness: Completeness
  latest_alerts: DashboardAlertItem[]
  sites: DashboardSiteHealthItem[]
  as_of: Timestamp | null
  is_final: boolean
  reason: AnyMessageRef | null
}

export type DashboardTrend = TrendPoint[]
