import { describe, expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

const dashboardPage = new URL('./dashboard-page.tsx', import.meta.url)

describe('dashboard information architecture', () => {
  test('keeps the five fixed business sections in workflow order', async () => {
    const source = await readFile(dashboardPage, 'utf8')
    const today = source.indexOf("id='today'")
    const trend = source.indexOf("id='trend'")
    const realtime = source.indexOf("id='realtime'")
    const ranking = source.indexOf("id='ranking'")
    const health = source.indexOf("id='health'")

    expect(source).not.toContain("id='attention'")
    expect(today).toBeGreaterThan(0)
    expect(today).toBeLessThan(trend)
    expect(trend).toBeLessThan(realtime)
    expect(realtime).toBeLessThan(ranking)
    expect(ranking).toBeLessThan(health)
    expect(source).toContain('<OperationalAttention data={healthQuery.data} />')
  })

  test('uses the fixed 12-column cockpit with direct drill-downs', async () => {
    const source = await readFile(dashboardPage, 'utf8')

    expect(source).toContain('fixedContent')
    expect(source).toContain('lg:grid-cols-12')
    expect(source).toContain("className='lg:col-span-8'")
    expect(source).toContain("className='lg:col-span-4'")
    expect(source).toContain("className='lg:col-span-7'")
    expect(source).toContain("className='lg:col-span-5'")
    expect(source).toContain("to='/alerts'")
    expect(source).toContain("to='/statistics/global'")
    expect(source).toContain("to='/sites'")
  })

  test('shows exception sites instead of repeating every healthy site', async () => {
    const source = await readFile(dashboardPage, 'utf8')

    expect(source).toContain('data.sites.filter(isDashboardProblemSite)')
    expect(source).toContain('problemSites.slice(0, 6)')
    expect(source).toContain("t('dashboard.health.allSitesHealthy')")
    expect(source).not.toContain('data.sites.map((site)')
  })

  test('loads only the visible ranking dimension and gives every row a drill-down', async () => {
    const source = await readFile(dashboardPage, 'utf8')

    expect(source).toContain("enabled: topType === 'site'")
    expect(source).toContain("enabled: topType === 'customer'")
    expect(source).toContain("enabled: topType === 'model'")
    expect(source).toContain("enabled: topType === 'channel'")
    expect(source).toContain(
      '<RankingDetailLink item={item} metric={metric} />'
    )
    expect(source).toContain("to='/statistics/sites'")
    expect(source).toContain("to='/statistics/customers'")
    expect(source).toContain("to='/statistics/models'")
    expect(source).toContain("to='/statistics/channels'")
  })

  test('preserves context in every operational drill-down', async () => {
    const source = await readFile(dashboardPage, 'utf8')

    expect(source).toContain("search: { ...alertSearch, status: ['firing'] }")
    expect(source).toContain("level: ['critical'], status: ['firing']")
    expect(source).toContain("management: ['active'], online: ['offline']")
    expect(source).toContain("auth: ['expired'], management: ['active']")
    expect(source).toContain("<Link search={search} to='/statistics/global' />")
    expect(source).toContain('id={`dashboard-ranking-tab-${value}`}')
    expect(source).toContain("aria-controls='dashboard-ranking-panel'")
    expect(source).toContain('<StaleSiteLinks')
  })
})
