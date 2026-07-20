import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const rangeStart = 1_783_789_200
const rangeEnd = 1_783_875_600

type DashboardEndpoint = 'health' | 'summary' | 'top' | 'trend'
type DashboardFixtureState = 'complete' | 'empty' | 'partial'

const dashboardSectionNames = [
  '今日运营',
  '实时吞吐',
  '近 30 天趋势',
  '今日快速排行',
  '站点健康、完整性与告警',
] as const

const viewer = {
  display_name: '跨区域运营只读审核用户',
  id: '9007199254740994',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'viewer',
}

function envelope<T>(data: T, requestId = 'req_f5') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function errorEnvelope(requestId = 'req_f5_error') {
  return {
    code: 'INTERNAL_ERROR',
    data: null,
    message: 'INTERNAL_ERROR',
    request_id: requestId,
    success: false,
  }
}

const missingReason = {
  code: 'DATA_WINDOW_MISSING',
  params: {
    end_timestamp: rangeEnd,
    site_id: '9007199254740993',
    start_timestamp: rangeStart,
  },
  technical_detail: '',
}

const siteBreakdown = [
  {
    data_status: 'complete',
    quota: '9223372036854775807',
    quota_per_unit: '500000',
    rate_source: 'site',
    rate_updated_at: rangeEnd - 3600,
    site_id: '9007199254740993',
    site_name: '华东超长名称生产站点用于验证移动端不会横向溢出',
    usd_exchange_rate: '7.3',
  },
]

function completeness(status: 'complete' | 'partial' = 'partial') {
  return {
    complete_site_count: status === 'complete' ? 1 : 0,
    complete_unit_count: status === 'complete' ? 24 : 12,
    completeness_rate: status === 'complete' ? 1 : 0.5,
    data_status: status,
    expected_site_count: 1,
    expected_unit_count: 24,
    last_verified_at: rangeEnd - 3600,
    missing_range_total: status === 'complete' ? 0 : 1,
    missing_ranges:
      status === 'complete'
        ? []
        : [
            {
              end_timestamp: rangeEnd,
              reason: missingReason,
              site_id: '9007199254740993',
              start_timestamp: rangeStart,
              status: 'missing',
            },
          ],
    missing_ranges_truncated: false,
    missing_site_ids: status === 'complete' ? [] : ['9007199254740993'],
    unit_type: 'hour',
  }
}

function emptyCompleteness() {
  return {
    ...completeness('complete'),
    complete_site_count: 0,
    complete_unit_count: 0,
    completeness_rate: 1,
    expected_site_count: 0,
    expected_unit_count: 0,
    last_verified_at: null,
  }
}

function trendPoint(
  bucketStart: number,
  status: 'complete' | 'missing' = 'complete'
) {
  const complete = status === 'complete'
  return {
    active_users: complete ? '12' : null,
    as_of: complete ? bucketStart + 3600 : null,
    bucket_end: bucketStart + 86_400,
    bucket_start: bucketStart,
    complete_site_count: complete ? 1 : 0,
    data_status: status,
    expected_site_count: 1,
    is_final: complete,
    quota: complete ? '9223372036854775807' : null,
    reason: complete ? null : missingReason,
    request_count: complete ? '9007199254740995' : null,
    site_breakdown: complete ? siteBreakdown : [],
    token_used: complete ? '9007199254740997' : null,
  }
}

function dashboardSummary(state: DashboardFixtureState = 'partial') {
  const empty = state === 'empty'
  const dataStatus = state === 'partial' ? 'partial' : 'complete'
  const expectedSiteCount = empty ? 0 : 2
  const completeSiteCount = state === 'partial' ? 1 : expectedSiteCount
  return {
    active_accounts_today: empty ? null : '9007199254740995',
    customer_count: empty ? 0 : 8,
    instance_count: empty ? null : 6,
    managed_account_count: empty ? 0 : 12,
    offline_site_count: empty ? 0 : 1,
    online_instance_count: empty ? null : 5,
    online_site_count: empty ? 0 : 1,
    realtime_as_of: empty ? null : rangeEnd - 60,
    realtime_complete_site_count: completeSiteCount,
    realtime_data_status: dataStatus,
    realtime_expected_site_count: expectedSiteCount,
    realtime_reason: state === 'partial' ? missingReason : null,
    resource_as_of: empty ? null : rangeEnd - 60,
    resource_complete_site_count: completeSiteCount,
    resource_data_status: dataStatus,
    resource_expected_site_count: expectedSiteCount,
    resource_reason: state === 'partial' ? missingReason : null,
    resource_stale_site_ids: state === 'partial' ? ['9007199254740996'] : [],
    rpm: empty ? null : '9007199254740997',
    site_count: empty ? 0 : 2,
    stale_site_ids: state === 'partial' ? ['9007199254740996'] : [],
    today: {
      active_users: empty ? null : '99',
      as_of: empty ? null : rangeEnd - 3600,
      data_status: dataStatus,
      is_final: false,
      is_partial: state === 'partial',
      quota: empty ? null : '9223372036854775807',
      reason: state === 'partial' ? missingReason : null,
      request_count: empty ? null : '9007199254740995',
      site_breakdown: empty ? [] : siteBreakdown,
      token_used: empty ? null : '9007199254740997',
    },
    tpm: empty ? null : '9223372036854775807',
  }
}

function dashboardHealth(state: DashboardFixtureState = 'partial') {
  const empty = state === 'empty'
  const complete = state === 'complete'
  return {
    as_of: empty ? null : rangeEnd - 60,
    auth_expired_site_ids: state === 'partial' ? ['9007199254740996'] : [],
    completeness: empty
      ? emptyCompleteness()
      : completeness(complete ? 'complete' : 'partial'),
    critical_alert_count: empty ? 0 : 1,
    firing_alert_count: empty ? 0 : 2,
    is_final: state !== 'partial',
    latest_alerts: empty
      ? []
      : [
          {
            first_observed_at: rangeEnd - 600,
            id: '9007199254740998',
            last_fired_at: rangeEnd - 60,
            level: 'critical',
            message: missingReason,
            site_id: '9007199254740993',
            site_name: siteBreakdown[0]?.site_name,
            status: 'firing',
            target_name: 'CPU 使用率连续超过阈值的超长实例名称',
          },
        ],
    reason: state === 'partial' ? missingReason : null,
    sites: empty
      ? []
      : [
          {
            auth_status: 'authorized',
            health_status: complete ? 'ok' : 'warning',
            management_status: 'active',
            online_status: 'online',
            site_id: '9007199254740993',
            site_name: siteBreakdown[0]?.site_name,
            statistics_status: complete ? 'ready' : 'partial',
            updated_at: rangeEnd - 60,
          },
        ],
    statistics_not_ready_site_ids:
      state === 'partial' ? ['9007199254740993'] : [],
    warning_alert_count: empty ? 0 : 1,
    yesterday_validation_status: state === 'partial' ? 'partial' : 'complete',
  }
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
    await route.fulfill({ json: envelope(viewer, 'req_self_f5') })
  })
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

async function fulfillDashboard(
  route: Route,
  endpoint: DashboardEndpoint,
  failed: string | null,
  state: DashboardFixtureState = 'partial'
) {
  if (failed === endpoint) {
    await route.fulfill({ json: errorEnvelope(), status: 500 })
    return
  }
  let data: unknown
  if (endpoint === 'summary') data = dashboardSummary(state)
  else if (endpoint === 'trend') {
    data =
      state === 'empty'
        ? []
        : [
            trendPoint(rangeStart),
            trendPoint(
              rangeStart + 86_400,
              state === 'complete' ? 'complete' : 'missing'
            ),
          ]
  } else if (endpoint === 'top') {
    const url = new URL(route.request().url())
    const type = url.searchParams.get('type') ?? 'customer'
    const rankingNames: Record<string, string> = {
      channel: '跨站通道超长名称',
      customer: '重点客户超长中文名称用于移动端换行',
      model: '跨站模型超长名称',
      site: '华东生产站点超长名称',
    }
    data =
      state === 'empty'
        ? []
        : [
            {
              as_of: rangeEnd - 3600,
              data_status: state === 'complete' ? 'complete' : 'partial',
              dimension_id: '9007199254740995',
              dimension_name: rankingNames[type],
              dimension_type: type,
              is_final: state === 'complete',
              reason: state === 'partial' ? missingReason : null,
              site_breakdown: siteBreakdown,
              site_id: null,
              value: '9007199254740997',
            },
          ]
  } else data = dashboardHealth(state)
  await route.fulfill({ json: envelope(data, `req_${endpoint}`) })
}

async function mockDashboard(page: Page, state: DashboardFixtureState) {
  for (const endpoint of ['summary', 'trend', 'top', 'health'] as const) {
    await page.route(`**/api/dashboard/${endpoint}*`, async (route) => {
      await fulfillDashboard(route, endpoint, null, state)
    })
  }
}

async function expectFiveDashboardSections(page: Page) {
  await expect(
    page.locator('main section[aria-labelledby^="dashboard-"]')
  ).toHaveCount(5)
  for (const name of dashboardSectionNames) {
    await expect(page.getByRole('heading', { name })).toBeVisible()
  }
}

async function expectAccessibleSkipLink(page: Page) {
  const skipLink = page.getByRole('link', { name: '跳到主要内容' })
  await page.keyboard.press('Tab')
  await expect(skipLink).toBeFocused()
  await expect(skipLink).toBeVisible()
  const box = await skipLink.boundingBox()
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(40)
  expect(box?.width ?? 0).toBeGreaterThanOrEqual(40)
  await skipLink.press('Enter')
  await expect(page.locator('#main-content')).toBeFocused()
}

test('Dashboard renders the complete fixture across all five sections', async ({
  page,
}) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  await mockDashboard(page, 'complete')
  await page.goto('/dashboard')
  await expectFiveDashboardSections(page)
  await expectAccessibleSkipLink(page)

  const today = page.getByRole('region', { name: '今日运营' })
  await expect(today.getByText('完整', { exact: true })).toHaveCount(2)
  await expect(today.getByText('9,007,199,254,740,995').first()).toBeVisible()
  const resource = today.getByRole('region', { name: '实例资源完整性' })
  await expect(resource.getByText('完整', { exact: true })).toBeVisible()
  await expect(resource.getByText('完整站点 2 / 预期 2')).toBeVisible()
  await expect(resource.getByText('资源过期站点 ID：')).toHaveCount(0)

  const realtime = page.getByRole('region', { name: '实时吞吐' })
  await expect(realtime.getByText('完整', { exact: true })).toBeVisible()
  await expect(realtime.getByText('完整站点 2 / 预期 2')).toBeVisible()

  const trend = page.getByRole('region', { name: '近 30 天趋势' })
  await expect(trend.getByRole('img', { name: '统计趋势图' })).toBeVisible()
  await expect(
    trend.getByTestId('statistics-chart-exact-values')
  ).not.toContainText('原始指标值 -')
  await expect(
    trend.getByTestId('statistics-chart-exact-values')
  ).not.toContainText('当前显示值 -')

  const ranking = page.getByRole('region', { name: '今日快速排行' })
  await expect(
    ranking.getByText('重点客户超长中文名称用于移动端换行')
  ).toBeVisible()
  await expect(ranking.getByText('完整', { exact: true })).toBeVisible()

  const health = page.getByRole('region', {
    name: '站点健康、完整性与告警',
  })
  await expect(
    health.getByText('已完成 24 / 24 个统计单元，完整率 100%')
  ).toBeVisible()
  await expect(health.getByText('昨日校验状态')).toBeVisible()
  await expect(health.getByText('已最终确认')).toBeVisible()
  await expect(health.getByText('授权过期站点 ID')).toBeVisible()
  await expect(health.getByText('统计未就绪站点 ID')).toBeVisible()
  await expect(health.getByText('无', { exact: true })).toHaveCount(2)
  await expect(
    health.getByText('华东超长名称生产站点用于验证移动端不会横向溢出')
  ).toBeVisible()
  await expect(health.getByText('统计就绪')).toBeVisible()
  await expect(
    health.getByText('CPU 使用率连续超过阈值的超长实例名称')
  ).toBeVisible()
  await expect(page.getByText('该区块加载失败')).toHaveCount(0)
  await expect(page.getByText('暂无可展示数据')).toHaveCount(0)
})

test('Dashboard renders partial values without presenting missing data as zero', async ({
  page,
}) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  await mockDashboard(page, 'partial')
  await page.goto('/dashboard')
  await expectFiveDashboardSections(page)

  const today = page.getByRole('region', { name: '今日运营' })
  await expect(today.getByText('部分完整', { exact: true })).toHaveCount(2)
  await expect(today.getByText('该范围的数据缺失')).toHaveCount(2)
  const resource = today.getByRole('region', { name: '实例资源完整性' })
  await expect(resource.getByText('部分完整', { exact: true })).toBeVisible()
  await expect(resource.getByText('完整站点 1 / 预期 2')).toBeVisible()
  await expect(
    resource.getByText('资源过期站点 ID：9007199254740996')
  ).toBeVisible()

  const realtime = page.getByRole('region', { name: '实时吞吐' })
  await expect(realtime.getByText('部分完整', { exact: true })).toBeVisible()
  await expect(realtime.getByText('完整站点 1 / 预期 2')).toBeVisible()
  await expect(
    realtime.getByText('数据过期站点 ID：9007199254740996')
  ).toBeVisible()

  const trend = page.getByRole('region', { name: '近 30 天趋势' })
  await expect(
    trend.getByTestId('statistics-chart-exact-values')
  ).toContainText('-')

  const ranking = page.getByRole('region', { name: '今日快速排行' })
  await expect(ranking.getByText('部分完整', { exact: true })).toBeVisible()
  await expect(ranking.getByText('尚未最终确认')).toBeVisible()

  const health = page.getByRole('region', {
    name: '站点健康、完整性与告警',
  })
  await expect(
    health.getByText('已完成 12 / 24 个统计单元，完整率 50%')
  ).toBeVisible()
  await expect(health.getByText('缺失站点 ID：9007199254740993')).toBeVisible()
  await expect(health.getByText('昨日校验状态')).toBeVisible()
  await expect(health.getByText('尚未最终确认')).toBeVisible()
  await expect(
    health.getByText('9007199254740996', { exact: true })
  ).toBeVisible()
  await expect(
    health.getByText('9007199254740993', { exact: true })
  ).toBeVisible()
  await expect(health.getByText('统计部分完整')).toBeVisible()
  await expect(health.getByTestId('dashboard-health-reason')).toHaveText(
    '该范围的数据缺失'
  )
  await expect(page.getByText('该区块加载失败')).toHaveCount(0)
  await expect(page.getByText('暂无可展示数据')).toHaveCount(0)
})

test('Dashboard renders empty trend, ranking, site and alert states', async ({
  page,
}) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  await mockDashboard(page, 'empty')
  await page.goto('/dashboard')
  await expectFiveDashboardSections(page)

  const today = page.getByRole('region', { name: '今日运营' })
  await expect(today.getByText('不可用', { exact: true }).first()).toBeVisible()
  const resource = today.getByRole('region', { name: '实例资源完整性' })
  await expect(resource.getByText('完整站点 0 / 预期 0')).toBeVisible()
  await expect(resource.getByText('暂无更新时间')).toBeVisible()

  const realtime = page.getByRole('region', { name: '实时吞吐' })
  await expect(realtime.getByText('不可用', { exact: true })).toHaveCount(2)
  await expect(realtime.getByText('完整站点 0 / 预期 0')).toBeVisible()

  const trend = page.getByRole('region', { name: '近 30 天趋势' })
  await expect(trend.getByText('暂无可展示数据')).toBeVisible()
  await expect(trend.getByRole('img', { name: '统计趋势图' })).toHaveCount(0)

  const ranking = page.getByRole('region', { name: '今日快速排行' })
  await expect(ranking.getByText('暂无可展示数据')).toBeVisible()
  await expect(ranking.getByRole('tablist')).toHaveCount(0)

  const health = page.getByRole('region', {
    name: '站点健康、完整性与告警',
  })
  await expect(health.getByText('尚无站点健康数据。')).toBeVisible()
  await expect(health.getByText('当前没有触发中的告警。')).toBeVisible()
  await expect(health.getByText('昨日校验状态')).toBeVisible()
  await expect(health.getByText('已最终确认')).toBeVisible()
  await expect(health.getByText('无', { exact: true })).toHaveCount(2)
  await expect(page.getByText('该区块加载失败')).toHaveCount(0)
})

test('Dashboard isolates each of its four endpoint failures', async ({
  page,
}, testInfo) => {
  test.setTimeout(90_000)
  await seedAuth(page)
  let failed: string | null = 'summary'
  let cycleRequests: string[] = []
  let topRequests: URL[] = []
  for (const endpoint of ['summary', 'trend', 'top', 'health'] as const) {
    await page.route(`**/api/dashboard/${endpoint}*`, async (route) => {
      cycleRequests.push(endpoint)
      if (endpoint === 'top') topRequests.push(new URL(route.request().url()))
      await fulfillDashboard(route, endpoint, failed)
    })
  }

  for (const endpoint of ['summary', 'trend', 'top', 'health'] as const) {
    failed = endpoint
    cycleRequests = []
    topRequests = []
    await page.goto('/dashboard')
    await expect(page.getByRole('heading', { name: '今日运营' })).toBeVisible()
    await expect(page.getByRole('heading', { name: '实时吞吐' })).toBeVisible()
    await expect(
      page.getByRole('heading', { name: '近 30 天趋势' })
    ).toBeVisible()
    await expect(
      page.getByRole('heading', { name: '今日快速排行' })
    ).toBeVisible()
    await expect(
      page.getByRole('heading', { name: '站点健康、完整性与告警' })
    ).toBeVisible()
    const failedSections = page.getByText('该区块加载失败')
    await expect(failedSections).toHaveCount(endpoint === 'summary' ? 2 : 1)
    await expect
      .poll(() => cycleRequests.length)
      .toBe(endpoint === 'top' ? 15 : 9)
    expect(new Set(cycleRequests)).toEqual(
      new Set(['health', 'summary', 'top', 'trend'])
    )
    expect(cycleRequests.filter((value) => value === endpoint)).toHaveLength(
      endpoint === 'top' ? 12 : 3
    )
    expect(
      new Set(topRequests.map((url) => url.searchParams.get('type')))
    ).toEqual(new Set(['site', 'customer', 'model', 'channel']))
    if (endpoint !== 'summary') {
      await expect(
        page.getByText('9,007,199,254,740,995').first()
      ).toBeVisible()
      await expect(
        page.getByText('9,007,199,254,740,997').first()
      ).toBeVisible()
    }
    if (endpoint !== 'top') {
      await expect(
        page.getByText('重点客户超长中文名称用于移动端换行')
      ).toBeVisible()
    }
    if (endpoint !== 'health') {
      await expect(
        page.getByText('CPU 使用率连续超过阈值的超长实例名称')
      ).toBeVisible()
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth
      )
    ).toBe(false)
  }

  failed = null
  cycleRequests = []
  topRequests = []
  await page.goto('/dashboard')
  await hideDeveloperOverlays(page)
  await expect(page.getByRole('heading', { name: '今日运营' })).toBeVisible()
  await expect(page.getByRole('img', { name: '统计趋势图' })).toBeVisible()
  await page.screenshot({
    path: testInfo.outputPath('dashboard.png'),
  })
  await page.getByRole('img', { name: '统计趋势图' }).scrollIntoViewIfNeeded()
  await page.screenshot({
    path: testInfo.outputPath('dashboard-chart.png'),
  })
  const dashboardLongName = page.getByText('重点客户超长中文名称用于移动端换行')
  await dashboardLongName.scrollIntoViewIfNeeded()
  await expect(dashboardLongName).toBeVisible()
  await page.screenshot({
    path: testInfo.outputPath('dashboard-top.png'),
  })
  const rankingRegion = page.getByRole('region', { name: '今日快速排行' })
  await rankingRegion
    .getByRole('button', { name: 'quota', exact: true })
    .click()
  await rankingRegion.getByLabel('显示数量').fill('10')
  await expect
    .poll(
      () =>
        new Set(
          topRequests
            .filter(
              (url) =>
                url.searchParams.get('metric') === 'quota' &&
                url.searchParams.get('limit') === '10'
            )
            .map((url) => url.searchParams.get('type'))
        ).size
    )
    .toBe(4)
  for (const [tab, name] of [
    ['站点', '华东生产站点超长名称'],
    ['模型', '跨站模型超长名称'],
    ['通道', '跨站通道超长名称'],
  ] as const) {
    await page.getByRole('tab', { name: tab, exact: true }).click()
    await expect(page.getByText(name)).toBeVisible()
  }
  await page.screenshot({
    fullPage: true,
    path: testInfo.outputPath('dashboard-ranking-controls.png'),
  })
})

function statisticsResponse(scope: string, url: URL) {
  const start = Number(url.searchParams.get('start_timestamp') ?? rangeStart)
  const end = Number(url.searchParams.get('end_timestamp') ?? rangeEnd)
  const granularity = url.searchParams.get('granularity') ?? 'hour'
  const base = {
    active_users: scope === 'account' ? null : '9007199254740995',
    as_of: end - 3600,
    bucket_end: start + 3600,
    bucket_start: start,
    completeness_rate: 0.5,
    data_status: 'partial',
    dimension_id: scope === 'global' ? 'global' : '9007199254740993',
    dimension_name: '跨站统计超长中文维度名称用于验证表格和移动卡片换行',
    is_final: false,
    quota: '9223372036854775807',
    request_count: null,
    site_breakdown: siteBreakdown,
    site_id:
      scope === 'global' || scope === 'customer' ? null : '9007199254740993',
    site_name:
      scope === 'global' || scope === 'customer'
        ? null
        : siteBreakdown[0]?.site_name,
    token_used: '9007199254740997',
  }
  const extras: Record<string, unknown> = { dimension_type: scope }
  if (scope === 'global') {
    extras.complete_site_count = 0
    extras.expected_site_count = 1
  } else if (scope === 'site') {
    Object.assign(extras, {
      auth_status: 'authorized',
      health_status: 'warning',
      management_status: 'active',
      online_status: 'online',
      rate: {
        quota_per_unit: '500000',
        source: 'site',
        updated_at: end - 3600,
        usd_exchange_rate: '7.3',
      },
      statistics_status: 'partial',
    })
  } else if (scope === 'customer') {
    extras.account_count = 3
    extras.site_count = 1
  } else if (scope === 'account') {
    Object.assign(extras, {
      customer_id: '9007199254740995',
      customer_name: '长名称客户',
      remote_user_id: '9007199254740997',
    })
  } else if (scope === 'model') extras.model_name = base.dimension_name
  else if (scope === 'channel') {
    extras.remote_channel_id = '0'
    extras.remote_missing = true
  } else if (scope === 'group') {
    base.dimension_name = ''
    extras.use_group = ''
  } else if (scope === 'token') {
    base.dimension_id = '9007199254740993:9007199254740997'
    base.dimension_name = ''
    extras.token_id = '9007199254740997'
    extras.token_name = ''
  } else {
    base.dimension_name = ''
    extras.node_name = ''
  }
  return {
    breakdown: {
      items: [{ ...base, ...extras }],
      page: Number(url.searchParams.get('p') ?? 1),
      page_size: Number(url.searchParams.get('page_size') ?? 20),
      total: 1,
    },
    completeness: completeness(),
    granularity,
    range: {
      as_of: end - 3600,
      end_timestamp: end,
      start_timestamp: start,
      timezone: 'Asia/Shanghai',
    },
    scope,
    site_breakdown: siteBreakdown,
    summary: {
      active_users: scope === 'account' ? null : '9007199254740995',
      data_status: 'partial',
      is_partial: true,
      quota: '9223372036854775807',
      request_count: null,
      token_used: '9007199254740997',
    },
    trend: [
      {
        ...trendPoint(start),
        bucket_end: start + 3600,
      },
      {
        ...trendPoint(start + 3600, 'missing'),
        bucket_end: start + 7200,
      },
    ],
  }
}

test('nine statistics scopes preserve URL filters, partial and null contracts', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  const statisticsRequests: URL[] = []
  await page.route(
    /\/api\/statistics\/(global|sites|customers|accounts|models|channels)(?:\?.*)?$/,
    async (route) => {
      const url = new URL(route.request().url())
      statisticsRequests.push(url)
      const segment = url.pathname.split('/').at(-1) ?? 'global'
      const scopes: Record<string, string> = {
        accounts: 'account',
        channels: 'channel',
        customers: 'customer',
        global: 'global',
        models: 'model',
        sites: 'site',
      }
      const scope = scopes[segment] ?? 'global'
      await route.fulfill({ json: envelope(statisticsResponse(scope, url)) })
    }
  )
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({
        items: [
          {
            id: '9007199254740993',
            name: siteBreakdown[0]?.site_name,
          },
        ],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
  await page.route(
    /\/api\/statistics\/options\/models(?:\?.*)?$/,
    async (route) => {
      await route.fulfill({
        json: envelope({
          items: [
            {
              key: '9007199254740993:跨区域超长中文模型名称',
              model_name: '跨区域超长中文模型名称',
              site_id: '9007199254740993',
              site_name: siteBreakdown[0]?.site_name,
            },
          ],
          page: 1,
          page_size: 50,
          total: 1,
        }),
      })
    }
  )

  await page.goto(
    `/statistics/global?start=${rangeStart}&end=${rangeEnd}&granularity=hour&metric=request_count&view=chart`
  )
  await expect(page.getByRole('heading', { name: '全局统计' })).toBeVisible()
  const statisticsFilters = page.getByRole('region', { name: '统计筛选' })
  for (const granularity of ['小时', '日', '月', '年']) {
    const button = statisticsFilters.getByRole('button', {
      exact: true,
      name: granularity,
    })
    const box = await button.boundingBox()
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(40)
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(40)
  }
  await expect(
    page.getByText(
      '当前结果包含未完成或不可用的时间桶，金额与指标仅代表可用数据。'
    )
  ).toBeVisible()
  await expect(
    page
      .getByRole('region', { name: '范围汇总' })
      .getByText('不可用', { exact: true })
  ).toBeVisible()
  await expect(page.getByTestId('statistics-chart-exact-values')).toContainText(
    '-'
  )
  const scopeNavigation = page.getByRole('navigation', { name: '统计作用域' })
  await expect(scopeNavigation.getByRole('link')).toHaveCount(9)
  await scopeNavigation.getByRole('link', { name: '模型' }).click()
  await expect(page.getByRole('heading', { name: '模型统计' })).toBeVisible()

  await page.getByRole('button', { name: '对象筛选' }).click()
  const filter = page.getByRole('dialog', { name: '统计对象筛选' })
  await expect(filter).toBeVisible()
  await filter
    .getByLabel(
      '华东超长名称生产站点用于验证移动端不会横向溢出（ID 9007199254740993）'
    )
    .check()
  await filter.getByPlaceholder('输入名称或标识').fill('跨区域')
  await filter
    .getByLabel(
      '华东超长名称生产站点用于验证移动端不会横向溢出 / 跨区域超长中文模型名称'
    )
    .check()
  await filter.getByRole('button', { name: '应用' }).click()
  await expect
    .poll(() =>
      JSON.parse(new URL(page.url()).searchParams.get('siteIds') ?? '[]')
    )
    .toEqual(['9007199254740993'])
  await expect
    .poll(() =>
      JSON.parse(new URL(page.url()).searchParams.get('models') ?? '[]')
    )
    .toEqual(['跨区域超长中文模型名称'])
  await expect
    .poll(() => statisticsRequests.at(-1)?.searchParams.get('site_ids'))
    .toBe('9007199254740993')
  await expect
    .poll(() => statisticsRequests.at(-1)?.searchParams.getAll('model_names'))
    .toEqual(['跨区域超长中文模型名称'])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
  await expect(page.getByRole('img', { name: '统计趋势图' })).toBeVisible()
  await page.screenshot({
    path: testInfo.outputPath('statistics-model.png'),
  })
  await page.getByRole('img', { name: '统计趋势图' }).scrollIntoViewIfNeeded()
  await page.screenshot({
    path: testInfo.outputPath('statistics-model-chart.png'),
  })
  await page.getByRole('button', { name: '表格视图' }).click()
  const statisticsLongName = page
    .getByText('跨站统计超长中文维度名称用于验证表格和移动卡片换行')
    .filter({ visible: true })
  await statisticsLongName.scrollIntoViewIfNeeded()
  await expect(statisticsLongName).toBeVisible()
  await page.screenshot({
    path: testInfo.outputPath('statistics-model-breakdown.png'),
  })
  const accessibility = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(accessibility.violations).toEqual([])
})

test('group token and node pages preserve identity filters, states, export and mobile layout', async ({
  page,
}) => {
  test.setTimeout(60_000)
  await seedAuth(page)
  await page.setViewportSize({ height: 812, width: 375 })
  const requests: URL[] = []
  await page.route(
    /\/api\/statistics\/(groups|tokens|nodes)(?:\?.*)?$/,
    async (route) => {
      const url = new URL(route.request().url())
      requests.push(url)
      const segment = url.pathname.split('/').at(-1)
      const scopeBySegment: Record<string, 'group' | 'token' | 'node'> = {
        groups: 'group',
        nodes: 'node',
        tokens: 'token',
      }
      const scope = scopeBySegment[segment ?? ''] ?? 'group'
      await route.fulfill({ json: envelope(statisticsResponse(scope, url)) })
    }
  )
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({
        items: [{ id: '9007199254740993', name: siteBreakdown[0]?.site_name }],
        page: 1,
        page_size: 100,
        total: 1,
      }),
    })
  })
  await page.route(
    /\/api\/statistics\/options\/(groups|tokens|nodes)(?:\?.*)?$/,
    async (route) => {
      const segment = new URL(route.request().url()).pathname.split('/').at(-1)
      let items: Record<string, unknown>[]
      if (segment === 'groups') {
        items = [
          {
            key: '9007199254740993:',
            site_id: '9007199254740993',
            site_name: siteBreakdown[0]?.site_name,
            use_group: '',
          },
        ]
      } else if (segment === 'tokens') {
        items = [
          {
            key: '9007199254740993:9007199254740997',
            site_id: '9007199254740993',
            site_name: siteBreakdown[0]?.site_name,
            token_id: '9007199254740997',
            token_name: '',
          },
        ]
      } else {
        items = [
          {
            key: '9007199254740993:',
            node_name: '',
            site_id: '9007199254740993',
            site_name: siteBreakdown[0]?.site_name,
          },
        ]
      }
      await route.fulfill({
        json: envelope({ items, page: 1, page_size: 50, total: items.length }),
      })
    }
  )

  const cases = [
    {
      filterKey: 'useGroups',
      filterLabel: '未知分组',
      route: 'groups',
      scope: 'group',
      title: '分组统计',
      value: [''],
    },
    {
      filterKey: 'tokenKeys',
      filterLabel: '未命名 Token',
      route: 'tokens',
      scope: 'token',
      title: 'Token 统计',
      value: ['9007199254740993:9007199254740997'],
    },
    {
      filterKey: 'nodeNames',
      filterLabel: '未知节点',
      route: 'nodes',
      scope: 'node',
      title: '节点统计',
      value: [''],
    },
  ] as const
  for (const item of cases) {
    const query = `${item.filterKey}=${encodeURIComponent(JSON.stringify(item.value))}`
    await page.goto(`/statistics/${item.route}?${query}&view=table`)
    await expect(page.getByRole('heading', { name: item.title })).toBeVisible()
    await expect(
      page
        .getByText(item.filterLabel, { exact: true })
        .filter({ visible: true })
        .first()
    ).toBeVisible()
    await expect(
      page.getByRole('button', { name: '导出', exact: true })
    ).toBeVisible()
    await expect(page.getByRole('button', { name: '对象筛选' })).toBeVisible()
    await page.getByRole('button', { name: '对象筛选' }).click()
    const filter = page.getByRole('dialog', { name: '统计对象筛选' })
    await expect(filter.getByText(item.filterLabel)).toBeVisible()
    await filter.getByRole('button', { name: '应用', exact: true }).click()
    await expect
      .poll(() => requests.at(-1)?.pathname)
      .toBe(`/api/statistics/${item.route}`)
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth
      )
    ).toBe(true)
    const accessibility = await new AxeBuilder({ page }).analyze()
    expect(accessibility.violations).toEqual([])
  }
})
