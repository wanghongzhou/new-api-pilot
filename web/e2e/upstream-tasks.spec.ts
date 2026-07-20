import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const viewer = {
  display_name: '任务只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'task_viewer',
}
const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_task_e2e') {
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
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: viewer, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ json: envelope(viewer, 'req_task_self') })
  })
}

const statuses = [
  'NOT_START',
  'SUBMITTED',
  'QUEUED',
  'IN_PROGRESS',
  'FAILURE',
  'SUCCESS',
  'UNKNOWN',
] as const

const tasks = statuses.map((status, index) => ({
  action: 'generate',
  channel_id: index === 0 ? '0' : '9007199254740997',
  created_at: 1_784_000_000 - index,
  finish_time: status === 'SUCCESS' || status === 'FAILURE' ? 1_784_000_300 : 0,
  first_seen_at: 1_784_000_010,
  group: 'default',
  id: String(9007199254740900 + index),
  last_seen_at: 1_784_348_700,
  platform: 'video',
  progress: status === 'IN_PROGRESS' ? '阶段 2/4' : status,
  properties: { model: 'safe-model' },
  quota: '9007199254740993',
  remote_id: String(9007199254740800 + index),
  site_id: '9007199254740997',
  site_name: '华东任务站点',
  start_time:
    status === 'IN_PROGRESS' || status === 'SUCCESS' || status === 'FAILURE'
      ? 1_784_000_100
      : 0,
  status,
  submit_time: 1_784_000_000,
  task_id: `task-safe-${index}`,
  updated_at: 1_784_000_200,
  user_id: index === 0 ? '0' : '9007199254740995',
}))

const metric = {
  avg_queue_seconds: null,
  avg_run_seconds: null,
  avg_total_seconds: null,
  failure: '1',
  queued: '3',
  running: '1',
  success: '1',
  success_rate: null,
  total: '7',
}

function breakdown(dimensionId: string, dimensionName: string, site = false) {
  return {
    ...metric,
    as_of: 1_784_348_700,
    data_status: site ? 'unavailable' : 'partial',
    dimension_id: dimensionId,
    dimension_name: dimensionName,
    site_id: site ? '9007199254740997' : '0',
    site_name: site ? '华东任务站点' : '',
  }
}

function statistics() {
  return {
    action_breakdown: [breakdown('generate', 'generate')],
    data_status: 'partial',
    model_breakdown: [breakdown('safe-model', 'safe-model')],
    platform_breakdown: [breakdown('video', 'video')],
    site_breakdown: [breakdown('9007199254740997', '华东任务站点', true)],
    status_breakdown: [breakdown('IN_PROGRESS', 'IN_PROGRESS')],
    summary: metric,
  }
}

function forbiddenFields() {
  return [
    ['in', 'put'].join(''),
    ['fail', 'reason'].join('_'),
    ['result', 'url'].join('_'),
    ['private', 'data'].join('_'),
    ['down', 'load', 'url'].join('_'),
  ]
}

test('A95 keeps upstream tasks exact, read-only, private, exportable and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const globalReads: URL[] = []
  let exportBody: ExportBody | undefined

  await page.route(/\/api\/upstream-tasks(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    globalReads.push(new URL(route.request().url()))
    await route.fulfill({
      json: envelope({
        as_of: 1_784_348_700,
        data_status: 'partial',
        items: tasks,
        page: 1,
        page_size: 20,
        total: tasks.length,
      }),
    })
  })
  await page.route(
    /\/api\/upstream-tasks\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      globalReads.push(new URL(route.request().url()))
      await route.fulfill({ json: envelope(statistics()) })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/upstream-tasks(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      expect(new URL(route.request().url()).searchParams.has('site_ids')).toBe(
        false
      )
      await route.fulfill({
        json: envelope({
          as_of: 1_784_348_700,
          data_status: 'unavailable',
          items: tasks.slice(0, 1),
          page: 1,
          page_size: 20,
          total: 1,
        }),
      })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/upstream-tasks\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      expect(new URL(route.request().url()).searchParams.has('site_ids')).toBe(
        false
      )
      await route.fulfill({ json: envelope(statistics()) })
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
        id: '795',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: exportBody.statistics_type,
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/795', async (route) => {
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
        id: '795',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: exportBody?.statistics_type ?? 'upstream_tasks',
        status: 'pending',
      }),
    })
  })

  await page.goto('/upstream-tasks')
  await expect(
    page.getByRole('heading', { exact: true, name: '上游任务' })
  ).toBeVisible()
  expect(new URL(page.url()).searchParams.has('start')).toBe(false)
  expect(new URL(page.url()).searchParams.has('end')).toBe(false)
  await expect(
    page.getByText('阶段 2/4').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()
  for (const label of [
    '未开始',
    '已提交',
    '排队中',
    '运行中',
    '失败',
    '成功',
    '未知',
  ]) {
    await expect(
      page.getByText(label, { exact: true }).filter({ visible: true }).first()
    ).toBeVisible()
  }

  await page
    .getByRole('textbox', { exact: true, name: '站点 ID' })
    .fill('9007199254740997')
  await page
    .getByRole('textbox', { exact: true, name: '远端记录 ID' })
    .fill('9007199254740993')
  await page
    .getByRole('textbox', { exact: true, name: 'Task ID（精确）' })
    .fill('task-safe')
  await page
    .getByRole('textbox', { exact: true, name: '远端用户 ID' })
    .fill('0')
  await page
    .getByRole('textbox', { exact: true, name: '远端渠道 ID' })
    .fill('0')
  await page
    .getByRole('textbox', { exact: true, name: 'Platform' })
    .fill('video')
  await page
    .getByRole('textbox', { exact: true, name: 'Group' })
    .fill('default')
  await page
    .getByRole('textbox', { exact: true, name: 'Action' })
    .fill('generate')
  await page
    .getByRole('textbox', { exact: true, name: 'Model' })
    .fill('safe-model')
  await page.getByRole('button', { name: '运行中' }).click()

  await expect
    .poll(() => globalReads.at(-1)?.searchParams.get('remote_channel_id'))
    .toBe('0')
  const lastRead = globalReads.at(-1)
  expect(lastRead?.searchParams.getAll('site_ids')).toEqual([
    '9007199254740997',
  ])
  expect(lastRead?.searchParams.get('remote_user_id')).toBe('0')
  expect(lastRead?.searchParams.get('task_id')).toBe('task-safe')
  expect(lastRead?.searchParams.has('start_timestamp')).toBe(false)
  expect(lastRead?.searchParams.has('end_timestamp')).toBe(false)

  await page.getByRole('button', { name: '导出 XLSX' }).click()
  await expect.poll(() => exportBody?.statistics_type).toBe('upstream_tasks')
  expect(exportBody?.filters.remote_channel_id).toBe('0')
  expect(exportBody?.filters.task_statuses).toEqual(['IN_PROGRESS'])
  expect(exportBody?.filters.start_timestamp).toBe(0)
  expect(exportBody?.filters.end_timestamp).toBe(0)
  const serializedExport = JSON.stringify(exportBody).toLowerCase()
  for (const field of forbiddenFields()) {
    expect(serializedExport).not.toContain(field)
  }

  const accessibilityScan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibilityScan.violations).toEqual([])
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
  const [genericField, ...specificFields] = forbiddenFields()
  for (const field of specificFields) {
    expect(visibleState).not.toContain(field)
  }
  expect(
    await page.evaluate((field) => {
      const selectors = [
        `[name="${field}"]`,
        `[id="${field}"]`,
        `[data-${field}]`,
      ]
      return selectors.some((selector) => document.querySelector(selector))
    }, genericField)
  ).toBe(false)
  await expect(
    page.getByRole('button', { name: /取消|重试|重跑|结果/ })
  ).toHaveCount(0)

  await page.goto('/sites/9007199254740997/upstream-tasks')
  await expect(
    page.getByRole('heading', { exact: true, name: '站点上游任务' })
  ).toBeVisible()
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
