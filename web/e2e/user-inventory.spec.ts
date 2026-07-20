import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const viewer = {
  display_name: '上游库存只读运营员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'inventory_viewer',
}

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_inventory_e2e') {
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
    await route.fulfill({ json: envelope(viewer, 'req_inventory_self') })
  })
}

const metric = {
  active_user_count: '9007199254740995',
  balance: '9223372036854775807',
  new_user_count: '9007199254740994',
  quota: '9223372036854775807',
  request_count: '9007199254740997',
  used_quota: '0',
  user_count: '9007199254740993',
}

function inventoryItem(
  remoteState: 'normal' | 'missing' | 'deleted' | 'identity_mismatch',
  id: string,
  accountId: string | null = null
) {
  return {
    account_id: accountId,
    balance: '9223372036854775807',
    display_name: `用户-${remoteState}`,
    first_seen_at: 1_784_176_000,
    group: 'vip',
    id,
    last_login_at: 1_784_262_300,
    last_seen_at: remoteState === 'missing' ? null : 1_784_262_300,
    missing_count: remoteState === 'missing' ? 3 : 0,
    quota: '9223372036854775807',
    remote_created_at: 1_700_000_000,
    remote_state: remoteState,
    remote_user_id: id,
    request_count: '9007199254740997',
    role: remoteState === 'normal' ? 1 : 10,
    site_id: '9007199254740993',
    site_name: '华东生产站点',
    status: remoteState === 'deleted' ? 2 : 1,
    used_quota: '0',
    username: `user_${remoteState}`,
  }
}

function statisticsResponse(url: URL, status = 'partial') {
  const start = Number(url.searchParams.get('start_timestamp'))
  const end = Number(url.searchParams.get('end_timestamp'))
  return {
    data_status: status,
    group_breakdown: [
      { ...metric, dimension_id: 'vip', dimension_name: 'vip', site_id: '' },
    ],
    role_breakdown: [
      { ...metric, dimension_id: '1', dimension_name: '1', site_id: '' },
    ],
    site_breakdown: [
      {
        ...metric,
        as_of: end - 60,
        data_status: status,
        site_id: '9007199254740993',
        site_name: '华东生产站点',
      },
    ],
    status_breakdown: [
      { ...metric, dimension_id: '1', dimension_name: '1', site_id: '' },
    ],
    summary: metric,
    trend: [
      {
        ...metric,
        bucket_end: start + 3600,
        bucket_start: start,
        data_status: status,
      },
    ],
  }
}

test('keeps upstream inventory distinct, bigint-safe, filterable, exportable and responsive', async ({
  page,
}) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  const listReads: URL[] = []
  const statsReads: URL[] = []
  let exportBody: ExportBody | undefined
  await page.route(/\/api\/user-inventory(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    const url = new URL(route.request().url())
    listReads.push(url)
    await route.fulfill({
      json: envelope({
        data_status: 'partial',
        items: [
          inventoryItem('normal', '9007199254740993', '88'),
          inventoryItem('missing', '9007199254740994'),
          inventoryItem('deleted', '9007199254740995'),
          inventoryItem('identity_mismatch', '9007199254740996'),
        ],
        page: Number(url.searchParams.get('p') ?? 1),
        page_size: 20,
        total: 41,
      }),
    })
  })
  await page.route(
    /\/api\/user-inventory\/statistics(?:\?.*)?$/,
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
        created_at: 1_784_262_400,
        data_snapshot_at: null,
        deduplicated: false,
        error: null,
        expires_at: null,
        file_name: '',
        file_size: '0',
        filters: exportBody.filters,
        finished_at: null,
        format: exportBody.format,
        id: '601',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'user_inventory',
        status: 'pending',
      }),
    })
  })
  await page.route('**/api/statistics/exports/601', async (route) => {
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
        id: '601',
        progress: 0,
        row_count: '0',
        started_at: null,
        statistics_type: 'user_inventory',
        status: 'pending',
      }),
    })
  })

  await page.goto('/user-inventory')
  await expect(
    page.getByRole('heading', { name: '全局上游用户库存' })
  ).toBeVisible()
  await expect(page.getByText('上游库存不等于已纳管账户')).toBeVisible()
  await expect(
    page
      .getByText(/9223372036854775807/)
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText('本轮缺失', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText('远端已删除', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText('身份不一致', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect(
    page.getByRole('link', { name: '打开已纳管账户' }).filter({ visible: true })
  ).toHaveAttribute('href', '/accounts/88')

  await page.getByLabel('用户名或显示名').fill('alice')
  await page.getByLabel('远端用户 ID').fill('9007199254740997')
  await page.getByLabel('分组').fill('vip')
  await page.getByRole('button', { name: '普通用户' }).click()
  await page.getByRole('button', { name: '身份不一致' }).click()
  await expect
    .poll(() => listReads.at(-1)?.searchParams.get('keyword'))
    .toBe('alice')
  await expect
    .poll(() => listReads.at(-1)?.searchParams.get('remote_user_id'))
    .toBe('9007199254740997')
  await expect
    .poll(() => listReads.at(-1)?.searchParams.getAll('states'))
    .toEqual(['identity_mismatch'])
  await expect
    .poll(() => statsReads.at(-1)?.searchParams.getAll('roles'))
    .toEqual(['1'])
  await expect(page).toHaveURL(/remoteUserId=9007199254740997/)

  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect(page.getByRole('dialog', { name: '导出任务' })).toBeVisible()
  await expect.poll(() => exportBody?.statistics_type).toBe('user_inventory')
  expect(exportBody?.filters).toMatchObject({
    inventory_roles: [1],
    inventory_states: ['identity_mismatch'],
    keyword: 'alice',
    remote_user_id: '9007199254740997',
    site_ids: [],
    use_groups: ['vip'],
  })
  expect(exportBody?.filters).not.toHaveProperty('p')
  expect(exportBody?.filters).not.toHaveProperty('page_size')
  const bodyText = await page.locator('body').innerText()
  expect(bodyText).not.toMatch(/email|oauth|password|access[_ ]?token/i)
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

test('uses forced site inventory endpoints and keeps unavailable distinct from zero inventory', async ({
  page,
}) => {
  await seedAuth(page)
  const listReads: URL[] = []
  const statsReads: URL[] = []
  await page.route(
    /\/api\/sites\/1\/user-inventory(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      listReads.push(new URL(route.request().url()))
      await route.fulfill({
        json: envelope({
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
    /\/api\/sites\/1\/user-inventory\/statistics(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      statsReads.push(url)
      await route.fulfill({
        json: envelope({
          ...statisticsResponse(url, 'unavailable'),
          group_breakdown: [],
          role_breakdown: [],
          site_breakdown: [],
          status_breakdown: [],
          trend: [],
        }),
      })
    }
  )

  await page.goto('/sites/1/user-inventory')
  await expect(
    page.getByRole('heading', { name: '站点上游用户库存' })
  ).toBeVisible()
  await expect(page.getByText('等待采集')).toBeVisible()
  await expect(page.getByText('不可用')).toBeVisible()
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
  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])
})
