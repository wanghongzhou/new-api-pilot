import { readFileSync } from 'node:fs'

import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

type F12Task = {
  category: 'durable' | 'fast' | 'hourly' | 'rebuild' | 'usage'
  purpose_key: string
  task_type: string
  trigger_class: string
}

const f12 = JSON.parse(
  readFileSync(
    new URL(
      '../../testdata/design/f12-site-task-catalog.json',
      import.meta.url
    ),
    'utf8'
  )
) as { fixture_id: 'F12'; tasks: F12Task[] }
const zh = JSON.parse(
  readFileSync(
    new URL('../src/i18n/locales/zh-CN.json', import.meta.url),
    'utf8'
  )
) as Record<string, string>

const viewer = {
  display_name: '只读验收用户',
  id: '9007199254740994',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'a101-viewer',
}

function envelope<T>(data: T) {
  return {
    code: '',
    data,
    message: '',
    request_id: 'req_a101',
    success: true,
  }
}

function assertRead(route: Route) {
  expect(route.request().method()).toBe('GET')
  expect(route.request().headers()['new-api-user']).toBe(viewer.id)
  expect(route.request().headers()['x-request-id']).toMatch(/^web_/)
}

async function seedAuth(page: Page) {
  await page.addInitScript(
    ({ user }) => {
      window.localStorage.setItem('pilot-auth-user', JSON.stringify(user))
      window.localStorage.setItem('uid', user.id)
    },
    { user: viewer }
  )
}

function siteDetail() {
  return {
    auth_status: 'authorized',
    backfill: {
      completed_windows: 0,
      end_timestamp: null,
      failed_windows: 0,
      latest_error: null,
      progress: 0,
      run_id: null,
      start_timestamp: null,
      status: 'none',
      total_windows: 0,
    },
    base_url: 'https://a101.example.com',
    completeness: {
      complete_site_count: 1,
      complete_unit_count: 24,
      completeness_rate: 1,
      data_status: 'complete',
      expected_site_count: 1,
      expected_unit_count: 24,
      last_verified_at: 1_783_872_000,
      missing_range_total: 0,
      missing_ranges: [],
      missing_ranges_truncated: false,
      missing_site_ids: [],
      unit_type: 'site_hour',
    },
    completeness_rate: 1,
    config_version: 7,
    data_export_enabled: true,
    disabled_at: null,
    health_status: 'ok',
    id: '1',
    last_probe_at: 1_783_872_000,
    last_probe_success_at: 1_783_872_000,
    management_status: 'active',
    monitoring_start_at: 1_780_000_000,
    name: 'A101 任务目录站点',
    online_status: 'online',
    rate: {
      quota_per_unit: '500000',
      source: 'site',
      updated_at: 1_783_872_000,
      usd_exchange_rate: '7.3',
    },
    realtime: {
      expired: false,
      rpm: '20',
      tpm: '3000',
      updated_at: 1_783_872_000,
    },
    remark: 'F12 deterministic site',
    resource: {
      cpu_max_percent: 20,
      data_status: 'complete',
      disk_max_used_percent: 30,
      instance_count: 1,
      memory_max_percent: 40,
      online_instance_count: 1,
      updated_at: 1_783_872_000,
    },
    root_created_at: 1_700_000_000,
    root_user_id: '1',
    statistics_end_at: null,
    statistics_start_at: 1_700_000_000,
    statistics_start_source: 'root_created_at',
    statistics_status: 'ready',
    system_name: 'New API',
    today: {
      active_users: '2',
      as_of: 1_783_872_000,
      data_status: 'complete',
      is_final: false,
      quota: '1000',
      request_count: '10',
      token_used: '2000',
    },
    updated_at: 1_783_872_000,
    version: 'v0.8.0',
  }
}

function targetType(taskType: string) {
  if (taskType === 'account_rebuild') return 'account'
  if (taskType === 'customer_rebuild') return 'customer'
  return 'site'
}

function run(task: F12Task, index: number) {
  const status = ['pending', 'running', 'success', 'failed'][index % 4]
  const target = targetType(task.task_type)
  const windowed = task.category === 'usage' || task.category === 'rebuild'
  return {
    completed_windows: status === 'success' ? 1 : 0,
    created_at: 1_783_872_000 + index,
    created_request_id: `req_a101_${index}`,
    deduplicated: false,
    end_timestamp: windowed ? 1_783_872_000 : null,
    error: null,
    failed_windows: status === 'failed' && windowed ? 1 : 0,
    fetched_rows: String(index + 1),
    finished_at: ['success', 'failed'].includes(status)
      ? 1_783_872_020 + index
      : null,
    id: String(index + 1),
    last_request_id: `req_a101_last_${index}`,
    next_attempt_at: null,
    priority: 10,
    progress: ['success', 'failed'].includes(status) ? 1 : 0.5,
    retry_count: 0,
    site_config_version: 7,
    site_id: '1',
    start_timestamp: windowed ? 1_783_868_400 : null,
    started_at: status === 'pending' ? null : 1_783_872_000 + index,
    status,
    target_id: target === 'site' ? '1' : String(100 + index),
    target_type: target,
    task_type: task.task_type,
    total_windows: windowed ? 1 : 0,
    trigger_type: 'schedule',
    windows_initialized: windowed,
    written_rows: String(index + 1),
  }
}

async function mockPage(page: Page) {
  const collectionRequests: (string | null)[] = []
  const fastRequests: string[] = []
  await page.route('**/api/**', async (route) => {
    throw new Error(
      `A101 unexpected API request: ${route.request().method()} ${route.request().url()}`
    )
  })
  await page.route('**/api/user/self', async (route) => {
    assertRead(route)
    await route.fulfill({ json: envelope(viewer) })
  })
  await page.route('**/api/sites/1/instances', async (route) => {
    assertRead(route)
    await route.fulfill({ json: envelope([]) })
  })
  await page.route(/\/api\/sites\/1\/performance(?:\?.*)?$/, async (route) => {
    assertRead(route)
    await route.fulfill({
      json: envelope({
        avg_latency_ms: 0,
        avg_tps: 0,
        data_status: 'missing',
        hours: 24,
        models: [],
        request_count: '0',
        sampled_at: null,
        success_rate: 0,
      }),
    })
  })
  await page.route(/\/api\/sites\/1\/stats(?:\?.*)?$/, async (route) => {
    assertRead(route)
    const url = new URL(route.request().url())
    const start = Number(url.searchParams.get('start_timestamp'))
    const end = Number(url.searchParams.get('end_timestamp'))
    await route.fulfill({
      json: envelope({
        breakdown: { items: [], page: 1, page_size: 20, total: 0 },
        completeness: {
          complete_site_count: 1,
          complete_unit_count: 0,
          completeness_rate: 1,
          data_status: 'complete',
          expected_site_count: 1,
          expected_unit_count: 0,
          last_verified_at: end,
          missing_range_total: 0,
          missing_ranges: [],
          missing_ranges_truncated: false,
          missing_site_ids: [],
          unit_type: 'site_hour',
        },
        granularity: 'hour',
        range: {
          as_of: end,
          end_timestamp: end,
          start_timestamp: start,
          timezone: 'Asia/Shanghai',
        },
        scope: 'site',
        site_breakdown: [],
        summary: {
          active_users: '0',
          data_status: 'complete',
          is_partial: false,
          quota: '0',
          request_count: '0',
          token_used: '0',
        },
        trend: [],
      }),
    })
  })
  await page.route(/\/api\/fast-tasks(?:\?.*)?$/, async (route) => {
    assertRead(route)
    const url = new URL(route.request().url())
    const taskType = url.searchParams.get('task_type') ?? 'site_probe'
    fastRequests.push(taskType)
    await route.fulfill({
      json: envelope({
        has_more: false,
        items: [
          {
            duration_ms: 1200,
            error: '',
            finished_at: 1_783_872_002,
            request_id: 'req_fast_a101',
            site_id: '1',
            started_at: 1_783_872_000,
            status: 'success',
            task_type: taskType,
          },
        ],
        limit: 50,
        offset: 0,
        total: 1,
      }),
    })
  })
  await page.route(
    /\/api\/sites\/1\/collection-runs(?:\?.*)?$/,
    async (route) => {
      assertRead(route)
      const selected = new URL(route.request().url()).searchParams.get(
        'task_type'
      )
      collectionRequests.push(selected)
      const tasks = selected
        ? f12.tasks.filter(({ task_type }) => task_type === selected)
        : f12.tasks
      await route.fulfill({
        json: envelope({
          items: tasks.map((task) => run(task, f12.tasks.indexOf(task))),
          page: 1,
          page_size: 20,
          total: tasks.length,
        }),
      })
    }
  )
  await page.route('**/api/sites/1', async (route) => {
    assertRead(route)
    await route.fulfill({ json: envelope(siteDetail()) })
  })
  return { collectionRequests, fastRequests }
}

test('A101 shows the F12 nineteen-task catalog and exact fast-task subset', async ({
  page,
}, testInfo) => {
  expect(f12.fixture_id).toBe('F12')
  expect(f12.tasks).toHaveLength(19)
  await seedAuth(page)
  const requests = await mockPage(page)
  await page.goto('/sites/1')

  await expect(
    page.getByRole('heading', { name: 'A101 任务目录站点' })
  ).toBeVisible()
  const taskFilters = page.getByRole('combobox', {
    name: zh['collection.filterTaskType'],
  })
  await expect(taskFilters).toHaveCount(2)
  const collectionFilter = taskFilters.nth(0)
  const fastFilter = taskFilters.nth(1)
  await expect(collectionFilter.locator('option')).toHaveCount(20)
  await expect(fastFilter.locator('option')).toHaveCount(3)

  for (const task of f12.tasks) {
    const label = zh[`collection.task.${task.task_type}`]
    const purpose = zh[task.purpose_key]
    expect(label).toBeTruthy()
    expect(purpose).toBeTruthy()
    await expect(
      collectionFilter.locator(`option[value="${task.task_type}"]`)
    ).toHaveText(label)
  }

  for (const status of ['pending', 'running', 'success', 'failed']) {
    await expect(
      page
        .getByText(zh[`collection.status.${status}`], { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
  }

  for (const task of f12.tasks) {
    await collectionFilter.selectOption(task.task_type)
    await expect
      .poll(() => requests.collectionRequests.at(-1))
      .toBe(task.task_type)
    await expect(
      page
        .getByText(zh[task.purpose_key], { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
  }

  await fastFilter.selectOption('resource_snapshot')
  await expect
    .poll(() => requests.fastRequests.at(-1))
    .toBe('resource_snapshot')
  await expect(
    page
      .getByText(zh['siteTasks.purpose.resourceSnapshot'], { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()

  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  expect(['chromium-desktop', 'chromium-mobile']).toContain(
    testInfo.project.name
  )
})
