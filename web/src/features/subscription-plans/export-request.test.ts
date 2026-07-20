import { describe, expect, test } from 'bun:test'

import { parseIdString } from '@/lib/api-types'

import { buildSubscriptionPlanExportRequest } from './export-request'
import { buildSubscriptionPlanSearch } from './search'

describe('subscription plan export request', () => {
  test('freezes global safe filters including explicit disabled', () => {
    const request = buildSubscriptionPlanExportRequest(
      'xlsx',
      buildSubscriptionPlanSearch({
        enabled: false,
        keyword: 'safe-plan',
        siteIds: ['9007199254740993'],
        states: ['missing'],
      })
    )

    expect(request.statistics_type).toBe('subscription_plans')
    expect(request.filters.subscription_plan_enabled).toBe(false)
    expect(request.filters.inventory_states).toEqual(['missing'])
    expect(request.filters.site_ids).toEqual([
      parseIdString('9007199254740993'),
    ])
  })

  test('forces site scope without payment or policy material', () => {
    const request = buildSubscriptionPlanExportRequest(
      'csv',
      buildSubscriptionPlanSearch({ siteIds: ['9007199254740995'] }),
      parseIdString('9007199254740993')
    )

    expect(request.filters.site_ids).toEqual([
      parseIdString('9007199254740993'),
    ])
    const serialized = JSON.stringify(request).toLowerCase()
    for (const field of [
      ['stripe', 'price', 'id'].join('_'),
      ['creem', 'product', 'id'].join('_'),
      ['waffo', 'product', 'id'].join('_'),
      ['allow', 'balance', 'pay'].join('_'),
      ['wallet', 'overflow'].join('_'),
      ['max', 'purchase'].join('_'),
      ['upgrade', 'group'].join('_'),
      ['downgrade', 'group'].join('_'),
    ]) {
      expect(serialized).not.toContain(field)
    }
  })
})
