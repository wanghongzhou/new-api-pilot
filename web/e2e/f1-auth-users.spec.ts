import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

type TestUser = {
  id: string
  username: string
  display_name: string
  role: 'admin' | 'viewer'
  status: 1 | 2
  must_change_password: boolean
}

type UserItem = TestUser & {
  last_login_at: number | null
  created_at: number
  updated_at: number
}

const admin: TestUser = {
  id: '9007199254740993',
  username: 'admin',
  display_name: '平台管理员',
  role: 'admin',
  status: 1,
  must_change_password: false,
}

const viewer: TestUser = {
  id: '9007199254740994',
  username: 'viewer',
  display_name: '只读用户',
  role: 'viewer',
  status: 1,
  must_change_password: false,
}

const userItems: UserItem[] = [
  {
    ...admin,
    last_login_at: 1_783_872_000,
    created_at: 1_780_000_000,
    updated_at: 1_783_872_000,
  },
  {
    ...viewer,
    last_login_at: null,
    created_at: 1_781_000_000,
    updated_at: 1_783_000_000,
  },
]

function envelope<T>(data: T, requestId = 'req_e2e') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
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

async function mockSelf(page: Page, user: TestUser, onCall?: () => void) {
  await page.route('**/api/user/self', async (route) => {
    expect(route.request().headers()['new-api-user']).toBe(user.id)
    onCall?.()
    await route.fulfill({ json: envelope(user, 'req_self') })
  })
}

async function mockUserList(
  page: Page,
  getViewerStatus: () => 1 | 2 = () => 1
) {
  await page.route(/\/api\/user\/(?:\?.*)?$/, async (route) => {
    const url = new URL(route.request().url())
    const adminCountQuery =
      url.searchParams.get('role') === 'admin' &&
      url.searchParams.get('status') === '1' &&
      url.searchParams.get('page_size') === '1'
    const items = [userItems[0], { ...userItems[1], status: getViewerStatus() }]
    await route.fulfill({
      json: envelope({
        items: adminCountQuery ? [userItems[0]] : items,
        page: 1,
        page_size: adminCountQuery ? 1 : 20,
        total: adminCountQuery ? 1 : items.length,
      }),
    })
  })
}

async function fulfillMutation(route: Route, data: unknown = null) {
  await route.fulfill({ json: envelope(data, 'req_mutation') })
}

async function mockDashboard(page: Page) {
  await page.route('**/api/dashboard/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let data: unknown[] | Record<string, unknown> = []
    if (path === '/api/dashboard/summary') {
      data = {
        active_accounts_today: '0',
        customer_count: 0,
        instance_count: 0,
        managed_account_count: 0,
        offline_site_count: 0,
        online_instance_count: 0,
        online_site_count: 0,
        realtime_as_of: null,
        realtime_complete_site_count: 0,
        realtime_data_status: 'complete',
        realtime_expected_site_count: 0,
        realtime_reason: null,
        resource_as_of: null,
        resource_complete_site_count: 0,
        resource_data_status: 'complete',
        resource_expected_site_count: 0,
        resource_reason: null,
        resource_stale_site_ids: [],
        rpm: '0',
        site_count: 0,
        stale_site_ids: [],
        tpm: '0',
        today: {
          as_of: null,
          data_status: 'complete',
          is_final: true,
          is_partial: false,
          active_users: '0',
          quota: '0',
          reason: null,
          request_count: '0',
          site_breakdown: [],
          token_used: '0',
        },
      }
    } else if (path === '/api/dashboard/health') {
      data = {
        as_of: null,
        auth_expired_site_ids: [],
        completeness: {
          complete_site_count: 0,
          complete_unit_count: 0,
          completeness_rate: 1,
          data_status: 'complete',
          expected_site_count: 0,
          expected_unit_count: 0,
          last_verified_at: null,
          missing_range_total: 0,
          missing_ranges: [],
          missing_ranges_truncated: false,
          missing_site_ids: [],
          unit_type: 'site_hour',
        },
        critical_alert_count: 0,
        firing_alert_count: 0,
        is_final: true,
        latest_alerts: [],
        reason: null,
        sites: [],
        statistics_not_ready_site_ids: [],
        warning_alert_count: 0,
        yesterday_validation_status: 'complete',
      }
    }
    await route.fulfill({ json: envelope(data, 'req_dashboard') })
  })
}

async function followAppNavigation(page: Page, name: string) {
  const mobileNavigationButton = page.getByRole('button', {
    name: '打开导航',
  })
  if (await mobileNavigationButton.isVisible()) {
    await mobileNavigationButton.click()
    await page
      .getByRole('dialog', { name: '主导航' })
      .getByRole('link', { name, exact: true })
      .click()
    return
  }

  await page.getByRole('link', { name, exact: true }).click()
}

test('signs in, enforces password change, and keeps passwords out of storage', async ({
  page,
}) => {
  await mockDashboard(page)
  await page.route('**/api/user/login', async (route) => {
    expect(route.request().headers()['new-api-user']).toBeUndefined()
    expect(route.request().postDataJSON()).toEqual({
      password: 'Bootstrap123!',
      username: 'admin',
    })
    await route.fulfill({
      json: envelope({ ...admin, must_change_password: true }, 'req_login'),
    })
  })
  await page.route('**/api/user/password', async (route) => {
    expect(route.request().headers()['new-api-user']).toBe(admin.id)
    expect(route.request().postDataJSON()).toEqual({
      new_password: 'Changed123!',
      original_password: 'Bootstrap123!',
    })
    await fulfillMutation(route)
  })

  await page.goto('/sign-in?redirect=%2Fsettings%2Fusers')
  await page.getByLabel('用户名').fill('admin')
  await page
    .getByRole('textbox', { name: '密码', exact: true })
    .fill('Bootstrap123!')
  await page.getByRole('button', { name: '登录', exact: true }).click()

  await expect(page).toHaveURL(/\/change-password$/)
  await page
    .getByRole('textbox', { name: '当前密码', exact: true })
    .fill('Bootstrap123!')
  await page
    .getByRole('textbox', { name: '新密码', exact: true })
    .fill('Changed123!')
  await page
    .getByRole('textbox', { name: '确认密码', exact: true })
    .fill('Changed123!')
  await page.getByRole('button', { name: '修改密码', exact: true }).click()

  await expect(page).toHaveURL(/\/dashboard\/?$/)
  const storedValues = await page.evaluate(() =>
    Object.values(window.localStorage).join(' ')
  )
  expect(storedValues).not.toContain('Bootstrap123!')
  expect(storedValues).not.toContain('Changed123!')
})

test('restores a deep link, verifies the session once, and shows viewer read-only UI', async ({
  page,
}) => {
  let selfCalls = 0
  await seedAuth(page, viewer)
  await mockSelf(page, viewer, () => {
    selfCalls += 1
  })
  await mockUserList(page)
  await mockDashboard(page)

  await page.goto('/settings/users?page=1')
  await expect(page.getByRole('heading', { name: '平台用户' })).toBeVisible()
  await expect(
    page
      .locator('#main-content')
      .getByText('只读用户', { exact: true })
      .filter({ visible: true })
  ).toBeVisible()
  await expect(page.getByRole('button', { name: '新建用户' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '编辑用户' })).toHaveCount(0)

  await followAppNavigation(page, '仪表盘')
  await followAppNavigation(page, '平台用户')
  await expect(page).toHaveURL(/\/settings\/users/)
  expect(selfCalls).toBe(1)

  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])
})

test('supports administrator user creation and protects the last administrator', async ({
  page,
}) => {
  let createdBody: unknown
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockUserList(page)
  await page.route(/\/api\/user\/(?:\?.*)?$/, async (route) => {
    if (route.request().method() !== 'POST') {
      await route.fallback()
      return
    }
    createdBody = route.request().postDataJSON()
    await fulfillMutation(route, {
      ...viewer,
      created_at: 1_783_872_000,
      last_login_at: null,
      must_change_password: true,
      updated_at: 1_783_872_000,
    })
  })

  await page.goto('/settings/users')
  await page.getByRole('button', { name: '新建用户' }).click()
  const createDialog = page.getByRole('dialog', { name: '新建平台用户' })
  await createDialog
    .getByRole('button', { name: '新建用户', exact: true })
    .click()
  const usernameInput = createDialog.getByRole('textbox', {
    name: '用户名',
    exact: true,
  })
  await expect(usernameInput).toHaveAttribute(
    'aria-errormessage',
    'create-username-error'
  )
  await expect(usernameInput).toHaveAttribute(
    'aria-describedby',
    /create-username-error/
  )
  const invalidFormAccessibility = await new AxeBuilder({ page })
    .include('[role="dialog"]')
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(invalidFormAccessibility.violations).toEqual([])
  await createDialog
    .getByRole('textbox', { name: '用户名', exact: true })
    .fill('operator')
  await createDialog
    .getByRole('textbox', { name: '显示名称', exact: true })
    .fill('运营人员')
  await createDialog
    .getByRole('combobox', { name: '角色', exact: true })
    .selectOption('viewer')
  await createDialog
    .getByRole('textbox', { name: '临时密码', exact: true })
    .fill('Operator123!')
  await createDialog
    .getByRole('textbox', { name: '确认密码', exact: true })
    .fill('Operator123!')
  await createDialog
    .getByRole('button', { name: '新建用户', exact: true })
    .click()

  await expect
    .poll(() => createdBody)
    .toEqual({
      display_name: '运营人员',
      password: 'Operator123!',
      role: 'viewer',
      username: 'operator',
    })
  const disableButtons = page.getByRole('button', { name: '禁用用户' })
  await expect(disableButtons.first()).toBeDisabled()
  await expect(disableButtons.nth(1)).toBeEnabled()
})

test('edits, resets, disables, and re-enables a platform user', async ({
  page,
}) => {
  let viewerStatus: 1 | 2 = 1
  let updateBody: unknown
  let resetBody: unknown
  let disableCalls = 0
  let enableCalls = 0

  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockUserList(page, () => viewerStatus)
  await page.route(`**/api/user/${viewer.id}`, async (route) => {
    expect(route.request().method()).toBe('PUT')
    updateBody = route.request().postDataJSON()
    await fulfillMutation(route, {
      ...userItems[1],
      display_name: '运营查看者',
      role: 'admin',
      username: 'viewer-ops',
    })
  })
  await page.route(`**/api/user/${viewer.id}/reset-password`, async (route) => {
    expect(route.request().method()).toBe('POST')
    resetBody = route.request().postDataJSON()
    await fulfillMutation(route)
  })
  await page.route(`**/api/user/${viewer.id}/disable`, async (route) => {
    expect(route.request().method()).toBe('POST')
    disableCalls += 1
    viewerStatus = 2
    await fulfillMutation(route)
  })
  await page.route(`**/api/user/${viewer.id}/enable`, async (route) => {
    expect(route.request().method()).toBe('POST')
    enableCalls += 1
    viewerStatus = 1
    await fulfillMutation(route)
  })

  const viewerSurface = () =>
    page
      .locator('tr, article')
      .filter({ hasText: 'viewer' })
      .filter({ visible: true })

  await page.goto('/settings/users')
  await expect(viewerSurface()).toHaveCount(1)
  await viewerSurface()
    .getByRole('button', { name: '编辑用户', exact: true })
    .click()

  const editDialog = page.getByRole('dialog', { name: '编辑平台用户' })
  await editDialog
    .getByRole('textbox', { name: '用户名', exact: true })
    .fill('viewer-ops')
  await editDialog
    .getByRole('textbox', { name: '显示名称', exact: true })
    .fill('运营查看者')
  await editDialog
    .getByRole('combobox', { name: '角色', exact: true })
    .selectOption('admin')
  await editDialog
    .getByRole('button', { name: '保存更改', exact: true })
    .click()
  await expect
    .poll(() => updateBody)
    .toEqual({
      display_name: '运营查看者',
      role: 'admin',
      username: 'viewer-ops',
    })
  await expect(editDialog).toHaveCount(0)

  await viewerSurface()
    .getByRole('button', { name: '重置密码', exact: true })
    .click()
  const resetDialog = page.getByRole('dialog', { name: '重置密码' })
  await resetDialog
    .getByRole('textbox', { name: '临时密码', exact: true })
    .fill('ResetViewer123!')
  await resetDialog
    .getByRole('textbox', { name: '确认密码', exact: true })
    .fill('ResetViewer123!')
  await resetDialog
    .getByRole('button', { name: '重置密码', exact: true })
    .click()
  await expect
    .poll(() => resetBody)
    .toEqual({
      new_password: 'ResetViewer123!',
    })
  await expect(resetDialog).toHaveCount(0)

  await viewerSurface()
    .getByRole('button', { name: '禁用用户', exact: true })
    .click()
  const disableDialog = page.getByRole('alertdialog', {
    name: '禁用平台用户',
  })
  await disableDialog
    .getByRole('button', { name: '禁用用户', exact: true })
    .click()
  await expect.poll(() => disableCalls).toBe(1)
  await expect(
    viewerSurface().getByRole('button', { name: '启用用户', exact: true })
  ).toBeVisible()

  await viewerSurface()
    .getByRole('button', { name: '启用用户', exact: true })
    .click()
  const enableDialog = page.getByRole('dialog', { name: '启用平台用户' })
  await enableDialog
    .getByRole('button', { name: '启用用户', exact: true })
    .click()
  await expect.poll(() => enableCalls).toBe(1)
  await expect(
    viewerSurface().getByRole('button', { name: '禁用用户', exact: true })
  ).toBeVisible()
})

test('keeps the current user identity in sync after editing it', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockUserList(page)
  let updateHeader: string | undefined
  await page.route(`**/api/user/${admin.id}`, async (route) => {
    updateHeader = route.request().headers()['new-api-user']
    await route.fulfill({
      json: envelope({
        ...userItems[0],
        display_name: '值班管理员',
        username: 'admin-duty',
      }),
    })
  })

  await page.goto('/settings/users')
  const adminSurface = page
    .locator('tr, article')
    .filter({ hasText: 'admin' })
    .filter({ visible: true })
  await adminSurface
    .getByRole('button', { name: '编辑用户', exact: true })
    .click()
  const dialog = page.getByRole('dialog', { name: '编辑平台用户' })
  await dialog
    .getByRole('textbox', { name: '用户名', exact: true })
    .fill('admin-duty')
  await dialog
    .getByRole('textbox', { name: '显示名称', exact: true })
    .fill('值班管理员')
  await dialog.getByRole('button', { name: '保存更改', exact: true }).click()

  expect(updateHeader).toBe(admin.id)
  const visibleUpdatedName = page
    .getByText('值班管理员', { exact: true })
    .filter({ visible: true })
  const mobileNavigation = page.getByRole('button', { name: '打开导航' })
  if (await mobileNavigation.isVisible()) await mobileNavigation.click()
  await expect(visibleUpdatedName.first()).toBeVisible()
  const stored = await page.evaluate(
    ({ authKey, uidKey }) => ({
      auth: JSON.parse(window.localStorage.getItem(authKey) ?? 'null'),
      uid: window.localStorage.getItem(uidKey),
    }),
    { authKey: authStorageKey, uidKey: uidStorageKey }
  )
  expect(stored).toMatchObject({
    auth: {
      display_name: '值班管理员',
      id: admin.id,
      username: 'admin-duty',
    },
    uid: admin.id,
  })
})

test('clears local authentication and returns to sign-in on self 401', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await page.route('**/api/user/self', async (route) => {
    expect(route.request().headers()['new-api-user']).toBe(admin.id)
    await route.fulfill({
      json: {
        code: 'AUTH_INVALID',
        data: null,
        field_errors: null,
        message: 'invalid session',
        request_id: 'req_unauthorized',
        success: false,
      },
      status: 401,
    })
  })
  let loginUserHeader: string | undefined
  await page.route('**/api/user/login', async (route) => {
    loginUserHeader = route.request().headers()['new-api-user']
    await route.fulfill({ json: envelope(viewer, 'req_login_after_401') })
  })

  await page.goto('/settings/users')
  await expect(page).toHaveURL(/\/sign-in\?redirect=/)
  const storage = await page.evaluate(
    ({ authKey, uidKey }) => ({
      auth: window.localStorage.getItem(authKey),
      uid: window.localStorage.getItem(uidKey),
    }),
    { authKey: authStorageKey, uidKey: uidStorageKey }
  )
  expect(storage).toEqual({ auth: null, uid: null })

  await page
    .getByRole('textbox', { name: '用户名', exact: true })
    .fill('viewer')
  await page
    .getByRole('textbox', { name: '密码', exact: true })
    .fill('ViewerPassword123!')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect.poll(() => loginUserHeader).toBeUndefined()
})

test('sends the user header on logout and clears it with the session', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockDashboard(page)
  let logoutUserHeader: string | undefined
  await page.route('**/api/user/logout', async (route) => {
    logoutUserHeader = route.request().headers()['new-api-user']
    await fulfillMutation(route)
  })

  await page.goto('/dashboard')
  await page.getByRole('button', { name: '退出登录', exact: true }).click()
  await expect(page).toHaveURL(/\/sign-in$/)
  expect(logoutUserHeader).toBe(admin.id)
  const storage = await page.evaluate(
    ({ authKey, uidKey }) => ({
      auth: window.localStorage.getItem(authKey),
      uid: window.localStorage.getItem(uidKey),
    }),
    { authKey: authStorageKey, uidKey: uidStorageKey }
  )
  expect(storage).toEqual({ auth: null, uid: null })
})

test('restores user filters through browser history and uses the mobile filter sheet', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockUserList(page)

  await page.goto('/settings/users')
  const mobileFilterButton = page.getByRole('button', {
    name: '筛选用户',
    exact: true,
  })
  const mobileFilters = await mobileFilterButton.isVisible()
  if (mobileFilters) {
    await mobileFilterButton.click()
    await expect(
      page.getByRole('dialog', { name: '筛选平台用户' })
    ).toBeVisible()
  }

  await page
    .getByLabel('按角色筛选', { exact: true })
    .filter({ visible: true })
    .selectOption('viewer')
  await expect(page).toHaveURL(/role=viewer/)
  await page
    .getByLabel('搜索平台用户', { exact: true })
    .filter({ visible: true })
    .fill('ops')
  const applyButton = page.getByRole('button', {
    name: mobileFilters ? '应用筛选' : '搜索',
    exact: true,
  })
  await applyButton.click()
  await expect(page).toHaveURL(/filter=ops/)
  if (mobileFilters) {
    await expect(
      page.getByRole('dialog', { name: '筛选平台用户' })
    ).toHaveCount(0)
  }

  await page.goBack()
  await expect(page).not.toHaveURL(/filter=ops/)
  await expect(page).toHaveURL(/role=viewer/)
  await page.goBack()
  await expect(page).not.toHaveURL(/role=viewer/)
  await page.goForward()
  await page.goForward()
  await expect(page).toHaveURL(/filter=ops/)
  await expect(page).toHaveURL(/role=viewer/)
})

test('uses cards without horizontal clipping at intermediate widths', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockUserList(page)

  await page.goto('/settings/users')
  for (const width of [640, 768, 1024]) {
    await page.setViewportSize({ height: 900, width })
    await expect(
      page.locator('#main-content article').filter({ visible: true }).first()
    ).toBeVisible()
    await expect(
      page.locator('#main-content table').filter({ visible: true })
    ).toHaveCount(0)
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth
      )
    ).toBe(false)
  }
})

test('keeps the 375px workspace within the viewport and exposes mobile navigation', async ({
  page,
}) => {
  await page.setViewportSize({ height: 844, width: 375 })
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockUserList(page)

  await page.goto('/settings/users')
  await expect(page.getByRole('heading', { name: '平台用户' })).toBeVisible()
  const horizontalOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth
  )
  expect(horizontalOverflow).toBe(false)
  await expect(
    page.getByRole('button', { name: '筛选用户', exact: true })
  ).toBeVisible()
  await expect(
    page.getByLabel('按角色筛选', { exact: true }).filter({ visible: true })
  ).toHaveCount(0)

  await page.getByRole('button', { name: '打开导航' }).click()
  await expect(page.getByRole('dialog', { name: '主导航' })).toBeVisible()
  await expect(page.getByRole('link', { name: '仪表盘' })).toBeVisible()
})
