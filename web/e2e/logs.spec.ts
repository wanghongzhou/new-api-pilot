import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

import { mockAuthenticatedShell } from './helpers/auth'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const viewer = {
  display_name: '日志只读运营员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'log_viewer',
}

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_logs_e2e') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function failureEnvelope(code = 'INTERNAL_ERROR') {
  return {
    code,
    data: null,
    message: code,
    request_id: 'req_logs_failure',
    success: false,
  }
}

async function seedAuth(page: Page) {
  await mockAuthenticatedShell(page)
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: viewer, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ json: envelope(viewer, 'req_logs_self') })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        items: [{ id: '9007199254740993', name: '华东生产站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

const logItem = {
  cache_creation_tokens: '0',
  cache_creation_tokens_1h: '0',
  cache_creation_tokens_5m: '0',
  cache_read_tokens: '0',
  channel_id: '9007199254740997',
  completion_tokens: '9223372036854775807',
  content: '[redacted]',
  created_at: 1_784_262_300,
  first_response_time_ms: '250',
  group: 'vip',
  id: '9007199254740999',
  ip: '',
  is_stream: true,
  model_name: 'gpt-4.1',
  prompt_tokens: '9007199254740995',
  quota: '9223372036854775806',
  rate: {
    quota_per_unit: null,
    source: 'unavailable',
    updated_at: null,
    usd_exchange_rate: null,
  },
  remote_user_id: '9007199254740993',
  request_id: 'req-local-safe',
  site_id: '9007199254740993',
  site_name: '华东生产站点',
  stream_end_reason: 'stop',
  stream_error_count: '0',
  stream_status: 'completed',
  token_id: '9007199254740995',
  token_name: 'production-token',
  type: 5,
  upstream_request_id: 'req-upstream-safe',
  use_time_seconds: '17',
  username: 'alice',
}

test('queries, inspects and exports global redacted logs without treating them as financial facts', async ({
  page,
}) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  const reads: URL[] = []
  let exportBody: ExportBody | undefined
  await page.route(/\/api\/logs\/stat(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        quota: logItem.quota,
        rpm: '1',
        site_breakdown: [],
        tpm: '2',
      }),
    })
  })
  await page.route(/\/api\/logs(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    reads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        as_of: 1_784_262_400,
        data_status: 'partial',
        items: [logItem],
        page: Number(new URL(route.request().url()).searchParams.get('p') ?? 1),
        page_size: 20,
        total: 41,
      }),
    })
  })
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticated(route)
    exportBody = route.request().postDataJSON() as ExportBody
    await route.fulfill({
      json: envelope({
        created_at: 1_784_262_400,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody?.filters,
        finished_at: null,
        format: exportBody?.format,
        id: '501',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'logs',
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/501', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        created_at: 1_784_262_400,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody?.filters ?? {},
        finished_at: null,
        format: exportBody?.format ?? 'csv',
        id: '501',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'logs',
        status: 'pending',
      }),
    })
  })

  await page.goto('/logs')
  await expect(
    page.getByRole('heading', { name: '全局使用日志' })
  ).toBeVisible()
  await expect(page.getByText('使用日志不是财务事实')).toBeVisible()
  await expect(page.getByText('请求数')).toBeVisible()
  await expect(page.getByText('部分站点或时间窗口未完整采集')).toBeVisible()
  await expect(
    page
      .getByText(/华东生产站点/)
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText(/9007199254740993/)
      .filter({ visible: true })
      .first()
  ).toBeVisible()

  await page.getByLabel('用户名').fill('alice')
  await page.getByRole('button', { name: '日志类型' }).click()
  await page.getByRole('button', { name: '错误' }).click()
  await page.getByLabel('Channel ID').fill('9007199254740997')
  await page.getByRole('button', { name: /更多筛选/ }).click()
  await page.getByLabel('分组').fill('vip')
  await page.getByLabel('Request ID', { exact: true }).fill('req-local-safe')
  await page.getByLabel('上游 Request ID').fill('req-upstream-safe')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: /更多筛选 3/ })).toBeVisible()
  await expect
    .poll(() => reads.at(-1)?.searchParams.get('username'))
    .toBe('alice')
  await expect
    .poll(() => reads.at(-1)?.searchParams.get('channel_id'))
    .toBe('9007199254740997')
  await expect(page).toHaveURL(/username=alice/)
  await expect(page).toHaveURL(/channelId=9007199254740997/)

  await page
    .getByRole('button', { name: /查看/ })
    .filter({ visible: true })
    .first()
    .click()
  const detail = page.getByRole('dialog', { name: '日志详情' })
  await expect(detail.getByText('[redacted]')).toBeVisible()
  await expect(detail.getByText('未记录')).toBeVisible()
  await expect(detail.getByText('9007199254740999')).toBeVisible()
  await expect(detail).not.toContainText('Bearer secret-token')
  await detail.getByRole('button', { name: '关闭' }).last().click()

  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect(page.getByRole('dialog', { name: '导出任务' })).toBeVisible()
  await expect.poll(() => exportBody?.statistics_type).toBe('logs')
  expect(exportBody?.filters).toMatchObject({
    channel_id: '9007199254740997',
    log_type: 5,
    request_id: 'req-local-safe',
    site_ids: [],
    upstream_request_id: 'req-upstream-safe',
    use_groups: ['vip'],
    username: 'alice',
  })
  expect(exportBody?.filters).not.toHaveProperty('p')
  expect(exportBody?.filters).not.toHaveProperty('page_size')

  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})

test('uses the site-scoped endpoint and preserves unavailable as a distinct empty state', async ({
  page,
}) => {
  await seedAuth(page)
  const reads: URL[] = []
  await page.route(/\/api\/sites\/1\/logs\/stat(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({ quota: '0', rpm: '0', site_breakdown: [], tpm: '0' }),
    })
  })
  await page.route(/\/api\/sites\/1\/logs(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    reads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        as_of: null,
        data_status: 'unavailable',
        items: [],
        page: 1,
        page_size: 20,
        total: 0,
      }),
    })
  })

  await page.goto('/sites/1/logs')
  await expect(
    page.getByRole('heading', { name: '站点使用日志' })
  ).toBeVisible()
  await expect(
    page
      .getByText('上游日志暂未获取，当前列表可能不完整。', { exact: true })
      .first()
  ).toBeVisible()
  await expect(page.getByLabel('站点 ID')).toHaveCount(0)
  await page.getByLabel('模型').fill('gpt-4.1')
  await expect
    .poll(() => reads.at(-1)?.searchParams.get('model_name'))
    .toBe('gpt-4.1')
  expect(reads.at(-1)?.searchParams.has('site_ids')).toBe(false)
  await page.reload()
  await expect(page.getByLabel('模型')).toHaveValue('gpt-4.1')
  await expect(page.getByRole('button', { name: '返回站点详情' })).toBeVisible()
  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])
})

test('retains safe rows and bigint totals when a filtered refresh fails', async ({
  page,
}) => {
  await seedAuth(page)
  let reads = 0
  await page.route(/\/api\/logs\/stat(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({
        quota: '9223372036854775806',
        rpm: '0',
        site_breakdown: [],
        tpm: '0',
      }),
    })
  })
  await page.route(/\/api\/logs(?:\?.*)?$/, async (route) => {
    reads += 1
    if (reads === 1) {
      await route.fulfill({
        json: envelope({
          as_of: 1_784_262_400,
          data_status: 'complete',
          items: [logItem],
          page: 1,
          page_size: 20,
          total: '9007199254740993',
        }),
      })
      return
    }
    await route.fulfill({ json: failureEnvelope(), status: 503 })
  })

  await page.goto('/logs')
  await expect(page.getByText('9,007,199,254,740,993').first()).toBeVisible()
  await expect(
    page.getByText('alice').filter({ visible: true }).first()
  ).toBeVisible()
  await page.getByLabel('用户名').fill('refresh-failure')
  await page.getByRole('button', { name: '搜索' }).click()
  await expect.poll(() => reads).toBeGreaterThan(1)
  await expect(page.getByRole('alert')).toContainText('数据刷新失败')
  await expect(
    page.getByText('alice').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(page.getByText('9,007,199,254,740,993').first()).toBeVisible()
})

test('shows a blocking state when the initial logs request fails', async ({
  page,
}) => {
  await seedAuth(page)
  await page.route(/\/api\/logs\/stat(?:\?.*)?$/, async (route) => {
    await route.fulfill({ json: failureEnvelope(), status: 503 })
  })
  await page.route(/\/api\/logs(?:\?.*)?$/, async (route) => {
    await route.fulfill({ json: failureEnvelope(), status: 503 })
  })
  await page.goto('/logs?username=initial-failure')
  await expect(
    page
      .getByRole('region', { name: '使用日志列表' })
      .getByRole('heading', { name: '无法加载数据' })
  ).toBeVisible()
  await expect(page.getByText('alice').filter({ visible: true })).toHaveCount(0)
})
