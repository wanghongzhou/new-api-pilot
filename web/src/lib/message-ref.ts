import i18n from '@/i18n/config'
import { dynamicI18nKey } from '@/i18n/dynamic-keys'

import type { IdString, Timestamp } from './api-types'
import { isStableI18nCode, type MessageCode } from './message-codes'

type SiteRangeParams = {
  site_id: IdString
  start_timestamp: Timestamp
  end_timestamp: Timestamp
}

type ExportParams = { export_id: IdString }
type CapabilityParams = { site_id: IdString; capability_key: string }
type SiteAlertParams = { site_id: IdString; site_name: string }
type InstanceAlertParams = { site_id: IdString; instance_name: string }
type AccountAlertParams = {
  account_id: IdString
  account_name: string
  site_id?: IdString
}

export type MessageParamsByCode = {
  COLLECTION_RETRY_EXHAUSTED: { site_id: IdString; run_id: IdString }
  DATA_VALIDATION_MISMATCH: SiteRangeParams
  UPSTREAM_RESPONSE_INVALID: {
    site_id: IdString
    capability_key?: string
  }
  UPSTREAM_RESPONSE_TOO_LARGE: {
    site_id: IdString
    response_bytes: string
    limit_bytes: string
  }
  SITE_CONFIG_CHANGED: {
    site_id: IdString
    expected_config_version: number
    actual_config_version: number
  }
  DEPENDENCY_WINDOWS_MISSING: SiteRangeParams & { run_id: IdString }
  WORKER_LEASE_LOST: {
    site_id: IdString
    run_id: IdString
    hour_ts: Timestamp
  }
  EXPORT_DISK_LOW: ExportParams & {
    free_bytes: string
    threshold_bytes: string
  }
  EXPORT_FILE_TOO_LARGE: ExportParams & {
    file_bytes: string
    limit_bytes: string
  }
  EXPORT_SNAPSHOT_FAILED: ExportParams
  EXPORT_WRITE_FAILED: ExportParams
  EXPORT_EXPIRED: ExportParams
  EXPORT_FILE_MISSING: ExportParams
  NOTIFICATION_DISABLED: {
    alert_event_id: IdString | null
    delivery_id: IdString | null
  }
  NOTIFICATION_NOT_CONFIGURED: {
    alert_event_id: IdString | null
    delivery_id: IdString | null
  }
  NOTIFICATION_TEST_SUCCEEDED: {
    delivery_id: IdString
  }
  DINGTALK_ADDRESS_FORBIDDEN: {
    alert_event_id: IdString | null
    delivery_id: IdString | null
  }
  DINGTALK_REJECTED: {
    alert_event_id: IdString | null
    delivery_id: IdString
    errcode: string
  }
  DELIVERY_RETRY_EXHAUSTED: {
    alert_event_id: IdString | null
    delivery_id: IdString
  }
  DELIVERY_RETRY_SCHEDULED: {
    delivery_id: IdString
    next_retry_at: string
  }
  DATA_PENDING: {
    scope_type: string
    scope_id: IdString | null
    progress: number
  }
  DATA_BACKFILLING: {
    scope_type: string
    scope_id: IdString | null
    progress: number
  }
  DATA_WINDOW_MISSING: SiteRangeParams
  DATA_UPSTREAM_UNAVAILABLE: SiteRangeParams
  DATA_SCOPE_PAUSED: {
    scope_type: string
    scope_id: IdString
    start_timestamp: Timestamp
    end_timestamp: Timestamp
  }
  DATA_PARTIAL_SITES: {
    complete_site_count: number
    expected_site_count: number
  }
  DATA_VALIDATION_FAILED: SiteRangeParams
  CAPABILITY_OK: CapabilityParams
  CAPABILITY_UPSTREAM_UNAVAILABLE: CapabilityParams
  CAPABILITY_RESPONSE_INVALID: CapabilityParams
  CAPABILITY_EXPORT_DISABLED: CapabilityParams
  CAPABILITY_IDENTITY_FAILED: CapabilityParams
  CAPABILITY_FIRST_USER_PROOF_FAILED: CapabilityParams
  CAPABILITY_NO_TRAFFIC_SKIPPED: CapabilityParams
  ALERT_SITE_OFFLINE: SiteAlertParams
  ALERT_AUTH_EXPIRED: SiteAlertParams
  ALERT_EXPORT_DISABLED: SiteAlertParams
  ALERT_COLLECTION_MISSING: SiteRangeParams
  ALERT_BACKFILL_FAILED: { site_id: IdString; run_id: IdString }
  ALERT_VALIDATION_FAILED: SiteRangeParams & {
    failure_kind: 'data_mismatch' | 'execution_failed'
  }
  ALERT_INSTANCE_STALE: InstanceAlertParams
  ALERT_INSTANCE_OFFLINE: InstanceAlertParams
  ALERT_NO_INSTANCE: SiteAlertParams
  ALERT_CPU_HIGH: {
    site_id: IdString
    target_type: string
    target_name: string
    value: string
    threshold: string
  }
  ALERT_MEMORY_HIGH: {
    site_id: IdString
    target_type: string
    target_name: string
    value: string
    threshold: string
  }
  ALERT_DISK_HIGH: {
    site_id: IdString
    target_type: string
    target_name: string
    value: string
    threshold: string
  }
  ALERT_ACCOUNT_MISSING: AccountAlertParams
  ALERT_ACCOUNT_IDENTITY_MISMATCH: AccountAlertParams
  ALERT_ACCOUNT_DISABLED: AccountAlertParams
  ALERT_ACCOUNT_QUOTA_EMPTY: AccountAlertParams
  ALERT_CHANNEL_BALANCE_LOW: {
    site_id: IdString
    site_name: string
    value: string
    threshold: string
  }
  ALERT_CHANNEL_RESPONSE_TIME_HIGH: {
    site_id: IdString
    site_name: string
    value: string
    threshold: string
  }
  ALERT_CHANNEL_AVAILABILITY_LOW: {
    site_id: IdString
    site_name: string
    value: string
    threshold: string
  }
  ALERT_SCOPE_INACTIVE: {
    scope_type: string
    scope_id: IdString
    scope_name: string
  }
  SLO_USAGE_DELAY_TOO_HIGH: { value: number; threshold: number }
  SLO_USAGE_CONCURRENCY_TOO_LOW: { value: number; threshold: number }
  INTERNAL_CONTRACT_ERROR: { component: string; value?: string }
}

export interface MessageRef<C extends MessageCode> {
  code: C
  params: MessageParamsByCode[C]
  technical_detail: string
}

export type AnyMessageRef = {
  [C in MessageCode]: MessageRef<C>
}[MessageCode]

export interface UnknownMessageRef {
  code?: unknown
  params?: unknown
  technical_detail?: unknown
}

export function translateMessageRef(
  ref: AnyMessageRef | UnknownMessageRef | null | undefined,
  fallbackKey = 'Request failed'
): string {
  if (!ref || !isStableI18nCode(ref.code)) {
    return i18n.t(dynamicI18nKey('api', fallbackKey))
  }
  const params =
    ref.params && typeof ref.params === 'object'
      ? (ref.params as Record<string, unknown>)
      : {}
  return i18n.t(dynamicI18nKey('api', ref.code), params)
}
