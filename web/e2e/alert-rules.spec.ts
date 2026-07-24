import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const siteId = '1'

const admin = {
  display_name: '告警规则管理员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'admin' as const,
  status: 1 as const,
  username: 'alert-admin',
}

const viewer = {
  display_name: '告警规则只读用户',
  id: '9007199254740992',
  must_change_password: false,
  role: 'viewer' as const,
  status: 1 as const,
  username: 'alert-viewer',
}

function envelope<T>(data: T, requestId = 'req_alert_rules') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function percentageRule(level: 'critical' | 'warning', inherited = false) {
  const warning = level === 'warning'
  const id = warning ? '11' : '12'
  return {
    base_rule_id: id,
    category: 'instance',
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
    inherited,
    level,
    metric: 'cpu_usage_percentage',
    name: warning ? 'CPU warning' : 'CPU critical',
    override_rule_id: null,
    rule_key: 'cpu_high',
    scope_id: inherited ? siteId : '0',
    scope_type: inherited ? 'site' : 'global',
    threshold_value: warning ? '70' : '80',
    updated_at: 1_784_000_000,
  }
}

function booleanRule() {
  return {
    base_rule_id: '13',
    category: 'site',
    compare_operator: '==',
    constraints: {
      for_times_editable: false,
      for_times_max: 1,
      for_times_min: 1,
      paired_rule_id: null,
      relation: null,
      threshold_editable: false,
      threshold_max: null,
      threshold_min: null,
      threshold_step: null,
      value_kind: 'boolean',
    },
    editable_fields: ['enabled'],
    effective_rule_id: '13',
    enabled: true,
    for_times: 1,
    id: '13',
    inherited: false,
    level: 'warning',
    metric: 'site_online',
    name: 'Site offline',
    override_rule_id: null,
    rule_key: 'site_offline',
    scope_id: '0',
    scope_type: 'global',
    threshold_value: null,
    updated_at: 1_784_000_000,
  }
}

async function setup(page: Page, user: typeof admin | typeof viewer) {
  const updateBodies: unknown[] = []
  const createBodies: unknown[] = []
  const mutationMethods: string[] = []
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: user, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    expect(route.request().headers()['new-api-user']).toBe(user.id)
    await route.fulfill({ json: envelope(user, 'req_alert_self') })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({
        items: [{ id: siteId, name: '华东告警站点' }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
  await page.route(/\/api\/alert-rules(?:\?.*)?$/, async (route) => {
    const scope = new URL(route.request().url()).searchParams.get('scope_type')
    const inherited = scope === 'site'
    await route.fulfill({
      json: envelope({
        items: [
          percentageRule('warning', inherited),
          percentageRule('critical', inherited),
          booleanRule(),
        ],
        page: 1,
        page_size: 20,
        total: 3,
      }),
    })
  })
  await page.route('**/api/alert-rules/overrides', async (route) => {
    mutationMethods.push(route.request().method())
    createBodies.push(route.request().postDataJSON())
    await route.fulfill({ json: envelope(percentageRule('warning')) })
  })
  await page.route(/\/api\/alert-rules\/\d+$/, async (route: Route) => {
    mutationMethods.push(route.request().method())
    updateBodies.push(route.request().postDataJSON())
    await route.fulfill({ json: envelope(percentageRule('warning')) })
  })
  return { createBodies, mutationMethods, updateBodies }
}

function encodedId(id: string) {
  return encodeURIComponent(id)
}

test('A71 lets an admin submit only allowed global and site-override rule fields', async ({
  page,
}) => {
  const state = await setup(page, admin)
  await page.goto('/alerts?tab=rules&scope=global')

  await page
    .getByRole('button', { name: '编辑规则', exact: true })
    .first()
    .click()
  let dialog = page.getByRole('dialog', { name: '编辑规则' })
  const threshold = dialog.getByRole('textbox', { name: '阈值', exact: true })
  await threshold.fill('80')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(dialog.getByText('警告阈值必须小于严重阈值')).toBeVisible()
  expect(state.updateBodies).toHaveLength(0)

  await threshold.fill('72.5000')
  await dialog
    .getByRole('spinbutton', { name: '连续次数', exact: true })
    .fill('4')
  await dialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByText('告警规则已更新')).toBeVisible()
  expect(state.updateBodies).toEqual([
    { for_times: 4, threshold_value: '72.5000' },
  ])

  await page.goto(
    `/alerts?tab=rules&scope=site&ruleSiteId=${encodedId(siteId)}`
  )
  await page
    .getByRole('button', { name: '创建站点覆盖', exact: true })
    .first()
    .click()
  dialog = page.getByRole('dialog', { name: '创建站点覆盖' })
  await dialog
    .getByRole('textbox', { name: '阈值', exact: true })
    .fill('72.5000')
  await dialog.getByRole('button', { name: '创建站点覆盖' }).click()
  await expect(page.getByText('站点规则覆盖已创建')).toBeVisible()
  expect(state.createBodies).toEqual([
    {
      base_rule_id: '11',
      enabled: true,
      for_times: 3,
      site_id: siteId,
      threshold_value: '72.5000',
    },
  ])

  await page.goto('/alerts?tab=rules&scope=global')
  await page
    .getByRole('button', { name: '编辑规则', exact: true })
    .nth(2)
    .click()
  dialog = page.getByRole('dialog', { name: '编辑规则' })
  await expect(dialog.getByText('系统固定，只允许修改启用状态')).toBeVisible()
  await expect(
    dialog.getByRole('textbox', { name: '阈值', exact: true })
  ).toHaveCount(0)
})

test('A71 keeps rule mutations unavailable to a viewer', async ({ page }) => {
  const state = await setup(page, viewer)
  await page.goto('/alerts?tab=rules&scope=global')

  await expect(
    page.getByText('CPU 使用率过高').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: '编辑规则', exact: true })
  ).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '创建站点覆盖', exact: true })
  ).toHaveCount(0)
  expect(state.mutationMethods).toEqual([])
})
