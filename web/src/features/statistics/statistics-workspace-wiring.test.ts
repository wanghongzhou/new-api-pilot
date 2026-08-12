import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

test('uses the shared statistics workspace for site-detail statistics', async () => {
  const source = await readFile(
    new URL('../sites/components/site-stats-page.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain(
    "from '@/features/statistics/components/statistics-page'"
  )
  expect(source).toContain('<StatisticsPage')
  expect(source).toContain('dataSource={dataSource}')
  expect(source).toContain("entity={{ id: parsedSiteId, scope: 'site' }}")
  expect(source).toContain('hideScopeNavigation')
  expect(source).not.toContain('<EntityStatistics')
})
