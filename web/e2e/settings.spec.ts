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

type SettingItemFixture = {
  configured: boolean
  constraints: Record<string, unknown>
  decrypt_error: boolean
  key: string
  masked_value: string
  read_only: boolean
  secret: boolean
  updated_at: number | null
  value: boolean | number | string | null
  value_type: 'bool' | 'decimal' | 'int' | 'string'
}

type SettingGroupFixture = {
  items: SettingItemFixture[]
  key: string
  label_key: string
}

const admin: TestUser = {
  display_name: '负责跨区域超长中文生产环境配置审查与通知联调的平台管理员',
  id: '9007199254740993',
  must_change_password: false,
  role: 'admin',
  status: 1,
  username: 'admin',
}

const viewer: TestUser = {
  display_name: '只读运营值班人员',
  id: '9007199254740994',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'viewer',
}

function envelope<T>(data: T, requestId = 'req_settings_e2e') {
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
    request_id: 'req_settings_error',
    success: false,
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
    await route.fulfill({ json: envelope(user, 'req_settings_self') })
  })
}

const integerConstraints: Record<string, [number | string, number | string]> = {
  'collector.probe_interval_seconds': [60, 3600],
  'collector.realtime_interval_seconds': [60, 3600],
  'collector.resource_interval_seconds': [60, 3600],
  'collector.usage_delay_minutes': [1, 59],
  'collector.minute_retention_days': [1, 3650],
  'logs.retention_days': [1, 3650],
  'performance.retention_days': [1, 3650],
  'task.retention_days': [1, 3650],
  system_task_terminal_retention_days: [1, 3650],
  'collector.probe_concurrency': [1, 100],
  'collector.realtime_concurrency': [1, 100],
  'collector.resource_concurrency': [1, 100],
  'collector.metadata_concurrency': [1, 100],
  'collector.usage_concurrency': [1, 100],
  'collector.backfill_concurrency': [1, 100],
  'collector.manual_backfill_max_days': [1, 3660],
  'fast_task.history_retention_seconds': [60, 31_536_000],
  'fast_task.history_count': [1, 1000],
  'upstream.connect_timeout_seconds': [1, 60],
  'upstream.response_header_timeout_seconds': [1, 300],
  'upstream.request_timeout_seconds': [1, 600],
  'upstream.export_timeout_seconds': [1, 3600],
  'upstream.rate_limit_requests': [1, 10_000],
  'upstream.rate_limit_window_seconds': [1, 3600],
  'upstream.max_inflight_per_origin': [1, 100],
  'export.file_ttl_hours': [1, 168],
  'export.max_active_per_user': [1, 100],
  'export.max_active_global': [1, 100],
  'export.max_file_bytes': ['1', '9223372036854775807'],
  'export.min_free_disk_bytes': ['1', '9223372036854775807'],
}

function settingItem(
  key: string,
  value: boolean | number | string | null,
  overrides: Partial<SettingItemFixture> = {}
): SettingItemFixture {
  const secret =
    key === 'notification.dingtalk.webhook' ||
    key === 'notification.dingtalk.secret'
  let valueType: SettingItemFixture['value_type'] = 'int'
  if (key === 'notification.dingtalk.enabled') valueType = 'bool'
  else if (key.startsWith('rate.')) valueType = 'decimal'
  else if (key === 'system.public_origin' || secret) valueType = 'string'
  const range = integerConstraints[key]
  let constraints: Record<string, unknown> = {}
  if (range) constraints = { maximum: range[1], minimum: range[0] }
  else if (valueType === 'decimal') {
    constraints = {
      json_representation: 'decimal_string',
      maximum_digits: 30,
      maximum_integer_digits: 20,
      maximum_scale: 10,
      optional: true,
      positive: true,
    }
  }
  return {
    configured: secret ? true : value !== null && value !== '',
    constraints,
    decrypt_error: false,
    key,
    masked_value: secret ? '********' : '',
    read_only: key === 'system.public_origin',
    secret,
    updated_at: key === 'system.public_origin' ? null : 1_752_400_800,
    value: secret ? null : value,
    value_type: valueType,
    ...overrides,
  }
}

function settingsFixture(
  options: {
    decryptSecret?: boolean
    retentionDays?: number
  } = {}
): SettingGroupFixture[] {
  const collector = [
    settingItem('collector.probe_interval_seconds', 60),
    settingItem('collector.realtime_interval_seconds', 60),
    settingItem('collector.resource_interval_seconds', 60),
    settingItem('collector.usage_delay_minutes', 12),
    settingItem('collector.minute_retention_days', options.retentionDays ?? 37),
    settingItem('logs.retention_days', 90),
    settingItem('performance.retention_days', 90),
    settingItem('task.retention_days', 90),
    settingItem('system_task_terminal_retention_days', 90),
    settingItem('collector.probe_concurrency', 20),
    settingItem('collector.realtime_concurrency', 10),
    settingItem('collector.resource_concurrency', 10),
    settingItem('collector.metadata_concurrency', 5),
    settingItem('collector.usage_concurrency', 3),
    settingItem('collector.backfill_concurrency', 2),
    settingItem('collector.manual_backfill_max_days', 366),
    settingItem('fast_task.history_retention_seconds', 86400),
    settingItem('fast_task.history_count', 100),
  ]
  const groups: SettingGroupFixture[] = [
    {
      items: collector,
      key: 'collector',
      label_key: 'settings.groups.collector',
    },
    {
      items: [
        settingItem('export.file_ttl_hours', 24),
        settingItem('export.max_active_per_user', 3),
        settingItem('export.max_active_global', 10),
        settingItem('export.max_file_bytes', '2147483648'),
        settingItem('export.min_free_disk_bytes', '5368709120'),
      ],
      key: 'export',
      label_key: 'settings.groups.export',
    },
    {
      items: [
        settingItem('upstream.allowed_host_suffixes', '', {
          constraints: { maximum_length: 8192, optional: true },
        }),
        settingItem('upstream.allowed_cidrs', '', {
          constraints: { maximum_length: 8192, optional: true },
        }),
        settingItem('upstream.connect_timeout_seconds', 5),
        settingItem('upstream.response_header_timeout_seconds', 15),
        settingItem('upstream.request_timeout_seconds', 30),
        settingItem('upstream.export_timeout_seconds', 120),
        settingItem('upstream.rate_limit_requests', 300),
        settingItem('upstream.rate_limit_window_seconds', 180),
        settingItem('upstream.max_inflight_per_origin', 4),
      ],
      key: 'upstream',
      label_key: 'settings.groups.upstream',
    },
    {
      items: [
        settingItem('rate.fallback_quota_per_unit', '500000'),
        settingItem('rate.fallback_usd_exchange_rate', '6.8'),
      ],
      key: 'rate',
      label_key: 'settings.groups.rate',
    },
    {
      items: [
        settingItem('notification.dingtalk.enabled', true),
        settingItem('notification.dingtalk.webhook', null),
        settingItem('notification.dingtalk.secret', null, {
          decrypt_error: options.decryptSecret ?? false,
        }),
      ],
      key: 'notification',
      label_key: 'settings.groups.notification',
    },
    {
      items: [
        settingItem(
          'system.public_origin',
          'https://pilot-production-operations-very-long-hostname.example.com'
        ),
      ],
      key: 'system',
      label_key: 'settings.groups.system',
    },
  ]
  if (options.decryptSecret) {
    const enabled = groups
      .flatMap((group) => group.items)
      .find((item) => item.key === 'notification.dingtalk.enabled')
    if (enabled) enabled.value = false
  }
  return groups
}

function applyPatch(
  source: SettingGroupFixture[],
  body: { items: Array<{ clear?: boolean; key: string; value?: unknown }> }
): SettingGroupFixture[] {
  const result = structuredClone(source)
  const items = result.flatMap((group) => group.items)
  for (const patch of body.items) {
    const item = items.find((candidate) => candidate.key === patch.key)
    if (!item) continue
    item.updated_at = (item.updated_at ?? 1_752_400_800) + 1
    if (patch.clear) {
      item.configured = false
      item.decrypt_error = false
      item.masked_value = ''
      item.value = null
    } else if (item.secret) {
      item.configured = typeof patch.value === 'string' && patch.value !== ''
      item.decrypt_error = false
      item.masked_value = item.configured ? '********' : ''
      item.value = null
    } else {
      item.value = patch.value as boolean | number | string
      item.configured = patch.value !== ''
    }
  }
  return result
}

type SettingsMock = {
  failNextPut: boolean
  groups: SettingGroupFixture[]
  putBodies: Array<{
    items: Array<{ clear?: boolean; key: string; value?: unknown }>
  }>
}

async function mockSettings(
  page: Page,
  user: TestUser,
  options: { decryptSecret?: boolean; retentionDays?: number } = {}
): Promise<SettingsMock> {
  const state: SettingsMock = {
    failNextPut: false,
    groups: settingsFixture(options),
    putBodies: [],
  }
  await page.route('**/api/settings', async (route) => {
    assertAuthenticatedRequest(route, user)
    if (route.request().method() === 'PUT') {
      const body = route
        .request()
        .postDataJSON() as SettingsMock['putBodies'][number]
      state.putBodies.push(body)
      if (state.failNextPut) {
        state.failNextPut = false
        await route.fulfill({
          json: errorEnvelope('VALIDATION_ERROR', {
            'items[0].value': 'invalid scalar',
          }),
          status: 400,
        })
        return
      }
      state.groups = applyPatch(state.groups, body)
      await route.fulfill({ json: envelope(state.groups, 'req_settings_put') })
      return
    }
    await route.fulfill({ json: envelope(state.groups, 'req_settings_get') })
  })
  return state
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

test('renders the complete settings surface for admin without overflow or axe violations', async ({
  page,
}, testInfo) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockSettings(page, admin)
  await page.goto('/settings/system')
  await expect(page).toHaveURL(/\/settings\/system$/)

  await expect(
    page.getByRole('heading', { level: 1, name: '系统设置' })
  ).toBeVisible()
  const categoryTabs = page.getByRole('tablist', { name: '系统设置分类' })
  await expect(categoryTabs.getByRole('tab')).toHaveCount(6)
  const scrollSettingsContent = (scrollTop: number) =>
    categoryTabs.evaluate((element, nextScrollTop) => {
      let ancestor = element.parentElement
      let scrollContainer: HTMLElement | null = null
      let scrollRange = 0
      while (ancestor) {
        const overflowY = window.getComputedStyle(ancestor).overflowY
        const nextScrollRange = ancestor.scrollHeight - ancestor.clientHeight
        if (/auto|scroll/.test(overflowY) && nextScrollRange > scrollRange) {
          scrollContainer = ancestor
          scrollRange = nextScrollRange
        }
        ancestor = ancestor.parentElement
      }
      if (!scrollContainer) return -1
      scrollContainer.scrollTop = nextScrollTop
      return scrollContainer.scrollTop
    }, scrollTop)
  expect(await scrollSettingsContent(300)).toBeGreaterThan(0)
  const stickyTabsY = Math.round((await categoryTabs.boundingBox())?.y ?? -1)
  expect(await scrollSettingsContent(700)).toBeGreaterThan(300)
  await expect
    .poll(async () => Math.round((await categoryTabs.boundingBox())?.y ?? -1))
    .toBe(stickyTabsY)
  await scrollSettingsContent(0)
  await expect(page.getByRole('heading', { name: '采集策略' })).toBeVisible()
  for (const label of [
    '站点状态检查周期（分钟）',
    '实时统计周期（分钟）',
    '资源采集周期（分钟）',
  ]) {
    await expect(page.getByLabel(label)).toHaveValue('1')
    await expect(page.getByLabel(label)).toHaveAttribute('type', 'number')
    await expect(page.getByLabel(label)).toHaveAttribute('min', '1')
    await expect(page.getByLabel(label)).toHaveAttribute('max', '60')
  }
  const fastTaskRetention = page.getByLabel('快速任务历史有效期（小时）')
  await expect(fastTaskRetention).toHaveValue('24')
  await expect(fastTaskRetention).toHaveAttribute('type', 'number')
  await expect(
    page.getByText(
      '每个站点、每种快速任务最多保留的最近记录数；每次写入都会立即截断超出上限的旧记录。'
    )
  ).toBeVisible()
  await expect(page.getByRole('heading', { name: '任务并发' })).toHaveCount(0)
  await categoryTabs.getByRole('tab', { name: '任务并发' }).click()
  await expect(page).toHaveURL(/section=concurrency/)
  await expect(page.getByRole('heading', { name: '任务并发' })).toBeVisible()
  await categoryTabs.getByRole('tab', { name: '导出策略' }).click()
  await expect(page.getByRole('heading', { name: '导出策略' })).toBeVisible()
  await expect(page.getByLabel('导出文件大小上限（MB）')).toHaveAttribute(
    'type',
    'number'
  )
  await expect(page.getByLabel('最低磁盘余量（MB）')).toHaveAttribute(
    'type',
    'number'
  )
  await categoryTabs.getByRole('tab', { name: '上游访问策略' }).click()
  await expect(
    page.getByRole('heading', { name: '上游访问策略' })
  ).toBeVisible()
  await categoryTabs.getByRole('tab', { name: '费率兜底' }).click()
  await expect(page.getByRole('heading', { name: '费率兜底' })).toBeVisible()
  await expect(page.getByLabel('兜底额度单价基数（quota）')).toHaveAttribute(
    'type',
    'number'
  )
  await expect(page.getByLabel('兜底美元汇率')).toHaveAttribute(
    'type',
    'number'
  )
  await expect(page.getByLabel('兜底美元汇率')).toHaveAttribute(
    'inputmode',
    'decimal'
  )
  await categoryTabs.getByRole('tab', { name: '平台与通知' }).click()
  const notificationHeading = page.getByRole('heading', {
    name: '平台与通知',
  })
  await expect(notificationHeading).toBeVisible()
  await expect(
    page.getByText(
      '用于校验浏览器写操作是否来自本平台，并生成钉钉告警中的详情链接；来自运行环境配置，只读。'
    )
  ).toBeVisible()
  await expect
    .poll(async () => (await notificationHeading.boundingBox())?.x ?? -1)
    .toBeGreaterThanOrEqual(0)
  await expect(page.getByText('H+15 发布资格')).toHaveCount(0)
  await assertNoHorizontalOverflow(page)
  const accessibility = await new AxeBuilder({ page }).analyze()
  expect(accessibility.violations).toEqual([])
  await page.screenshot({
    path: testInfo.outputPath('settings-admin-top.png'),
  })
  await page.screenshot({
    path: testInfo.outputPath('settings-admin-notification.png'),
  })
})

test('keeps Viewer settings read-only while retaining both settings navigation entries', async ({
  page,
}) => {
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockSettings(page, viewer)
  await page.goto('/settings')

  await expect(
    page.getByRole('heading', { level: 1, name: '系统设置' })
  ).toBeVisible()
  await expect(page.locator('#settings-form input')).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: '保存', exact: true })
  ).toHaveCount(0)
  await expect(page.getByRole('button', { name: '测试发送' })).toHaveCount(0)
  if ((await page.getByRole('navigation').count()) > 0) {
    await expect(page.getByRole('link', { name: '系统设置' })).toBeVisible()
    await expect(page.getByRole('link', { name: '平台用户' })).toBeVisible()
  }
  await assertNoHorizontalOverflow(page)
})

test('stacks setting descriptions above controls at medium viewport widths', async ({
  page,
}) => {
  await page.setViewportSize({ height: 800, width: 1024 })
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  await mockSettings(page, admin)
  await page.goto('/settings/system?section=export')

  const description = page.getByText(
    '单个导出文件允许写入的最大容量；1 MB = 1024 × 1024 字节，超过该值的导出不会完成。'
  )
  const input = page.getByLabel('导出文件大小上限（MB）')
  await expect(description).toBeVisible()
  await expect(input).toBeVisible()

  const descriptionBox = await description.boundingBox()
  const inputBox = await input.boundingBox()
  expect(descriptionBox).not.toBeNull()
  expect(inputBox).not.toBeNull()
  if (!descriptionBox || !inputBox) {
    throw new Error(
      'Expected the setting description and input to have layout boxes'
    )
  }
  expect(inputBox.y).toBeGreaterThanOrEqual(
    descriptionBox.y + descriptionBox.height
  )
  expect(inputBox.width).toBeLessThanOrEqual(512)
  await assertNoHorizontalOverflow(page)
})

test('submits only changed settings and renders success, retry, and failed notification MessageRefs', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  const settings = await mockSettings(page, admin)
  const testResults = [
    {
      delivery_id: '9007199254740995',
      message: {
        code: 'NOTIFICATION_TEST_SUCCEEDED',
        params: { delivery_id: '9007199254740995' },
        technical_detail: '',
      },
      response_code: 200,
      status: 'success',
    },
    {
      delivery_id: '9007199254740996',
      message: {
        code: 'DELIVERY_RETRY_SCHEDULED',
        params: {
          delivery_id: '9007199254740996',
          next_retry_at: '1752400860',
        },
        technical_detail: '',
      },
      response_code: 429,
      status: 'failed',
    },
    {
      delivery_id: '9007199254740997',
      message: {
        code: 'DINGTALK_REJECTED',
        params: {
          alert_event_id: null,
          delivery_id: '9007199254740997',
          errcode: '310000',
        },
        technical_detail: '',
      },
      response_code: 200,
      status: 'failed',
    },
  ]
  let testCalls = 0
  await page.route('**/api/notifications/dingtalk/test', async (route) => {
    assertAuthenticatedRequest(route, admin)
    const result = testResults[Math.min(testCalls, testResults.length - 1)]
    testCalls += 1
    await route.fulfill({ json: envelope(result, `req_test_${testCalls}`) })
  })

  await page.goto('/settings')
  await page.getByLabel('站点状态检查周期（分钟）').fill('2')
  await page.getByLabel('小时用量采集延迟（分钟）').fill('4')
  await page.getByLabel('快速任务历史有效期（小时）').fill('48')
  await page.getByRole('tab', { name: '导出策略' }).click()
  await expect(page.getByLabel('导出文件大小上限（MB）')).toHaveValue('2048')
  await expect(page.getByLabel('最低磁盘余量（MB）')).toHaveValue('5120')
  await page.getByLabel('导出文件大小上限（MB）').fill('4096')
  await page.getByRole('tab', { name: '费率兜底' }).click()
  await page.getByLabel('兜底美元汇率').fill('7.3000')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect.poll(() => settings.putBodies.length).toBe(1)
  expect(settings.putBodies[0]).toEqual({
    items: [
      { key: 'collector.probe_interval_seconds', value: 120 },
      { key: 'collector.usage_delay_minutes', value: 4 },
      { key: 'fast_task.history_retention_seconds', value: 172800 },
      { key: 'export.max_file_bytes', value: '4294967296' },
      { key: 'rate.fallback_usd_exchange_rate', value: '7.3000' },
    ],
  })
  const savedToast = page.getByText('系统设置已保存')
  await expect(savedToast).toBeVisible()
  await expect(savedToast).toBeHidden({ timeout: 10_000 })

  await page.getByRole('tab', { name: '平台与通知' }).click()
  const testButton = page.getByRole('button', { name: '测试发送' })
  const notificationResult = page.locator('#settings-form')
  await testButton.press('Enter')
  await expect(
    notificationResult.getByText('钉钉测试消息发送成功')
  ).toBeVisible()
  await testButton.press('Enter')
  await expect(
    notificationResult.getByText('通知发送失败，已安排自动重试')
  ).toBeVisible()
  await testButton.press('Enter')
  await expect(notificationResult.getByText('钉钉拒绝了本次通知')).toBeVisible()
  expect(testCalls).toBe(3)
})

test('keeps the edited batch after an atomic field error and reset restores the server snapshot', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  const settings = await mockSettings(page, admin)
  settings.failNextPut = true
  await page.goto('/settings')

  const delay = page.getByLabel('小时用量采集延迟（分钟）')
  await delay.fill('7')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.getByText('请检查标出的字段后重试')).toBeVisible()
  await expect(delay).toHaveValue('7')
  expect(settings.putBodies).toEqual([
    { items: [{ key: 'collector.usage_delay_minutes', value: 7 }] },
  ])
  await page.getByRole('button', { name: '撤销未保存的修改' }).click()
  await expect(delay).toHaveValue('12')
})

test('requires explicit decrypt-error clear confirmation and supports later replacement', async ({
  page,
}) => {
  await seedAuth(page, admin)
  await mockSelf(page, admin)
  const settings = await mockSettings(page, admin, { decryptSecret: true })
  await page.goto('/settings')
  await page.getByRole('tab', { name: '平台与通知' }).click()

  const secretHeading = page.getByRole('heading', { name: '钉钉签名密钥' })
  const secretRow = secretHeading.locator('xpath=../..')
  await expect(secretRow.getByText('密文不可解密')).toBeVisible()
  await expect(secretRow.getByRole('button', { name: '保持' })).toBeDisabled()
  await secretRow.getByRole('button', { name: '清除' }).click()
  const confirm = page.getByRole('alertdialog', { name: '确认清除敏感配置' })
  await expect(confirm).toBeVisible()
  await confirm.getByRole('button', { name: '确认清除' }).click()
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect.poll(() => settings.putBodies.length).toBe(1)
  expect(settings.putBodies[0]).toEqual({
    items: [{ clear: true, key: 'notification.dingtalk.secret' }],
  })
  await expect(secretRow.getByText('未配置')).toBeVisible()

  await secretRow.getByRole('button', { name: '替换' }).click()
  await secretRow.getByLabel('新签名密钥').fill('replacement-signing-secret')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect.poll(() => settings.putBodies.length).toBe(2)
  expect(settings.putBodies[1]).toEqual({
    items: [
      {
        key: 'notification.dingtalk.secret',
        value: 'replacement-signing-secret',
      },
    ],
  })
})

async function mockSiteStatusReads(page: Page, user: TestUser) {
  await page.route('**/api/sites/1/instances', async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({ json: envelope([]) })
  })
  await page.route(/\/api\/sites\/1(?:\?.*)?$/, async (route) => {
    assertAuthenticatedRequest(route, user)
    await route.fulfill({
      json: envelope({
        base_url: 'https://site-one.example.com',
        health_status: 'ok',
        id: '1',
        name: '动态留存验证站点',
      }),
    })
  })
}

test('blocks minute resource requests while retention is loading and resumes from the shared setting', async ({
  page,
}) => {
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockSiteStatusReads(page, viewer)
  let releaseSettings = () => {}
  const settingsGate = new Promise<void>((resolve) => {
    releaseSettings = resolve
  })
  await page.route('**/api/settings', async (route) => {
    assertAuthenticatedRequest(route, viewer)
    await settingsGate
    await route.fulfill({
      json: envelope(settingsFixture({ retentionDays: 2 })),
    })
  })
  let statusCalls = 0
  await page.route(/\/api\/sites\/1\/status(?:\?.*)?$/, async (route) => {
    statusCalls += 1
    await route.fulfill({
      json: envelope({
        granularity: 'minute',
        node_name: null,
        site_id: '1',
        summary: null,
        trend: [],
      }),
    })
  })

  await page.goto(
    '/sites/1/status?granularity=minute&metric=cpu&aggregation=max&start=1783872000&end=1783875600'
  )
  await expect(
    page.getByText('正在读取分钟数据留存配置，分钟趋势查询暂不可用。')
  ).toBeVisible()
  expect(statusCalls).toBe(0)
  releaseSettings()
  await expect.poll(() => statusCalls).toBe(1)
  await expect(
    page.getByText('正在读取分钟数据留存配置，分钟趋势查询暂不可用。')
  ).toHaveCount(0)
})

test('fails closed when retention cannot load and uses the configured day limit', async ({
  page,
}, testInfo) => {
  await seedAuth(page, viewer)
  await mockSelf(page, viewer)
  await mockSiteStatusReads(page, viewer)
  let failSettings = true
  await page.route('**/api/settings', async (route) => {
    assertAuthenticatedRequest(route, viewer)
    if (failSettings) {
      await route.fulfill({
        json: errorEnvelope('INTERNAL_ERROR'),
        status: 500,
      })
      return
    }
    await route.fulfill({
      json: envelope(settingsFixture({ retentionDays: 1 })),
    })
  })
  let statusCalls = 0
  await page.route(/\/api\/sites\/1\/status(?:\?.*)?$/, async (route) => {
    statusCalls += 1
    await route.fulfill({ json: envelope({ summary: null, trend: [] }) })
  })

  await page.goto(
    '/sites/1/status?granularity=minute&metric=cpu&aggregation=max&start=1783612800&end=1783872000'
  )
  await expect(
    page.getByText('无法读取分钟数据留存配置，已停止分钟趋势查询。')
  ).toBeVisible({ timeout: 10_000 })
  expect(statusCalls).toBe(0)

  failSettings = false
  await page.getByRole('button', { name: '刷新' }).click()
  const retentionLimit = page.getByText('分钟趋势最多查询最近 1 天。')
  await expect(retentionLimit).toBeVisible()
  expect(statusCalls).toBe(0)
  await retentionLimit.scrollIntoViewIfNeeded()
  await page.screenshot({
    path: testInfo.outputPath('site-retention-limit.png'),
  })
})
