import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const entityStatistics = new URL(
  './components/entity-statistics.tsx',
  import.meta.url
)

describe('statistics chart runtime', () => {
  test('loads VChart eagerly so redirects cannot leave the chart suspended', async () => {
    const source = await readFile(entityStatistics, 'utf8')

    expect(source).toContain(
      "import { VChart, type ILineChartSpec } from '@visactor/react-vchart'"
    )
    expect(source).toContain('<VChart spec={spec} />')
    expect(source).not.toContain('lazy(() =>')
    expect(source).not.toContain('<Suspense')
    expect(source).not.toContain('<LazyVChart')
  })

  test('checks renderable finite values before mounting VChart', async () => {
    const source = await readFile(entityStatistics, 'utf8')
    const emptyState = source.indexOf(
      'if (!hasRenderableTrendValues(model.values))'
    )
    const chart = source.indexOf('<VChart spec={spec} />')

    expect(emptyState).toBeGreaterThan(0)
    expect(chart).toBeGreaterThan(emptyState)
  })
})
