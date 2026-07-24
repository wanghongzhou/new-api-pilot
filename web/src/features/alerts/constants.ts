export const alertTabs = ['events', 'rules'] as const
export const alertStatuses = ['firing', 'pending', 'resolved'] as const
export const alertLevels = ['critical', 'warning', 'info'] as const
export const alertTargetTypes = [
  'site',
  'instance',
  'account',
  'collection',
] as const
export const alertSortFields = [
  'rule_key',
  'status',
  'level',
  'site_name',
  'first_fired_at',
  'last_fired_at',
  'resolved_at',
] as const
export const alertRuleScopes = ['global', 'site'] as const
export const alertRuleCategories = [
  'site',
  'collection',
  'instance',
  'account',
  'channel',
] as const
export const alertRuleSortFields = [
  'category',
  'rule_key',
  'level',
  'metric',
  'enabled',
  'updated_at',
] as const
export const alertDeliveryStatuses = ['pending', 'success', 'failed'] as const
export const alertDeliveryEventTypes = ['firing', 'resolved', 'test'] as const

export const builtInAlertRuleKeys = [
  'site_offline',
  'site_auth_expired',
  'site_export_disabled',
  'collection_missing',
  'backfill_failed',
  'validation_failed',
  'instance_stale',
  'instance_offline',
  'site_no_instance',
  'cpu_high',
  'memory_high',
  'disk_high',
  'account_missing',
  'account_identity_mismatch',
  'account_disabled',
  'account_quota_empty',
  'channel_balance_low',
  'channel_response_time_high',
  'channel_availability_low',
] as const
