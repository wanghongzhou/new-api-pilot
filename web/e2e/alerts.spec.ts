import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const siteId = '9007199254740993'
const eventId = '9007199254740997'

type TestUser = {
  display_name: string
  id: string
  must_change_password: boolean
  role: 'admin' | 'viewer'
  status: 1
  username: string
}

type RuleFixture = {
  base_rule_id: string
  compare_operator: '>='
  constraints: {
    for_times_editable: boolean
    for_times_max: number
    for_times_min: number
    paired_rule_id: string
    relation: 'warning_lt_critical'
    threshold_editable: boolean
    threshold_max: string
    threshold_min: string
    threshold_step: string
    value_kind: 'percentage'
  }
  editable_fields: string[]
  effective_rule_id: string
  enabled: boolean
  for_times: number
  id: string
  inherited: boolean
  level: 'critical' | 'warning'
  metric: string
  name: string
  override_rule_id: string | null
  rule_key: string
  scope_id: string
  scope_type: 'global' | 'site'
  threshold_value: string
  updated_at: number
}

const admin: TestUser = {
  display_name: '跨区域告警值班平台管理员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'admin',
  status: 1,
  username: 'admin',
}

const viewer: TestUser = {
  display_name: '只读告警审阅员',
  id: '9007199254740992',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'viewer',
}

const longSiteName =
  '华东生产站点用于验证超长简体中文名称在桌面和移动端均不会造成横向溢出'
const longTargetName =
  '核心推理服务实例用于验证告警对象名称和投递详情在窄屏下可以完整换行显示'

function encodedSearchId(id: string): string {
  return encodeURIComponent(id)
}

function envelope<T>(data: T, requestId = 'req_alerts_e2e') {
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
  fieldErrors: Record<string, string> | null = null
) {
  return {
    code,
    data: null,
    field_errors: fieldErrors,
    message: code,
    params: null,
    request_id: 'req_alerts_error',
    success: false,
  }
}

function ruleFixture(
  level: RuleFixture['level'],
  overrides: Partial<RuleFixture> = {}
): RuleFixture {
  const warning = level === 'warning'
  const id = warning ? '11' : '12'
  return {
    base_rule_id: id,
    compare_operator: '>=',
    constraints: {
      for_times_editable: true,
      for_times_max: 60,
      for_times_min: 1,
      paired_rule_id: warning ? '12' : '11',
      relation: 'warning_lt_critical',
      threshold_editable: true,
      threshold_max: '100',
      threshold_min: '0',
      threshold_step: '0.1',
      value_kind: 'percentage',
    },
    editable_fields: ['enabled', 'threshold_value', 'for_times'],
    effective_rule_id: id,
    enabled: true,
    for_times: 3,
    id,
    inherited: false,
    level,
    metric: 'cpu_usage_percentage',
    name: warning ? 'CPU warning' : 'CPU critical',
    override_rule_id: null,
    rule_key: 'cpu_high',
    scope_id: '0',
    scope_type: 'global',
    threshold_value: warning ? '70' : '80',
    updated_at: 1_784_000_000,
    ...overrides,
  }
}

const alertEvent = {
  current_value: '92.5000',
  first_fired_at: 1_783_997_200,
  first_observed_at: 1_783_997_140,
  id: eventId,
  last_fired_at: 1_784_000_000,
  level: 'critical',
  message: {
    code: 'ALERT_CPU_HIGH',
    params: {
      site_id: siteId,
      target_name: longTargetName,
      target_type: 'instance',
      threshold: '80',
      value: '92.5',
    },
    technical_detail: 'sample=92.5 source=resource_snapshot',
  },
  resolved_at: null,
  rule_id: '31',
  rule_key: 'cpu_high',
  site_id: siteId,
  site_name: longSiteName,
  status: 'firing',
  target_key: 'instance-primary',
  target_name: longTargetName,
  target_type: 'instance',
  threshold_value: '80.0000',
}

const alertDetail = {
  ...alertEvent,
  consecutive_count: 4,
  deliveries: [
    {
      attempt_count: 2,
      error_code: 'DELIVERY_RETRY_SCHEDULED',
      event_type: 'firing',
      id: '51',
      next_retry_at: 1_784_000_300,
      response_code: 429,
      response_message: 'rate limited; retry scheduled by the backend',
      sent_at: null,
      status: 'pending',
    },
    {
      attempt_count: 1,
      error_code: '',
      event_type: 'resolved',
      id: '52',
      next_retry_at: null,
      response_code: 200,
      response_message: '',
      sent_at: 1_784_000_100,
      status: 'success',
    },
    {
      attempt_count: 3,
      error_code: 'DINGTALK_REJECTED',
      event_type: 'firing',
      id: '53',
      next_retry_at: null,
      response_code: 200,
      response_message:
        'DingTalk rejected the payload after the bounded backend retry policy completed',
      sent_at: null,
      status: 'failed',
    },
  ],
}

async function seedAuth(page: Page, user: TestUser) {
  await page.addInitScript(() => {
    const style = document.createElement('style')
    style.textContent = `
      button[aria-label='Open TanStack Router Devtools'],
      button[aria-label='Open Tanstack query devtools'] {
        display: none !important;
      }
    `
    const appendStyle = () => document.documentElement?.append(style)
    if (document.documentElement) appendStyle()
    else
      document.addEventListener('DOMContentLoaded', appendStyle, { once: true })
  })
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
    await route.fulfill({ json: envelope(user, 'req_alerts_self') })
  })
}

type MockOptions = {
  createFailures?: number
  detailFailures?: number
  missingDetailIds?: string[]
  summaryError?: boolean
  updateFailures?: number
}

async function mockAlerts(
  page: Page,
  user: TestUser,
  options: MockOptions = {}
) {
  const state = {
    createBodies: [] as Record<string, unknown>[],
    createFailures: options.createFailures ?? 0,
    deleteCalls: 0,
    detailFailures: options.detailFailures ?? 0,
    listSearches: [] as URLSearchParams[],
    missingDetailIds: new Set(options.missingDetailIds ?? []),
    overrideActive: false,
    overrideForTimes: 3,
    overrideThreshold: '70',
    summaryError: options.summaryError ?? false,
    updateBodies: [] as Record<string, unknown>[],
    updateFailures: options.updateFailures ?? 0,
  }
  const effectiveRules = (): RuleFixture[] => {
    const warning = state.overrideActive
      ? ruleFixture('warning', {
          base_rule_id: '11',
          effective_rule_id: '31',
          for_times: state.overrideForTimes,
          id: '31',
          inherited: false,
          override_rule_id: '31',
          scope_id: siteId,
          scope_type: 'site',
          threshold_value: state.overrideThreshold,
        })
      : ruleFixture('warning', { inherited: true })
    return [warning, ruleFixture('critical', { inherited: true })]
  }

  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({
      json: envelope({
        items: [{ id: siteId, name: longSiteName }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
  await page.route('**/api/alerts/summary', async (route) => {
    assertAuthenticatedRequest(route, user)
    if (state.summaryError) {
      await route.fulfill({
        json: errorEnvelope('INTERNAL_ERROR'),
        status: 400,
      })
      return
    }
    await route.fulfill({
      json: envelope({
        critical_count: 2,
        firing_count: 3,
        resolved_today_count: 5,
        updated_at: 1_784_000_000,
        warning_count: 1,
      }),
    })
  })
  await page.route(/\/api\/alerts\/\d+(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    const id = new URL(route.request().url()).pathname.split('/').at(-1) ?? ''
    if (state.missingDetailIds.has(id)) {
      await route.fulfill({
        json: errorEnvelope('NOT_FOUND'),
        status: 404,
      })
      return
    }
    if (state.detailFailures > 0) {
      state.detailFailures -= 1
      await route.fulfill({
        json: errorEnvelope('INTERNAL_ERROR'),
        status: 400,
      })
      return
    }
    await route.fulfill({ json: envelope(alertDetail) })
  })
  await page.route(/\/api\/alerts(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    const search = new URL(route.request().url()).searchParams
    state.listSearches.push(new URLSearchParams(search))
    const empty = search.getAll('status').includes('resolved')
    const pageNumber = Number(search.get('p') ?? 1)
    await route.fulfill({
      json: envelope({
        items: empty ? [] : [alertEvent],
        page: pageNumber,
        page_size: Number(search.get('page_size') ?? 20),
        total: empty ? 0 : 41,
      }),
    })
  })
  await page.route(/\/api\/alert-rules(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    const search = new URL(route.request().url()).searchParams
    const rules =
      search.get('scope_type') === 'site'
        ? effectiveRules()
        : [ruleFixture('warning'), ruleFixture('critical')]
    await route.fulfill({ json: envelope(rules) })
  })
  await page.route('**/api/alert-rules/overrides', async (route) => {
    assertAuthenticatedRequest(route, user)
    const body = route.request().postDataJSON() as Record<string, unknown>
    state.createBodies.push(body)
    if (state.createFailures > 0) {
      state.createFailures -= 1
      await route.fulfill({
        json: errorEnvelope('CONFLICT', {
          threshold_value: 'conflicting threshold',
        }),
        status: 409,
      })
      return
    }
    state.overrideActive = true
    state.overrideForTimes = Number(body.for_times)
    state.overrideThreshold = String(body.threshold_value)
    await route.fulfill({ json: envelope(effectiveRules()[0]) })
  })
  await page.route(/\/api\/alert-rules\/\d+(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    if (route.request().method() === 'DELETE') {
      state.deleteCalls += 1
      state.overrideActive = false
      await route.fulfill({ json: envelope(null) })
      return
    }
    const body = route.request().postDataJSON() as Record<string, unknown>
    state.updateBodies.push(body)
    if (state.updateFailures > 0) {
      state.updateFailures -= 1
      await route.fulfill({
        json: errorEnvelope('INTERNAL_ERROR', { body: 'service failure' }),
        status: 500,
      })
      return
    }
    if (body.threshold_value != null) {
      state.overrideThreshold = String(body.threshold_value)
    }
    if (body.for_times != null) state.overrideForTimes = Number(body.for_times)
    await route.fulfill({ json: envelope(effectiveRules()[0]) })
  })
  return state
}

async function setup(page: Page, user: TestUser, options: MockOptions = {}) {
  await seedAuth(page, user)
  await mockSelf(page, user)
  return mockAlerts(page, user, options)
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

async function openFirstAlert(page: Page) {
  const mobileButton = page
    .getByRole('button', { name: '查看告警详情', exact: true })
    .filter({ visible: true })
    .first()
  if (await mobileButton.isVisible()) {
    await mobileButton.click()
    return
  }
  await page
    .getByRole('button', { name: 'CPU 使用率过高', exact: true })
    .click()
}

test('supports URL filters, sorting, pagination, detail retry, and all delivery states', async ({
  page,
}, testInfo) => {
  const state = await setup(page, admin, { detailFailures: 1 })
  await page.goto('/alerts')

  await expect(
    page.getByRole('heading', { level: 1, name: '告警中心' })
  ).toBeVisible()
  await expect(page.getByText('当前触发').locator('..')).toContainText('3')
  await expect(
    page.getByText(longTargetName).filter({ visible: true }).first()
  ).toBeVisible()

  const sortButton = page.getByRole('button', {
    name: '最近触发',
    exact: true,
  })
  if (await sortButton.isVisible()) {
    await sortButton.click()
  } else {
    await page.goto('/alerts?sort=last_fired_at&order=desc')
  }
  await expect(page).toHaveURL(/sort=last_fired_at/)
  await expect(page).toHaveURL(/order=desc/)
  await page
    .getByRole('button', { name: '下一页' })
    .evaluate((button: HTMLButtonElement) => button.click())
  await expect(page).toHaveURL(/page=2/)

  await page.getByRole('button', { name: '筛选', exact: true }).click()
  const filters = page.getByRole('dialog', { name: '筛选' })
  const statusFilters = filters.getByRole('group', { name: '状态' })
  await statusFilters.getByLabel('触发中', { exact: true }).check()
  await statusFilters.getByLabel('累计中', { exact: true }).check()
  const levelFilters = filters.getByRole('group', { name: '级别' })
  await levelFilters.getByLabel('严重', { exact: true }).check()
  await levelFilters.getByLabel('警告', { exact: true }).check()
  const targetFilters = filters.getByRole('group', { name: '目标类型' })
  await targetFilters.getByLabel('实例', { exact: true }).check()
  await targetFilters.getByLabel('账户', { exact: true }).check()
  await filters.locator('#alerts-filter-site').selectOption(siteId)
  await hideDeveloperOverlays(page)
  await assertNoHorizontalOverflow(page)
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('alerts-filters.png'),
  })
  await filters.getByRole('button', { name: '应用', exact: true }).click()
  await expect
    .poll(() =>
      JSON.parse(new URL(page.url()).searchParams.get('status') ?? '[]')
    )
    .toEqual(['firing', 'pending'])
  await expect
    .poll(() =>
      JSON.parse(new URL(page.url()).searchParams.get('level') ?? '[]')
    )
    .toEqual(['critical', 'warning'])
  await expect
    .poll(() =>
      JSON.parse(new URL(page.url()).searchParams.get('targetType') ?? '[]')
    )
    .toEqual(['instance', 'account'])
  expect(new URL(page.url()).searchParams.get('siteId')).toBe(siteId)
  await expect.poll(() => state.listSearches.at(-1)?.get('p')).toBe('1')
  expect(state.listSearches.at(-1)?.getAll('status')).toEqual([
    'firing',
    'pending',
  ])
  expect(state.listSearches.at(-1)?.getAll('level')).toEqual([
    'critical',
    'warning',
  ])
  expect(state.listSearches.at(-1)?.getAll('target_type')).toEqual([
    'instance',
    'account',
  ])

  await openFirstAlert(page)
  const detail = page.getByRole('dialog', { name: '告警事件详情' })
  await expect(detail.getByText('告警详情加载失败')).toBeVisible()
  await detail.getByRole('button', { name: '重试', exact: true }).click()
  await expect(detail.getByText('等待重试')).toBeVisible()
  await expect(detail.getByText('发送成功')).toBeVisible()
  await expect(detail.getByText('发送失败', { exact: true })).toBeVisible()
  await expect(detail.getByText('通知发送失败，已安排自动重试')).toBeVisible()
  await expect(detail.getByText('钉钉拒绝了本次通知')).toBeVisible()
  await expect(detail.getByText(longTargetName).first()).toBeVisible()
  await expect(
    detail.getByRole('button', { name: /投递.*重试|人工重试/ })
  ).toHaveCount(0)
  await hideDeveloperOverlays(page)
  await assertNoHorizontalOverflow(page)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('alerts-events-detail.png'),
  })
  await detail
    .getByText('钉钉投递', { exact: true })
    .evaluate((heading) => heading.scrollIntoView({ block: 'start' }))
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('alerts-deliveries.png'),
  })
})

test('isolates summary and missing-detail failures, renders empty state, and keeps Viewer rules read-only', async ({
  page,
}) => {
  await setup(page, viewer, {
    missingDetailIds: ['999'],
    summaryError: true,
  })
  await page.goto(`/alerts?alertId=${encodedSearchId('999')}`)

  const detail = page.getByRole('dialog', { name: '告警事件详情' })
  await expect(detail.getByText('告警事件不存在')).toBeVisible()
  await expect(
    detail.getByRole('button', { name: '重试', exact: true })
  ).toHaveCount(0)
  await detail.getByRole('button', { name: '关闭' }).click()
  await expect(page).not.toHaveURL(/alertId=/)
  await expect(page.getByText('告警计数暂不可用')).toBeVisible()
  await expect(
    page.getByText(longTargetName).filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('button', { name: '筛选', exact: true }).click()
  const filters = page.getByRole('dialog', { name: '筛选' })
  await filters
    .getByRole('group', { name: '状态' })
    .getByLabel('已恢复', { exact: true })
    .check()
  await filters.getByRole('button', { name: '应用', exact: true }).click()
  await expect(
    page.getByRole('heading', { name: '当前没有匹配告警' })
  ).toBeVisible()

  await page.getByRole('tab', { name: '规则', exact: true }).click()
  await page
    .getByRole('combobox', { name: '作用域', exact: true })
    .selectOption('site')
  await page
    .getByRole('combobox', { name: '站点', exact: true })
    .selectOption(siteId)
  await expect(
    page.getByText('继承全局').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: '创建站点覆盖', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '编辑规则', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '恢复全局规则', exact: true })
  ).toHaveCount(0)
  await assertNoHorizontalOverflow(page)
})

test('preserves Admin rule edits across conflict and service errors, then restores inheritance without removing history', async ({
  page,
}, testInfo) => {
  const state = await setup(page, admin, {
    createFailures: 1,
    updateFailures: 1,
  })
  await page.goto(
    `/alerts?tab=rules&scope=site&ruleSiteId=${encodedSearchId(siteId)}`
  )

  await expect(
    page.getByText('继承全局').filter({ visible: true }).first()
  ).toBeVisible()
  await page
    .getByRole('button', { name: '创建站点覆盖', exact: true })
    .first()
    .click()
  let dialog = page.getByRole('dialog', { name: '创建站点覆盖' })
  const createThreshold = dialog.getByRole('textbox', {
    name: '阈值',
    exact: true,
  })
  await createThreshold.fill('80')
  await dialog.getByRole('button', { name: '创建站点覆盖' }).click()
  await expect(dialog.getByText('警告阈值必须小于严重阈值')).toBeVisible()
  expect(state.createBodies).toHaveLength(0)

  await createThreshold.fill('72.5000')
  await dialog.getByRole('button', { name: '创建站点覆盖' }).click()
  await expect(dialog.getByText('请求的变更与当前状态冲突')).toBeVisible()
  await expect(createThreshold).toHaveValue('72.5000')
  await dialog.getByRole('button', { name: '创建站点覆盖' }).click()
  await expect(page.getByText('站点规则覆盖已创建')).toBeVisible()
  expect(state.createBodies.at(-1)).toEqual({
    base_rule_id: '11',
    enabled: true,
    for_times: 3,
    site_id: siteId,
    threshold_value: '72.5000',
  })
  await expect(
    page.getByRole('button', { name: '恢复全局规则', exact: true })
  ).toBeVisible()

  await page.getByRole('button', { name: '编辑规则', exact: true }).click()
  dialog = page.getByRole('dialog', { name: '编辑规则' })
  const updateThreshold = dialog.getByRole('textbox', {
    name: '阈值',
    exact: true,
  })
  await updateThreshold.fill('73.5000')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByText('服务器发生内部错误')).toBeVisible()
  await expect(updateThreshold).toHaveValue('73.5000')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByText('告警规则已更新')).toBeVisible()
  expect(state.updateBodies.at(-1)).toEqual({
    threshold_value: '73.5000',
  })
  await hideDeveloperOverlays(page)
  await assertNoHorizontalOverflow(page)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('alerts-admin-rules.png'),
  })

  await page.getByRole('button', { name: '恢复全局规则', exact: true }).click()
  const confirm = page.getByRole('alertdialog', {
    name: '确认恢复全局规则',
  })
  await confirm
    .getByRole('button', { name: '恢复全局规则', exact: true })
    .click()
  await expect(page.getByText('已恢复使用全局规则')).toBeVisible()
  expect(state.deleteCalls).toBe(1)
  await expect(
    page.getByText('继承全局').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('tab', { name: '事件', exact: true }).click()
  await expect(
    page.getByText(longTargetName).filter({ visible: true }).first()
  ).toBeVisible()
  await openFirstAlert(page)
  await expect(
    page
      .getByRole('dialog', { name: '告警事件详情' })
      .getByText('CPU 使用率过高')
  ).toBeVisible()
})
