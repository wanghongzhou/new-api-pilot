import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import zhCN from '@/i18n/locales/zh-CN.json'

const featureRoot = new URL('../', import.meta.url)

async function source(path: string) {
  return readFile(new URL(path, featureRoot), 'utf8')
}

describe('operations analytics workspace', () => {
  test('links all four modules only inside forced-site scope', async () => {
    const workspace = await source(
      'operations-analytics/components/operations-analytics-workspace.tsx'
    )

    expect(workspace).not.toContain("to='/financial-operations'")
    expect(workspace).not.toContain("to='/statistics/global'")
    expect(workspace).not.toContain("to='/rankings'")
    expect(workspace).not.toContain("to='/performance-history'")
    expect(workspace).toContain("to='/sites/$siteId/financial-operations'")
    expect(workspace).toContain("to='/sites/$siteId/stats'")
    expect(workspace).toContain("to='/sites/$siteId/rankings'")
    expect(workspace).toContain("to='/sites/$siteId/performance-history'")
    expect(workspace).toContain("aria-current={active ? 'page' : undefined}")
    expect(workspace).toContain(
      "className='grid grid-cols-2 gap-1 sm:flex sm:min-w-max'"
    )
  })

  test('keeps global pages fixed without duplicating the sidebar navigation', async () => {
    const pages = await Promise.all([
      source('financial-operations/components/financial-operations-page.tsx'),
      source('statistics/components/statistics-page.tsx'),
      source('rankings/components/rankings-page.tsx'),
      source('performance-history/components/performance-history-page.tsx'),
    ])

    for (const page of pages) {
      expect(page).toContain('fixedContent')
      expect(page).toContain(
        "className='flex h-full min-h-0 min-w-0 flex-col gap-4'"
      )
    }
    expect(pages[0]).toContain('{siteId && (')
    expect(pages[1]).not.toContain('<OperationsAnalyticsNavigation')
    expect(pages[2]).toContain('{siteId && (')
    expect(pages[3]).toContain('{siteId && (')
    expect(pages[1]).toContain('nativeButton={false}')
  })

  test('separates rankings and performance results into URL views', async () => {
    const rankings = await source('rankings/components/rankings-page.tsx')
    const performance = await source(
      'performance-history/components/performance-history-page.tsx'
    )

    expect(rankings).toContain("<TabsTrigger value='ranking'>")
    expect(rankings).toContain("<TabsTrigger value='movement'>")
    expect(rankings).toContain("<TabsTrigger value='history'>")
    expect(rankings).toContain("<TabsTrigger value='sites'>")
    expect(rankings).toContain("search.view === 'movement'")
    expect(rankings).toContain("search.view === 'history'")
    expect(rankings).toContain("search.view === 'sites'")
    expect(rankings).toContain("search.view === 'ranking'")
    expect(rankings).toContain('<FacetedFilter')
    expect(rankings).not.toContain(".split(',')")

    expect(performance).toContain("<TabsTrigger value='list'>")
    expect(performance).toContain("<TabsTrigger value='trend'>")
    expect(performance).toContain("<TabsTrigger value='sites'>")
    expect(performance).toContain("search.view === 'list'")
    expect(performance).toContain("search.view === 'trend'")
    expect(performance).toContain("search.view === 'sites'")
    expect(performance).toContain('<FacetedFilter')
    expect(performance).not.toContain('siteIds: event.target.value')
  })

  test('provides complete Chinese navigation and view copy', () => {
    expect(zhCN['operationsAnalytics.navigation.label']).toBe('站点运营切换')
    expect(zhCN['operationsAnalytics.navigation.financial']).toBe('财务运营')
    expect(zhCN['operationsAnalytics.navigation.statistics']).toBe('站点统计')
    expect(zhCN['operationsAnalytics.navigation.rankings']).toBe('本地排行')
    expect(zhCN['operationsAnalytics.navigation.performance']).toBe('性能趋势')
    expect(zhCN['rankings.views.ranking']).toBe('主榜')
    expect(zhCN['performanceHistory.views.list']).toBe('性能明细')
  })
})
