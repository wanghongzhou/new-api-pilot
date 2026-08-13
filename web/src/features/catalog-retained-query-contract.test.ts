import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const catalogPages = [
  new URL('./model-catalog/components/model-catalog-page.tsx', import.meta.url),
  new URL(
    './pricing-groups/components/pricing-groups-page.tsx',
    import.meta.url
  ),
  new URL(
    './subscription-plans/components/subscription-plans-page.tsx',
    import.meta.url
  ),
  new URL(
    './channel-inventory/components/channel-inventory-page.tsx',
    import.meta.url
  ),
] as const

describe('resource catalog retained query contract', () => {
  for (const page of catalogPages) {
    test(`${page.pathname.split('/').at(-3)} retains the last successful response after a refresh failure`, async () => {
      const source = await readFile(page, 'utf8')

      expect(source).toContain('useRetainedQueryData')
      expect(source).toContain("siteId ? `site:${siteId}` : 'global'")
      expect(source).toContain('common.retainedDataRefreshFailed')
    })
  }
})
