import { describe, expect, test } from 'bun:test'

import { parseIdString, parseNonNegativeIdString } from '@/lib/api-types'

import { buildFinancialOperationsExportRequest } from './export-request'
import { buildFinancialOperationsSearch } from './search'

describe('financial operations export request', () => {
  test('freezes topup safe filters without pagination or secret material', () => {
    const request = buildFinancialOperationsExportRequest(
      'xlsx',
      buildFinancialOperationsSearch({
        end: 1_784_348_800,
        methods: ['stripe'],
        providers: ['provider-a'],
        remoteId: parseIdString('9007199254740993'),
        remoteUserId: parseNonNegativeIdString('0'),
        siteIds: [parseIdString('9007199254740995')],
        start: 1_784_262_400,
        states: ['normal'],
        statuses: ['success'],
        tab: 'topups',
      })
    )
    expect(request.statistics_type).toBe('topup_inventory')
    expect(request.filters.finance_providers).toEqual(['provider-a'])
    expect(request.filters.remote_user_id).toBe(parseNonNegativeIdString('0'))
    const serialized = JSON.stringify(request).toLowerCase()
    for (const forbidden of [
      ['trade', 'no'].join('_'),
      'secret',
      'cipher',
      'payment_reference',
    ]) {
      expect(serialized).not.toContain(forbidden)
    }
  })

  test('selects the redemption export scope from the deep-linked tab', () => {
    const request = buildFinancialOperationsExportRequest(
      'csv',
      buildFinancialOperationsSearch({ tab: 'redemptions' })
    )
    expect(request.statistics_type).toBe('redemption_inventory')
  })
})
