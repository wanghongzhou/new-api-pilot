import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const resourcePages = [
  new URL(
    './user-inventory/components/user-inventory-page.tsx',
    import.meta.url
  ),
  new URL(
    './upstream-tasks/components/upstream-tasks-page.tsx',
    import.meta.url
  ),
  new URL('./system-tasks/components/system-tasks-page.tsx', import.meta.url),
] as const

describe('resource retained query contract', () => {
  for (const page of resourcePages) {
    test(`${page.pathname.split('/').at(-3)} retains successful data without crossing site scopes`, async () => {
      const source = await readFile(page, 'utf8')

      expect(source).toContain('useRetainedQueryData')
      expect(source).toContain("siteId ? `site:${siteId}` : 'global'")
      expect(source).toContain('common.retainedDataRefreshFailed')
    })
  }
})
