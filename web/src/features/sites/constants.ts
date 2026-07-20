import type {
  SiteAuthStatus,
  SiteHealthStatus,
  SiteManagementStatus,
  SiteOnlineStatus,
  SiteStatisticsStatus,
  SiteCapabilityKey,
  CollectionTaskType,
  FastCollectionTaskType,
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
  'performance_sync',
  'topup_sync',
  'redemption_sync',
  'upstream_task_sync',
  'model_meta_sync',
  'plan_sync',
  'pricing_group_sync',
  'system_task_sync',
  'user_sync',
  'channel_sync',
  'log_sync',
  'usage_hour',
  'usage_backfill',
  'usage_validation',
  'account_rebuild',
  'customer_rebuild',
] as const satisfies readonly CollectionTaskType[]

export type CollectionTaskCategory =
  | 'fast'
  | 'durable'
  | 'hourly'
  | 'usage'
  | 'rebuild'

export type CollectionTaskTriggerClass =
  | 'fast_interval'
  | 'resource_interval'
  | 'hour_boundary'
  | 'usage_delay'
  | 'event_backfill'
  | 'validation_calendar'
  | 'event_rebuild'

interface CollectionTaskCatalogItem {
  category: CollectionTaskCategory
  purposeKey: `siteTasks.purpose.${string}`
  targetType: 'account' | 'customer' | 'site'
  triggerClass: CollectionTaskTriggerClass
}

export const collectionTaskCatalog = {
  site_probe: {
    category: 'fast',
    purposeKey: 'siteTasks.purpose.siteProbe',
    targetType: 'site',
    triggerClass: 'fast_interval',
  },
  realtime_stat: {
    category: 'fast',
    purposeKey: 'siteTasks.purpose.realtimeStat',
    targetType: 'site',
    triggerClass: 'fast_interval',
  },
  resource_snapshot: {
    category: 'fast',
    purposeKey: 'siteTasks.purpose.resourceSnapshot',
    targetType: 'site',
    triggerClass: 'fast_interval',
  },
  performance_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.performanceSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  topup_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.topupSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  redemption_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.redemptionSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  upstream_task_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.upstreamTaskSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  model_meta_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.modelMetaSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  plan_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.planSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  pricing_group_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.pricingGroupSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  system_task_sync: {
    category: 'durable',
    purposeKey: 'siteTasks.purpose.systemTaskSync',
    targetType: 'site',
    triggerClass: 'resource_interval',
  },
  user_sync: {
    category: 'hourly',
    purposeKey: 'siteTasks.purpose.userSync',
    targetType: 'site',
    triggerClass: 'hour_boundary',
  },
  channel_sync: {
    category: 'hourly',
    purposeKey: 'siteTasks.purpose.channelSync',
    targetType: 'site',
    triggerClass: 'hour_boundary',
  },
  log_sync: {
    category: 'hourly',
    purposeKey: 'siteTasks.purpose.logSync',
    targetType: 'site',
    triggerClass: 'hour_boundary',
  },
  usage_hour: {
    category: 'usage',
    purposeKey: 'siteTasks.purpose.usageHour',
    targetType: 'site',
    triggerClass: 'usage_delay',
  },
  usage_backfill: {
    category: 'usage',
    purposeKey: 'siteTasks.purpose.usageBackfill',
    targetType: 'site',
    triggerClass: 'event_backfill',
  },
  usage_validation: {
    category: 'usage',
    purposeKey: 'siteTasks.purpose.usageValidation',
    targetType: 'site',
    triggerClass: 'validation_calendar',
  },
  account_rebuild: {
    category: 'rebuild',
    purposeKey: 'siteTasks.purpose.accountRebuild',
    targetType: 'account',
    triggerClass: 'event_rebuild',
  },
  customer_rebuild: {
    category: 'rebuild',
    purposeKey: 'siteTasks.purpose.customerRebuild',
    targetType: 'customer',
    triggerClass: 'event_rebuild',
  },
} as const satisfies Record<CollectionTaskType, CollectionTaskCatalogItem>

export const collectionTaskCategories = [
  'fast',
  'durable',
  'hourly',
  'usage',
  'rebuild',
] as const satisfies readonly CollectionTaskCategory[]

export const fastCollectionTaskTypes = [
  'site_probe',
  'realtime_stat',
  'resource_snapshot',
] as const satisfies readonly FastCollectionTaskType[]

export function isFastCollectionTaskType(
  value: unknown
): value is FastCollectionTaskType {
  return (
    typeof value === 'string' &&
    (fastCollectionTaskTypes as readonly string[]).includes(value)
  )
}

export const retryableSiteUsageTaskTypes = [
  'usage_hour',
  'usage_backfill',
  'usage_validation',
] as const satisfies readonly CollectionTaskType[]
