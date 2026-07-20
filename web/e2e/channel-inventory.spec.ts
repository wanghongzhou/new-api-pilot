import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const viewer = {
  display_name: '渠道库存只读运营员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'channel_viewer',
}

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_channel_inventory_e2e') {
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
    await route.fulfill({ json: envelope(viewer, 'req_channel_self') })
  })
}

const metric = {
  availability_rate: '99.5000000000',
  available_count: '9007199254740993',
  balance_total: '9007199254740993.1234567890',
  channel_count: '9007199254740995',
  missing_count: '1',
  response_time_avg_ms: '123.4567890000',
  response_time_max_ms: '9223372036854775807',
  unavailable_count: '1',
  used_quota: '9223372036854775807',
}

function channel(remoteState: 'normal' | 'missing', id: string) {
  return {
    auto_ban: 1,
    balance: '9007199254740993.1234567890',
    balance_updated_at: 1_784_348_700,
    first_seen_at: 1_784_262_400,
    group: 'vip',
    id,
    last_seen_at: remoteState === 'missing' ? null : 1_784_348_700,
    missing_count: remoteState === 'missing' ? 2 : 0,
    models: 'gpt-4.1,gpt-5',
    name: `渠道-${remoteState}`,
    priority: '9007199254740993',
    remote_channel_id: id,
    remote_state: remoteState,
    response_time_ms: '9223372036854775807',
    site_id: '9007199254740993',
    site_name: '华东生产站点',
    status: remoteState === 'normal' ? 1 : 3,
    tag: 'primary',
    test_time: 1_784_348_600,
    type: 8,
    used_quota: '9223372036854775807',
    weight: '9007199254740994',
  }
}

function statisticsResponse(url: URL, status = 'partial') {
  const start = Number(url.searchParams.get('start_timestamp'))
  const end = Number(url.searchParams.get('end_timestamp'))
  const breakdown = {
    ...metric,
    as_of: end - 60,
    data_status: status,
    dimension_id: '8',
    dimension_name: '8',
    site_id: '',
    site_name: '',
  }
  return {
    data_status: status,
    group_breakdown: [
      { ...breakdown, dimension_id: 'vip', dimension_name: 'vip' },
    ],
    site_breakdown: [
      {
        ...breakdown,
        dimension_id: '9007199254740993',
        dimension_name: '华东生产站点',
        site_id: '9007199254740993',
        site_name: '华东生产站点',
      },
    ],
    status_breakdown: [
      { ...breakdown, dimension_id: '1', dimension_name: '1' },
    ],
    summary: metric,
    tag_breakdown: [
      { ...breakdown, dimension_id: 'primary', dimension_name: 'primary' },
    ],
    trend: [
      {
        ...metric,
        bucket_end: start + 3600,
        bucket_start: start,
        data_status: status,
      },
    ],
    type_breakdown: [breakdown],
  }
}

test('keeps channel inventory exact, filterable, exportable, secure and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const listReads: URL[] = []
  const statsReads: URL[] = []
  let exportBody: ExportBody | undefined
  await page.route(/\/api\/channel-inventory(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    const url = new URL(route.request().url())
    listReads.push(url)
    await route.fulfill({
      json: envelope({
        as_of: 1_784_348_700,
        data_status: 'partial',
        items: [
          channel('normal', '9007199254740993'),
          channel('missing', '9007199254740994'),
        ],
        page: Number(url.searchParams.get('p') ?? 1),
        page_size: 20,
        total: 2,
      }),
    })
  })
  await page.route(
    /\/api\/channel-inventory\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      statsReads.push(url)
      await route.fulfill({ json: envelope(statisticsResponse(url)) })
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
        id: '701',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'channel_inventory',
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/701', async (route) => {
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
        id: '701',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'channel_inventory',
        status: 'pending',
      }),
    })
  })

  await page.goto('/channel-inventory')
  await expect(
    page.getByRole('heading', { name: '全局渠道库存' })
  ).toBeVisible()
  await expect(
    page
      .getByText('9007199254740993.1234567890')
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText('本轮缺失', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()

  await page.getByLabel('渠道名称或模型').fill('gpt')
  await page.getByLabel('渠道类型 ID').fill('8')
  await page.getByLabel('分组').fill('vip')
  await page.getByLabel('标签').fill('primary')
  await page.getByRole('button', { name: '已启用' }).click()
  await page.getByRole('button', { name: '本轮缺失' }).click()
  await expect
    .poll(() => listReads.at(-1)?.searchParams.get('keyword'))
    .toBe('gpt')
  await expect
    .poll(() => listReads.at(-1)?.searchParams.getAll('types'))
    .toEqual(['8'])
  await expect
    .poll(() => listReads.at(-1)?.searchParams.getAll('states'))
    .toEqual(['missing'])
  await expect
    .poll(() => statsReads.at(-1)?.searchParams.getAll('statuses'))
    .toEqual(['1'])
  await expect(page).toHaveURL(/keyword=gpt/)

  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect.poll(() => exportBody?.statistics_type).toBe('channel_inventory')
  expect(exportBody?.filters).toMatchObject({
    channel_states: ['missing'],
    channel_statuses: [1],
    channel_tags: ['primary'],
    channel_types: [8],
    keyword: 'gpt',
    site_ids: [],
    use_groups: ['vip'],
  })
  expect(exportBody?.filters).not.toHaveProperty('key')
  expect(exportBody?.filters).not.toHaveProperty('multi_key')
  const bodyText = await page.locator('body').innerText()
  expect(bodyText).not.toMatch(/sk-[a-z0-9]|authorization:|base_url/i)
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

test('uses forced site channel endpoints and preserves unavailable semantics', async ({
  page,
}, testInfo) => {
  await seedAuth(page, testInfo)
  const listReads: URL[] = []
  const statsReads: URL[] = []
  await page.route(
    /\/api\/sites\/1\/channel-inventory(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      listReads.push(new URL(route.request().url()))
      await route.fulfill({
        json: envelope({
          as_of: null,
          data_status: 'pending',
          items: [],
          page: 1,
          page_size: 20,
          total: 0,
        }),
      })
    }
  )
  await page.route(
    /\/api\/sites\/1\/channel-inventory\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      statsReads.push(url)
      await route.fulfill({
        json: envelope({
          ...statisticsResponse(url, 'unavailable'),
          group_breakdown: [],
          site_breakdown: [],
          status_breakdown: [],
          tag_breakdown: [],
          trend: [],
          type_breakdown: [],
        }),
      })
    }
  )

  await page.goto('/sites/1/channel-inventory')
  await expect(
    page.getByRole('heading', { name: '站点渠道库存' })
  ).toBeVisible()
  await expect(page.getByLabel('站点 ID')).toHaveCount(0)
  await page.getByLabel('分组').fill('vip')
  await expect
    .poll(() => listReads.at(-1)?.searchParams.getAll('groups'))
    .toEqual(['vip'])
  await expect
    .poll(() => statsReads.at(-1)?.searchParams.getAll('groups'))
    .toEqual(['vip'])
  expect(listReads.at(-1)?.searchParams.has('site_ids')).toBe(false)
  expect(statsReads.at(-1)?.searchParams.has('site_ids')).toBe(false)
  await page.reload()
  await expect(page.getByLabel('分组')).toHaveValue('vip')
  await expect(page.getByRole('button', { name: '返回站点详情' })).toBeVisible()
  const statuses = page.locator('[role="status"]')
  await expect(statuses).toHaveCount(2)
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
