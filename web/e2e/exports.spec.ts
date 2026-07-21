import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const siteId = '9007199254740993'
const rangeStart = 1_783_789_200
const rangeEnd = 1_783_875_600
const longModelName =
  '跨区域超长简体中文模型名称用于验证导出筛选和移动端任务卡片可以稳定换行显示'
const longFileName =
  '跨区域超长简体中文统计导出文件用于验证下载名称和移动端换行显示.csv'

type TestUser = {
  display_name: string
  id: string
  must_change_password: boolean
  role: 'admin' | 'viewer'
  status: 1
  username: string
}

type ExportFormat = 'csv' | 'xlsx'
type ExportStatus = 'pending' | 'running' | 'success' | 'failed' | 'expired'
type ExportRequest = {
  filters: Record<string, unknown>
  format: ExportFormat
  statistics_type: string
}

const viewer: TestUser = {
  display_name: '导出任务只读运营员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'viewer',
}

const admin: TestUser = {
  display_name: '导出任务平台管理员',
  id: '9007199254740992',
  must_change_password: false,
  role: 'admin',
  status: 1,
  username: 'admin',
}

const frozenFilters = {
  account_ids: [],
  channel_keys: [],
  customer_ids: [],
  end_timestamp: rangeEnd,
  granularity: 'hour',
  model_names: [longModelName],
  node_names: [],
  site_ids: [siteId],
  sort_by: 'quota',
  sort_order: 'desc',
  start_timestamp: rangeStart,
  token_keys: [],
  use_groups: [],
}

function envelope<T>(data: T, requestId = 'req_exports_e2e') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function errorEnvelope(code: string, exportId: string) {
  return {
    code,
    data: null,
    message: code,
    params: { export_id: exportId },
    request_id: `req_${code.toLowerCase()}`,
    success: false,
  }
}

async function followAppNavigation(page: Page, name: string) {
  const directLink = page
    .getByRole('navigation', { name: '主导航' })
    .getByRole('link', { name, exact: true })
  if (await directLink.isVisible()) {
    await directLink.click()
    return
  }
  await page.getByRole('button', { name: '打开导航' }).click()
  await page
    .getByRole('dialog', { name: '主导航' })
    .getByRole('link', { name, exact: true })
    .click()
}

function exportError(
  code: 'EXPORT_SNAPSHOT_FAILED' | 'EXPORT_WRITE_FAILED',
  id: string
) {
  return {
    code,
    params: { export_id: id },
    technical_detail:
      'safe worker diagnostic: bounded retry completed without exposing paths or credentials',
  }
}

function exportJob(
  id: string,
  status: ExportStatus,
  overrides: Record<string, unknown> = {}
) {
  const success = status === 'success'
  const terminal = success || status === 'failed' || status === 'expired'
  let progress = 0
  if (status === 'running') progress = 55
  else if (terminal) progress = 100
  return {
    created_at: rangeEnd - 120,
    data_snapshot_at: success ? rangeEnd - 60 : null,
    deduplicated: false,
    error: status === 'failed' ? exportError('EXPORT_WRITE_FAILED', id) : null,
    expires_at: success ? rangeEnd + 86_400 : null,
    file_name: success ? longFileName : '',
    file_size: success ? '9007199254740995' : '0',
    filters: frozenFilters,
    finished_at: terminal ? rangeEnd : null,
    format: 'csv' as const,
    id,
    progress,
    row_count: success ? '9007199254740993' : '0',
    started_at: status === 'pending' ? null : rangeEnd - 90,
    statistics_type: 'model',
    status,
    ...overrides,
  }
}

function statisticsResponse(url: URL) {
  const start = Number(url.searchParams.get('start_timestamp') ?? rangeStart)
  const end = Number(url.searchParams.get('end_timestamp') ?? rangeEnd)
  const breakdown = {
    active_users: '7',
    as_of: end - 60,
    bucket_end: end,
    bucket_start: start,
    completeness_rate: 1,
    data_status: 'complete',
    dimension_id: longModelName,
    dimension_name: longModelName,
    dimension_type: 'model',
    is_final: true,
    model_name: longModelName,
    quota: '9007199254740995',
    request_count: '9007199254740993',
    site_breakdown: [],
    site_id: null,
    site_name: null,
    token_used: '9007199254740994',
  }
  return {
    breakdown: { items: [breakdown], page: 4, page_size: 50, total: 101 },
    completeness: {
      complete_site_count: 1,
      complete_unit_count: 24,
      completeness_rate: 1,
      data_status: 'complete',
      expected_site_count: 1,
      expected_unit_count: 24,
      last_verified_at: end - 60,
      missing_range_total: 0,
      missing_ranges: [],
      missing_ranges_truncated: false,
      missing_site_ids: [],
      unit_type: 'hour',
    },
    granularity: 'hour',
    range: {
      as_of: end - 60,
      end_timestamp: end,
      start_timestamp: start,
      timezone: 'Asia/Shanghai',
    },
    scope: 'model',
    site_breakdown: [],
    summary: {
      active_users: '7',
      data_status: 'complete',
      is_partial: false,
      quota: '9007199254740995',
      request_count: '9007199254740993',
      token_used: '9007199254740994',
    },
    trend: [
      {
        active_users: '7',
        as_of: end - 60,
        bucket_end: start + 3600,
        bucket_start: start,
        complete_site_count: 1,
        data_status: 'complete',
        expected_site_count: 1,
        is_final: true,
        quota: '9007199254740995',
        reason: null,
        request_count: '9007199254740993',
        site_breakdown: [],
        token_used: '9007199254740994',
      },
    ],
  }
}

async function seedAuth(page: Page, user: TestUser) {
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: user, uidKey: uidStorageKey }
  )
}

function assertAuthenticatedRequest(route: Route, user: TestUser) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(user.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function mockSelf(page: Page, user: TestUser) {
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(user, 'req_exports_self') })
  })
}

async function setup(page: Page, user: TestUser) {
  await seedAuth(page, user)
  await mockSelf(page, user)
  await page.route(
    /\/api\/statistics\/options\/models(?:\?.*)?$/,
    async (route) => {
      await route.fulfill({
        json: envelope({ items: [], page: 1, page_size: 50, total: 0 }),
      })
    }
  )
}

async function hideDeveloperOverlays(page: Page) {
  await page.addStyleTag({
    content: `
      button[aria-label='Open TanStack Router Devtools'],
      button[aria-label='Open Tanstack query devtools'] {
        display: none !important;
      }
    `,
  })
}

async function assertNoHorizontalOverflow(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth
      )
    )
    .toBe(true)
}

function encodedSearchId(id: string): string {
  return encodeURIComponent(id)
}

async function openFirstVisibleTask(page: Page) {
  await page
    .getByRole('button', { name: '查看详情', exact: true })
    .filter({ visible: true })
    .first()
    .click()
}

test('creates CSV and XLSX from the complete current filters and surfaces active-task deduplication', async ({
  page,
}) => {
  await setup(page, viewer)
  const createRequests: ExportRequest[] = []
  const jobs = new Map<string, ReturnType<typeof exportJob>>()
  await page.route(/\/api\/statistics\/models(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    await route.fulfill({
      json: envelope(statisticsResponse(new URL(route.request().url()))),
    })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    await route.fulfill({
      json: envelope({
        items: [{ id: siteId, name: '华东导出验证站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticatedRequest(route, viewer)
    const request = route.request().postDataJSON() as ExportRequest
    createRequests.push(request)
    const csv = request.format === 'csv'
    const job = exportJob(csv ? '501' : '502', 'pending', {
      deduplicated: csv,
      filters: request.filters,
      format: request.format,
    })
    jobs.set(job.id, job)
    await route.fulfill({ json: envelope(job) })
  })
  await page.route(
    /\/api\/statistics\/exports\/\d+(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, viewer)
      const id = new URL(route.request().url()).pathname.split('/').at(-1) ?? ''
      await route.fulfill({
        json: envelope(jobs.get(id) ?? exportJob(id, 'pending')),
      })
    }
  )
  await page.route(/\/api\/statistics\/exports(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    await route.fulfill({
      json: envelope({
        items: [...jobs.values()],
        page: 1,
        page_size: 20,
        total: jobs.size,
      }),
    })
  })

  const siteIds = encodeURIComponent(JSON.stringify([siteId]))
  const models = encodeURIComponent(JSON.stringify([longModelName]))
  await page.goto(
    `/statistics/models?start=${rangeStart}&end=${rangeEnd}&granularity=hour&sort=quota&order=desc&page=4&pageSize=50&siteIds=${siteIds}&models=${models}`
  )
  await expect(page.getByRole('heading', { name: '模型统计' })).toBeVisible()
  await page.getByRole('button', { name: '导出', exact: true }).click()
  let dialog = page.getByRole('dialog', { name: '确认导出统计' })
  await dialog.getByRole('button', { name: 'CSV', exact: true }).click()
  await dialog.getByRole('button', { name: '创建导出任务' }).click()

  let sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('已有相同条件的活跃任务')).toBeVisible()
  await expect(page.getByText('已打开相同条件的现有导出任务')).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()

  await page.getByRole('button', { name: '导出', exact: true }).click()
  dialog = page.getByRole('dialog', { name: '确认导出统计' })
  await expect(dialog.getByRole('button', { name: 'XLSX' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await dialog.getByRole('button', { name: '创建导出任务' }).click()
  sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('等待中')).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()

  expect(createRequests).toHaveLength(2)
  expect(createRequests.map((request) => request.format)).toEqual([
    'csv',
    'xlsx',
  ])
  for (const request of createRequests) {
    expect(request.statistics_type).toBe('model')
    expect(request.filters).toEqual(frozenFilters)
    expect(request.filters).not.toHaveProperty('p')
    expect(request.filters).not.toHaveProperty('page_size')
  }

  await followAppNavigation(page, '导出任务')
  await expect(page).toHaveURL(/\/exports/)
  await expect(
    page.getByRole('heading', { name: '导出任务', exact: true })
  ).toBeVisible()
})

test('polls active jobs, preserves URL list controls, shows two failures, recreates, and downloads the completed file', async ({
  page,
}, testInfo) => {
  await setup(page, viewer)
  let listCalls = 0
  const listUrls: URL[] = []
  const recreateRequests: ExportRequest[] = []
  const firstFailure = exportJob('601', 'failed')
  const active = exportJob('602', 'pending')
  const detailJobs = new Map<string, ReturnType<typeof exportJob>>([
    [firstFailure.id, firstFailure],
    [active.id, active],
  ])
  await page.route(/\/api\/statistics\/exports(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    const url = new URL(route.request().url())
    listUrls.push(url)
    listCalls += 1
    if (listCalls >= 2) {
      detailJobs.set('602', exportJob('602', 'success'))
    }
    const requestedStatuses = url.searchParams.getAll('status')
    let items = [...detailJobs.values()].filter(
      (job) =>
        requestedStatuses.length === 0 || requestedStatuses.includes(job.status)
    )
    if (requestedStatuses.includes('failed')) items = [firstFailure]
    await route.fulfill({
      json: envelope({
        items,
        page: Number(url.searchParams.get('p') ?? 1),
        page_size: Number(url.searchParams.get('page_size') ?? 20),
        total: requestedStatuses.includes('failed') ? 41 : items.length,
      }),
    })
  })
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticatedRequest(route, viewer)
    const request = route.request().postDataJSON() as ExportRequest
    recreateRequests.push(request)
    const secondFailure = recreateRequests.length === 1
    const job = secondFailure
      ? exportJob('603', 'failed', {
          error: exportError('EXPORT_SNAPSHOT_FAILED', '603'),
        })
      : exportJob('604', 'success')
    detailJobs.set(job.id, job)
    await route.fulfill({ json: envelope(job) })
  })
  await page.route(
    /\/api\/statistics\/exports\/\d+\/download(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, viewer)
      await route.fulfill({
        body: '模型,请求数\n超长模型,9007199254740993',
        contentType: 'text/csv; charset=utf-8',
        headers: {
          'Content-Disposition': `attachment; filename="${longFileName}"`,
        },
        status: 200,
      })
    }
  )
  await page.route(
    /\/api\/statistics\/exports\/\d+(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, viewer)
      const id = new URL(route.request().url()).pathname.split('/').at(-1) ?? ''
      await route.fulfill({
        json: envelope(detailJobs.get(id) ?? exportJob(id, 'failed')),
      })
    }
  )

  await page.goto('/exports')
  await expect(
    page.getByRole('heading', { name: '导出任务', exact: true })
  ).toBeVisible()
  await expect
    .poll(() => listCalls, { timeout: 5_000 })
    .toBeGreaterThanOrEqual(2)
  await expect(
    page.getByText('已完成').filter({ visible: true }).first()
  ).toBeVisible()

  const statusFilters = page.getByRole('group', { name: '状态' })
  await statusFilters
    .getByRole('checkbox', { name: '失败', exact: true })
    .click()
  await statusFilters
    .getByRole('checkbox', { name: '已过期', exact: true })
    .click()
  await expect
    .poll(() =>
      JSON.parse(new URL(page.url()).searchParams.get('status') ?? '[]')
    )
    .toEqual(['failed', 'expired'])
  await expect
    .poll(() => listUrls.at(-1)?.searchParams.getAll('status'))
    .toEqual(['failed', 'expired'])
  const fileSizeSort = page.getByRole('button', {
    name: '文件字节数',
    exact: true,
  })
  if (await fileSizeSort.isVisible()) {
    await fileSizeSort.click()
  } else {
    await page.goto('/exports?status=failed&sort=file_size&order=asc')
  }
  await expect(page).toHaveURL(/sort=file_size/)
  await expect(page).toHaveURL(/order=asc/)
  await page
    .getByRole('button', { name: '下一页' })
    .evaluate((button: HTMLButtonElement) => button.click())
  await expect(page).toHaveURL(/page=2/)
  await expect.poll(() => listUrls.at(-1)?.searchParams.get('p')).toBe('2')
  await hideDeveloperOverlays(page)
  await assertNoHorizontalOverflow(page)
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('exports-list.png'),
  })

  await openFirstVisibleTask(page)
  const sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('无法写入导出文件')).toBeVisible()
  await sheet.getByRole('button', { name: '按相同条件重新导出' }).click()
  await expect(sheet.getByText('无法创建导出数据快照')).toBeVisible()
  await sheet.getByRole('button', { name: '按相同条件重新导出' }).click()
  await expect(sheet.getByText('已完成')).toBeVisible()
  await expect(sheet).toContainText(longFileName)
  expect(recreateRequests).toHaveLength(2)
  expect(recreateRequests[0]).toEqual(recreateRequests[1])
  const downloadPromise = page.waitForEvent('download')
  await sheet.getByRole('button', { name: '下载文件' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toBe(longFileName)
  await hideDeveloperOverlays(page)
  await assertNoHorizontalOverflow(page)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('exports-success-detail.png'),
  })
  await expect(
    sheet.getByRole('button', { name: /取消任务|取消导出/ })
  ).toHaveCount(0)
})

test('keeps Admin exports owner-scoped and safely handles expired and missing 410 downloads', async ({
  page,
}, testInfo) => {
  await setup(page, admin)
  const jobs = new Map<string, ReturnType<typeof exportJob>>([
    ['801', exportJob('801', 'success')],
    ['802', exportJob('802', 'success', { file_name: 'missing-file.csv' })],
  ])
  const listUrls: URL[] = []
  await page.route(/\/api\/statistics\/exports(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    const url = new URL(route.request().url())
    listUrls.push(url)
    await route.fulfill({
      json: envelope({
        items: [...jobs.values()],
        page: 1,
        page_size: 20,
        total: jobs.size,
      }),
    })
  })
  await page.route(
    /\/api\/statistics\/exports\/\d+\/download(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      const id = new URL(route.request().url()).pathname.split('/').at(-2) ?? ''
      const expired = id === '801'
      jobs.set(
        id,
        expired
          ? exportJob(id, 'expired')
          : exportJob(id, 'failed', {
              error: {
                code: 'EXPORT_FILE_MISSING',
                params: { export_id: id },
                technical_detail: '',
              },
            })
      )
      await route.fulfill({
        json: errorEnvelope(
          expired ? 'EXPORT_EXPIRED' : 'EXPORT_FILE_MISSING',
          id
        ),
        status: 410,
      })
    }
  )
  await page.route(
    /\/api\/statistics\/exports\/\d+(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      const id = new URL(route.request().url()).pathname.split('/').at(-1) ?? ''
      await route.fulfill({
        json: envelope(jobs.get(id) ?? exportJob(id, 'failed')),
      })
    }
  )

  await page.goto(`/exports?exportId=${encodedSearchId('801')}`)
  let sheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(sheet.getByText('已完成')).toBeVisible()
  await sheet.getByRole('button', { name: '下载文件' }).click()
  await expect(sheet.getByText('导出文件已过期')).toHaveCount(1)
  await expect(
    sheet.getByRole('button', { name: '按相同条件重新导出' })
  ).toBeVisible()
  await sheet.getByRole('button', { name: '关闭' }).click()

  await page
    .getByRole('button', { name: '查看详情', exact: true })
    .filter({ visible: true })
    .last()
    .click()
  sheet = page.getByRole('dialog', { name: '导出任务' })
  await sheet.getByRole('button', { name: '下载文件' }).click()
  await expect(sheet.getByText('导出文件已不可用')).toHaveCount(1)
  await expect(
    sheet.getByRole('button', { name: '按相同条件重新导出' })
  ).toBeVisible()
  await expect(
    sheet.getByRole('button', { name: /取消任务|取消导出/ })
  ).toHaveCount(0)
  expect(listUrls.length).toBeGreaterThan(0)
  for (const url of listUrls) {
    expect(url.searchParams.has('user_id')).toBe(false)
    expect(url.searchParams.has('owner_id')).toBe(false)
  }
  await expect(page.getByText('其他用户任务')).toHaveCount(0)
  await hideDeveloperOverlays(page)
  await assertNoHorizontalOverflow(page)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('exports-download-410.png'),
  })
})
