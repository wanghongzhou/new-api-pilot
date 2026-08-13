import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const pagePath = new URL(
  './components/subscription-plans-page.tsx',
  import.meta.url
)

describe('subscription plans information architecture', () => {
  test('separates the fixed-height plan list from site analysis', async () => {
    const source = await readFile(pagePath, 'utf8')
    expect(source).toContain('<FacetedFilter')
    expect(source).toContain('subscriptionPlans.filters.allSites')
    expect(source).not.toContain('<FilterPanel')
    expect(source).not.toContain(".split(',')")
    expect(source).toContain('subscriptionPlans.purpose.description')
    expect(source).toContain("<TabsTrigger value='plans'>")
    expect(source).toContain("<TabsTrigger value='site-analysis'>")
    expect(source).toContain('fixedContent')
    expect(source).not.toContain('fillAvailableHeight={false}')
    expect(source).not.toContain('paginationInFooter={false}')
    expect(source).not.toContain('preserveHeaderWhenEmpty={false}')
    expect(source).toContain(
      "className='min-h-0 overflow-visible lg:flex-1 lg:overflow-y-auto'"
    )
    expect(source).toContain('changeSubscriptionPlanTab')
    expect(source).toContain('canonicalizedSearch.current')
    expect(source).toContain(
      "<div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>"
    )
    expect(source).toContain('<dl>')
    expect(source).not.toContain(
      "<dl className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>"
    )
  })

  test('keeps missing and lifecycle evidence visible on every layout', async () => {
    const source = await readFile(pagePath, 'utf8')

    expect(source).toContain('row.original.missing_count')
    expect(source).toContain('item.missing_count')
    expect(source).toContain('item.created_at')
    expect(source).toContain('item.updated_at')
    expect(source).toContain('item.sort_order')
    expect(source).toContain('subscriptionPlans.createdAt')
    expect(source).toContain('subscriptionPlans.updatedAt')
    expect(source).toContain('break-all')
    expect(source).toContain('break-words')
    expect(source).toContain("className='max-w-72 min-w-44 whitespace-normal'")
    expect(source).toContain('formatDecimalDisplayValue')
    expect(source).toContain('formatMetricDisplayValue')
    expect(source).not.toContain('value: item.total_amount,')
    expect(source).not.toContain('value: row.original.total_amount,')
    expect(source).not.toContain('value: item.quota_reset_custom_seconds,')
  })
})
