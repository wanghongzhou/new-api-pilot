import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const detailPagePath = new URL('./site-detail-page.tsx', import.meta.url)
const collectionPagePath = new URL(
  './site-collection-runs-page.tsx',
  import.meta.url
)

describe('SiteDetailPage information architecture', () => {
  test('moves detailed destinations into grouped in-page navigation', async () => {
    const source = await readFile(detailPagePath, 'utf8')
    const relatedPages = source.slice(
      source.indexOf('function SiteRelatedPages'),
      source.indexOf('function RecentCollectionActivity')
    )

    for (const route of [
      'financial-operations',
      'performance-history',
      'channel-inventory',
      'user-inventory',
      'upstream-tasks',
      'model-catalog',
      'rankings',
      'pricing-groups',
      'subscription-plans',
      'system-tasks',
      'logs',
      'stats',
      'status',
      'collection-runs',
    ]) {
      expect(relatedPages).toContain(route)
    }
    expect(relatedPages).toContain("t('site.related.operations')")
    expect(relatedPages).toContain("t('site.related.resources')")
    expect(relatedPages).toContain("t('site.related.records')")
    expect(relatedPages).toContain("t('site.related.infrastructure')")
  })

  test('keeps only the administrator mutation menu in the page header', async () => {
    const source = await readFile(detailPagePath, 'utf8')
    const headerActions = source.slice(
      source.indexOf('const actions ='),
      source.indexOf('\n\n  let detailContent')
    )

    expect(headerActions).toContain('<SiteActions')
    expect(headerActions).not.toContain("to='/sites/$siteId/")
  })

  test('does not duplicate the full statistics dashboard or instance list', async () => {
    const source = await readFile(detailPagePath, 'utf8')

    expect(source).not.toContain('SiteDataDashboard')
    expect(source).not.toContain('InstancePreview')
    expect(source).not.toContain('listSiteInstances')
    expect(source).not.toContain('getSiteStatistics')
  })

  test('labels today metrics explicitly and normalizes rate decimals', async () => {
    const source = await readFile(detailPagePath, 'utf8')

    expect(source).toContain("t('site.dashboard.todayCount')")
    expect(source).toContain("t('site.dashboard.todayQuota')")
    expect(source).toContain("t('site.dashboard.todayTokens')")
    expect(source).not.toContain("t('site.dashboard.totalCount')")
    expect(source).toContain(
      'formatDecimalDisplayValue(site.rate.quota_per_unit)'
    )
    expect(source).toContain(
      'formatDecimalDisplayValue(site.rate.usd_exchange_rate)'
    )
  })

  test('loads only three recent durable collection records in the detail', async () => {
    const source = await readFile(detailPagePath, 'utf8')
    const recentCollection = source.slice(
      source.indexOf('function RecentCollectionActivity'),
      source.indexOf('export function SiteDetailPage')
    )

    expect(recentCollection).toContain('page_size: 3')
    expect(recentCollection).toContain('listSiteCollectionRuns')
    expect(recentCollection).toContain("to='/sites/$siteId/collection-runs'")
    expect(recentCollection).not.toContain('FastTaskHistoryPanel')
  })

  test('keeps the full collection history on its dedicated page', async () => {
    const source = await readFile(collectionPagePath, 'utf8')

    expect(source).toContain('<CollectionRunsPanel')
    expect(source).toContain("to='/sites/$siteId'")
  })
})
