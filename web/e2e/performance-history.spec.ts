import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const viewer = {
  display_name: '性能历史只读运营员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'performance_viewer',
}

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_performance_history_e2e') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
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
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: viewer, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ json: envelope(viewer, 'req_performance_self') })
  })
}

const officialRow = {
  avg_latency_ms: '123.4567890000',
  avg_tps: '9.8765432100',
  avg_ttft_ms: '45.6789000000',
  bucket_start: 1_784_262_400,
  collected_at: 1_784_266_000,
  generation_ms: null,
  group: 'vip',
  id: '9007199254740993',
  metric_source: 'official_average',
  model_name: 'gpt-5',
  output_tokens: null,
  request_count: null,
  series_schema: 'official-v1',
  site_id: '9007199254740995',
  site_name: '华东生产站点',
  success_count: null,
  success_rate: '0.9912345678',
  total_latency_ms: null,
  ttft_count: null,
  ttft_sum_ms: null,
}

const counterRow = {
  ...officialRow,
  generation_ms: '100000',
  id: '9007199254740994',
  metric_source: 'counter_ready',
  output_tokens: '987654321',
  request_count: '9007199254740993',
  success_count: '8917127262193553',
  total_latency_ms: '1111999897873513893',
  ttft_count: '9007199254740000',
  ttft_sum_ms: '411567467108923000',
}

function listResponse(site: boolean) {
  const row = site ? counterRow : officialRow
  return {
    as_of: 1_784_266_000,
    data_status: 'complete',
    items: [row],
    page: 1,
    page_size: 20,
    total: 1,
  }
}

function statisticsResponse(site: boolean) {
  const row = site ? counterRow : officialRow
  return {
    aggregation_status: site ? 'complete' : 'unavailable',
    data_status: 'complete',
    site_breakdown: [row],
    summary: site
      ? {
          avg_latency_ms: '123.4567890000',
          avg_tps: '9876.5432100000',
          avg_ttft_ms: '45.6789000000',
          request_count: '9007199254740993',
          success_rate: '0.9900000000',
        }
      : {
          avg_latency_ms: null,
          avg_tps: null,
          avg_ttft_ms: null,
          request_count: null,
          success_rate: null,
        },
    trend: [row],
    unavailable_reason: site ? '' : 'upstream_standard_api_missing_counters',
  }
}

test('preserves official performance values, URL filters, weighted boundary, export and accessibility', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const globalReads: URL[] = []
  let exportBody: ExportBody | undefined

  await page.route(/\/api\/performance-history(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    globalReads.push(new URL(route.request().url()))
    await route.fulfill({ json: envelope(listResponse(false)) })
  })
  await page.route(
    /\/api\/performance-history\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      globalReads.push(new URL(route.request().url()))
      await route.fulfill({ json: envelope(statisticsResponse(false)) })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740995\/performance-history(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      await route.fulfill({ json: envelope(listResponse(true)) })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740995\/performance-history\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      await route.fulfill({ json: envelope(statisticsResponse(true)) })
    }
  )
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticated(route)
    exportBody = route.request().postDataJSON() as ExportBody
    await route.fulfill({
      json: envelope({
        created_at: 1_784_348_800,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody.filters,
        finished_at: null,
        format: exportBody.format,
        id: '792',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'performance_history',
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/792', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        created_at: 1_784_348_800,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody?.filters ?? {},
        finished_at: null,
        format: exportBody?.format ?? 'csv',
        id: '792',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'performance_history',
        status: 'pending',
      }),
    })
  })

  await page.goto('/performance-history')
  const globalHeading = page.getByRole('heading', { name: '全局性能历史' })
  await expect(globalHeading).toBeVisible()
  await expect(page.getByText('跨站总值不可用，逐站原值仍可查看')).toBeVisible()
  await expect(
    page
      .getByText('123.4567890000', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText('0.9912345678', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page.getByText('官方平均原值').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('textbox', { exact: true, name: '模型' }).fill('gpt-5')
  await page.getByRole('textbox', { exact: true, name: '分组' }).fill('vip')
  await page
    .getByRole('textbox', { exact: true, name: '站点 ID' })
    .fill('9007199254740995')
  await page.getByRole('button', { name: '7 天' }).click()
  await expect
    .poll(() => globalReads.at(-1)?.searchParams.getAll('model_names'))
    .toEqual(['gpt-5'])
  await expect
    .poll(() => globalReads.at(-1)?.searchParams.getAll('groups'))
    .toEqual(['vip'])
  await expect
    .poll(() => globalReads.at(-1)?.searchParams.getAll('site_ids'))
    .toEqual(['9007199254740995'])
  await expect
    .poll(() => new URL(page.url()).searchParams.get('models'))
    .toBe('["gpt-5"]')
  await expect
    .poll(() => new URL(page.url()).searchParams.get('groups'))
    .toBe('["vip"]')
  await expect
    .poll(() => new URL(page.url()).searchParams.get('hours'))
    .toBe('168')

  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect
    .poll(() => exportBody?.statistics_type)
    .toBe('performance_history')
  expect(exportBody?.filters.site_ids).toEqual(['9007199254740995'])
  expect(exportBody?.filters.model_names).toEqual(['gpt-5'])
  expect(exportBody?.filters.use_groups).toEqual(['vip'])

  const accessibilityScan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibilityScan.violations).toEqual([])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)

  await page.goto('/sites/9007199254740995/performance-history')
  await expect(
    page.getByRole('heading', { name: '站点性能历史' })
  ).toBeVisible()
  await expect(page.getByText('计数器加权值可用')).toBeVisible()
  await expect(page.getByText('9,007,199,254,740,993')).toBeVisible()
  await expect(page.getByText('0.9900000000')).toBeVisible()
  await expect(
    page.getByText('计数器完整').filter({ visible: true }).first()
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
