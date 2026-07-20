import { expect, test, type Page } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const rangeStart = 1_783_785_600
const rangeEnd = rangeStart + 86_400

const viewer = {
  display_name: '深链验收只读用户',
  id: '9007199254740994',
  must_change_password: false,
  role: 'viewer' as const,
  status: 1 as const,
  username: 'deep-link-viewer',
}

function envelope<T>(data: T, requestId = 'req_deep_link') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function errorEnvelope(code: string) {
  return {
    code,
    data: null,
    field_errors: null,
    message: code,
    request_id: `req_${code.toLowerCase()}`,
    success: false,
  }
}

function encodedSearch(value: string | string[]) {
  return encodeURIComponent(JSON.stringify(value))
}

function encodedId(value: string) {
  return encodeURIComponent(value)
}

async function seedViewer(page: Page) {
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: viewer, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    expect(route.request().headers()['new-api-user']).toBe(viewer.id)
    await route.fulfill({ json: envelope(viewer, 'req_self') })
  })
}

function statisticsResponse(scope: 'account' | 'customer') {
  const point = {
    active_users: '1',
    as_of: rangeEnd - 60,
    bucket_end: rangeEnd,
    bucket_start: rangeStart,
    data_status: 'complete',
    is_final: true,
    quota: '500000',
    reason: null,
    request_count: '42',
    site_breakdown: [],
    token_used: '8400',
  }
  const item =
    scope === 'customer'
      ? {
          ...point,
          account_count: 1,
          completeness_rate: 1,
          dimension_id: '7',
          dimension_name: '示例客户',
          dimension_type: 'customer',
          site_count: 1,
          site_id: null,
          site_name: null,
        }
      : {
          ...point,
          completeness_rate: 1,
          customer_id: '7',
          customer_name: '示例客户',
          dimension_id: '88',
          dimension_name: 'customer_prod',
          dimension_type: 'account',
          remote_user_id: '9007199254740995',
          site_id: '1',
          site_name: '华东站点',
        }
  return {
    breakdown: { items: [item], page: 2, page_size: 20, total: 1 },
    completeness: {
      complete_site_count: 1,
      complete_unit_count: 24,
      completeness_rate: 1,
      data_status: 'complete',
      expected_site_count: 1,
      expected_unit_count: 24,
      last_verified_at: rangeEnd - 60,
      missing_range_total: 0,
      missing_ranges: [],
      missing_ranges_truncated: false,
      missing_site_ids: [],
      unit_type: 'hour',
    },
    granularity: 'hour',
    range: {
      as_of: rangeEnd - 60,
      end_timestamp: rangeEnd,
      start_timestamp: rangeStart,
      timezone: 'Asia/Shanghai',
    },
    scope,
    site_breakdown: [],
    summary: {
      active_users: '1',
      data_status: 'complete',
      is_partial: false,
      quota: '500000',
      request_count: '42',
      token_used: '8400',
    },
    trend: [point],
  }
}

function statisticsSearch() {
  return new URLSearchParams({
    end: String(rangeEnd),
    granularity: 'hour',
    metric: 'request_count',
    page: '2',
    pageSize: '20',
    start: String(rangeStart),
    view: 'table',
  }).toString()
}

test('A72 restores direct customer and account statistics routes through reload and history', async ({
  page,
}) => {
  await seedViewer(page)
  await page.route('**/api/customers/7', async (route) => {
    await route.fulfill({ json: envelope({ id: '7', name: '示例客户' }) })
  })
  await page.route('**/api/accounts/88', async (route) => {
    await route.fulfill({
      json: envelope({ id: '88', username: 'customer_prod' }),
    })
  })
  await page.route(/\/api\/customers\/7\/stats(?:\?.*)?$/, async (route) => {
    await route.fulfill({ json: envelope(statisticsResponse('customer')) })
  })
  await page.route(/\/api\/accounts\/88\/stats(?:\?.*)?$/, async (route) => {
    await route.fulfill({ json: envelope(statisticsResponse('account')) })
  })

  const search = statisticsSearch()
  await page.goto(`/customers/7/stats?${search}`)
  await expect(
    page.getByRole('heading', { name: '示例客户 的客户统计' })
  ).toBeVisible()
  await expect(page.getByRole('heading', { name: '范围汇总' })).toBeVisible()
  await page.reload()
  await expect(
    page.getByRole('heading', { name: '示例客户 的客户统计' })
  ).toBeVisible()

  await page.goto(`/accounts/88/stats?${search}`)
  await expect(
    page.getByRole('heading', { name: 'customer_prod 的账户统计' })
  ).toBeVisible()
  await page.goBack()
  await expect(page).toHaveURL(/\/customers\/7\/stats/)
  await expect(
    page.getByRole('heading', { name: '示例客户 的客户统计' })
  ).toBeVisible()
  await page.goForward()
  await expect(page).toHaveURL(/\/accounts\/88\/stats/)
  await expect(
    page.getByRole('heading', { name: 'customer_prod 的账户统计' })
  ).toBeVisible()
})

const alertEvent = {
  current_value: '92.5',
  first_fired_at: rangeEnd - 600,
  first_observed_at: rangeEnd - 660,
  id: '101',
  last_fired_at: rangeEnd - 60,
  level: 'critical',
  message: {
    code: 'ALERT_CPU_HIGH',
    params: {
      site_id: '1',
      target_name: '推理实例',
      target_type: 'instance',
      threshold: '80',
      value: '92.5',
    },
    technical_detail: '',
  },
  resolved_at: null,
  rule_id: '11',
  rule_key: 'cpu_high',
  site_id: '1',
  site_name: '华东站点',
  status: 'firing',
  target_key: 'instance-primary',
  target_name: '推理实例',
  target_type: 'instance',
  threshold_value: '80',
}

async function mockAlerts(page: Page) {
  const listSearches: URLSearchParams[] = []
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({
        items: [{ id: '1', name: '华东站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
  await page.route('**/api/alerts/summary', async (route) => {
    await route.fulfill({
      json: envelope({
        critical_count: 1,
        firing_count: 1,
        resolved_today_count: 0,
        updated_at: rangeEnd,
        warning_count: 0,
      }),
    })
  })
  await page.route(/\/api\/alerts\/\d+(?:\?.*)?$/, async (route) => {
    const id = new URL(route.request().url()).pathname.split('/').at(-1)
    if (id === '999') {
      await route.fulfill({ json: errorEnvelope('NOT_FOUND'), status: 404 })
      return
    }
    await route.fulfill({
      json: envelope({ ...alertEvent, consecutive_count: 3, deliveries: [] }),
    })
  })
  await page.route(/\/api\/alerts(?:\?.*)?$/, async (route) => {
    const search = new URL(route.request().url()).searchParams
    listSearches.push(new URLSearchParams(search))
    await route.fulfill({
      json: envelope({
        items: [alertEvent],
        page: Number(search.get('p') ?? 1),
        page_size: Number(search.get('page_size') ?? 20),
        total: 1,
      }),
    })
  })
  return { listSearches }
}

test('A72 preserves alert deep-link state and isolates a missing alert detail', async ({
  page,
}) => {
  await seedViewer(page)
  const state = await mockAlerts(page)
  const filters = `page=2&status=${encodedSearch(['firing'])}`
  await page.goto(`/alerts?alertId=${encodedId('101')}&${filters}`)

  let sheet = page.getByRole('dialog', { name: '告警事件详情' })
  await expect(sheet.getByText('CPU 使用率过高')).toBeVisible()
  await expect.poll(() => state.listSearches.at(-1)?.get('p')).toBe('2')
  expect(state.listSearches.at(-1)?.getAll('status')).toEqual(['firing'])
  await page.reload()
  sheet = page.getByRole('dialog', { name: '告警事件详情' })
  await expect(sheet.getByText('CPU 使用率过高')).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()
  await expect(page).not.toHaveURL(/alertId=/)
  await expect(page).toHaveURL(/page=2/)
  await expect(
    page.getByText('CPU 使用率过高').filter({ visible: true }).first()
  ).toBeVisible()
  await page.goBack()
  sheet = page.getByRole('dialog', { name: '告警事件详情' })
  await expect(sheet.getByText('CPU 使用率过高')).toBeVisible()
  await page.goForward()
  await expect(page.getByRole('dialog', { name: '告警事件详情' })).toHaveCount(
    0
  )

  await page.goto(`/alerts?alertId=${encodedId('999')}&${filters}`)
  sheet = page.getByRole('dialog', { name: '告警事件详情' })
  await expect(sheet.getByText('告警事件不存在')).toBeVisible()
  await expect(
    page.getByText('CPU 使用率过高').filter({ visible: true }).first()
  ).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()
  await expect(page).toHaveURL(/page=2/)
  await expect(page).not.toHaveURL(/alertId=/)
})

function exportJob(id: string) {
  return {
    created_at: rangeEnd - 60,
    data_snapshot_at: rangeEnd - 30,
    deduplicated: false,
    error: null,
    expires_at: rangeEnd + 86_400,
    file_name: 'statistics.csv',
    file_size: '1024',
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: [],
      end_timestamp: rangeEnd,
      granularity: 'hour' as const,
      model_names: [],
      site_ids: ['1'],
      sort_by: 'request_count' as const,
      sort_order: 'asc' as const,
      start_timestamp: rangeStart,
    },
    finished_at: rangeEnd,
    format: 'csv' as const,
    id,
    progress: 100,
    row_count: '42',
    started_at: rangeEnd - 50,
    statistics_type: 'global' as const,
    status: 'success' as const,
  }
}

async function mockExports(page: Page) {
  const listSearches: URLSearchParams[] = []
  await page.route(
    /\/api\/statistics\/exports\/\d+(?:\?.*)?$/,
    async (route) => {
      const id = new URL(route.request().url()).pathname.split('/').at(-1)
      if (id === '999') {
        await route.fulfill({ json: errorEnvelope('NOT_FOUND'), status: 404 })
        return
      }
      await route.fulfill({ json: envelope(exportJob(id ?? '801')) })
    }
  )
  await page.route(/\/api\/statistics\/exports(?:\?.*)?$/, async (route) => {
    const search = new URL(route.request().url()).searchParams
    listSearches.push(new URLSearchParams(search))
    await route.fulfill({
      json: envelope({
        items: [exportJob('801')],
        page: Number(search.get('p') ?? 1),
        page_size: Number(search.get('page_size') ?? 20),
        total: 1,
      }),
    })
  })
  return { listSearches }
}

test('A72 preserves export deep-link state and retains the list when a task is missing', async ({
  page,
}) => {
  await seedViewer(page)
  const state = await mockExports(page)
  const filters = `page=2&status=${encodedSearch(['success'])}`
  await page.goto(`/exports?exportId=${encodedId('801')}&${filters}`)

  let sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('801', { exact: true })).toBeVisible()
  await expect.poll(() => state.listSearches.at(-1)?.get('p')).toBe('2')
  expect(state.listSearches.at(-1)?.getAll('status')).toEqual(['success'])
  await page.reload()
  sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('801', { exact: true })).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()
  await expect(page).not.toHaveURL(/exportId=/)
  await expect(page).toHaveURL(/page=2/)
  await expect(
    page
      .getByRole('button', { name: '查看详情', exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await page.goBack()
  sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('801', { exact: true })).toBeVisible()
  await page.goForward()
  await expect(page.getByRole('dialog', { name: '导出任务' })).toHaveCount(0)

  await page.goto(`/exports?exportId=${encodedId('999')}&${filters}`)
  sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('无法读取导出任务，请重试。')).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()
  await expect(page).toHaveURL(/page=2/)
  await expect(page).not.toHaveURL(/exportId=/)
  await expect(
    page
      .getByRole('button', { name: '查看详情', exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
})
