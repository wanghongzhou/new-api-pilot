import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL(
  './components/pricing-groups-page.tsx',
  import.meta.url
)

describe('pricing and groups information architecture', () => {
  test('separates lists and each analysis dimension into fixed-workspace tabs', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('<FacetedFilter')
    expect(source).toContain('pricingGroups.filters.allSites')
    expect(source).not.toContain('<FilterPanel')
    expect(source).not.toContain(".split(',')")
    expect(source).toContain('pricingGroups.purpose.pricing.description')
    expect(source).toContain('pricingGroups.purpose.groups.description')
    expect(source).toContain("value: 'site-analysis'")
    expect(source).toContain("value: 'vendor-analysis'")
    expect(source).toContain("value: 'group-model-analysis'")
    expect(source).toContain("value: 'group-availability-analysis'")
    expect(source).toContain('fixedContent')
    expect(source).not.toContain('fillAvailableHeight={false}')
    expect(source).not.toContain('paginationInFooter={false}')
    expect(source).not.toContain('preserveHeaderWhenEmpty={false}')
    expect(source).toContain("className='min-h-0 flex-1 overflow-y-auto'")
    expect(source).toContain('isPricingAnalysisTab(search.tab)')
    expect(source).toContain('statisticsQuery.isError')
    expect(source).toContain('canonicalizedSearch.current')
  })
})
