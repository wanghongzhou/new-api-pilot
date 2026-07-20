import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const detailPagePath = new URL('./site-detail-page.tsx', import.meta.url)

async function detailPageSource() {
  return readFile(detailPagePath, 'utf8')
}

describe('SiteDetailPage embedded statistics dashboard', () => {
  test('links the loaded site detail to the forced financial operations route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/financial-operations'")
    expect(actions).toContain('buildFinancialOperationsSearch({})')
    expect(actions).toContain("t('site.actions.financialOperations')")
  })

  test('links the loaded site detail to the forced performance history route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/performance-history'")
    expect(actions).toContain('buildPerformanceHistorySearch({})')
    expect(actions).toContain("t('site.actions.performanceHistory')")
  })

  test('links the loaded site detail to the forced channel inventory route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/channel-inventory'")
    expect(actions).toContain('buildChannelInventorySearch({})')
    expect(actions).toContain("t('site.actions.channelInventory')")
  })

  test('links the loaded site detail to the forced user inventory route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/user-inventory'")
    expect(actions).toContain('buildUserInventorySearch({})')
    expect(actions).toContain("t('site.actions.userInventory')")
  })

  test('links the loaded site detail to the forced upstream task route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/upstream-tasks'")
    expect(actions).toContain('buildUpstreamTaskSearch({})')
    expect(actions).toContain("t('site.actions.upstreamTasks')")
  })

  test('links the loaded site detail to the forced model catalog route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/model-catalog'")
    expect(actions).toContain('buildModelCatalogSearch({})')
    expect(actions).toContain("t('site.actions.modelCatalog')")
  })

  test('links the loaded site detail to the forced local rankings route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/rankings'")
    expect(actions).toContain('buildRankingSearch({})')
    expect(actions).toContain("t('site.actions.rankings')")
  })

  test('links the loaded site detail to the forced pricing and group catalog', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/pricing-groups'")
    expect(actions).toContain('buildPricingGroupSearch({})')
    expect(actions).toContain("t('site.actions.pricingGroups')")
  })

  test('links the loaded site detail to the forced subscription plan catalog', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/subscription-plans'")
    expect(actions).toContain('buildSubscriptionPlanSearch({})')
    expect(actions).toContain("t('site.actions.subscriptionPlans')")
  })

  test('links the loaded site detail to the forced system task catalog', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )
    expect(actions).toContain("to='/sites/$siteId/system-tasks'")
    expect(actions).toContain('buildSystemTaskSearch({})')
    expect(actions).toContain("t('site.actions.systemTasks')")
  })

  test('links the loaded site detail to the forced site log route', async () => {
    const source = await detailPageSource()
    const actions = source.slice(
      source.indexOf('const actions = site ?'),
      source.indexOf('\n  let detailContent')
    )

    expect(actions).toContain("to='/sites/$siteId/logs'")
    expect(actions).toContain('params={{ siteId }}')
    expect(actions).toContain("t('site.actions.logs')")
  })

  test('mounts the site-scoped statistics dashboard only after a valid detail loads', async () => {
    const source = await detailPageSource()
    const successBranch = source.slice(
      source.indexOf('} else if (detailQuery.isPending || !site) {'),
      source.indexOf('\n  return (\n    <SectionPageLayout')
    )

    expect(successBranch).toContain('} else {')
    expect(successBranch).toContain('<SiteDataDashboard siteId={siteId} />')
  })

  test('binds the embedded dashboard to the selected site statistics endpoint and renderer', async () => {
    const source = await detailPageSource()
    const dashboard = source.slice(
      source.indexOf('function SiteDataDashboard'),
      source.indexOf('\nexport function SiteDetailPage')
    )

    expect(dashboard).toContain(
      'queryFn: () => getSiteStatistics(parseIdString(siteId), params)'
    )
    expect(dashboard).toContain('queryKey: siteKeys.statistics(siteId, params)')
    expect(dashboard).toContain('<EntityStatistics')
    expect(dashboard).toContain("scope='site'")
    expect(dashboard).toContain('entityId={parseIdString(siteId)}')
  })
})
