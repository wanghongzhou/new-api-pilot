import type { StatisticsExportScope } from '../types'

export const exportScopes: StatisticsExportScope[] = [
  'global',
  'site',
  'customer',
  'account',
  'model',
  'channel',
  'group',
  'token',
  'node',
  'logs',
  'user_inventory',
  'channel_inventory',
  'performance_history',
  'topup_inventory',
  'redemption_inventory',
  'upstream_tasks',
  'model_catalog',
  'model_rankings',
  'vendor_rankings',
  'subscription_plans',
  'pricing_catalog',
  'group_catalog',
  'system_tasks',
]

export const exportScopeGroups: Array<{
  key: 'finance' | 'operations' | 'resources' | 'tasks'
  scopes: StatisticsExportScope[]
}> = [
  {
    key: 'operations',
    scopes: [
      'global',
      'site',
      'customer',
      'account',
      'model',
      'channel',
      'group',
      'token',
      'node',
    ],
  },
  { key: 'tasks', scopes: ['logs', 'upstream_tasks', 'system_tasks'] },
  {
    key: 'resources',
    scopes: [
      'user_inventory',
      'channel_inventory',
      'model_catalog',
      'subscription_plans',
      'pricing_catalog',
      'group_catalog',
    ],
  },
  {
    key: 'finance',
    scopes: [
      'topup_inventory',
      'redemption_inventory',
      'model_rankings',
      'vendor_rankings',
      'performance_history',
    ],
  },
]
