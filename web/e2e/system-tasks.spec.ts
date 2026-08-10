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
  display_name: '系统任务只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'system_task_viewer',
}
const f11 = JSON.parse(
  readFileSync(
    new URL('../../testdata/design/f11-system-tasks.json', import.meta.url),
    'utf8'
  )
) as {
  fixture_id: 'F11'
  tasks: Array<{
    created_at: number
    error_code: string | null
    error_present: boolean
    progress: null | {
      processed: string
      progress: number
      remaining: string
      total: string
    }
    remote_id: string
    result: Record<string, string> | null
    status: string
    task_id: string
    type: string
    updated_at: number
  }>
}
const envelope = <T>(data: T, requestId = 'req_system_task_e2e') => ({
  code: '',
  data,
  message: '',
  request_id: requestId,
  success: true,
})
async function seedAuth(page: Page, testInfo: TestInfo) {
  await mockAuthenticatedShell(page)
  if (testInfo.project.name === 'chromium-mobile') {
    await page.setViewportSize({ height: 812, width: 375 })
  }
  await page.addInitScript((user) => {
    localStorage.setItem('pilot-auth-user', JSON.stringify(user))
    localStorage.setItem('uid', user.id)
  }, viewer)
  await page.route(/\/api\/user\/self(?:\?.*)?$/, (route) =>
    route.fulfill({ json: envelope(viewer, 'req_system_self') })
  )
  await page.route(/\/api\/sites(?:\?.*)?$/, (route) =>
    route.fulfill({
      json: envelope({
        items: [{ id: '9007199254740997', name: '华东维护站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  )
}

const base = {
  collected_at: 1_784_348_700,
  data_status: 'partial',
  error_code: '',
  error_present: false,
  progress: {
    processed: '9007199254740993',
    progress: '75',
    remaining: '3',
    total: '9007199254740996',
  },
  remote_created_at: 1_784_300_000,
  remote_updated_at: 1_784_348_600,
  site_id: '9007199254740997',
  site_name: '华东维护站点',
  status: 'running',
}
const items = f11.tasks.map((task, index) => ({
  ...base,
  collected_at: task.updated_at + 1,
  error_code: task.error_code ?? '',
  error_present: task.error_present,
  id: String(9007199254740801 + index),
  progress: task.progress
    ? { ...task.progress, progress: String(task.progress.progress) }
    : null,
  remote_created_at: task.created_at,
  remote_id: task.remote_id,
  remote_updated_at: task.updated_at,
  result: task.result,
  status: task.status,
  task_id: task.task_id,
  type: task.type,
}))
const metric = {
  active: '4',
  error_present: '1',
  failed: '1',
  succeeded: '1',
  total: '6',
}
const breakdown = (id: string, name: string) => ({
  ...metric,
  as_of: 1_784_348_700,
  data_status: 'partial',
  dimension_id: id,
  dimension_name: name,
  site_id: '',
  site_name: '',
})
const statistics = {
  as_of: 1_784_348_700,
  data_error_code: '',
  data_status: 'partial',
  site_breakdown: [
    {
      ...breakdown('9007199254740997', '华东维护站点'),
      site_id: '9007199254740997',
      site_name: '华东维护站点',
    },
  ],
  status_breakdown: [
    breakdown('running', 'running'),
    breakdown('failed', 'failed'),
  ],
  summary: metric,
  type_breakdown: [
    breakdown('log_cleanup', 'log_cleanup'),
    breakdown('log_detail_cleanup', 'log_detail_cleanup'),
    breakdown('channel_test', 'channel_test'),
  ],
}
const pageData = {
  as_of: 1_784_348_700,
  data_error_code: '',
  data_status: 'partial',
  items,
  observed_count: '100',
  page: 1,
  page_size: 20,
  source_limit: '100',
  total: '9007199254740995',
  truncated: true,
  truncation_reason: 'source_limit',
}

test('A100 system tasks stay read-only, typed, bounded and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  expect(f11.fixture_id).toBe('F11')
  await seedAuth(page, testInfo)
  const systemRequests: { method: string; path: string; url: URL }[] = []
  const exportBodies: Record<string, unknown>[] = []
  const fulfill = async (route: Route, data: unknown) => {
    const url = new URL(route.request().url())
    systemRequests.push({
      method: route.request().method(),
      path: url.pathname,
      url,
    })
    await route.fulfill({ json: envelope(data) })
  }
  await page.route(/\/api\/system-tasks\/statistics(?:\?.*)?$/, (route) =>
    fulfill(route, statistics)
  )
  await page.route(/\/api\/system-tasks(?:\?.*)?$/, (route) =>
    fulfill(route, pageData)
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/system-tasks\/statistics(?:\?.*)?$/,
    (route) => fulfill(route, statistics)
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/system-tasks(?:\?.*)?$/,
    (route) => fulfill(route, pageData)
  )
  await page.route('**/api/statistics/export', async (route) => {
    exportBodies.push(route.request().postDataJSON() as Record<string, unknown>)
    const body = exportBodies.at(-1)
    expect(body).toBeDefined()
    if (!body) throw new Error('missing export body')
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
        id: '800',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: body.statistics_type,
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/800', (route) =>
    route.fulfill({ status: 404, json: envelope(null) })
  )

  await page.goto(
    '/system-tasks?siteIds=9007199254740997&types=log_cleanup&statuses=running&errorPresent=false'
  )
  await expect(
    page.getByRole('heading', { exact: true, name: '系统任务' })
  ).toBeVisible()
  await expect(page.getByText('9007199254740995').first()).toBeVisible()
  await expect(page.getByText(/本轮观察 100 条/)).toBeVisible()
  for (const text of [
    '日志清理',
    '日志明细归档',
    '渠道测试',
    '模型更新',
    'Midjourney 轮询',
    '异步任务轮询',
    '9007199254740993',
    '上游系统任务失败',
  ]) {
    await expect(
      page.getByText(text).filter({ visible: true }).first()
    ).toBeVisible()
  }
  await expect(page.getByRole('button', { name: '导出 CSV' })).toBeVisible()
  expect(systemRequests.every((request) => request.method === 'GET')).toBe(true)
  expect(
    systemRequests.some((request) => /\/system-tasks\/[^s]/.test(request.path))
  ).toBe(false)
  const listRequest = systemRequests.find(
    (request) => request.path === '/api/system-tasks'
  )
  expect(listRequest?.url.searchParams.getAll('site_ids')).toEqual([
    '9007199254740997',
  ])
  expect(listRequest?.url.searchParams.getAll('types')).toEqual(['log_cleanup'])
  expect(listRequest?.url.searchParams.get('error_present')).toBe('false')

  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect.poll(() => exportBodies.length).toBe(1)
  expect(exportBodies[0]).toMatchObject({
    statistics_type: 'system_tasks',
    filters: {
      error_present: false,
      site_ids: ['9007199254740997'],
      statuses: ['running'],
      types: ['log_cleanup'],
    },
  })
  const safe = JSON.stringify(exportBodies[0]).toLowerCase()
  for (const field of [
    'active_key',
    'locked_by',
    'raw_json',
    'error_message',
    'credential',
    'payload',
    'private_data',
  ]) {
    expect(safe).not.toContain(field)
  }
  await page.getByRole('button', { name: '关闭' }).click()

  await page.getByRole('tab', { name: /类型分析/ }).click()
  await expect(page).toHaveURL(/tab=types/)
  await expect(page).not.toHaveURL(/types=log_cleanup/)
  await expect(page.getByText('任务类型分析')).toBeVisible()
  await page.getByRole('tab', { name: /任务列表/ }).click()
  await expect(page.getByRole('button', { name: '导出 CSV' })).toBeVisible()

  systemRequests.length = 0
  await page.goto('/sites/9007199254740997/system-tasks?siteIds=9')
  await expect(
    page.getByRole('heading', { exact: true, name: '站点系统任务' })
  ).toBeVisible()
  await expect.poll(() => systemRequests.length).toBeGreaterThanOrEqual(2)
  expect(
    systemRequests.every(
      (request) =>
        request.method === 'GET' && !request.url.searchParams.has('site_ids')
    )
  ).toBe(true)
  expect(
    await page.evaluate(
      () =>
        document.documentElement.scrollWidth >
        document.documentElement.clientWidth
    )
  ).toBe(false)
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
})

test('A100 unavailable collection is diagnosed instead of shown as a real empty inventory', async ({
  page,
}, testInfo) => {
  await seedAuth(page, testInfo)
  await page.route(/\/api\/system-tasks\/statistics(?:\?.*)?$/, (route) =>
    route.fulfill({
      json: envelope({
        ...statistics,
        as_of: null,
        data_error_code: 'SYSTEM_TASK_UPSTREAM_UNAVAILABLE',
        data_status: 'unavailable',
        site_breakdown: [],
        status_breakdown: [],
        summary: {
          active: '0',
          error_present: '0',
          failed: '0',
          succeeded: '0',
          total: '0',
        },
        type_breakdown: [],
      }),
    })
  )
  await page.route(/\/api\/system-tasks(?:\?.*)?$/, (route) =>
    route.fulfill({
      json: envelope({
        ...pageData,
        as_of: null,
        data_error_code: 'SYSTEM_TASK_UPSTREAM_UNAVAILABLE',
        data_status: 'unavailable',
        items: [],
        observed_count: '0',
        total: '0',
        truncated: false,
        truncation_reason: null,
      }),
    })
  )
  await page.goto('/system-tasks')
  await expect(page.getByText('上游系统任务采集失败')).toBeVisible()
  await expect(
    page.getByText('系统任务采集不可用').filter({ visible: true })
  ).toBeVisible()
  await expect(page.getByText('当前筛选下没有系统任务')).toHaveCount(0)
})
