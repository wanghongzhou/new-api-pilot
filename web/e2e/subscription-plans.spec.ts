import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const viewer = {
  display_name: '计划目录只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'plan_catalog_viewer',
}

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_subscription_plan_e2e') {
  return { code: '', data, message: '', request_id: requestId, success: true }
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function seedAuth(page: Page, testInfo: TestInfo) {
  if (testInfo.project.name === 'chromium-mobile') {
    await page.setViewportSize({ height: 812, width: 375 })
  }
  await page.addInitScript((authUser) => {
    window.localStorage.setItem('pilot-auth-user', JSON.stringify(authUser))
    window.localStorage.setItem('uid', authUser.id)
  }, viewer)
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ json: envelope(viewer, 'req_subscription_self') })
  })
}

const units = ['year', 'month', 'day', 'hour', 'custom'] as const
const resets = ['never', 'daily', 'weekly', 'monthly', 'custom'] as const
const plans = units.map((unit, index) => ({
  collected_at: 1_784_348_700,
  created_at: 1_700_000_000 + index,
  currency: index === 1 ? 'CNY' : 'USD',
  custom_seconds: unit === 'custom' ? '9007199254740993' : '0',
  data_status: index === 1 ? 'unavailable' : 'partial',
  duration_unit: unit,
  duration_value: index + 1,
  enabled: index % 2 === 0,
  first_seen_at: 1_784_000_000,
  id: String(9007199254740800 + index),
  last_seen_at: index === 1 ? null : 1_784_348_700,
  missing_count: index === 1 ? 1 : 0,
  price_amount: index === 0 ? '19.990000' : `${index}.000001`,
  quota_reset_custom_seconds:
    resets[index] === 'custom' ? '9007199254740995' : '0',
  quota_reset_period: resets[index],
  remote_id: String(9007199254740900 + index),
  remote_state: index === 1 ? 'missing' : 'normal',
  site_id: '9007199254740997',
  site_name: '华东计划站点',
  sort_order: 10 - index,
  subtitle: `安全副标题 ${index}`,
  title: `计划 ${unit}`,
  total_amount: index === 1 ? '0' : '900719925474099312345',
  updated_at: 1_700_000_100 + index,
}))

function listResponse() {
  return {
    data_status: 'partial',
    items: plans,
    page: 1,
    page_size: 20,
    total: plans.length,
  }
}

function statistics() {
  return {
    data_status: 'partial',
    disabled: '2',
    enabled: '3',
    missing: '1',
    site_breakdown: [
      {
        as_of: 1_784_348_700,
        data_status: 'unavailable',
        disabled: '2',
        enabled: '3',
        missing: '1',
        site_id: '9007199254740997',
        site_name: '华东计划站点',
        total: '5',
      },
    ],
    total: '5',
  }
}

function exportJob(body: ExportBody) {
  return {
    created_at: 1_784_348_800,
    data_snapshot_at: null,
    deduplicated: false,
    error: null,
    expires_at: null,
    file_name: '',
    file_size: '0',
    filters: body.filters,
    finished_at: null,
    format: body.format,
    id: '798',
    progress: 0,
    row_count: '0',
    started_at: null,
    statistics_type: body.statistics_type,
    status: 'pending',
  }
}

function forbiddenFields() {
  return [
    ['stripe', 'price', 'id'].join('_'),
    ['creem', 'product', 'id'].join('_'),
    ['waffo', 'pancake', 'product', 'id'].join('_'),
    ['allow', 'balance', 'pay'].join('_'),
    ['allow', 'wallet', 'overflow'].join('_'),
    ['max', 'purchase', 'per', 'user'].join('_'),
    ['upgrade', 'group'].join('_'),
    ['downgrade', 'group'].join('_'),
    ['payment', 'payload'].join('_'),
  ]
}

test('A98 keeps subscription plan catalog exact, private, bounded and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const reads: URL[] = []
  const planApiPaths: string[] = []
  const exports: ExportBody[] = []

  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/') && path.includes('subscription')) {
      planApiPaths.push(path)
    }
  })
  await page.route(/\/api\/subscription-plans(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    reads.push(new URL(route.request().url()))
    await route.fulfill({ json: envelope(listResponse()) })
  })
  await page.route(
    /\/api\/subscription-plans\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      reads.push(new URL(route.request().url()))
      await route.fulfill({ json: envelope(statistics()) })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/subscription-plans(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      expect(url.searchParams.has('site_ids')).toBe(false)
      reads.push(url)
      await route.fulfill({ json: envelope(listResponse()) })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/subscription-plans\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      expect(url.searchParams.has('site_ids')).toBe(false)
      reads.push(url)
      await route.fulfill({ json: envelope(statistics()) })
    }
  )
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticated(route)
    const body = route.request().postDataJSON() as ExportBody
    exports.push(body)
    await route.fulfill({ json: envelope(exportJob(body)) })
  })
  await page.route('**/api/statistics/exports/798', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope(
        exportJob(
          exports.at(-1) ?? {
            filters: {},
            format: 'csv',
            statistics_type: 'subscription_plans',
          }
        )
      ),
    })
  })

  await page.goto(
    '/subscription-plans?siteIds=9007199254740997&keyword=plan&enabled=false&states=missing'
  )
  await expect(
    page.getByRole('heading', { exact: true, name: '订阅计划目录' })
  ).toBeVisible()
  await expect.poll(() => reads.length).toBe(2)
  expect(reads.map((url) => url.pathname).sort()).toEqual([
    '/api/subscription-plans',
    '/api/subscription-plans/statistics',
  ])
  for (const read of reads) {
    expect(read.searchParams.get('enabled')).toBe('false')
    expect(read.searchParams.get('keyword')).toBe('plan')
    expect(read.searchParams.getAll('site_ids')).toEqual(['9007199254740997'])
    expect(read.searchParams.getAll('states')).toEqual(['missing'])
  }
  await expect(
    page.getByText('USD 19.990000').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page
      .getByText('额度：900719925474099312345')
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page.getByText('无限额度').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page
      .getByText('自定义有效期 9007199254740993 秒')
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText('每 9007199254740995 秒重置额度')
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  for (const label of [
    '额度不重置',
    '每日重置额度',
    '每周重置额度',
    '每月重置额度',
  ]) {
    await expect(
      page.getByText(label, { exact: true }).filter({ visible: true }).first()
    ).toBeVisible()
  }
  await expect(page.getByText('站点计划拆分')).toBeVisible()
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(page.getByText(/不是订阅收入、订单或用户订阅库存/)).toBeVisible()

  await page.getByRole('button', { name: '导出 XLSX' }).click()
  await expect
    .poll(() => exports.at(-1)?.statistics_type)
    .toBe('subscription_plans')
  expect(exports.at(-1)?.filters.subscription_plan_enabled).toBe(false)
  expect(exports.at(-1)?.filters.inventory_states).toEqual(['missing'])
  expect(exports.at(-1)?.filters.site_ids).toEqual(['9007199254740997'])
  await page.getByRole('button', { name: '关闭' }).click()

  const scan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(scan.violations).toEqual([])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
  const browserState = await page.evaluate(() => ({
    attributes: [...document.querySelectorAll('*')]
      .flatMap((element) =>
        [...element.attributes].map(
          (attribute) => `${attribute.name}=${attribute.value}`
        )
      )
      .join('\n'),
    localStorage: JSON.stringify(window.localStorage),
    url: window.location.href,
  }))
  const visibleState = JSON.stringify(browserState).toLowerCase()
  const serializedExport = JSON.stringify(exports).toLowerCase()
  for (const field of forbiddenFields()) {
    expect(visibleState).not.toContain(field)
    expect(serializedExport).not.toContain(field)
  }

  await page.goto(
    '/sites/9007199254740997/subscription-plans?siteIds=9007199254740995'
  )
  await expect(
    page.getByRole('heading', {
      exact: true,
      name: '站点订阅计划目录',
    })
  ).toBeVisible()
  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect.poll(() => exports.at(-1)?.format).toBe('csv')
  expect(exports.at(-1)?.filters.site_ids).toEqual(['9007199254740997'])
  expect(
    planApiPaths.every((path) =>
      /^\/api\/(?:subscription-plans(?:\/statistics)?|sites\/9007199254740997\/subscription-plans(?:\/statistics)?)$/.test(
        path
      )
    )
  ).toBe(true)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
