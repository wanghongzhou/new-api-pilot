import { expect, test, type Page, type Route } from '@playwright/test'

import { mockAuthenticatedShell } from './helpers/auth'

const viewer = {
  display_name: 'Resource viewer',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'resource_viewer',
}

function envelope(data: unknown) {
  return {
    code: '',
    data,
    message: '',
    request_id: 'req_resource_pagination',
    success: true,
  }
}

async function fulfill(route: Route, data: unknown) {
  await route.fulfill({ json: envelope(data) })
}

async function seed(page: Page) {
  await mockAuthenticatedShell(page)
  await page.addInitScript((user) => {
    window.localStorage.setItem('pilot-auth-user', JSON.stringify(user))
    window.localStorage.setItem('uid', user.id)
  }, viewer)
  await page.route('**/api/user/self', (route) => fulfill(route, viewer))
  await page.route(/\/api\/sites(?:\?.*)?$/, (route) =>
    fulfill(route, {
      items: [{ id: '1', name: 'Site One' }],
      page: 1,
      page_size: 100,
      total: 1,
    })
  )

  const userMetric = {
    active_user_count: '0',
    balance: '0',
    new_user_count: '0',
    quota: '0',
    request_count: '0',
    used_quota: '0',
    user_count: '0',
  }
  await page.route(/\/api\/user-inventory\/statistics(?:\?.*)?$/, (route) =>
    fulfill(route, {
      data_status: 'complete',
      group_breakdown: [],
      role_breakdown: [],
      site_breakdown: [],
      status_breakdown: [],
      summary: userMetric,
      trend: [],
    })
  )
  await page.route(/\/api\/user-inventory(?:\?.*)?$/, (route) =>
    fulfill(route, pageResponse(route))
  )

  const channelMetric = {
    availability_rate: '0',
    available_count: '0',
    balance_total: '0',
    channel_count: '0',
    missing_count: '0',
    response_time_avg_ms: '0',
    response_time_max_ms: '0',
    unavailable_count: '0',
    used_quota: '0',
  }
  await page.route(/\/api\/channel-inventory\/statistics(?:\?.*)?$/, (route) =>
    fulfill(route, {
      data_status: 'complete',
      group_breakdown: [],
      site_breakdown: [],
      status_breakdown: [],
      summary: channelMetric,
      tag_breakdown: [],
      trend: [],
      type_breakdown: [],
    })
  )
  await page.route(/\/api\/channel-inventory(?:\?.*)?$/, (route) =>
    fulfill(route, pageResponse(route))
  )

  await page.route(/\/api\/model-catalog\/coverage(?:\?.*)?$/, (route) =>
    fulfill(route, {
      catalog_models: '0',
      channel_mappings: '0',
      data_status: 'complete',
      exact_covered_models: '0',
      exact_missing_models: '0',
      site_breakdown: [],
      status_breakdown: [],
      vendor_breakdown: [],
    })
  )
  await page.route(/\/api\/model-catalog(?:\?.*)?$/, (route) =>
    fulfill(route, pageResponse(route))
  )

  await page.route(/\/api\/pricing-catalog\/statistics(?:\?.*)?$/, (route) =>
    fulfill(route, {
      data_status: 'complete',
      group_active: '0',
      group_missing: '0',
      pricing_active: '0',
      pricing_missing: '0',
      site_count: '1',
      sites: [],
    })
  )
  await page.route(
    /\/api\/(?:pricing-catalog|group-catalog)(?:\?.*)?$/,
    (route) => fulfill(route, pageResponse(route))
  )

  await page.route(/\/api\/subscription-plans\/statistics(?:\?.*)?$/, (route) =>
    fulfill(route, {
      data_status: 'complete',
      disabled: '0',
      enabled: '0',
      missing: '0',
      site_breakdown: [],
      total: '0',
    })
  )
  await page.route(/\/api\/subscription-plans(?:\?.*)?$/, (route) =>
    fulfill(route, pageResponse(route))
  )
}

function pageResponse(route: Route) {
  const url = new URL(route.request().url())
  return {
    as_of: 1_784_348_700,
    data_status: 'complete',
    items: [],
    page: Number(url.searchParams.get('p') ?? '1'),
    page_size: 20,
    total: 41,
  }
}

const userItem = {
  account_id: null,
  balance: '70',
  display_name: 'Production display name with a long suffix',
  first_seen_at: 1_784_176_000,
  group: 'vip-production',
  id: '1',
  last_login_at: 1_784_262_300,
  last_seen_at: 1_784_262_300,
  missing_count: 0,
  quota: '100',
  remote_created_at: 1_700_000_000,
  remote_state: 'normal',
  remote_user_id: '9007199254740993',
  request_count: '12',
  role: 1,
  site_id: '1',
  site_name: 'Site One',
  status: 1,
  used_quota: '30',
  username: 'production_user_with_a_long_name',
}

const channelItem = {
  auto_ban: 1,
  balance: '88.5',
  balance_updated_at: 1_784_262_300,
  first_seen_at: 1_784_176_000,
  group: 'default',
  id: '1',
  last_seen_at: 1_784_262_300,
  missing_count: 0,
  models: 'gpt-production-model-with-a-long-name',
  name: 'Production channel',
  priority: '10',
  remote_channel_id: '9007199254740994',
  remote_state: 'normal',
  response_time_ms: '123',
  site_id: '1',
  site_name: 'Site One',
  status: 1,
  tag: 'primary-production',
  test_time: 1_784_262_300,
  type: 1,
  used_quota: '55',
  weight: '100',
}

for (const path of [
  '/user-inventory',
  '/channel-inventory',
  '/model-catalog',
  '/pricing-groups',
  '/subscription-plans',
] as const) {
  test(`${path} replaces an out-of-range page with the last valid page`, async ({
    page,
  }) => {
    await seed(page)
    await page.goto(`${path}?page=999`)
    await expect(page).toHaveURL(new RegExp(`${path}\\?page=3(?:&|$)`))
  })
}

test('keeps retained user rows visible and warns after a refresh failure', async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium-desktop')
  test.setTimeout(45_000)
  await seed(page)
  let reads = 0
  await page.route(/\/api\/user-inventory(?:\?.*)?$/, async (route) => {
    reads += 1
    if (reads === 1) {
      await fulfill(route, {
        ...pageResponse(route),
        items: [userItem],
        total: 1,
      })
      return
    }
    await route.fulfill({ status: 500, json: envelope(null) })
  })
  await page.goto('/user-inventory')
  await expect(
    page.getByRole('table').getByText(userItem.username)
  ).toBeVisible()
  const keyword = page.getByLabel('用户名或显示名')
  await keyword.fill('refresh failure')
  await keyword.press('Tab')
  await expect.poll(() => reads).toBe(2)
  await expect(page.getByRole('alert')).toContainText('数据刷新失败', {
    timeout: 15_000,
  })
  await expect(
    page.getByRole('table').getByText(userItem.username)
  ).toBeVisible()
})

test('keeps retained channel rows visible and warns after a refresh failure', async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium-desktop')
  test.setTimeout(45_000)
  await seed(page)
  let reads = 0
  await page.route(/\/api\/channel-inventory(?:\?.*)?$/, async (route) => {
    reads += 1
    if (reads === 1) {
      await fulfill(route, {
        ...pageResponse(route),
        items: [channelItem],
        total: 1,
      })
      return
    }
    await route.fulfill({ status: 500, json: envelope(null) })
  })
  await page.goto('/channel-inventory')
  await expect(
    page.getByRole('table').getByText(channelItem.name)
  ).toBeVisible()
  const keyword = page.getByLabel('渠道名称或模型')
  await keyword.fill('refresh failure')
  await keyword.press('Tab')
  await expect.poll(() => reads).toBe(2)
  await expect(page.getByRole('alert')).toContainText('数据刷新失败', {
    timeout: 15_000,
  })
  await expect(
    page.getByRole('table').getByText(channelItem.name)
  ).toBeVisible()
})

test('debounces keyword navigation into one query and one history entry', async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium-desktop')
  await seed(page)
  let reads = 0
  await page.route(/\/api\/user-inventory(?:\?.*)?$/, async (route) => {
    reads += 1
    await fulfill(route, pageResponse(route))
  })
  await page.goto('/user-inventory')
  await expect.poll(() => reads).toBe(1)
  const initialHistoryLength = await page.evaluate(() => history.length)
  const input = page.getByLabel('用户名或显示名')
  await input.fill('p')
  await page.waitForTimeout(100)
  await input.fill('pr')
  await page.waitForTimeout(100)
  await input.fill('prod')
  await expect(page).toHaveURL(/keyword=prod/)
  await expect.poll(() => reads).toBe(2)
  await expect
    .poll(() => page.evaluate(() => history.length))
    .toBe(initialHistoryLength + 1)
})

test('shows critical user fields on a 375px mobile card without overflow', async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== 'chromium-mobile')
  await page.setViewportSize({ width: 375, height: 812 })
  await seed(page)
  await page.route(/\/api\/user-inventory(?:\?.*)?$/, (route) =>
    fulfill(route, { ...pageResponse(route), items: [userItem], total: 1 })
  )
  await page.goto('/user-inventory')
  const card = page.getByRole('article')
  await expect(card.getByText(userItem.display_name)).toBeVisible()
  await expect(card.getByText('额度', { exact: true })).toBeVisible()
  await expect(card.getByText('已用额度', { exact: true })).toBeVisible()
  await expect(card.getByText('最近活动', { exact: true })).toBeVisible()
  await expect(card.getByText(/远端创建：/)).toBeVisible()
  await expect(card.getByText(/首次发现：/)).toBeVisible()
  await expect(card.getByText(/连续缺失：0 次/)).toBeVisible()
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth)
  ).toBeLessThanOrEqual(375)
})
