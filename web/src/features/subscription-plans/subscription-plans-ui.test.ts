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
    expect(source).toContain("className='min-h-0 flex-1 overflow-y-auto'")
    expect(source).toContain('changeSubscriptionPlanTab')
    expect(source).toContain('canonicalizedSearch.current')
  })
})
