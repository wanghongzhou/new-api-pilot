import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pages = {
  finance: new URL(
    '../financial-operations/components/financial-operations-page.tsx',
    import.meta.url
  ),
  performance: new URL(
    '../performance-history/components/performance-history-page.tsx',
    import.meta.url
  ),
  rankings: new URL(
    '../rankings/components/rankings-page.tsx',
    import.meta.url
  ),
} as const

describe('operations analytics retained query boundaries', () => {
  test('rankings retains failed filtered refreshes without mixing ranking dimensions', async () => {
    const source = await readFile(pages.rankings, 'utf8')

    expect(source).toContain('useRetainedQueryData(')
    expect(source).toContain("'global'}:${search.tab}")
  })

  test('performance retains list and statistics only inside the same site scope', async () => {
    const source = await readFile(pages.performance, 'utf8')

    expect(source.match(/useRetainedQueryData\(/g)).toHaveLength(2)
    expect(source).toContain("siteId ? `site:${siteId}` : 'global'")
  })

  test('finance retains lists and statistics without crossing site or operation type', async () => {
    const source = await readFile(pages.finance, 'utf8')

    expect(source.match(/useRetainedQueryData\(/g)).toHaveLength(3)
    expect(source).toContain("'global'}:${search.tab}")
  })
})
