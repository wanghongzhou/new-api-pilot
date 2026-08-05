import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import zhCN from '@/i18n/locales/zh-CN.json'

const featureRoot = new URL('../', import.meta.url)

async function source(path: string) {
  return readFile(new URL(path, featureRoot), 'utf8')
}

describe('operations analytics presentation boundaries', () => {
  test('keeps repeated site details collapsed behind a keyboard-native disclosure', async () => {
    const [statisticsPage, entityStatistics] = await Promise.all([
      source('statistics/components/statistics-page.tsx'),
      source('statistics/components/entity-statistics.tsx'),
    ])

    expect(statisticsPage).toContain(
      '<SiteBreakdownDisclosure sites={item.site_breakdown} />'
    )
    expect(entityStatistics).toContain('<details')
    expect(entityStatistics).toContain('<summary')
    expect(entityStatistics).toContain('statistics.siteBreakdown.expand')
  })

  test('labels the two independent financial data statuses', async () => {
    const financial = await source(
      'financial-operations/components/financial-operations-page.tsx'
    )

    expect(financial).toContain("t('financialOperations.statisticsStatus')")
    expect(financial).toContain("t('financialOperations.listStatus')")
    expect(zhCN['financialOperations.statisticsStatus']).toBe('统计数据状态')
    expect(zhCN['financialOperations.listStatus']).toBe('列表数据状态')
  })

  test('exposes backend completeness and weighted model/group breakdowns', async () => {
    const [financial, performance, performanceTypes] = await Promise.all([
      source('financial-operations/components/financial-operations-page.tsx'),
      source('performance-history/components/performance-history-page.tsx'),
      source('performance-history/types.ts'),
    ])

    expect(financial).toContain("t('financialOperations.completeness'")
    expect(performance).toContain("value='models'")
    expect(performance).toContain("value='groups'")
    expect(performance).toContain('statistics.model_breakdown')
    expect(performance).toContain('statistics.group_breakdown')
    expect(performance).toContain("t('performanceHistory.completeness'")
    expect(performanceTypes).toContain('model_breakdown:')
    expect(performanceTypes).toContain('group_breakdown:')
    expect(performanceTypes).toContain('PerformanceDimensionBreakdown[]')
    expect(performanceTypes).toContain('completeness:')
  })
})
