import type { IdString, PageData, Timestamp } from '@/lib/api-types'
import type { AnyMessageRef } from '@/lib/message-ref'

import type {
  alertDeliveryEventTypes,
  alertDeliveryStatuses,
  alertLevels,
  alertRuleScopes,
  alertSortFields,
  alertStatuses,
  alertTabs,
  alertTargetTypes,
} from './constants'

export type AlertTab = (typeof alertTabs)[number]
export type AlertStatus = (typeof alertStatuses)[number]
export type AlertLevel = (typeof alertLevels)[number]
export type AlertTargetType = (typeof alertTargetTypes)[number]
export type AlertSortField = (typeof alertSortFields)[number]
export type AlertRuleScope = (typeof alertRuleScopes)[number]
export type AlertDeliveryStatus = (typeof alertDeliveryStatuses)[number]
export type AlertDeliveryEventType = (typeof alertDeliveryEventTypes)[number]
export type AlertResolutionReason =
  | 'recovered'
  | 'remediated'
  | 'retired'
  | 'superseded'

export interface AlertSearch {
  alertId?: IdString
  end?: Timestamp
  level: AlertLevel[]
  order: 'asc' | 'desc'
  page: number
  pageSize: number
  ruleSiteId?: IdString
  scope: AlertRuleScope
  siteId?: IdString
  sort?: AlertSortField
  start?: Timestamp
  status: AlertStatus[]
  tab: AlertTab
  targetType: AlertTargetType[]
}

export interface AlertListParams {
  p: number
  page_size: number
  status?: AlertStatus[]
  level?: AlertLevel[]
  target_type?: AlertTargetType[]
  site_id?: IdString
  start_timestamp?: Timestamp
  end_timestamp?: Timestamp
  sort_by?: AlertSortField
  sort_order: 'asc' | 'desc'
}

export interface AlertEventItem {
  id: IdString
  rule_id: IdString
  rule_key: string
  site_id: IdString | null
  site_name: string
  target_type: AlertTargetType
  target_key: string
  target_name: string
  level: AlertLevel
  status: AlertStatus
  current_value: string | null
  threshold_value: string | null
  message: AnyMessageRef
  first_observed_at: Timestamp
  first_fired_at: Timestamp | null
  last_fired_at: Timestamp | null
  resolved_at: Timestamp | null
  resolution_reason: AlertResolutionReason | null
}

export interface AlertDeliveryItem {
  id: IdString
  event_type: AlertDeliveryEventType
  status: AlertDeliveryStatus
  attempt_count: number
  error_code: string
  response_code: number | null
  response_message: string
  next_retry_at: Timestamp | null
  sent_at: Timestamp | null
}

export interface AlertEventDetail extends AlertEventItem {
  consecutive_count: number
  deliveries: AlertDeliveryItem[]
}

export interface AlertSummary {
  firing_count: number
  critical_count: number
  warning_count: number
  resolved_today_count: number
  updated_at: Timestamp
}

export type AlertEventPage = PageData<AlertEventItem>

export type AlertRuleValueKind =
  | 'boolean'
  | 'percentage'
  | 'seconds'
  | 'count'
  | 'quota'

export interface AlertRuleConstraints {
  value_kind: AlertRuleValueKind
  threshold_editable: boolean
  threshold_min: string | null
  threshold_max: string | null
  threshold_step: string | null
  for_times_editable: boolean
  for_times_min: number
  for_times_max: number
  paired_rule_id: IdString | null
  relation: 'warning_lt_critical' | null
}

export type AlertRuleEditableField = 'enabled' | 'threshold_value' | 'for_times'

export interface AlertRuleItem {
  id: IdString
  effective_rule_id: IdString
  base_rule_id: IdString
  override_rule_id: IdString | null
  rule_key: string
  name: string
  enabled: boolean
  level: AlertLevel
  metric: string
  compare_operator: '>=' | '<=' | '=='
  threshold_value: string | null
  for_times: number
  scope_type: AlertRuleScope
  scope_id: '0' | IdString
  inherited: boolean
  editable_fields: AlertRuleEditableField[]
  constraints: AlertRuleConstraints
  updated_at: Timestamp
}

export interface AlertRuleUpdateRequest {
  enabled?: boolean
  threshold_value?: string
  for_times?: number
}

export interface AlertRuleOverrideRequest extends AlertRuleUpdateRequest {
  base_rule_id: IdString
  site_id: IdString
}

export interface AlertRuleFormValues {
  enabled: boolean
  thresholdValue: string
  forTimes: string
}
