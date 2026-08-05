import { describe, expect, test } from 'bun:test'

const read = (path: string) => Bun.file(new URL(path, import.meta.url)).text()

describe('operations analytics query state boundary', () => {
  test('retained operational data is always marked stale after refresh errors', async () => {
    const [financial, rankings, performance] = await Promise.all([
      read('../financial-operations/components/financial-operations-page.tsx'),
      read('../rankings/components/rankings-page.tsx'),
      read('../performance-history/components/performance-history-page.tsx'),
    ])

    expect(financial).toContain('activeListQuery.isError && currentPage')
    expect(financial).toContain('statisticsQuery.isError && statistics')
    expect(rankings).toContain('rankingQuery.isError && data')
    expect(performance).toContain('listQuery.isError && list')
    expect(performance).toContain('statisticsQuery.isError && statistics')

    for (const source of [financial, rankings, performance]) {
      expect(source).toContain('<QueryStateAlert')
      expect(source).toContain('onRetry={() => void')
    }
  })

  test('failed option queries are not presented as a genuine empty option list', async () => {
    const [financial, rankings, performance, statistics] = await Promise.all([
      read('../financial-operations/components/financial-operations-page.tsx'),
      read('../rankings/components/rankings-page.tsx'),
      read('../performance-history/components/performance-history-page.tsx'),
      read('../statistics/components/statistics-filters.tsx'),
    ])

    for (const source of [financial, rankings, performance]) {
      expect(source).toContain('sitesQuery.isError')
      expect(source).toContain('operationsAnalytics.siteOptionsError')
      expect(source).toContain('sitesQuery.refetch()')
    }
    expect(statistics).toContain('failedOptionQueries')
    expect(statistics).toContain('operationsAnalytics.filterOptionsError')
    expect(statistics).toContain('retryFailedOptions')
  })
})
