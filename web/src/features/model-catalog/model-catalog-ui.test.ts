import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

import zhCN from '@/i18n/locales/zh-CN.json'

const pagePath = new URL('./components/model-catalog-page.tsx', import.meta.url)

describe('model catalog information architecture', () => {
  test('uses model audit as the single business-facing page name', () => {
    expect(zhCN['Model catalog']).toBe('模型审计')
    expect(zhCN['site.actions.modelCatalog']).toBe('模型审计')
    expect(zhCN['modelCatalog.title']).toBe('模型审计')
    expect(zhCN['modelCatalog.siteTitle']).toBe('站点模型审计')
    expect(zhCN['modelCatalog.tabs.label']).toBe('模型审计视图')
  })

  test('uses Chinese business-facing tab names', () => {
    expect(zhCN['modelCatalog.tabs.catalog']).toBe('上游登记')
    expect(zhCN['modelCatalog.tabs.coverage']).toBe('覆盖分析')
    expect(zhCN['modelCatalog.tabs.missing']).toBe('渠道未登记')
  })

  test('describes grouped coverage as coverage status instead of splitting', () => {
    expect(zhCN['modelCatalog.breakdown.site']).toBe('站点覆盖情况')
    expect(zhCN['modelCatalog.breakdown.vendor']).toBe('供应商覆盖情况')
    expect(zhCN['modelCatalog.breakdown.status']).toBe('模型状态覆盖情况')
  })

  test('uses a compact new-api-style toolbar instead of manual site ids', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('<FacetedFilter')
    expect(source).toContain('modelCatalog.filters.keywordPlaceholder')
    expect(source).not.toContain(".split(',')")
    expect(source).not.toContain('<FilterPanel')
  })

  test('shows catalog-only filters conditionally and clears them for missing', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain("search.tab !== 'missing'")
    expect(source).toContain('{supportsCatalogFilters && (')
    expect(source).toContain(
      "changeModelCatalogTab(tab as ModelCatalogSearch['tab'])"
    )
    expect(source).toContain("search.tab !== 'coverage'")
    expect(source).toContain('buildModelCatalogSearch({})')
    expect(source).toContain('canonicalizedSearch.current')
  })

  test('keeps coverage context and the active view purpose visible', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('<CoverageGrid')
    expect(source).toContain('missingValue={coverage?.exact_missing_models}')
    expect(source).toContain('<TabPurpose')
    expect(source).toContain('modelCatalog.purpose.catalogDescription')
    expect(source).toContain('modelCatalog.purpose.coverageDescription')
    expect(source).toContain('modelCatalog.purpose.missingDescription')
  })

  test('keeps desktop pagination fixed while small screens use natural scrolling', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('fixedContent')
    expect(source).toContain('mobileScrollableContent')
    expect(source).toContain(
      "className='flex min-w-0 flex-col gap-4 lg:h-full lg:min-h-0'"
    )
    expect(source).not.toContain('fillAvailableHeight={false}')
    expect(source).not.toContain('paginationInFooter={false}')
    expect(source).not.toContain('preserveHeaderWhenEmpty={false}')
    expect(source).toContain(
      "className='min-h-0 overflow-visible lg:flex-1 lg:overflow-y-auto'"
    )
  })

  test('uses a single border level for coverage analysis cards', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain(
      "<section className='grid min-w-0 content-start gap-3'>"
    )
    expect(source).toContain(
      "className='border-border grid gap-2 rounded-lg border p-3'"
    )
    expect(source).not.toContain(
      "className='border-border bg-muted/20 grid gap-2 rounded-lg border p-3'"
    )
  })

  test('uses a valid definition-list structure for summary metrics', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain(
      "<div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>"
    )
    expect(source).toContain("<dl className='min-w-0'>")
    expect(source).not.toContain(
      "<dl className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>"
    )
  })

  test('keeps long catalog metadata expandable and mobile audit fields complete', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('function ExpandableText')
    expect(source).toContain('value={row.original.description}')
    expect(source).toContain('value={row.original.tags}')
    expect(source).toContain('value={row.original.icon}')
    expect(source).toContain("t('modelCatalog.sync.official')")
    expect(source).toContain("t('common.createdAt')")
    expect(source).toContain("t('common.updatedAt')")
    expect(source).toContain("t('statistics.dataStatus')")
    expect(source).toContain(
      "t('modelCatalog.asOf', { time: timestamp(item.as_of) })"
    )
  })

  test('shows an alert when the initial coverage request fails', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('{coverageQuery.isError && (')
    expect(source).toContain("'common.dataLoadFailed'")
    expect(source).toContain("tone={coverage ? 'warning' : 'destructive'}")
  })
})
