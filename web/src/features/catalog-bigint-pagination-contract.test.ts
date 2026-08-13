import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

const featureTypeFiles = [
  'model-catalog/types.ts',
  'pricing-groups/types.ts',
  'user-inventory/types.ts',
  'channel-inventory/types.ts',
  'subscription-plans/types.ts',
]

describe('resource catalog pagination precision', () => {
  test.each(featureTypeFiles)('%s keeps page total as MetricString', (file) => {
    const source = readFileSync(new URL(file, import.meta.url), 'utf8')
    expect(source).not.toMatch(/\btotal:\s*number\b/)
    expect(source).toMatch(/\btotal:\s*MetricString\b/)
  })

  test('all catalog pages pass backend totals directly to the shared table', () => {
    for (const file of [
      'model-catalog/components/model-catalog-page.tsx',
      'pricing-groups/components/pricing-groups-page.tsx',
      'user-inventory/components/user-inventory-page.tsx',
      'channel-inventory/components/channel-inventory-page.tsx',
      'subscription-plans/components/subscription-plans-page.tsx',
    ]) {
      const source = readFileSync(new URL(file, import.meta.url), 'utf8')
      expect(source).not.toMatch(/Number\([^)]*\.total\)/)
      expect(source).not.toMatch(/Math\.ceil\([^)]*\.total/)
    }
  })
})
