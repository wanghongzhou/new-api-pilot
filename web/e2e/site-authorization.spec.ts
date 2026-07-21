import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

const admin = {
  display_name: '站点授权验收管理员',
  id: '9007199254740993',
  must_change_password: false,
  role: 'admin' as const,
  status: 1 as const,
  username: 'authorization-admin',
}

function envelope<T>(data: T, requestId = 'req_site_authorization') {
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

function siteFixture() {
  return {
    auth_status: 'authorized',
    base_url: 'https://east.example.com',
    completeness_rate: 1,
    data_export_enabled: true,
    disabled_at: null,
    health_status: 'ok',
    id: '1',
    management_status: 'active',
    name: '华东站点',
    online_status: 'online',
    performance: {
      avg_latency_ms: 120,
      avg_tps: 18.5,
      data_status: 'complete',
      hours: 24,
      models: [],
      request_count: '42',
      sampled_at: 1_783_872_000,
      success_rate: 0.998,
    },
    rate: {
      quota_per_unit: '500000',
      source: 'site',
      updated_at: 1_783_872_000,
      usd_exchange_rate: '7.3',
    },
    realtime: {
      expired: false,
      rpm: '12',
      tpm: '34',
      updated_at: 1_783_872_000,
    },
    resource: {
      cpu_max_percent: 12,
      data_status: 'complete',
      disk_max_used_percent: 20,
      instance_count: 1,
      memory_max_percent: 18,
      online_instance_count: 1,
      updated_at: 1_783_872_000,
    },
    statistics_status: 'ready',
    system_name: 'New API',
    today: {
      active_users: '1',
      as_of: 1_783_872_000,
      data_status: 'complete',
      quota: '500000',
      request_count: '42',
      token_used: '8400',
    },
    updated_at: 1_783_872_000,
    version: 'v1.2.3',
  }
}

const authorizationResult = {
  backfill_run_id: null,
  capabilities: [],
  data_export_enabled: true,
  first_user_proof: {
    earliest_created_at: 1_700_000_000,
    min_user_id: '1',
    passed: true,
    snapshot_total: 1,
  },
  flow_data_validation: 'passed' as const,
  root_created_at: 1_700_000_000,
  root_user_id: '1',
  statistics_start_at: 1_700_000_000,
  system_name: 'New API',
  version: 'v1.2.3',
}

async function seedAuthenticatedAdmin(page: Page) {
  await page.addInitScript(
    ({ authKey, authUser, uidKey }) => {
      window.localStorage.setItem(authKey, JSON.stringify(authUser))
      window.localStorage.setItem(uidKey, authUser.id)
    },
    { authKey: authStorageKey, authUser: admin, uidKey: uidStorageKey }
  )
  await page.route('**/api/user/self', async (route) => {
    expect(route.request().headers()['new-api-user']).toBe(admin.id)
    await route.fulfill({ json: envelope(admin, 'req_self') })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    expect(route.request().method()).toBe('GET')
    await route.fulfill({
      json: envelope({
        items: [siteFixture()],
        page: 1,
        page_size: 20,
        total: 1,
      }),
    })
  })
}

test('A05/A55 makes Token rotation explicit and requires an explicit retry after an uncertain result', async ({
  page,
}) => {
  await seedAuthenticatedAdmin(page)
  const requests: unknown[] = []
  await page.route('**/api/sites/1/authorize', async (route: Route) => {
    requests.push(route.request().postDataJSON())
    expect(route.request().headers()['new-api-user']).toBe(admin.id)
    if (requests.length === 1) {
      await route.fulfill({
        json: errorEnvelope('TOKEN_ROTATION_RESULT_UNKNOWN'),
        status: 502,
      })
      return
    }
    await route.fulfill({ json: envelope(authorizationResult) })
  })

  await page.goto('/sites')
  await page.getByLabel('打开站点操作').click()
  await page
    .getByRole('button', { name: '授权或重新授权', exact: true })
    .click()

  const dialog = page.getByRole('dialog', { name: '站点授权' })
  await dialog
    .getByRole('radio', { name: '登录并生成 Token', exact: true })
    .click()
  await expect(
    dialog.getByText(
      '此方式会覆盖远端原有 Access Token；请确认依赖旧 Token 的系统已做好切换准备。'
    )
  ).toBeVisible()
  await dialog.getByLabel('root 用户名').fill('root')
  await dialog.getByLabel('root 密码').fill('rotation-password')
  await dialog
    .getByRole('checkbox', { name: '我确认该操作会覆盖远端原有 Access Token' })
    .click()
  await dialog
    .getByRole('button', { name: '验证并检查能力', exact: true })
    .click()

  await expect(
    dialog.getByText('Token 变更结果不确定，请重新执行授权')
  ).toBeVisible()
  await page.waitForTimeout(300)
  expect(requests).toHaveLength(1)
  expect(requests[0]).toEqual({
    confirm_token_rotation: true,
    mode: 'login_generate_token',
    password: 'rotation-password',
    username: 'root',
  })

  await dialog
    .getByRole('button', { name: '验证并检查能力', exact: true })
    .click()
  await expect(dialog.getByText('首用户证明通过')).toBeVisible()
  expect(requests).toHaveLength(2)
})
