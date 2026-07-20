import { describe, expect, test } from 'bun:test'

import { exportScopes } from './exports-page'

describe('extended A89-A100 export scopes', () => {
  test('keeps every A89-A100 frontend export scope selectable', () => {
    expect(exportScopes).toEqual(
      expect.arrayContaining([
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
      ])
    )
  })
})
