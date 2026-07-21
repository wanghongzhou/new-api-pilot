import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'

const siteCapabilityKeys = [
  'status_contract',
  'self_identity',
  'root_identity',
  'first_user_proof',
  'user_pagination',
  'channel_pagination',
  'data_export_enabled',
  'flow_contract',
  'data_contract',
  'flow_data_consistency',
  'instance_contract',
  'realtime_contract',
] as const

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

function envelope<T>(data: T, requestId = 'req_f2') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function errorEnvelope(code: string, requestId = 'req_f2_error') {
  return {
    code,
    data: null,
    field_errors: null,
    message: code,
    request_id: requestId,
    success: false,
  }
}

function siteFixture(id = '1') {
  return {
    auth_status: 'authorized',
    backfill: {
      completed_windows: 0,
      end_timestamp: null,
      failed_windows: 0,
      latest_error: null,
      progress: 0,
      run_id: null,
      start_timestamp: null,
      status: 'none',
      total_windows: 0,
    },
    base_url: `https://site-${id}.example.com`,
    completeness: {
      complete_site_count: 1,
      complete_unit_count: 24,
      completeness_rate: 1,
      data_status: 'complete',
      expected_site_count: 1,
      expected_unit_count: 24,
      last_verified_at: 1_783_872_000,
      missing_range_total: 0,
      missing_ranges: [],
      missing_ranges_truncated: false,
      missing_site_ids: [],
      unit_type: 'site_hour',
    },
    completeness_rate: 1,
    config_version: 3,
    data_export_enabled: true,
    disabled_at: null as number | null,
    health_status: 'ok',
    id,
    last_probe_at: 1_783_872_000,
    last_probe_success_at: 1_783_872_000,
    management_status: 'active',
    monitoring_start_at: 1_780_000_000,
    name: id === '1' ? '华东站点' : '新建站点',
    online_status: 'online',
    performance: {
      avg_latency_ms: 120,
      avg_tps: 18.5,
      data_status: 'complete',
      hours: 24,
      models: [],
      request_count: '128340',
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
      rpm: '210',
      tpm: '84000',
      updated_at: 1_783_872_000,
    },
    remark: '主运营站点',
    resource: {
      cpu_max_percent: 47.2,
      data_status: 'complete',
      disk_max_used_percent: 71,
      instance_count: 2,
      memory_max_percent: 63.1,
      online_instance_count: 2,
      updated_at: 1_783_872_000,
    },
    root_created_at: 1_700_000_000,
    root_user_id: '1',
    statistics_end_at: null as number | null,
    statistics_start_at: 1_699_999_200,
    statistics_start_source: 'root_created_at',
    statistics_status: 'ready',
    system_name: 'New API',
    today: {
      active_users: '512',
      as_of: 1_783_872_000,
      data_status: 'complete',
      is_final: false,
      quota: '64000000',
      request_count: '128340',
      token_used: '84000000',
    },
    updated_at: 1_783_872_000,
    version: 'v1.2.3',
  }
}

const instancesFixture = [
  {
    cpu_percent: 42.1,
    current_status: 'online',
    data_status: 'complete',
    disk_total_bytes: '107374182400',
    disk_used_bytes: '53687091200',
    disk_used_percent: 50,
    effective_stale_after_seconds: 180,
    first_seen_at: 1_780_000_000,
    goarch: 'amd64',
    goos: 'linux',
    hostname: 'site-primary',
    is_master: true,
    last_seen_at: 1_783_872_000,
    last_synced_at: 1_783_872_000,
    memory_percent: 61.2,
    node_name: 'primary',
    runtime_version: 'go1.25.1',
    sampled_at: 1_783_872_000,
    site_id: '1',
    started_at: 1_780_000_000,
    upstream_stale_after_seconds: 180,
    upstream_status: 'online',
  },
  {
    cpu_percent: null,
    current_status: 'stale',
    data_status: 'missing',
    disk_total_bytes: null,
    disk_used_bytes: null,
    disk_used_percent: null,
    effective_stale_after_seconds: 180,
    first_seen_at: 1_781_000_000,
    goarch: 'amd64',
    goos: 'linux',
    hostname: 'site-worker',
    is_master: false,
    last_seen_at: 1_783_860_000,
    last_synced_at: 1_783_872_000,
    memory_percent: null,
    node_name: 'worker-1',
    runtime_version: 'go1.25.1',
    sampled_at: null,
    site_id: '1',
    started_at: 1_781_000_000,
    upstream_stale_after_seconds: 180,
    upstream_status: 'stale',
  },
]

function minuteRetentionSettingsFixture() {
  return [
    {
      h15_slo_eligible: true,
      h15_slo_reason_codes: [],
      items: [
        {
          configured: true,
          constraints: {},
          decrypt_error: false,
          key: 'collector.minute_retention_days',
          masked_value: '',
          read_only: false,
          secret: false,
          updated_at: 1_783_872_000,
          value: 90,
          value_type: 'int',
        },
      ],
      key: 'collector',
      label_key: 'settings.groups.collector',
    },
  ]
}

function runFixture(
  status: 'failed' | 'pending' | 'running' | 'success',
  siteId = '1',
  id = '10'
) {
  const terminal = status === 'failed' || status === 'success'
  return {
    completed_windows: terminal ? 1 : 0,
    created_at: 1_783_872_000,
    created_request_id: 'req_created',
    deduplicated: false,
    end_timestamp: 1_783_872_000,
    error:
      status === 'failed'
        ? {
            code: 'DATA_WINDOW_MISSING',
            params: {
              end_timestamp: 1_783_872_000,
              site_id: siteId,
              start_timestamp: 1_783_868_400,
            },
            technical_detail: 'upstream timeout',
          }
        : null,
    failed_windows: status === 'failed' ? 1 : 0,
    fetched_rows: '120',
    finished_at: terminal ? 1_783_872_010 : null,
    id,
    last_request_id: 'req_run_last',
    next_attempt_at: null,
    priority: 10,
    progress: terminal ? 1 : 0.5,
    retry_count: status === 'failed' ? 3 : 0,
    site_config_version: 3,
    site_id: siteId,
    start_timestamp: 1_783_868_400,
    started_at: 1_783_872_000,
    status,
    target_id: siteId,
    target_type: 'site',
    task_type: 'usage_backfill',
    total_windows: 1,
    trigger_type: 'manual',
    windows_initialized: true,
    written_rows: '100',
  }
}

function windowFixture(status: 'failed' | 'pending' | 'running' | 'success') {
  return {
    attempt_count: status === 'failed' ? 3 : 1,
    error:
      status === 'failed'
        ? {
            code: 'DATA_WINDOW_MISSING',
            params: {
              end_timestamp: 1_783_872_000,
              site_id: '1',
              start_timestamp: 1_783_868_400,
            },
            technical_detail: 'window timeout',
          }
        : null,
    fact_status: status === 'success' ? 'complete' : 'missing',
    fetched_rows: '120',
    finished_at:
      status === 'failed' || status === 'success' ? 1_783_872_010 : null,
    hour_ts: 1_783_868_400,
    id: '100',
    next_retry_at: null,
    run_id: '10',
    site_id: '1',
    started_at: 1_783_872_000,
    status,
    updated_at: 1_783_872_010,
    verified_at: status === 'success' ? 1_783_872_020 : null,
    written_rows: '100',
  }
}

type CapabilityKey = (typeof siteCapabilityKeys)[number]
type CapabilityStatus = 'failed' | 'passed' | 'skipped'

function capabilityMessage(
  key: CapabilityKey,
  siteId: string,
  status: CapabilityStatus
) {
  const params = { capability_key: key, site_id: siteId }
  if (status === 'passed') {
    return { code: 'CAPABILITY_OK', params, technical_detail: '' }
  }
  if (status === 'skipped') {
    return {
      code: 'CAPABILITY_NO_TRAFFIC_SKIPPED',
      params,
      technical_detail: '',
    }
  }
  if (key === 'data_export_enabled') {
    return {
      code: 'CAPABILITY_EXPORT_DISABLED',
      params,
      technical_detail: '',
    }
  }
  return {
    code: 'CAPABILITY_RESPONSE_INVALID',
    params,
    technical_detail: '',
  }
}

function capabilityResults(
  siteId: string,
  overrides: Partial<Record<CapabilityKey, CapabilityStatus>> = {}
) {
  return siteCapabilityKeys.map((key) => {
    const status = overrides[key] ?? 'passed'
    return { key, message: capabilityMessage(key, siteId, status), status }
  })
}

const authorizationResult = {
  backfill_run_id: '10',
  capabilities: capabilityResults('2'),
  data_export_enabled: true,
  first_user_proof: {
    earliest_created_at: 1_700_000_000,
    min_user_id: '1',
    passed: true,
    snapshot_total: 10,
  },
  flow_data_validation: 'passed',
  root_created_at: 1_700_000_000,
  root_user_id: '1',
  statistics_start_at: 1_699_999_200,
  system_name: 'New API',
  version: 'v1.2.3',
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

async function installHistoryAudit(page: Page) {
  await page.addInitScript(() => {
    const entries: Array<{ state: unknown; url: string }> = [
      { state: window.history.state, url: window.location.href },
    ]
    Object.defineProperty(window, '__PILOT_HISTORY_AUDIT__', {
      configurable: true,
      value: entries,
    })
    const pushState = window.history.pushState.bind(window.history)
    const replaceState = window.history.replaceState.bind(window.history)
    window.history.pushState = (state, unused, url) => {
      entries.push({ state, url: String(url ?? window.location.href) })
      return pushState(state, unused, url)
    }
    window.history.replaceState = (state, unused, url) => {
      entries.push({ state, url: String(url ?? window.location.href) })
      return replaceState(state, unused, url)
    }
  })
}

function assertAuthenticatedRequest(route: Route, user: TestUser) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(user.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function mockSelf(page: Page, user: TestUser) {
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(user, 'req_self_f2') })
  })
}

async function mockSiteReads(
  page: Page,
  user: TestUser,
  id = '1',
  getSite = () => siteFixture(id)
) {
  await page.route(`**/api/sites/${id}/instances`, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(instancesFixture) })
  })
  await page.route(
    new RegExp(`/api/sites/${id}/performance(?:\\?.*)?$`),
    async (route) => {
      assertAuthenticatedRequest(route, user)
      await route.fulfill({
        json: envelope({
          avg_latency_ms: 0,
          avg_tps: 0,
          data_status: 'missing',
          hours: 24,
          models: [],
          request_count: '0',
          sampled_at: null,
          success_rate: 0,
        }),
      })
    }
  )
  await page.route(
    new RegExp(`/api/sites/${id}/stats(?:\\?.*)?$`),
    async (route) => {
      assertAuthenticatedRequest(route, user)
      const url = new URL(route.request().url())
      const start = Number(url.searchParams.get('start_timestamp'))
      const end = Number(url.searchParams.get('end_timestamp'))
      const granularity = url.searchParams.get('granularity') ?? 'hour'
      await route.fulfill({
        json: envelope({
          breakdown: { items: [], page: 1, page_size: 20, total: 0 },
          completeness: {
            complete_site_count: 1,
            complete_unit_count: 0,
            completeness_rate: 1,
            data_status: 'complete',
            expected_site_count: 1,
            expected_unit_count: 0,
            last_verified_at: end,
            missing_range_total: 0,
            missing_ranges: [],
            missing_ranges_truncated: false,
            missing_site_ids: [],
            unit_type: 'site_hour',
          },
          granularity,
          range: {
            as_of: end,
            end_timestamp: end,
            start_timestamp: start,
            timezone: 'Asia/Shanghai',
          },
          scope: 'site',
          site_breakdown: [],
          summary: {
            active_users: '0',
            data_status: 'complete',
            is_partial: false,
            quota: '0',
            request_count: '0',
            token_used: '0',
          },
          trend: [],
        }),
      })
    }
  )
  await page.route(/\/api\/fast-tasks(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({
      json: envelope({
        has_more: false,
        items: [],
        limit: 50,
        offset: 0,
        total: 0,
      }),
    })
  })
  await page.route(
    new RegExp(`/api/sites/${id}/status(?:\\?.*)?$`),
    async (route) => {
      assertAuthenticatedRequest(route, user)
      await route.fulfill({
        json: envelope({
          granularity: 'minute',
          node_name: null,
          site_id: id,
          summary: {
            bucket_end: 1_783_872_000,
            bucket_start: 1_783_868_400,
            cpu_avg_percent: 30,
            cpu_max_percent: 47.2,
            data_status: 'complete',
            disk_last_used_percent: 69,
            disk_max_used_percent: 71,
            expected_sample_count: 60,
            health_status: 'ok',
            instance_count: 2,
            memory_avg_percent: 48,
            memory_max_percent: 63.1,
            online_instance_count: 2,
            sample_count: 60,
          },
          trend: [],
        }),
      })
    }
  )
  await page.route(
    new RegExp(`/api/sites/${id}/collection-runs(?:\\?.*)?$`),
    async (route) => {
      assertAuthenticatedRequest(route, user)
      await route.fulfill({
        json: envelope({
          items: [],
          page: 1,
          page_size: 20,
          total: 0,
        }),
      })
    }
  )
  await page.route(`**/api/sites/${id}`, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(getSite()) })
  })
}

async function mockOnboardingEndpoints(
  page: Page,
  user: TestUser,
  authorization: unknown
) {
  await mockSiteReads(page, user, '2')
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    if (route.request().method() === 'POST') {
      await route.fulfill({ json: envelope(siteFixture('2'), 'req_create') })
      return
    }
    await route.fulfill({
      json: envelope({ items: [], page: 1, page_size: 20, total: 0 }),
    })
  })
  await page.route('**/api/sites/2/base-url-preflight', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({
      json: envelope({
        candidate_public: {
          base_url: 'https://site-2.example.com',
          system_name: 'New API',
          version: 'v1.2.3',
        },
        change_type: 'none',
        contract_status: 'compatible',
        expires_at: 1_800_000_000,
        normalized_base_url: 'https://site-2.example.com',
        old_public: {
          base_url: 'https://site-2.example.com',
          system_name: 'New API',
          version: 'v1.2.3',
        },
        preflight_token: 'preflight-token',
      }),
    })
  })
  await page.route('**/api/sites/2/authorize', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope(authorization, 'req_authorize') })
  })
}

async function reachOnboardingBackfillStep(page: Page) {
  await page.goto('/sites')
  await page
    .getByRole('button', { name: '新建站点', exact: true })
    .first()
    .click()
  const drawer = page.getByRole('dialog', { name: '新建站点' })
  await drawer.getByLabel('站点名称').fill('新建站点')
  await drawer.getByLabel('API 地址').fill('https://site-2.example.com')
  await drawer
    .getByRole('button', { name: '创建并公开预检', exact: true })
    .click()
  await expect(drawer.getByText('公开预检通过')).toBeVisible()
  await drawer.getByLabel('root 用户 ID').fill('1')
  await drawer.getByLabel('Access Token').fill('existing-token')
  await drawer
    .getByRole('button', { name: '验证并检查能力', exact: true })
    .click()
  await expect(drawer.getByText('首用户证明通过')).toBeVisible()
  await drawer.getByRole('checkbox', { name: /我确认历史起点/ }).click()
  await drawer.getByRole('button', { name: '继续', exact: true }).click()
  return drawer
}

test('completes four-step onboarding without persisting site secrets', async ({
  page,
}) => {
  const secrets = [
    'site-token-secret',
    'root-password-secret',
    'preflight-secret',
  ]
  const consoleMessages: string[] = []
  const pageErrors: string[] = []
  const requests: Array<{
    headers: Record<string, string>
    method: string
    postData: string | null
    url: string
  }> = []
  page.on('console', (message) => consoleMessages.push(message.text()))
  page.on('pageerror', (error) => pageErrors.push(error.message))
  page.on('request', (request) =>
    requests.push({
      headers: request.headers(),
      method: request.method(),
      postData: request.postData(),
      url: request.url(),
    })
  )
  await installHistoryAudit(page)
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockSiteReads(page, admin, '2')
  let createBody: unknown
  let preflightBody: unknown
  const authorizeBodies: unknown[] = []

  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'POST') {
      createBody = route.request().postDataJSON()
      await route.fulfill({ json: envelope(siteFixture('2'), 'req_create') })
      return
    }
    await route.fulfill({
      json: envelope({
        items: [siteFixture('1')],
        page: 1,
        page_size: 20,
        total: 1,
      }),
    })
  })
  await page.route('**/api/sites/2/base-url-preflight', async (route) => {
    assertAuthenticatedRequest(route, admin)
    preflightBody = route.request().postDataJSON()
    expect(JSON.stringify(route.request().headers())).not.toContain(
      'site-token-secret'
    )
    await route.fulfill({
      json: envelope({
        candidate_public: {
          base_url: 'https://site-2.example.com',
          system_name: 'New API',
          version: 'v1.2.3',
        },
        change_type: 'none',
        contract_status: 'compatible',
        expires_at: 1_800_000_000,
        normalized_base_url: 'https://site-2.example.com',
        old_public: {
          base_url: 'https://site-2.example.com',
          system_name: 'New API',
          version: 'v1.2.3',
        },
        preflight_token: 'preflight-secret',
      }),
    })
  })
  await page.route('**/api/sites/2/authorize', async (route) => {
    assertAuthenticatedRequest(route, admin)
    authorizeBodies.push(route.request().postDataJSON())
    if (authorizeBodies.length === 1) {
      await route.fulfill({
        json: errorEnvelope('SITE_INCOMPATIBLE', 'req_authorize_rejected'),
        status: 422,
      })
      return
    }
    await route.fulfill({
      json: envelope(authorizationResult, 'req_authorize'),
    })
  })
  await page.route('**/api/collection-runs/10', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(runFixture('running', '2')) })
  })
  await page.route(
    /\/api\/collection-runs\/10\/windows(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      await route.fulfill({
        json: envelope({ items: [], page: 1, page_size: 20, total: 0 }),
      })
    }
  )

  await page.goto('/sites')
  await page.getByRole('button', { name: '新建站点', exact: true }).click()
  const drawer = page.getByRole('dialog', { name: '新建站点' })
  await drawer.getByLabel('站点名称').fill('新建站点')
  await drawer.getByLabel('API 地址').fill('https://site-2.example.com/')
  await drawer.getByLabel('备注').fill('接入验证')
  await drawer
    .getByRole('button', { name: '创建并公开预检', exact: true })
    .click()

  await expect(drawer.getByText('公开预检通过')).toBeVisible()
  await drawer.getByLabel('root 用户 ID').fill('1')
  await expect(drawer.getByLabel('Access Token')).toHaveAttribute(
    'type',
    'password'
  )
  await drawer.getByLabel('Access Token').fill('site-token-secret')
  await drawer
    .getByRole('button', { name: '验证并检查能力', exact: true })
    .click()
  await expect(drawer.getByText('站点身份或 API 不兼容')).toBeVisible()
  await drawer
    .getByRole('radio', { name: '登录并生成 Token', exact: true })
    .click()
  await drawer.getByLabel('root 用户名').fill('root')
  await expect(drawer.getByLabel('root 密码')).toHaveAttribute(
    'type',
    'password'
  )
  await drawer.getByLabel('root 密码').fill('root-password-secret')
  await drawer
    .getByRole('checkbox', { name: /覆盖远端原有 Access Token/ })
    .click()
  await drawer
    .getByRole('button', { name: '验证并检查能力', exact: true })
    .click()
  await expect(drawer.getByText('首用户证明通过')).toBeVisible()
  await drawer.getByRole('checkbox', { name: /我确认历史起点/ }).click()
  await drawer.getByRole('button', { name: '继续', exact: true }).click()
  await expect(drawer.getByText('回填任务 ID')).toBeVisible()
  await expect(drawer.getByText('10', { exact: true })).toBeVisible()
  await drawer
    .getByRole('button', { name: '进入站点详情', exact: true })
    .click()

  await expect(page).toHaveURL(/\/sites\/2\?.*runId=10(?:&|$)/)
  await expect(
    page.getByRole('dialog', { name: '任务 10 的执行窗口' })
  ).toBeVisible()
  expect(createBody).toEqual({
    base_url: 'https://site-2.example.com',
    name: '新建站点',
    remark: '接入验证',
  })
  expect(preflightBody).toEqual({ base_url: 'https://site-2.example.com' })
  expect(authorizeBodies).toEqual([
    {
      access_token: 'site-token-secret',
      mode: 'existing_token',
      root_user_id: '1',
    },
    {
      confirm_token_rotation: true,
      mode: 'login_generate_token',
      password: 'root-password-secret',
      username: 'root',
    },
  ])

  const browserAudit = await page.evaluate(() => {
    const auditWindow = window as typeof window & {
      __PILOT_CACHE_SNAPSHOT__?: () => unknown
      __PILOT_HISTORY_AUDIT__?: unknown[]
    }
    return {
      cache: auditWindow.__PILOT_CACHE_SNAPSHOT__?.(),
      currentHistoryState: window.history.state,
      history: auditWindow.__PILOT_HISTORY_AUDIT__,
      inputValues: [...document.querySelectorAll('input')].map(
        (input) => input.value
      ),
      localStorage: Object.fromEntries(Object.entries(window.localStorage)),
      sessionStorage: Object.fromEntries(Object.entries(window.sessionStorage)),
      url: window.location.href,
    }
  })
  const nonBodyRequestData = requests.map((request) => ({
    headers: request.headers,
    method: request.method,
    url: request.url,
  }))
  const getRequests = requests.filter((request) => request.method === 'GET')
  for (const secret of secrets) {
    expect(JSON.stringify(browserAudit)).not.toContain(secret)
    expect(JSON.stringify(nonBodyRequestData)).not.toContain(secret)
    expect(JSON.stringify(getRequests)).not.toContain(secret)
    expect(consoleMessages.join('\n')).not.toContain(secret)
    expect(pageErrors.join('\n')).not.toContain(secret)
  }
  expect(browserAudit.cache).toBeDefined()
  expect(pageErrors).toEqual([])
})

test('claims no backfill only when all required capabilities are ready', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockOnboardingEndpoints(page, admin, {
    ...authorizationResult,
    backfill_run_id: null,
    capabilities: capabilityResults('2', {
      flow_data_consistency: 'skipped',
    }),
  })

  const drawer = await reachOnboardingBackfillStep(page)
  await expect(
    drawer.getByText(
      '所有必需能力检查均已通过，当前没有预期历史窗口，无需回填。'
    )
  ).toBeVisible()
  await expect(
    drawer.getByRole('button', { name: '使用已保存凭据重新检查能力' })
  ).toHaveCount(0)
  await drawer
    .getByRole('button', { name: '进入站点详情', exact: true })
    .click()
  await expect(page).toHaveURL(/\/sites\/2$/)
})

test('keeps null failed capabilities unresolved and rechecks into a run', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockOnboardingEndpoints(page, admin, {
    ...authorizationResult,
    backfill_run_id: null,
    capabilities: capabilityResults('2', { data_export_enabled: 'failed' }),
  })
  let recheckCalls = 0
  await page.route('**/api/sites/2/recheck-capabilities', async (route) => {
    assertAuthenticatedRequest(route, admin)
    recheckCalls += 1
    const result =
      recheckCalls === 1
        ? {
            ...authorizationResult,
            backfill_run_id: null,
            capabilities: capabilityResults('2', {
              instance_contract: 'failed',
            }),
          }
        : authorizationResult
    await route.fulfill({
      json: envelope(result, `req_recheck_${recheckCalls}`),
    })
  })
  await page.route('**/api/collection-runs/10', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(runFixture('running', '2')) })
  })

  const drawer = await reachOnboardingBackfillStep(page)
  await expect(drawer.getByText('统计配置未就绪')).toBeVisible()
  await expect(drawer.getByText(/无需回填/)).toHaveCount(0)
  await expect(
    drawer.getByRole('button', { name: '进入站点详情继续修复' })
  ).toBeVisible()

  const recheck = drawer.getByRole('button', {
    name: '使用已保存凭据重新检查能力',
  })
  await recheck.click()
  await expect.poll(() => recheckCalls).toBe(1)
  await expect(drawer.getByText('能力检查错误')).toBeVisible()
  await expect(drawer.getByText('实例接口契约')).toBeVisible()
  await expect(drawer.getByText(/无需回填/)).toHaveCount(0)

  await recheck.click()
  await expect.poll(() => recheckCalls).toBe(2)
  await expect(drawer.getByText('回填任务 ID')).toBeVisible()
  await expect(drawer.getByText('10', { exact: true })).toBeVisible()
  await expect(
    drawer.getByRole('button', { name: '进入站点详情', exact: true })
  ).toBeVisible()
})

test('shows export capability failures without rotating credentials', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockSiteReads(page, admin)
  const failures = [['SITE_EXPORT_DISABLED', '请先在站点开启数据导出']] as const
  let failureIndex = 0
  await page.route('**/api/sites/1/recheck-capabilities', async (route) => {
    assertAuthenticatedRequest(route, admin)
    expect(route.request().postData()).toBeNull()
    await route.fulfill({
      json: errorEnvelope(failures[failureIndex][0]),
      status: 422,
    })
    failureIndex += 1
  })

  await page.goto('/sites/1')
  for (const [, message] of failures) {
    await page.getByLabel('打开站点操作').click()
    await page.getByRole('button', { name: '重新检查能力' }).click()
    const dialog = page.getByRole('dialog', { name: '重新检查站点能力' })
    await dialog.getByRole('button', { name: '开始重新检查' }).click()
    await expect(dialog.getByText(message)).toBeVisible()
    await dialog.getByRole('button', { name: '关闭' }).click()
  }
  expect(failureIndex).toBe(1)
})

test('keeps viewer detail read-only, accessible, and within responsive viewports', async ({
  page,
}) => {
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockSiteReads(page, viewer)

  await page.goto('/sites/1')
  await expect(page.getByRole('heading', { name: '华东站点' })).toBeVisible()
  await expect(page.getByText('已验证的 root 注册时间')).toBeVisible()
  await expect(page.getByLabel('打开站点操作')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '新建站点' })).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '使用已保存凭据重新检查能力' })
  ).toHaveCount(0)
  await expect(page.getByRole('button', { name: '重新检查能力' })).toHaveCount(
    0
  )
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)

  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])

  await page.setViewportSize({ height: 844, width: 375 })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
  await expect(page.getByRole('heading', { name: '当前概览' })).toBeVisible()
})

test('preflights edits and enforces lifecycle mutation boundaries', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  let currentSite = siteFixture('1')
  await mockSiteReads(page, admin, '1', () => currentSite)
  let detailGetCalls = 0
  let updateBody: unknown
  let disableCalls = 0

  await page.route('**/api/sites/1/base-url-preflight', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({
      json: envelope({
        candidate_public: {
          base_url: 'https://moved.example.com',
          system_name: 'New API',
          version: 'v1.2.3',
        },
        change_type: 'origin',
        contract_status: 'compatible',
        expires_at: 1_800_000_000,
        normalized_base_url: 'https://moved.example.com',
        old_public: {
          base_url: currentSite.base_url,
          system_name: 'New API',
          version: 'v1.2.3',
        },
        preflight_token: 'edit-preflight-token',
      }),
    })
  })
  await page.route('**/api/sites/1/disable', async (route) => {
    assertAuthenticatedRequest(route, admin)
    disableCalls += 1
    currentSite = {
      ...currentSite,
      disabled_at: 1_783_872_000,
      management_status: 'disabled',
      statistics_status: 'paused',
    }
    await route.fulfill({ json: envelope(currentSite) })
  })
  await page.route('**/api/sites/1', async (route) => {
    assertAuthenticatedRequest(route, admin)
    if (route.request().method() === 'PUT') {
      updateBody = route.request().postDataJSON()
      currentSite = {
        ...currentSite,
        auth_status: 'unauthorized',
        base_url: 'https://moved.example.com',
      }
      await route.fulfill({ json: envelope(currentSite) })
      return
    }
    if (route.request().method() === 'DELETE') {
      await route.fulfill({
        json: errorEnvelope('DELETE_RESTRICTED', 'req_delete_restricted'),
        status: 409,
      })
      return
    }
    detailGetCalls += 1
    await route.fulfill({ json: envelope(currentSite) })
  })

  await page.goto('/sites/1')
  await page.getByLabel('打开站点操作').click()
  await page.getByRole('button', { name: '编辑站点' }).click()
  const editDialog = page.getByRole('dialog', { name: '编辑站点' })
  await editDialog.getByLabel('API 地址').fill('https://moved.example.com')
  await editDialog.getByRole('button', { name: '运行公开预检' }).click()
  await expect(editDialog.getByText('origin 已变化')).toBeVisible()
  await expect(
    editDialog.getByRole('columnheader', { name: '当前地址' })
  ).toBeVisible()
  await expect(
    editDialog.getByRole('columnheader', { name: '候选地址' })
  ).toBeVisible()
  await expect(editDialog.getByText('https://site-1.example.com')).toBeVisible()
  await expect(editDialog.getByText('https://moved.example.com')).toBeVisible()
  await editDialog
    .getByRole('checkbox', { name: /确认它们代表同一逻辑站点/ })
    .click()
  await editDialog.getByRole('button', { name: '保存', exact: true }).click()
  await expect(editDialog).toHaveCount(0)
  expect(updateBody).toEqual({
    base_url: 'https://moved.example.com',
    base_url_preflight_token: 'edit-preflight-token',
    confirm_same_site: true,
    name: '华东站点',
    remark: '主运营站点',
  })

  await page.getByLabel('打开站点操作').click()
  await page.getByRole('button', { name: '停用站点' }).click()
  const disableDialog = page.getByRole('alertdialog', { name: '停用站点？' })
  await disableDialog.getByRole('button', { name: '停用站点' }).click()
  await expect.poll(() => disableCalls).toBe(1)

  currentSite = {
    ...currentSite,
    statistics_end_at: 1_783_868_400,
  }
  await page.reload()
  await expect(page.getByRole('heading', { name: '华东站点' })).toBeVisible()
  const getCallsBeforeLifecycle = detailGetCalls
  await page.getByLabel('打开站点操作').click()
  await page.getByRole('button', { name: '管理停用生命周期' }).click()
  const lifecycleDialog = page.getByRole('dialog', {
    name: '管理停用生命周期',
  })
  await expect
    .poll(() => detailGetCalls)
    .toBeGreaterThan(getCallsBeforeLifecycle)
  await expect(
    lifecycleDialog.getByRole('button', { name: '清除统计终止时间' })
  ).toBeVisible()
  await expect(
    lifecycleDialog.getByRole('button', { name: '恢复站点' })
  ).toHaveCount(0)
  await lifecycleDialog
    .getByRole('button', { name: '关闭', exact: true })
    .first()
    .click()

  await page.getByLabel('打开站点操作').click()
  await page.getByRole('button', { name: '删除站点' }).click()
  const deleteDialog = page.getByRole('alertdialog', { name: '删除站点？' })
  await deleteDialog.getByRole('button', { name: '删除站点' }).click()
  await expect(page.getByText('该对象存在关联数据，无法删除')).toBeVisible()
  await expect(page).toHaveURL(/\/sites\/1/)
})

test('loads all current instances once and uses correct resource aggregations', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await page.route('**/api/settings', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(minuteRetentionSettingsFixture()) })
  })
  let instanceCalls = 0
  let releaseInstances: (() => void) | undefined
  let resourceCalls = 0
  await page.route('**/api/sites/1', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope(siteFixture('1')) })
  })
  await page.route('**/api/sites/1/instances', async (route) => {
    assertAuthenticatedRequest(route, admin)
    instanceCalls += 1
    await new Promise<void>((resolve) => {
      releaseInstances = resolve
    })
    await route.fulfill({ json: envelope(instancesFixture) })
  })
  await page.route(/\/api\/sites\/1\/status(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, admin)
    resourceCalls += 1
    const url = new URL(route.request().url())
    const nodeName = url.searchParams.get('node_name')
    await route.fulfill({
      json: envelope({
        granularity: url.searchParams.get('granularity') ?? 'minute',
        node_name: nodeName,
        site_id: '1',
        summary: {
          bucket_end: 1_783_872_000,
          bucket_start: 1_783_868_400,
          cpu_avg_percent: 31,
          cpu_max_percent: 48,
          data_status: 'complete',
          disk_last_used_percent: 68,
          disk_max_used_percent: 72,
          expected_sample_count: 2,
          health_status: 'ok',
          instance_count: 2,
          memory_avg_percent: 49,
          memory_max_percent: 64,
          online_instance_count: 1,
          sample_count: 2,
        },
        trend: [
          {
            bucket_end: 1_783_868_460,
            bucket_start: 1_783_868_400,
            cpu_avg_percent: 30,
            cpu_max_percent: 45,
            data_status: 'complete',
            disk_last_used_percent: 67,
            disk_max_used_percent: 70,
            expected_sample_count: 1,
            health_status: 'ok',
            instance_count: 2,
            memory_avg_percent: 48,
            memory_max_percent: 62,
            online_instance_count: 1,
            sample_count: 1,
          },
          {
            bucket_end: 1_783_868_520,
            bucket_start: 1_783_868_460,
            cpu_avg_percent: null,
            cpu_max_percent: null,
            data_status: 'missing',
            disk_last_used_percent: null,
            disk_max_used_percent: null,
            expected_sample_count: 1,
            health_status: 'unavailable',
            instance_count: null,
            memory_avg_percent: null,
            memory_max_percent: null,
            online_instance_count: null,
            sample_count: 0,
          },
        ],
      }),
    })
  })

  await page.goto(
    '/sites/1/status?start=1783868400&end=1783872000&granularity=minute&metric=cpu&aggregation=max'
  )
  await expect.poll(() => releaseInstances).toBeDefined()
  const summary = page.getByRole('heading', { name: '实例概览' }).locator('..')
  await expect(summary.locator('.animate-pulse')).toHaveCount(6)
  await expect(summary.getByText('0', { exact: true })).toHaveCount(0)
  releaseInstances?.()
  await expect(
    page
      .getByText('worker-1', { exact: true })
      .filter({ visible: true })
      .first()
  ).toBeVisible()
  await expect.poll(() => instanceCalls).toBe(1)
  await expect.poll(() => resourceCalls).toBe(1)
  await expect(page.locator('canvas').first()).toBeVisible()

  await page
    .getByRole('combobox', { name: '实例', exact: true })
    .selectOption('worker-1')
  await expect.poll(() => resourceCalls).toBe(2)
  expect(instanceCalls).toBe(1)
  await page.getByRole('button', { name: '内存', exact: true }).click()
  await page.getByRole('button', { name: '平均值', exact: true }).click()
  await page.getByRole('button', { name: '磁盘', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '期末值', exact: true })
  ).toHaveAttribute('aria-pressed', 'true')
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
})

test('deep-links run windows, polls only until terminal, and retries with a new run', async ({
  page,
}) => {
  await page.clock.install()
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockSiteReads(page, admin)
  let runCalls = 0
  let terminal = false
  let windowCalls = 0
  let retryBody: unknown

  await page.route(
    /\/api\/sites\/1\/collection-runs(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      await route.fulfill({
        json: envelope({
          items: [runFixture('failed')],
          page: 1,
          page_size: 20,
          total: 1,
        }),
      })
    }
  )
  await page.route('**/api/collection-runs/10', async (route) => {
    assertAuthenticatedRequest(route, admin)
    runCalls += 1
    await route.fulfill({
      json: envelope(runFixture(terminal ? 'failed' : 'running')),
    })
  })
  await page.route(
    /\/api\/collection-runs\/10\/windows(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      windowCalls += 1
      await route.fulfill({
        json: envelope({
          items: [windowFixture(terminal ? 'failed' : 'pending')],
          page: 1,
          page_size: 20,
          total: 1,
        }),
      })
    }
  )
  await page.route('**/api/sites/1/backfill', async (route) => {
    assertAuthenticatedRequest(route, admin)
    retryBody = route.request().postDataJSON()
    await route.fulfill({
      json: envelope({ ...runFixture('pending'), id: '11' }),
    })
  })

  await page.goto('/sites/1?runId=10&runPage=1&windowPage=1')
  const sheet = page.getByRole('dialog', { name: '任务 10 的执行窗口' })
  await expect(sheet).toBeVisible()
  await expect.poll(() => runCalls).toBeGreaterThan(0)
  await expect.poll(() => windowCalls).toBeGreaterThan(0)
  const initialRunCalls = runCalls
  const initialWindowCalls = windowCalls
  terminal = true
  await page.clock.fastForward(5_100)
  await expect.poll(() => runCalls).toBeGreaterThan(initialRunCalls)
  await expect.poll(() => windowCalls).toBeGreaterThan(initialWindowCalls)
  await expect(sheet.getByText('该范围的数据缺失').first()).toBeVisible()
  await expect(sheet.getByText('请求 ID：req_run_last')).toBeVisible()

  const terminalRunCalls = runCalls
  const terminalWindowCalls = windowCalls
  await page.clock.fastForward(10_000)
  expect(runCalls).toBe(terminalRunCalls)
  expect(windowCalls).toBe(terminalWindowCalls)
  await sheet.getByRole('button', { name: '重试失败窗口' }).click()
  await expect
    .poll(() => retryBody)
    .toEqual({
      end_timestamp: 1_783_872_000,
      only_missing: true,
      start_timestamp: 1_783_868_400,
    })
})

test('keeps the last successful site detail when a background refresh fails', async ({
  page,
}) => {
  await page.clock.install()
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  let detailCalls = 0
  await mockSiteReads(page, admin)
  await page.route('**/api/sites/1', async (route) => {
    assertAuthenticatedRequest(route, admin)
    detailCalls += 1
    if (detailCalls === 1) {
      await route.fulfill({ json: envelope(siteFixture('1')) })
      return
    }
    await route.fulfill({
      json: errorEnvelope('INTERNAL_ERROR', 'req_refresh_failed'),
      status: 500,
    })
  })

  await page.goto('/sites/1')
  await expect(page.getByRole('heading', { name: '华东站点' })).toBeVisible()
  await page.clock.fastForward(60_100)
  await expect.poll(() => detailCalls).toBeGreaterThanOrEqual(2)
  await page.clock.fastForward(1_100)
  await expect.poll(() => detailCalls).toBeGreaterThanOrEqual(3)
  await page.clock.fastForward(2_100)
  await expect.poll(() => detailCalls).toBeGreaterThanOrEqual(4)
  await expect(page.getByText('后台刷新失败')).toBeVisible()
  await expect(page.getByRole('heading', { name: '当前概览' })).toBeVisible()
  await expect(page.getByText('210', { exact: true }).first()).toBeVisible()
})

test('clears authentication when a site endpoint returns 401', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await page.route('**/api/sites/1/instances', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({ json: envelope([]) })
  })
  await page.route(
    /\/api\/sites\/1\/collection-runs(?:\?.*)?$/,
    async (route) => {
      assertAuthenticatedRequest(route, admin)
      await route.fulfill({
        json: envelope({ items: [], page: 1, page_size: 20, total: 0 }),
      })
    }
  )
  await page.route('**/api/sites/1', async (route) => {
    assertAuthenticatedRequest(route, admin)
    await route.fulfill({
      json: errorEnvelope('AUTH_INVALID', 'req_site_unauthorized'),
      status: 401,
    })
  })

  await page.goto('/sites/1')
  await expect(page).toHaveURL(/\/sign-in\?redirect=/)
  const storage = await page.evaluate(
    ({ authKey, uidKey }) => ({
      auth: window.localStorage.getItem(authKey),
      uid: window.localStorage.getItem(uidKey),
    }),
    { authKey: authStorageKey, uidKey: uidStorageKey }
  )
  expect(storage).toEqual({ auth: null, uid: null })
})
