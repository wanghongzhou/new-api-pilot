import { expect, test } from 'bun:test'
import { readFile } from 'node:fs/promises'

test('uses distinct semantic icons for site card destinations', async () => {
  const source = await readFile(
    new URL('./site-card.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain('icon={Chart01Icon}')
  expect(source).toContain('icon={ServerStack01Icon}')
  expect(source).toContain('icon={ViewIcon}')
  expect(source).not.toContain('ArrowRight01Icon')
  expect(source).toContain("toast.success(t('site.toast.baseUrlCopied'))")
  expect(source).toContain("toast.error(t('site.toast.copyFailed'))")
  expect(source).toContain("t('site.performance.unavailable')")
  expect(source).toContain("freshnessDotClass = 'bg-muted-foreground'")
  expect(source).toContain("freshnessDotClass = 'bg-destructive'")
})

test('uses the ready performance contract in card and table views', async () => {
  const [cardSource, pageSource] = await Promise.all([
    readFile(new URL('./site-card.tsx', import.meta.url), 'utf8'),
    readFile(new URL('./sites-page.tsx', import.meta.url), 'utf8'),
  ])

  expect(cardSource).toContain('isSitePerformanceReady(')
  expect(pageSource).toContain('isSitePerformanceReady(')
  expect(cardSource).not.toContain("data_status === 'complete'")
  expect(pageSource).not.toContain("data_status !== 'complete'")
})

test('matches the new-api dashboard performance summary contract', async () => {
  const [cardSource, pageSource] = await Promise.all([
    readFile(new URL('./site-card.tsx', import.meta.url), 'utf8'),
    readFile(new URL('./sites-page.tsx', import.meta.url), 'utf8'),
  ])

  expect(cardSource).toContain('sitePerformanceDashboardSummary(')
  expect(cardSource).toContain("t('site.performance.successRate')")
  expect(cardSource).toContain("t('site.performance.avgLatency')")
  expect(cardSource).toContain("t('site.performance.avgTps')")
  expect(cardSource).not.toContain('performance.success_rate * 100')
  expect(pageSource).toContain('sitePerformanceDashboardSummary(')
  expect(pageSource).toContain("t('site.performance.unavailable')")
})

test('places total count with average RPM and TPM on the lower row', async () => {
  const source = await readFile(
    new URL('./site-card.tsx', import.meta.url),
    'utf8'
  )

  const lowerRow = source.slice(
    source.indexOf("<div className='grid grid-cols-3 gap-x-5 gap-y-4'>"),
    source.indexOf(
      "<section className='grid gap-3'>",
      source.indexOf("<div className='grid grid-cols-3 gap-x-5 gap-y-4'>")
    )
  )
  expect(lowerRow).toContain("t('site.dashboard.todayCount')")
  expect(lowerRow).toContain("t('site.averageRpm')")
  expect(lowerRow).toContain("t('site.averageTpm')")
  expect(source).toContain('formatAverageRate(site.today.avg_rpm)')
  expect(source).toContain('formatAverageRate(site.today.avg_tpm)')
  expect(source).toContain("t('site.resourceUpdatedAt'")
  expect(source).toContain('timestamp={site.resource.updated_at}')
})

test('keeps the approved card content in the grouped desktop list', async () => {
  const source = await readFile(
    new URL('./sites-page.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain("label={t('site.dashboard.todayQuota')}")
  expect(source).toContain("label={t('site.dashboard.todayTokens')}")
  expect(source).toContain("label={t('site.dashboard.todayCount')}")
  expect(source).toContain('formatAverageRate(today.avg_rpm)')
  expect(source).toContain('formatAverageRate(today.avg_tpm)')
  expect(source).toContain("label={t('site.performance.successRate')}")
  expect(source).toContain("label={t('site.performance.avgLatency')}")
  expect(source).toContain("label={t('site.performance.avgTps')}")
  expect(source).toContain("t('site.resourceUpdatedAt'")
  expect(source).not.toContain("labelKey='site.resourceUpdatedAt'")
  expect(source).toContain("t('site.performance.sampledAt'")
  expect(source).toContain("to='/sites/$siteId/stats'")
  expect(source).toContain("to='/sites/$siteId/status'")
  expect(source).toContain("to='/sites/$siteId'")
})

test('defaults desktop site management to the grouped table view', async () => {
  const source = await readFile(
    new URL('../../../routes/_authenticated/sites/index.tsx', import.meta.url),
    'utf8'
  )

  expect(source).toContain("'(min-width: 1024px)'")
  expect(source).toContain("? 'table'")
  expect(source).toContain("window.localStorage.getItem('sites:view-mode-v2')")
})
