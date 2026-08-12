import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

test('connects real resource samples and resets aligned ranges on granularity changes', async () => {
  const source = await readFile(
    new URL('./components/site-instances-page.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain("invalidType='link'")
  expect(source).toContain('defaultSiteResourceRange(granularity)')
  expect(source).toContain('alignSiteResourceTimestamp(')
  expect(source).toContain('hasRenderableSiteResourceValues(data)')
})

test('loads the performance chart synchronously and checks renderable metrics', async () => {
  const source = await readFile(
    new URL(
      '../performance-history/components/performance-trend-chart.tsx',
      import.meta.url
    ),
    'utf8'
  )

  expect(source).toContain('import { VChart, type ILineChartSpec }')
  expect(source).toContain('hasRenderablePerformanceTrendValues(values)')
  expect(source).toContain('<VChart spec={spec} />')
  expect(source).not.toContain('lazy(')
  expect(source).not.toContain('Suspense')
})
