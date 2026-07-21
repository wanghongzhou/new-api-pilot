import { readFileSync } from 'node:fs'

import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const viewer = {
  display_name: '定价目录只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'pricing_catalog_viewer',
}

const f10 = JSON.parse(
  readFileSync(
    new URL('../../testdata/design/f10-pricing-groups.json', import.meta.url),
    'utf8'
  )
) as {
  fixture_id: 'F10'
  groups: Array<{ group_name: string }>
  pricing: Array<{
    group_ratios: Record<string, string>
    input_price: string
    model_name: string
    vendor: string
  }>
}
const f10Pricing = f10.pricing[0]
const f10ZeroUsageGroup = f10.groups[1]
if (!f10Pricing || !f10ZeroUsageGroup) {
  throw new Error('F10 pricing/group fixture is incomplete')
}

function envelope<T>(data: T, requestId = 'req_pricing_groups_e2e') {
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
  await page.addInitScript((user) => {
    window.localStorage.setItem('pilot-auth-user', JSON.stringify(user))
    window.localStorage.setItem('uid', user.id)
  }, viewer)
  await page.route(/\/api\/user\/self(?:\?.*)?$/, async (route) => {
    await route.fulfill({ json: envelope(viewer, 'req_pricing_self') })
  })
}

const pricingItem = {
  audio_completion_ratio: null,
  audio_ratio: null,
  cache_ratio: '0.5000000000',
  collected_at: 1_784_348_700,
  completion_ratio: '2.0000000000',
  create_cache_ratio: null,
  data_status: 'partial',
  description: '安全定价说明',
  enable_groups: ['default', 'vip-zero-usage'],
  icon: 'https://icons.invalid/not-loaded.svg',
  id: '9007199254740801',
  image_ratio: null,
  missing_count: 0,
  model_name: f10Pricing.model_name,
  model_price: f10Pricing.input_price,
  model_ratio: '1.2500000000',
  owner_by: 'openai',
  pricing_version: 'pinned',
  quota_type: '1',
  remote_state: 'normal',
  root_visible: true,
  site_id: '9007199254740997',
  site_name: '华东定价站点',
  supported_endpoint_types: ['chat_completions', 'responses'],
  tags: 'chat',
  vendor_id: '9007199254740995',
  vendor_key: f10Pricing.vendor,
}

const groupItem = {
  collected_at: 1_784_348_700,
  data_status: 'complete',
  description: '尚无用量但已配置',
  id: '9007199254740811',
  missing_count: 0,
  name: f10ZeroUsageGroup.group_name,
  ratio: f10Pricing.group_ratios[f10ZeroUsageGroup.group_name],
  remote_state: 'normal',
  root_visible: true,
  site_id: '9007199254740997',
  site_name: '华东定价站点',
}

const statistics = {
  data_status: 'partial',
  group_total: '9007199254740993',
  group_breakdown: [
    { group_name: 'vip-zero-usage', model_count: '9007199254740994' },
  ],
  group_catalog_breakdown: [
    { count: '9007199254740993', ratio_available: true, root_visible: true },
  ],
  missing: '1',
  site_breakdown: [
    {
      as_of: 1_784_348_700,
      data_status: 'partial',
      missing: '1',
      site_id: '9007199254740997',
      site_name: '华东定价站点',
      total: '9007199254740995',
    },
  ],
  total: '9007199254740995',
  vendor_breakdown: [
    {
      missing: '1',
      total: '9007199254740995',
      vendor_id: '9007199254740995',
      vendor_key: 'openai',
    },
  ],
}

test('A99 keeps pricing and configured groups exact, passive, private and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  expect(f10.fixture_id).toBe('F10')
  await seedAuth(page, testInfo)
  const reads: URL[] = []
  const exportBodies: Record<string, unknown>[] = []
  const list = (item: unknown, status = 'partial') => ({
    as_of: 1_784_348_700,
    data_status: status,
    items: [item],
    page: 1,
    page_size: 20,
    site_breakdown: statistics.site_breakdown,
    total: 1,
  })
  const fulfill = async (route: Route, data: unknown) => {
    assertAuthenticated(route)
    reads.push(new URL(route.request().url()))
    await route.fulfill({ json: envelope(data) })
  }
  await page.route(/\/api\/pricing-catalog\/statistics(?:\?.*)?$/, (route) =>
    fulfill(route, statistics)
  )
  await page.route(/\/api\/pricing-catalog(?:\?.*)?$/, (route) =>
    fulfill(route, list(pricingItem))
  )
  await page.route(/\/api\/group-catalog(?:\?.*)?$/, (route) =>
    fulfill(route, list(groupItem, 'complete'))
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/pricing-catalog\/statistics(?:\?.*)?$/,
    (route) => fulfill(route, statistics)
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/pricing-catalog(?:\?.*)?$/,
    (route) => fulfill(route, list(pricingItem))
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/group-catalog(?:\?.*)?$/,
    (route) => fulfill(route, list(groupItem, 'complete'))
  )
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticated(route)
    const body = route.request().postDataJSON() as Record<string, unknown>
    exportBodies.push(body)
    await route.fulfill({
      json: envelope({
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
        id: '799',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: body.statistics_type,
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/799', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ status: 404, json: envelope(null) })
  })

  await page.goto(
    '/pricing-groups?siteIds=9007199254740997&keyword=gpt&group=vip&states=normal'
  )
  await expect(
    page.getByRole('heading', { exact: true, name: '定价与分组目录' })
  ).toBeVisible()
  await expect(
    page.getByText('1.2500000000').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('0.0000025000').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('chat_completions').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('responses').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('root 可见').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: 'Vendor 定价拆分' })
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: '可用分组模型拆分' })
  ).toBeVisible()
  await expect(
    page.getByText('Ratio 可用').filter({ visible: true }).first()
  ).toBeVisible()
  expect(
    reads
      .slice(0, 2)
      .map((url) => url.pathname)
      .sort()
  ).toEqual(['/api/pricing-catalog', '/api/pricing-catalog/statistics'])
  for (const url of reads.slice(0, 2)) {
    expect(url.searchParams.getAll('site_ids')).toEqual(['9007199254740997'])
    expect(url.searchParams.get('group')).toBe('vip')
  }
  await page.getByRole('tab', { name: '已配置分组' }).click()
  await expect(page).toHaveURL(/tab=groups/)
  await expect(
    page.getByText('vip-zero-usage').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('0.8500000000').filter({ visible: true }).first()
  ).toBeVisible()
  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect.poll(() => exportBodies.length).toBe(1)
  expect(exportBodies[0]).toMatchObject({ statistics_type: 'group_catalog' })
  const serialized = JSON.stringify(exportBodies[0]).toLowerCase()
  for (const forbidden of [
    'billing_expr',
    'custom_path',
    '"channel_key":',
    'oauth_token',
    'header_override',
    'param_override',
  ]) {
    expect(serialized).not.toContain(forbidden)
  }

  reads.length = 0
  await page.goto(
    '/sites/9007199254740997/pricing-groups?tab=pricing&siteIds=9'
  )
  await expect(
    page.getByRole('heading', { exact: true, name: '站点定价与分组目录' })
  ).toBeVisible()
  await expect.poll(() => reads.length).toBeGreaterThanOrEqual(2)
  for (const url of reads) expect(url.searchParams.has('site_ids')).toBe(false)

  const iconRequests: string[] = []
  page.on('request', (request) => {
    if (request.url().includes('icons.invalid')) {
      iconRequests.push(request.url())
    }
  })
  await page.waitForTimeout(50)
  expect(iconRequests).toEqual([])
  const overflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth >
      document.documentElement.clientWidth
  )
  expect(overflow).toBe(false)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
})
