import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import { exportScopeGroups, exportScopes } from './exports-filter-options'

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

  test('groups every export scope exactly once for the compact searchable filter', () => {
    const groupedScopes = exportScopeGroups.flatMap((group) => group.scopes)
    expect(groupedScopes).toHaveLength(exportScopes.length)
    expect(new Set(groupedScopes).size).toBe(exportScopes.length)
    expect(groupedScopes).toEqual(expect.arrayContaining(exportScopes))
  })

  test('keeps the export filters compact on desktop and full width on mobile', async () => {
    const source = await readFile(
      new URL('./exports-page.tsx', import.meta.url),
      'utf8'
    )
    expect(source).toContain('<MultiFacetedFilter')
    expect(source.match(/<FacetedFilter/g)).toHaveLength(2)
    expect(source).toContain("className='w-full justify-between sm:w-40'")
    expect(source).toContain("className='w-full justify-between sm:w-64'")
    expect(source).not.toContain('sm:grid-cols-3')
  })
})
