import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

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

async function seedAuth(page: Page) {
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
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

const logItem = {
  channel_id: '9007199254740997',
  completion_tokens: '9223372036854775807',
  content: '[redacted]',
  created_at: 1_784_262_300,
  group: 'vip',
  id: '9007199254740999',
  ip: '',
  is_stream: true,
  model_name: 'gpt-4.1',
  prompt_tokens: '9007199254740995',
  quota: '9223372036854775806',
  remote_user_id: '9007199254740993',
  request_id: 'req-local-safe',
  site_id: '9007199254740993',
  site_name: '华东生产站点',
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
    page.getByRole('heading', { name: '全局日志明细' })
  ).toBeVisible()
  await expect(page.getByText('日志不是财务事实')).toBeVisible()
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
  await page.getByLabel('日志类型').selectOption('5')
  await page.getByLabel('Channel ID').fill('9007199254740997')
  await page.getByLabel('分组').fill('vip')
  await page.getByLabel('Request ID', { exact: true }).fill('req-local-safe')
  await page.getByLabel('上游 Request ID').fill('req-upstream-safe')
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
  await expect(detail.getByText('9223372036854775807')).toBeVisible()
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
    page.getByRole('heading', { name: '站点日志明细' })
  ).toBeVisible()
  await expect(
    page.getByRole('status').getByText('上游日志暂不可用')
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
