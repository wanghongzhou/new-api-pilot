import type {
  SiteAuthStatus,
  SiteHealthStatus,
  SiteManagementStatus,
  SiteOnlineStatus,
  SiteStatisticsStatus,
  SiteCapabilityKey,
  CollectionTaskType,
} from './types'

export const siteManagementStatuses = [
  'active',
  'disabled',
] as const satisfies readonly SiteManagementStatus[]

export const siteOnlineStatuses = [
  'unknown',
  'online',
  'offline',
] as const satisfies readonly SiteOnlineStatus[]

export const siteAuthStatuses = [
  'unauthorized',
  'authorized',
  'expired',
] as const satisfies readonly SiteAuthStatus[]

export const siteStatisticsStatuses = [
  'pending_config',
  'backfilling',
  'ready',
  'partial',
  'error',
  'paused',
] as const satisfies readonly SiteStatisticsStatus[]

export const siteHealthStatuses = [
  'ok',
  'warning',
  'critical',
  'unavailable',
] as const satisfies readonly SiteHealthStatus[]

export const siteCapabilityKeys = [
  'status_contract',
  'self_identity',
  'root_identity',
  'first_user_proof',
  'user_pagination',
  'channel_pagination',
  'data_export_enabled',
  'flow_contract',
  'data_contract',
  'flow_data_consistency',
  'instance_contract',
  'realtime_contract',
] as const satisfies readonly SiteCapabilityKey[]

export const collectionRunStatuses = [
  'pending',
  'running',
  'success',
  'failed',
] as const

export const collectionRunWindowStatuses = [
  'pending',
  'running',
  'success',
  'failed',
  'unavailable',
] as const

export const collectionTaskTypes = [
  'site_probe',
  'realtime_stat',
  'resource_snapshot',
  'user_sync',
  'channel_sync',
  'usage_hour',
  'usage_backfill',
  'usage_validation',
  'account_rebuild',
  'customer_rebuild',
] as const satisfies readonly CollectionTaskType[]

export const retryableSiteUsageTaskTypes = [
  'usage_hour',
  'usage_backfill',
  'usage_validation',
] as const satisfies readonly CollectionTaskType[]
