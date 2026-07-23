import type { Timestamp } from '@/lib/api-types'
import type { AnyMessageRef } from '@/lib/message-ref'

export const platformSettingKeys = [
  'collector.probe_interval_seconds',
  'collector.realtime_interval_seconds',
  'collector.resource_interval_seconds',
  'collector.usage_delay_minutes',
  'collector.minute_retention_days',
  'logs.retention_days',
  'performance.retention_days',
  'task.retention_days',
  'system_task_terminal_retention_days',
  'collector.probe_concurrency',
  'collector.realtime_concurrency',
  'collector.resource_concurrency',
  'collector.metadata_concurrency',
  'collector.usage_concurrency',
  'collector.backfill_concurrency',
  'collector.manual_backfill_max_days',
  'fast_task.history_retention_seconds',
  'fast_task.history_count',
  'upstream.allowed_host_suffixes',
  'upstream.allowed_cidrs',
  'upstream.connect_timeout_seconds',
  'upstream.response_header_timeout_seconds',
  'upstream.request_timeout_seconds',
  'upstream.export_timeout_seconds',
  'upstream.rate_limit_requests',
  'upstream.rate_limit_window_seconds',
  'upstream.max_inflight_per_origin',
  'export.file_ttl_hours',
  'export.max_active_per_user',
  'export.max_active_global',
  'export.max_file_bytes',
  'export.min_free_disk_bytes',
  'rate.fallback_quota_per_unit',
  'rate.fallback_usd_exchange_rate',
  'notification.dingtalk.enabled',
  'notification.dingtalk.webhook',
  'notification.dingtalk.secret',
] as const

export type PlatformSettingKey = (typeof platformSettingKeys)[number]
export type SettingKey = PlatformSettingKey | 'system.public_origin'
export type SettingGroupKey =
  | 'collector'
  | 'export'
  | 'notification'
  | 'rate'
  | 'system'
  | 'upstream'
export type SettingValueType = 'bool' | 'decimal' | 'int' | 'string'
export interface SettingItem {
  key: SettingKey
  value_type: SettingValueType
  value: boolean | number | string | null
  read_only: boolean
  secret: boolean
  configured: boolean
  decrypt_error: boolean
  masked_value: string
  constraints: Readonly<Record<string, unknown>>
  updated_at: Timestamp | null
}

export interface SettingGroup {
  key: SettingGroupKey
  label_key: string
  items: SettingItem[]
}

export interface SettingPatchItem {
  key: PlatformSettingKey
  value?: boolean | number | string
  clear?: boolean
}

export interface SettingPatchRequest {
  items: SettingPatchItem[]
}

export interface NotificationTestResult {
  delivery_id: string | null
  status: 'failed' | 'pending' | 'success'
  response_code: number | null
  message: AnyMessageRef
}

export type SecretAction = 'clear' | 'keep' | 'replace'

export interface SettingsFormValues {
  usageDelayMinutes: string
  minuteRetentionDays: string
  logRetentionDays: string
  performanceRetentionDays: string
  taskRetentionDays: string
  systemTaskTerminalRetentionDays: string
  probeConcurrency: string
  realtimeConcurrency: string
  resourceConcurrency: string
  metadataConcurrency: string
  usageConcurrency: string
  backfillConcurrency: string
  manualBackfillMaxDays: string
  fastTaskHistoryRetentionSeconds: string
  fastTaskHistoryCount: string
  upstreamAllowedHostSuffixes: string
  upstreamAllowedCidrs: string
  upstreamConnectTimeoutSeconds: string
  upstreamResponseHeaderTimeoutSeconds: string
  upstreamRequestTimeoutSeconds: string
  upstreamExportTimeoutSeconds: string
  upstreamRateLimitRequests: string
  upstreamRateLimitWindowSeconds: string
  upstreamMaxInflightPerOrigin: string
  fileTtlHours: string
  maxActivePerUser: string
  maxActiveGlobal: string
  maxFileBytes: string
  minFreeDiskBytes: string
  fallbackQuotaPerUnit: string
  fallbackUsdExchangeRate: string
  dingTalkEnabled: boolean
  dingTalkWebhook: string
  dingTalkWebhookAction: SecretAction
  dingTalkSecret: string
  dingTalkSecretAction: SecretAction
}

export interface SecretSettingState {
  configured: boolean
  decryptError: boolean
}

export interface SettingsSecretState {
  webhook: SecretSettingState
  secret: SecretSettingState
}
