import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL(
  './components/pricing-groups-page.tsx',
  import.meta.url
)

describe('pricing and groups information architecture', () => {
  test('organizes the page around complete group configuration and model pricing', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('<FacetedFilter')
    expect(source).toContain('pricingGroups.filters.allSites')
    expect(source).not.toContain('<FilterPanel')
    expect(source).not.toContain(".split(',')")
    expect(source).toContain('pricingGroups.purpose.groups.description')
    expect(source).toContain("value: 'groups'")
    expect(source).toContain("value: 'pricing'")
    expect(source).toContain('row.original.model_names')
    expect(source).toContain('row.original.missing_model_names')
    expect(source).toContain('row.original.outgoing_overrides')
    expect(source).toContain('row.original.visible_to_groups')
    expect(source).toContain('row.original.billing_expr')
    expect(source).toContain('row.original.ability_available')
    expect(source).toContain('<PricingAuditMetadata item={row.original} />')
    expect(source).toContain('item.vendor_id')
    expect(source).toContain('item.quota_type')
    expect(source).toContain('item.owner_by')
    expect(source).toContain('item.pricing_version')
    expect(source).toContain('item.tags')
    expect(source).toContain('item.icon')
    expect(source).toContain('<CompletenessSummary')
    expect(source).toContain(
      "(search.tab === 'pricing' ? pricing : groups)?.as_of"
    )
    expect(source).toContain(
      "(search.tab === 'pricing' ? pricing : groups)?.site_breakdown"
    )
    expect(source).toContain('statistics?.sites')
    expect(source).toContain("t('pricingGroups.pricing.groups')")
    expect(source).toContain("t('pricingGroups.pricing.endpoints')")
    expect(source).toContain("t('common.expand')")
    expect(source).toContain("t('common.collapse')")
    expect(source).toContain('break-all whitespace-normal')
    expect(source).toContain('hiddenPricingValueCount')
    expect(source).toContain('pricingGroups.inspectPricing')
    expect(source).not.toContain("value: 'overview'")
    expect(source).not.toContain("value: 'vendors'")
    expect(source).not.toContain('供应商定价')
    expect(source).toContain('fixedContent')
    expect(source).not.toContain('fillAvailableHeight={false}')
    expect(source).not.toContain('paginationInFooter={false}')
    expect(source).not.toContain('preserveHeaderWhenEmpty={false}')
    expect(source).not.toContain('isPricingAnalysisTab(search.tab)')
    expect(source).toContain('statisticsQuery.isError')
    expect(source).toContain('canonicalizedSearch.current')
    expect(source).toContain(
      "<div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>"
    )
    expect(source).toContain('<dl>')
    expect(source).not.toContain("<dl className='grid gap-3 sm:grid-cols-3'>")
  })
})
