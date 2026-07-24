import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

import { clickOpenSelectOption } from './helpers/select-control'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const rangeStart = 1_783_789_200
const rangeEnd = 1_783_875_600

type TestUser = {
  id: string
  username: string
  display_name: string
  role: 'admin' | 'viewer'
  status: 1 | 2
  must_change_password: boolean
}

const admin: TestUser = {
  display_name: '平台管理员',
  id: '9007199254740993',
  must_change_password: false,
  role: 'admin',
  status: 1,
  username: 'admin',
}

const viewer: TestUser = {
  display_name: '只读用户',
  id: '9007199254740994',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'viewer',
}

function envelope<T>(data: T, requestId = 'req_f3') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function errorEnvelope(
  code: string,
  requestId = 'req_f3_error',
  fieldErrors: Record<string, string | string[]> | null = null
) {
  return {
    code,
    data: null,
    field_errors: fieldErrors,
    message: code,
    request_id: requestId,
    success: false,
  }
}

const emptyBackfill = {
  completed_windows: 0,
  end_timestamp: null,
  failed_windows: 0,
  latest_error: null,
  progress: 1,
  run_id: null,
  start_timestamp: null,
  status: 'none',
  total_windows: 0,
}

function customerRecoveryRun() {
  return {
    completed_windows: 0,
    created_at: rangeEnd,
    created_request_id: 'req_customer_enable',
    deduplicated: false,
    end_timestamp: rangeEnd,
    error: null,
    failed_windows: 0,
    fetched_rows: '0',
    finished_at: null,
    id: '700',
    last_request_id: 'req_customer_enable',
    next_attempt_at: rangeEnd,
    priority: 10,
    progress: 0,
    retry_count: 0,
    site_config_version: 0,
    site_id: null,
    start_timestamp: rangeStart,
    started_at: null,
    status: 'pending',
    target_id: '7',
    target_type: 'customer',
    task_type: 'customer_rebuild',
    total_windows: 24,
    trigger_type: 'recovery',
    windows_initialized: true,
    written_rows: '0',
  }
}

const completeness = {
  complete_site_count: 1,
  complete_unit_count: 23,
  completeness_rate: 23 / 24,
  data_status: 'partial',
  expected_site_count: 1,
  expected_unit_count: 24,
  last_verified_at: 1_783_872_000,
  missing_range_total: 1,
  missing_ranges: [
    {
      end_timestamp: 1_783_792_800,
      reason: {
        code: 'DATA_WINDOW_MISSING',
        params: {
          end_timestamp: 1_783_792_800,
          site_id: '1',
          start_timestamp: 1_783_789_200,
        },
        technical_detail: '',
      },
      site_id: '1',
      start_timestamp: 1_783_789_200,
      status: 'missing',
    },
  ],
  missing_ranges_truncated: false,
  missing_site_ids: ['1'],
  unit_type: 'hour',
}

const siteBreakdown = [
  {
    data_status: 'complete',
    quota: '500000',
    quota_per_unit: '500000',
    rate_source: 'site',
    rate_updated_at: 1_783_872_000,
    site_id: '1',
    site_name: '华东站点',
    usd_exchange_rate: '7.3',
  },
]

function customerFixture(status = 'using') {
  return {
    account_count: 1,
    active_account_count: status === 'disabled' ? 0 : 1,
    archived_account_count: status === 'disabled' ? 1 : 0,
    backfill: emptyBackfill,
    completeness,
    contact: '张经理 13800000000',
    created_at: 1_700_000_000,
    id: '7',
    name: '示例客户',
    remark: '重点客户',
    site_count: 1,
    statistics_paused_at: status === 'disabled' ? 1_783_000_000 : null,
    status,
    today: {
      active_users: status === 'disabled' ? null : '1',
      as_of: 1_783_872_000,
      data_status: status === 'disabled' ? 'paused' : 'partial',
      quota: status === 'disabled' ? null : '500000',
      request_count: status === 'disabled' ? null : '42',
      site_breakdown: status === 'disabled' ? [] : siteBreakdown,
      token_used: status === 'disabled' ? null : '8400',
    },
    updated_at: 1_783_872_000,
  }
}

function accountFixture(
  remoteState: 'normal' | 'missing' | 'identity_mismatch' = 'normal',
  managedStatus: 'active' | 'archived' = 'active'
) {
  return {
    backfill: emptyBackfill,
    completeness,
    created_at: 1_700_100_000,
    customer_id: '7',
    customer_name: '示例客户',
    display_name: '生产账户',
    id: '88',
    last_remote_seen_at: 1_783_800_000,
    last_synced_at: 1_783_872_000,
    managed_status: managedStatus,
    quota: '12000000',
    rate: {
      quota_per_unit: '500000',
      source: 'site',
      updated_at: 1_783_872_000,
      usd_exchange_rate: '7.3',
    },
    remark: '生产账户',
    remote_created_at: 1_700_000_000,
    remote_group: 'vip',
    remote_missing_count: remoteState === 'missing' ? 3 : 0,
    remote_state: remoteState,
    remote_status: 1,
    remote_user_id: '9007199254740995',
    request_count: '5000',
    site_id: '1',
    site_name: '华东站点',
    statistics_paused_at: managedStatus === 'archived' ? 1_783_000_000 : null,
    today: {
      active_users: null,
      as_of: 1_783_872_000,
      data_status: remoteState === 'normal' ? 'partial' : 'paused',
      quota: remoteState === 'normal' ? '500000' : null,
      request_count: remoteState === 'normal' ? '42' : null,
      token_used: remoteState === 'normal' ? '8400' : null,
    },
    updated_at: 1_783_872_000,
    used_quota: '88000000',
    username: 'customer_prod',
  }
}

function remoteUser(id = '9007199254740995', username = 'customer_prod') {
  return {
    already_managed: false,
    created_at: 1_700_000_000,
    display_name: username === 'fast_user' ? '快速结果' : '生产账户',
    group: 'vip',
    id,
    last_login_at: null,
    managed_account_id: null,
    managed_customer_name: '',
    quota: '12000000',
    request_count: '5000',
    role: 1,
    status: 1,
    used_quota: '88000000',
    username,
  }
}

function pageData<T>(items: T[]) {
  return { items, page: 1, page_size: 20, total: items.length }
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
    await route.fulfill({ json: envelope(user, 'req_self_f3') })
  })
}

async function mockEntityReads(
  page: Page,
  user: TestUser,
  options: {
    account?: ReturnType<typeof accountFixture>
    customer?: ReturnType<typeof customerFixture>
  } = {}
) {
  const account = options.account ?? accountFixture()
  const customer = options.customer ?? customerFixture()
  await page.route(/\/api\/customers\/7\/accounts(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(pageData([account])) })
  })
  await page.route('**/api/customers/7', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(customer) })
  })
  await page.route('**/api/accounts/88', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(account) })
  })
  await page.route(/\/api\/customers(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(pageData([customer])) })
  })
  await page.route(/\/api\/accounts(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(pageData([account])) })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({
      json: envelope(
        pageData([
          {
            auth_status: 'authorized',
            data_export_enabled: true,
            id: '1',
            management_status: 'active',
            name: '华东站点',
          },
        ])
      ),
    })
  })
}

function statisticsFixture(scope: 'account' | 'customer') {
  const unavailableSite = {
    data_status: 'partial',
    quota: '500000',
    quota_per_unit: null,
    rate_source: 'unavailable',
    rate_updated_at: null,
    site_id: '2',
    site_name: '华南站点',
    usd_exchange_rate: null,
  }
  const breakdownPoint = {
    active_users: scope === 'account' ? null : '1',
    as_of: rangeStart + 1800,
    bucket_end: rangeStart + 3600,
    bucket_start: rangeStart,
    complete_site_count: 1,
    data_status: 'partial',
    expected_site_count: 2,
    is_final: false,
    quota: '1000000',
    reason: null,
    request_count: '42',
    site_breakdown: [...siteBreakdown, unavailableSite],
    token_used: '8400',
  }
  const breakdownBase = {
    ...breakdownPoint,
    completeness_rate: 0.5,
    dimension_id: scope === 'account' ? '88' : '7',
    dimension_name: scope === 'account' ? 'customer_prod' : '示例客户',
    site_id: scope === 'account' ? '1' : null,
    site_name: scope === 'account' ? '华东站点' : null,
  }
  const breakdown =
    scope === 'account'
      ? {
          ...breakdownBase,
          customer_id: '7',
          customer_name: '示例客户',
          dimension_type: 'account',
          remote_user_id: '9007199254740995',
        }
      : {
          ...breakdownBase,
          account_count: 1,
          dimension_type: 'customer',
          site_count: 2,
        }
  return {
    breakdown: { items: [breakdown], page: 1, page_size: 20, total: 1 },
    completeness: {
      ...completeness,
      expected_site_count: 2,
      missing_site_ids: ['2'],
    },
    granularity: 'hour',
    range: {
      as_of: rangeEnd - 3600,
      end_timestamp: rangeEnd,
      start_timestamp: rangeStart,
      timezone: 'Asia/Shanghai',
    },
    scope,
    site_breakdown: [...siteBreakdown, unavailableSite],
    summary: {
      active_users: scope === 'account' ? null : '1',
      data_status: 'partial',
      is_partial: true,
      quota: '1000000',
      request_count: '42',
      token_used: '8400',
    },
    trend: [
      {
        ...breakdownPoint,
        bucket_end: rangeStart + 3600,
        data_status: 'partial',
        is_final: false,
        quota: '9223372036854775806',
        site_breakdown: [
          {
            ...siteBreakdown[0],
            quota: '9223372036854775806',
          },
        ],
      },
      {
        ...breakdownPoint,
        as_of: rangeStart + 5400,
        bucket_end: rangeStart + 7200,
        bucket_start: rangeStart + 3600,
        complete_site_count: 2,
        data_status: 'complete',
        is_final: true,
        quota: '9223372036854775807',
        site_breakdown: [
          {
            ...siteBreakdown[0],
            quota: '9223372036854775807',
          },
        ],
      },
      {
        ...breakdownPoint,
        active_users: null,
        as_of: null,
        bucket_end: rangeStart + 10_800,
        bucket_start: rangeStart + 7200,
        complete_site_count: 0,
        data_status: 'missing',
        is_final: false,
        quota: null,
        reason: completeness.missing_ranges[0]?.reason ?? null,
        request_count: null,
        site_breakdown: [],
        token_used: null,
      },
    ],
  }
}

async function mockOnboardingLists(page: Page, user: TestUser) {
  await page.route(/\/api\/customers(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    const status = new URL(route.request().url()).searchParams.getAll('status')
    if (status.length > 0) expect(status).toEqual(['using'])
    await route.fulfill({ json: envelope(pageData([customerFixture()])) })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({
      json: envelope(
        pageData([
          {
            auth_status: 'authorized',
            data_export_enabled: true,
            id: '1',
            management_status: 'active',
            name: '华东站点',
          },
        ])
      ),
    })
  })
}

async function beginOnboarding(page: Page) {
  await page.goto('/accounts')
  await page.getByRole('button', { name: '添加账户' }).first().click()
  await page.locator('#account-onboarding-customer').click()
  await clickOpenSelectOption(page, '7')
  await page.getByRole('button', { name: '继续' }).click()
  await page.locator('#account-onboarding-site').click()
  await clickOpenSelectOption(page, '1')
}

async function expectReferenceEmptyTableRow(page: Page) {
  const emptyRow = page.locator(
    '[data-slot="table-body"] > [data-slot="table-row"]'
  )
  await expect(emptyRow).toHaveCount(1)
  const box = await emptyRow.boundingBox()
  expect(box).not.toBeNull()
  expect(Math.round(box?.height ?? 0)).toBe(400)
  await emptyRow.hover()
  await expect
    .poll(() =>
      emptyRow.evaluate((element) => getComputedStyle(element).backgroundColor)
    )
    .not.toBe('rgba(0, 0, 0, 0)')
}

test('uses the refreshed new-api list chrome for account and customer pages', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)

  let accounts: ReturnType<typeof accountFixture>[] = []
  await page.route(/\/api\/accounts(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(pageData(accounts)) })
  })
  await page.route(/\/api\/customers(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(pageData([])) })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({
      json: envelope(
        pageData([
          {
            auth_status: 'authorized',
            data_export_enabled: true,
            id: '1',
            management_status: 'active',
            name: '华东站点',
          },
        ])
      ),
    })
  })

  await page.goto('/accounts')
  await expectReferenceEmptyTableRow(page)
  await expect(page.getByRole('button', { name: '添加账户' })).toHaveCount(1)
  await expect(page.getByRole('button', { name: '重置' })).toHaveCount(0)

  const siteSelect = page.getByRole('combobox', { name: '所属站点' })
  await siteSelect.click()
  const listbox = page.getByRole('listbox')
  await expect(listbox).toBeVisible()
  const [triggerBox, listboxBox] = await Promise.all([
    siteSelect.boundingBox(),
    listbox.boundingBox(),
  ])
  expect(triggerBox).not.toBeNull()
  expect(listboxBox).not.toBeNull()
  if (!triggerBox || !listboxBox) throw new Error('select geometry unavailable')
  expect(listboxBox.y).toBeGreaterThanOrEqual(triggerBox.y + triggerBox.height)
  await page.keyboard.press('Escape')

  accounts = [accountFixture()]
  await page.reload()
  const usernameSort = page.getByRole('button', { name: '用户名' })
  await usernameSort.click()
  await expect(page.getByRole('menuitem', { name: '升序' })).toBeVisible()
  await expect(page.getByRole('menuitem', { name: '降序' })).toBeVisible()

  await page.goto('/customers')
  await expectReferenceEmptyTableRow(page)
  await expect(page.getByRole('button', { name: '新建客户' })).toHaveCount(1)
})

test('keeps viewer customer and account detail read-only, accessible, and responsive', async ({
  page,
}) => {
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockEntityReads(page, viewer, {
    account: accountFixture('identity_mismatch'),
  })

  await page.goto('/customers')
  await expect(page.getByRole('heading', { name: '客户管理' })).toBeVisible()
  await page.getByRole('link', { name: '示例客户' }).click()
  await expect(page.getByRole('heading', { name: '客户档案' })).toBeVisible()
  await page.getByRole('link', { name: 'customer_prod' }).click()
  await expect(page.getByText('远端身份不一致')).toBeVisible()
  await expect(page.getByLabel('打开账户操作')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '添加账户' })).toHaveCount(0)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)

  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])

  await page.setViewportSize({ height: 812, width: 375 })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
  await expect(page.getByRole('heading', { name: '固定绑定' })).toBeVisible()
})

test('localizes customer and account 400 field errors without exposing server text', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockEntityReads(page, admin)
  let customerPuts = 0
  let accountPuts = 0
  await page.route('**/api/customers/7', async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'PUT') {
      customerPuts += 1
      await route.fulfill({
        json: errorEnvelope('VALIDATION_ERROR', 'req_customer_field', {
          name: 'must be unique across all customers',
        }),
        status: 400,
      })
      return
    }
    await route.fulfill({ json: envelope(customerFixture()) })
  })
  await page.route('**/api/accounts/88', async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'PUT') {
      accountPuts += 1
      await route.fulfill({
        json: errorEnvelope('VALIDATION_ERROR', 'req_account_field', {
          remark: ['must not contain reserved control characters'],
        }),
        status: 400,
      })
      return
    }
    await route.fulfill({ json: envelope(accountFixture()) })
  })

  await page.goto('/customers')
  await page
    .getByLabel('打开客户操作')
    .filter({ visible: true })
    .first()
    .click()
  await page.getByRole('button', { name: '编辑客户' }).click()
  let dialog = page.getByRole('dialog')
  await dialog.locator('#customer-name').fill('重复客户')
  await dialog.getByRole('button', { name: '保存' }).click()
  await expect(dialog.getByText('请检查标出的字段后重试')).toBeVisible()
  await expect(
    dialog.getByText('must be unique across all customers')
  ).toHaveCount(0)
  await expect(dialog.getByRole('heading', { name: '编辑客户' })).toBeVisible()
  await expect.poll(() => customerPuts).toBe(1)
  await dialog.getByRole('button', { name: '取消' }).click()

  await page.goto('/accounts')
  await page
    .getByLabel('打开账户操作')
    .filter({ visible: true })
    .first()
    .click()
  await page.getByRole('button', { name: '编辑备注' }).click()
  dialog = page.getByRole('dialog')
  await dialog.locator('#account-remark').fill('服务端拒绝')
  await dialog.getByRole('button', { name: '保存' }).click()
  await expect(dialog.getByText('请检查标出的字段后重试')).toBeVisible()
  await expect(
    dialog.getByText('must not contain reserved control characters')
  ).toHaveCount(0)
  await expect(
    dialog.getByRole('heading', { name: '编辑账户备注' })
  ).toBeVisible()
  await expect.poll(() => accountPuts).toBe(1)
})

test('uses four onboarding steps with IME debounce, race protection, exact review, and immutable POST body', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockOnboardingLists(page, admin)
  const keywords: string[] = []
  const selected = remoteUser('9007199254740995', 'fast_user')
  await page.route(
    /\/api\/accounts\/site\/1\/remote-users(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      const keyword =
        new URL(route.request().url()).searchParams.get('keyword') ?? ''
      keywords.push(keyword)
      if (keyword === 'slow') {
        await new Promise((resolve) => setTimeout(resolve, 1_200))
        await route.fulfill({
          json: envelope(pageData([remoteUser('6', 'slow_user')])),
        })
        return
      }
      await route.fulfill({
        json: envelope(
          pageData(keyword === selected.id ? [selected] : [selected])
        ),
      })
    }
  )
  let postedBody: unknown
  await page.route(/\/api\/accounts(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'POST') {
      postedBody = route.request().postDataJSON()
      await route.fulfill({ json: envelope(accountFixture()) })
    } else {
      await route.fulfill({ json: envelope(pageData([])) })
    }
  })
  await page.route('**/api/accounts/88', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(accountFixture()) })
  })

  await beginOnboarding(page)
  const search = page.locator('#account-remote-search')
  await search.dispatchEvent('compositionstart')
  await search.fill('输入中')
  await page.waitForTimeout(600)
  expect(keywords).toEqual([])
  await search.dispatchEvent('compositionend')
  await page.waitForTimeout(300)
  expect(keywords).toEqual([])
  await expect.poll(() => keywords.includes('输入中')).toBe(true)

  await search.fill('slow')
  await expect.poll(() => keywords.includes('slow')).toBe(true)
  await search.fill('fast')
  await expect(page.getByRole('button', { name: /fast_user/ })).toBeVisible()
  await page.waitForTimeout(800)
  await expect(page.getByRole('button', { name: /slow_user/ })).toHaveCount(0)
  await page.getByRole('button', { name: /fast_user/ }).click()
  await page.getByRole('button', { name: '继续' }).click()
  await expect(
    page.getByRole('heading', { name: '提交前精确复核' })
  ).toBeVisible()
  await page.getByRole('button', { name: '继续' }).click()
  await expect(
    page.getByRole('heading', { name: '确认不可变固定绑定' })
  ).toBeVisible()
  const onboardingAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(onboardingAccessibility.violations).toEqual([])
  await page.locator('#account-onboarding-remark').fill('生产账户')
  await page.getByRole('checkbox').click()
  await page.getByRole('button', { name: '创建并开始回填' }).click()

  await expect(page).toHaveURL(/\/accounts\/88/)
  expect(postedBody).toEqual({
    customer_id: '7',
    remote_user_id: '9007199254740995',
    remark: '生产账户',
    site_id: '1',
  })
  expect(keywords.filter((keyword) => keyword === selected.id)).toHaveLength(2)
})

test('blocks account creation when the final precise review drifts', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockOnboardingLists(page, admin)
  const selected = remoteUser()
  let exactReads = 0
  let posts = 0
  await page.route(
    /\/api\/accounts\/site\/1\/remote-users(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      const keyword = new URL(route.request().url()).searchParams.get('keyword')
      if (keyword === selected.id) exactReads += 1
      const user =
        keyword === selected.id && exactReads >= 2
          ? { ...selected, group: 'standard' }
          : selected
      await route.fulfill({ json: envelope(pageData([user])) })
    }
  )
  await page.route(/\/api\/accounts(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'POST') posts += 1
    await route.fulfill({ json: envelope(pageData([])) })
  })

  await beginOnboarding(page)
  await page.locator('#account-remote-search').fill('customer_prod')
  await page.getByRole('button', { name: /customer_prod/ }).click()
  await page.getByRole('button', { name: '继续' }).click()
  await page.getByRole('button', { name: '继续' }).click()
  await page.getByRole('checkbox').click()
  await page.getByRole('button', { name: '创建并开始回填' }).click()

  await expect(
    page.getByText('远端属性在复核后发生变化，已更新快照，请重新确认。')
  ).toBeVisible()
  await expect(
    page.getByRole('heading', { name: '提交前精确复核' })
  ).toBeVisible()
  await expect(page.getByText('standard')).toBeVisible()
  expect(posts).toBe(0)
})

test('keeps authoritative statistics exact, exportable, accessible, and responsive', async ({
  page,
}) => {
  test.setTimeout(45_000)
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockEntityReads(page, viewer)
  const statsRequests: string[] = []
  type ExportRequest = {
    filters: Record<string, unknown>
    format: 'xlsx' | 'csv'
    statistics_type: 'customer' | 'account'
  }
  const exportRequests: ExportRequest[] = []
  const exportReadTimes: number[] = []
  const exportJob = (
    request: ExportRequest,
    status: 'pending' | 'running' | 'success'
  ) => {
    let progress = 100
    if (status === 'pending') progress = 0
    else if (status === 'running') progress = 50
    return {
      created_at: rangeEnd,
      data_snapshot_at: status === 'success' ? rangeEnd - 3600 : null,
      deduplicated: false,
      error: null,
      expires_at: status === 'success' ? rangeEnd + 86_400 : null,
      file_name: status === 'success' ? 'customer-statistics.csv' : '',
      file_size: status === 'success' ? '2048' : '0',
      filters: request.filters,
      finished_at: status === 'success' ? rangeEnd : null,
      format: request.format,
      id: '501',
      progress,
      row_count: status === 'success' ? '9007199254740995' : '0',
      started_at: status === 'pending' ? null : rangeEnd - 10,
      statistics_type: request.statistics_type,
      status,
    }
  }
  for (const scope of ['customer', 'account'] as const) {
    const fixture = statisticsFixture(scope)
    for (const point of fixture.trend) {
      expect(point.bucket_end - point.bucket_start).toBe(3600)
      expect(point.bucket_start).toBeGreaterThanOrEqual(rangeStart)
      expect(point.bucket_start).toBeLessThan(rangeEnd)
      expect(point.bucket_end).toBeLessThanOrEqual(rangeEnd)
    }
  }
  await page.route(/\/api\/customers\/7\/stats(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    statsRequests.push(route.request().url())
    await route.fulfill({ json: envelope(statisticsFixture('customer')) })
  })
  await page.route(/\/api\/accounts\/88\/stats(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    statsRequests.push(route.request().url())
    await route.fulfill({ json: envelope(statisticsFixture('account')) })
  })
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticatedRequest(route, viewer)
    const request = route.request().postDataJSON() as ExportRequest
    exportRequests.push(request)
    await route.fulfill({
      json: envelope(exportJob(request, 'pending')),
    })
  })
  await page.route(
    /\/api\/statistics\/exports\/501(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, viewer)
      exportReadTimes.push(Date.now())
      const request = exportRequests[0]
      if (!request) throw new Error('export request was not captured')
      await route.fulfill({
        json: envelope(
          exportJob(
            request,
            exportReadTimes.length === 1 ? 'running' : 'success'
          )
        ),
      })
    }
  )
  const search = `start=${rangeStart}&end=${rangeEnd}&granularity=hour&metric=quota&display=usd&view=table`

  await page.goto(`/customers/7/stats?${search}`)
  await expect(page.getByText('部分站点汇率不可用').first()).toBeVisible()
  await expect(
    page.getByText(
      '当前结果包含未完成或不可用的时间桶，金额与指标仅代表可用数据。'
    )
  ).toBeVisible()
  await expect(
    page.getByText('华南站点').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('quota_per_unit').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('美元兑人民币汇率').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('站点当前汇率').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page
      .getByText('汇率更新于 2026-07-13 00:00:00')
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  const completenessPanel = page
    .getByRole('heading', { name: '数据完整性' })
    .locator('xpath=ancestor::section[1]')
  await expect(
    completenessPanel.getByText('统计单元', { exact: true })
  ).toBeVisible()
  await expect(
    completenessPanel.getByText('小时', { exact: true })
  ).toBeVisible()
  await expect(completenessPanel.getByText('站点覆盖')).toBeVisible()
  await expect(
    completenessPanel.getByText('完整 1 / 预期 2 个站点')
  ).toBeVisible()
  await expect(completenessPanel.getByText('缺失站点 ID：2')).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
  const tableAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(tableAccessibility.violations).toEqual([])

  await page.getByRole('button', { name: '导出', exact: true }).click()
  const exportDialog = page.getByRole('dialog', { name: '确认导出统计' })
  await expect(exportDialog).toContainText('客户（ID 7）')
  await expect(exportDialog).toContainText(
    '2026-07-12 01:00 - 2026-07-13 01:00'
  )
  await expect(exportDialog).toContainText('时间桶 / 升序')
  await expect(exportDialog).toContainText('部分完整')
  await expect(exportDialog).toContainText('1/2')
  const createExport = exportDialog.getByRole('button', {
    name: '创建导出任务',
  })
  await expect(createExport).toBeDisabled()
  await exportDialog.getByRole('button', { name: 'CSV' }).click()
  await exportDialog
    .getByRole('checkbox', {
      name: '我确认按当前不完整范围创建导出任务',
    })
    .click()
  await createExport.click()

  await expect
    .poll(() => new URL(page.url()).searchParams.get('exportId'))
    .toBe('501')
  const exportSheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(exportSheet.getByText('等待中')).toBeVisible()
  expect(exportRequests).toHaveLength(1)
  expect(exportRequests[0]).toEqual({
    filters: {
      account_ids: [],
      channel_keys: [],
      customer_ids: ['7'],
      end_timestamp: rangeEnd,
      granularity: 'hour',
      model_names: [],
      node_names: [],
      site_ids: [],
      sort_by: 'name',
      sort_order: 'asc',
      start_timestamp: rangeStart,
      token_keys: [],
      use_groups: [],
    },
    format: 'csv',
    statistics_type: 'customer',
  })
  await expect.poll(() => exportReadTimes.length, { timeout: 5_000 }).toBe(1)
  await expect(exportSheet.getByText('生成中')).toBeVisible()
  await expect(
    exportSheet.getByRole('progressbar', { name: '导出进度' })
  ).toBeVisible()
  await expect.poll(() => exportReadTimes.length, { timeout: 7_000 }).toBe(2)
  await expect(exportSheet.getByText('已完成')).toBeVisible()
  const pollingInterval = (exportReadTimes[1] ?? 0) - (exportReadTimes[0] ?? 0)
  expect(pollingInterval).toBeGreaterThanOrEqual(1_500)
  await expect(exportSheet).toContainText('501')
  await expect(exportSheet).toContainText('9007199254740995')
  await expect(exportSheet).toContainText('2048')
  await expect(exportSheet).toContainText('2026-07-13 00:00:00')
  await expect(exportSheet).toContainText('2026-07-14 01:00:00')
  const readsAtTerminal = exportReadTimes.length
  await page.waitForTimeout(2_500)
  expect(exportReadTimes).toHaveLength(readsAtTerminal)

  await page.reload()
  const recoveredSheet = page.getByRole('dialog', { name: '导出任务' })
  await expect(recoveredSheet.getByText('已完成')).toBeVisible()
  await expect
    .poll(() => exportReadTimes.length)
    .toBeGreaterThan(readsAtTerminal)
  await expect(recoveredSheet).toContainText('9007199254740995')
  await recoveredSheet.getByRole('button', { name: '关闭' }).click()
  await expect(page).not.toHaveURL(/exportId=/)

  await page.getByLabel('金额显示').click()
  await clickOpenSelectOption(page, 'quota')
  await page.getByRole('button', { name: '图表视图' }).click()
  await expect(page.getByRole('img', { name: '统计趋势图' })).toBeVisible()
  await expect(page.getByText('部分或未最终确认的数据')).toBeVisible()
  await expect(page.getByTestId('statistics-chart-scale')).toContainText(
    '9223372036854775806'
  )
  const exactValues = page.getByTestId('statistics-chart-exact-values')
  await expect(exactValues).toContainText('9223372036854775806')
  await expect(exactValues).toContainText('9223372036854775807')
  await expect(exactValues).toContainText('原始指标值 -')
  const chart = page.getByRole('img', { name: '统计趋势图' })
  const chartCanvas = chart.locator('canvas').last()
  await expect(chartCanvas).toBeVisible()
  await chartCanvas.scrollIntoViewIfNeeded()
  const chartBox = await chartCanvas.boundingBox()
  expect(chartBox).not.toBeNull()
  if (chartBox) {
    await page.mouse.move(chartBox.x + chartBox.width / 2, chartBox.y + 30)
  }
  await expect(
    page.getByText('原始指标值', { exact: true }).filter({ visible: true })
  ).toBeVisible()
  await expect(
    page.getByText('数据状态', { exact: true }).filter({ visible: true })
  ).toBeVisible()
  await expect(
    page
      .getByText('9223372036854775807', { exact: true })
      .filter({
        visible: true,
      })
      .first()
  ).toBeVisible()
  await expect(
    page
      .getByText(
        /华东站点（ID 1）：完整，quota 9223372036854775807，quota_per_unit 500000，汇率 7\.3（站点当前汇率，2026-07-13 00:00:00），USD/
      )
      .filter({
        visible: true,
      })
      .first()
  ).toBeVisible()
  const chartAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(chartAccessibility.violations).toEqual([])

  await page.setViewportSize({ height: 844, width: 390 })
  await page.goto(`/accounts/88/stats?${search}`)
  await expect(
    page.getByRole('heading', { name: 'customer_prod 的账户统计' })
  ).toBeVisible()
  await expect(page.getByText('—').first()).toBeVisible()
  await expect(
    page.getByText('站点 / ID').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('客户 / ID').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('远端用户 ID').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('9007199254740995').filter({ visible: true }).first()
  ).toBeVisible()
  const accountCard = page.getByRole('article').filter({
    has: page.getByRole('heading', { name: 'customer_prod' }),
  })
  const eastSite = accountCard.locator('li').filter({ hasText: '华东站点' })
  await expect(eastSite).toContainText('站点当前汇率')
  await expect(eastSite).toContainText('汇率更新于 2026-07-13 00:00:00')
  await expect(eastSite.getByText('美元金额', { exact: true })).toBeVisible()
  await expect(eastSite.getByText('1.000000', { exact: true })).toBeVisible()
  await expect(eastSite.getByText('人民币金额', { exact: true })).toBeVisible()
  await expect(eastSite.getByText('7.300000', { exact: true })).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
  const mobileAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(mobileAccessibility.violations).toEqual([])
  expect(statsRequests.length).toBeGreaterThanOrEqual(3)
  for (const request of statsRequests) {
    const params = new URL(request).searchParams
    expect(params.get('start_timestamp')).toBe(String(rangeStart))
    expect(params.get('end_timestamp')).toBe(String(rangeEnd))
    expect(params.get('granularity')).toBe('hour')
    expect(params.get('p')).toBe('1')
    expect(params.get('sort_by')).toBe('bucket_start')
  }
})

test('does not present stale statistics as an error while granularity changes', async ({
  page,
}) => {
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockEntityReads(page, viewer)
  let releaseDayRequest = () => {}
  const dayRequestGate = new Promise<void>((resolve) => {
    releaseDayRequest = resolve
  })
  await page.route(/\/api\/customers\/7\/stats(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, viewer)
    const params = new URL(route.request().url()).searchParams
    const granularity = params.get('granularity') as 'hour' | 'day'
    if (granularity === 'day') {
      await dayRequestGate
    }
    const fixture = statisticsFixture('customer')
    await route.fulfill({
      json: envelope({
        ...fixture,
        granularity,
        range: {
          ...fixture.range,
          end_timestamp: Number(params.get('end_timestamp')),
          start_timestamp: Number(params.get('start_timestamp')),
        },
        summary: {
          ...fixture.summary,
          request_count: granularity === 'day' ? '84' : '42',
        },
      }),
    })
  })

  await page.goto(
    `/customers/7/stats?start=${rangeStart}&end=${rangeEnd}&granularity=hour`
  )
  await expect(page.getByRole('heading', { name: '范围汇总' })).toBeVisible()
  await expect(
    page.getByText('42', { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  await page.getByRole('button', { name: '日' }).click()
  try {
    await expect(
      page.getByText('正在加载新范围，当前继续展示上一范围的数据。')
    ).toBeVisible()
    await expect(page.getByRole('heading', { name: '范围汇总' })).toBeVisible()
    await expect(
      page.getByText('42', { exact: true }).filter({ visible: true }).first()
    ).toBeVisible()
    await expect(page.getByText('84', { exact: true })).toHaveCount(0)
    await expect(
      page.getByRole('heading', { name: '无法加载统计数据' })
    ).toHaveCount(0)
    await expect(
      page.locator('div[aria-hidden="true"].h-64.animate-pulse')
    ).toHaveCount(0)
  } finally {
    releaseDayRequest()
  }
  await expect(
    page.getByText('84', { exact: true }).filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('正在加载新范围，当前继续展示上一范围的数据。')
  ).toHaveCount(0)
})

test('allows a disabled customer to recover only through the enable run', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockEntityReads(page, admin, {
    customer: customerFixture('disabled'),
  })
  let puts = 0
  let enables = 0
  await page.route('**/api/customers/7', async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'PUT') puts += 1
    await route.fulfill({ json: envelope(customerFixture('disabled')) })
  })
  await page.route('**/api/customers/7/enable', async (route) => {
    assertAuthenticatedRequest(route, admin)
    expect(route.request().postData()).toBeNull()
    enables += 1
    await route.fulfill({ json: envelope(customerRecoveryRun()) })
  })

  await page.goto('/customers')
  await page
    .getByLabel('打开客户操作')
    .filter({ visible: true })
    .first()
    .click()
  await expect(page.getByRole('button', { name: '编辑客户' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '删除客户' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '停用客户' })).toHaveCount(0)
  await page.getByRole('button', { name: '恢复客户' }).click()
  await page.getByRole('button', { name: '恢复并补齐' }).click()
  await expect(
    page.getByRole('heading', { name: '恢复与回填进度' })
  ).toBeVisible()
  await expect(page.getByText('700', { exact: true })).toBeVisible()
  expect(puts).toBe(0)
  expect(enables).toBe(1)
})

test('enforces admin state-action bodies and redirects F3 401 responses', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockEntityReads(page, admin)
  let disabled = 0
  let archived = 0
  await page.route('**/api/customers/7/disable', async (route) => {
    assertAuthenticatedRequest(route, admin)
    expect(route.request().postData()).toBeNull()
    disabled += 1
    await route.fulfill({ json: envelope(customerFixture('disabled')) })
  })
  await page.route('**/api/accounts/88/archive', async (route) => {
    assertAuthenticatedRequest(route, admin)
    expect(route.request().postData()).toBeNull()
    archived += 1
    await route.fulfill({
      json: envelope(accountFixture('normal', 'archived')),
    })
  })

  await page.goto('/customers')
  await page.getByLabel('打开客户操作').click()
  await page.getByRole('button', { name: '停用客户' }).click()
  const confirmAccessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(confirmAccessibility.violations).toEqual([])
  await page.getByRole('button', { name: '确认停用' }).click()
  await expect.poll(() => disabled).toBe(1)

  await page.goto('/accounts')
  await page
    .getByLabel('打开账户操作')
    .filter({ visible: true })
    .first()
    .click()
  await page.getByRole('button', { name: '归档账户' }).click()
  await page.getByRole('button', { name: '确认归档' }).click()
  await expect.poll(() => archived).toBe(1)

  await page.unroute(/\/api\/customers(?:\?.*)?$/)
  await page.route(/\/api\/customers(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({
      json: errorEnvelope('AUTH_INVALID'),
      status: 401,
    })
  })
  await page.goto('/customers')
  await expect(page).toHaveURL(/\/sign-in\?redirect=/)
  await expect(page.getByRole('heading', { name: '登录' })).toBeVisible()
})
