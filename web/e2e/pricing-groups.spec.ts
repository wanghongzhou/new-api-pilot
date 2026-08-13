import { readFileSync } from 'node:fs'

import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

import { mockAuthenticatedShell } from './helpers/auth'

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

const longGroupName = `long-${'pricing-group-'.repeat(30)}`
const longIconText = `https://icons.invalid/${'audit-segment/'.repeat(30)}icon.svg`

function envelope<T>(data: T, requestId = 'req_pricing_groups_e2e') {
  return { code: '', data, message: '', request_id: requestId, success: true }
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function seedAuth(page: Page, testInfo: TestInfo) {
  await mockAuthenticatedShell(page)
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
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        items: [{ id: '9007199254740997', name: '华东定价站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
}

const pricingItem = {
  ability_available: true,
  audio_completion_ratio: null,
  audio_ratio: null,
  billing_expr: 'tokens <= 100 ? 1 : 2',
  billing_mode: 'tiered_expr',
  cache_ratio: '0.5000000000',
  collected_at: 1_784_348_700,
  completion_ratio: '2.0000000000',
  create_cache_ratio: null,
  data_status: 'partial',
  description: '安全定价说明',
  enable_groups: [
    longGroupName,
    'default',
    'vip-zero-usage',
    'group-4',
    'group-5',
    'group-6',
    'group-7',
    'group-8',
  ],
  icon: longIconText,
  id: '9007199254740801',
  image_ratio: null,
  missing_count: 0,
  model_name: f10Pricing.model_name,
  model_price: f10Pricing.input_price,
  model_ratio: '1.2500000000',
  owner_by: 'openai',
  pricing_source: 'tiered_expr',
  pricing_version: 'pinned',
  quota_type: '1',
  remote_state: 'normal',
  site_id: '9007199254740997',
  site_name: '华东定价站点',
  supported_endpoint_types: [
    'chat_completions',
    'responses',
    'embeddings',
    'images',
    'audio',
    'rerank',
    'realtime',
  ],
  tags: 'chat',
  vendor_id: '9007199254740995',
  vendor_name: f10Pricing.vendor,
}

const groupItem = {
  active_pricing_count: '1',
  auto_priority: 1,
  collected_at: 1_784_348_700,
  data_status: 'complete',
  default_use_auto_group: true,
  description: '尚无用量但已配置',
  id: '9007199254740811',
  hidden_from_groups: ['blocked'],
  incoming_overrides: { default: '0.8500000000' },
  missing_count: 0,
  missing_model_names: [],
  missing_pricing_count: '0',
  model_names: [f10Pricing.model_name],
  name: f10ZeroUsageGroup.group_name,
  outgoing_overrides: { default: '0.8500000000' },
  ratio: f10Pricing.group_ratios[f10ZeroUsageGroup.group_name],
  remote_state: 'normal',
  site_id: '9007199254740997',
  site_name: '华东定价站点',
  topup_ratio: '1.0000000000',
  user_selectable: true,
  visible_to_groups: { default: 'VIP visible' },
}

const statistics = {
  data_status: 'partial',
  group_active: '9007199254740993',
  group_missing: '0',
  pricing_active: '9007199254740994',
  pricing_missing: '1',
  site_count: '1',
  sites: [
    {
      group_active: '9007199254740993',
      group_as_of: 1_784_348_700,
      group_data_status: 'complete',
      group_missing: '0',
      pricing_active: '9007199254740994',
      pricing_as_of: 1_784_348_700,
      pricing_data_status: 'partial',
      pricing_missing: '1',
      site_id: '9007199254740997',
      site_name: '华东定价站点',
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
    site_breakdown: [
      {
        as_of: 1_784_348_700,
        data_status: status,
        missing: status === 'complete' ? '0' : '1',
        site_id: '9007199254740997',
        site_name: '华东定价站点',
        total: '1',
      },
    ],
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
    '/pricing-groups?tab=pricing&siteIds=9007199254740997&keyword=gpt&group=vip&states=normal'
  )
  await expect(
    page.getByRole('heading', { exact: true, name: '定价与分组' })
  ).toBeVisible()
  expect(await page.locator('body').innerText()).not.toContain('pricingGroups.')
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
    page.getByText('有可用渠道能力').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('tokens <= 100 ? 1 : 2').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('供应商 ID').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('9007199254740995').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('额度类型').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('归属方').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('定价版本').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('图标文本').filter({ visible: true }).first()
  ).toBeVisible()
  const pricingSurface =
    testInfo.project.name === 'chromium-mobile' ||
    testInfo.project.name === 'chromium-tablet-768'
      ? 'article'
      : 'table'
  const groupScope = page.locator(
    `${pricingSurface} [role="group"][aria-label="可用分组"]`
  )
  const endpointScope = page.locator(
    `${pricingSurface} [role="group"][aria-label="支持端点类型"]`
  )
  await groupScope.scrollIntoViewIfNeeded()
  await expect(groupScope).toBeVisible()
  await expect(endpointScope).toBeVisible()
  await expect(groupScope.getByText('group-8', { exact: true })).toBeHidden()
  await groupScope.getByRole('button', { name: '展开 (+2)' }).click()
  await expect(groupScope.getByText('group-8', { exact: true })).toBeVisible()
  await expect(
    page.getByText('列表采集截至：2026-07-18 12:25:00', { exact: true })
  ).toBeVisible()
  await page.getByText('查看逐站完整性', { exact: true }).click()
  await expect(
    page.getByText('目录 1 项，历史缺失 1 项', { exact: true })
  ).toBeVisible()
  await expect(
    page.getByText('模型：现存 9007199254740994，缺失 1', { exact: true })
  ).toBeVisible()
  const longBadgeFits = await groupScope.evaluate((element) => {
    const badge = [...element.querySelectorAll('[data-slot="badge"]')].find(
      (candidate) => candidate.textContent?.startsWith('long-pricing-group-')
    )
    return badge != null && badge.scrollWidth <= badge.clientWidth + 1
  })
  expect(longBadgeFits).toBe(true)
  expect(
    reads
      .slice(0, 2)
      .map((url) => url.pathname)
      .sort()
  ).toEqual(['/api/pricing-catalog', '/api/pricing-catalog/statistics'])
  const pricingRead = reads.find(
    (url) => url.pathname === '/api/pricing-catalog'
  )
  const pricingStatisticsRead = reads.find(
    (url) => url.pathname === '/api/pricing-catalog/statistics'
  )
  expect(pricingRead?.searchParams.getAll('site_ids')).toEqual([
    '9007199254740997',
  ])
  expect(pricingRead?.searchParams.get('group')).toBe('vip')
  expect(pricingStatisticsRead?.search).toBe('')
  await page.getByRole('tab', { name: '分组配置' }).click()
  await expect(page.getByRole('tab', { name: '分组配置' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
  await expect(
    page.getByText('vip-zero-usage').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('0.8500000000').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('VIP visible').filter({ visible: true }).first()
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
    page.getByRole('heading', { exact: true, name: '站点定价与分组' })
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

test('separates pricing empty, first-error, stale and forced-site scopes', async ({
  page,
}, testInfo) => {
  await seedAuth(page, testInfo)
  let mode: 'empty' | 'error' | 'success' = 'empty'
  let globalReads = 0
  const list = (items: unknown[], total: string) => ({
    as_of: 1_784_348_700,
    data_status: 'complete',
    items,
    page: 1,
    page_size: 20,
    site_breakdown: [],
    total,
  })
  await page.route(/\/api\/pricing-catalog(?:\?.*)?$/, async (route) => {
    globalReads += 1
    if (mode === 'error') {
      await route.fulfill({ status: 503, json: envelope(null) })
      return
    }
    await route.fulfill({
      json: envelope(
        mode === 'empty' ? list([], '0') : list([pricingItem], '1')
      ),
    })
  })
  await page.route(/\/api\/pricing-catalog\/statistics(?:\?.*)?$/, (route) =>
    route.fulfill({ json: envelope(statistics) })
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/pricing-catalog(?:\?.*)?$/,
    (route) => route.fulfill({ status: 503, json: envelope(null) })
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/pricing-catalog\/statistics(?:\?.*)?$/,
    (route) => route.fulfill({ status: 503, json: envelope(null) })
  )

  await page.goto('/pricing-groups?tab=pricing')
  await expect(
    page.getByRole('heading', { name: '当前筛选下没有定价项' })
  ).toBeVisible()
  mode = 'success'
  await page.getByRole('textbox', { name: '名称关键词' }).fill('成功')
  await expect.poll(() => globalReads).toBe(2)
  await expect(
    page.getByText(pricingItem.model_name).filter({ visible: true }).first()
  ).toBeVisible()
  mode = 'error'
  await page.getByRole('textbox', { name: '名称关键词' }).fill('刷新失败')
  await expect.poll(() => globalReads).toBe(3)
  await expect(page.getByRole('alert')).toContainText('数据刷新失败')
  await expect(
    page.getByText(pricingItem.model_name).filter({ visible: true }).first()
  ).toBeVisible()

  await page.goto('/sites/9007199254740997/pricing-groups?tab=pricing')
  await expect(
    page.getByText('无法加载数据', { exact: true }).filter({ visible: true })
  ).toBeVisible()
  await expect(page.getByText(pricingItem.model_name)).toHaveCount(0)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
