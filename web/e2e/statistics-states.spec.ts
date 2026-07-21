import {
  expect,
  test,
  type Locator,
  type Page,
  type Route,
} from '@playwright/test'

const authStorageKey = 'pilot-auth-user'
const uidStorageKey = 'uid'
const baseRangeStart = 1_783_785_600
const rangeSeconds = 86_400
const siteId = '1'
const customerId = '7'
const accountId = '9'

type A50State =
  | 'complete-zero'
  | 'partial'
  | 'missing'
  | 'unavailable'
  | 'paused'

type StatisticsScope =
  | 'global'
  | 'site'
  | 'customer'
  | 'account'
  | 'model'
  | 'channel'

interface StatisticsRouteCase {
  apiPath: string
  detailApiPath?: string
  detailKind?: 'site' | 'customer' | 'account'
  heading: string
  key: string
  path: string
  scope: StatisticsScope
}

const viewer = {
  display_name: 'A50 统计状态只读审核员',
  id: '9007199254740994',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'a50-viewer',
}

const stateOrder: A50State[] = [
  'complete-zero',
  'partial',
  'missing',
  'unavailable',
  'paused',
]

const stateCopy: Record<
  Exclude<A50State, 'complete-zero'>,
  { description: string; emptyTitle?: string; reason: string; status: string }
> = {
  missing: {
    description: '当前范围包含缺失时间桶；缺失值不会按 0 展示。',
    emptyTitle: '当前范围的数据缺失',
    reason: '该范围的数据缺失',
    status: '缺失',
  },
  partial: {
    description:
      '当前结果包含未完成或不可用的时间桶，金额与指标仅代表可用数据。',
    reason: '2 个站点中有 1 个数据完整',
    status: '部分完整',
  },
  paused: {
    description: '当前范围统计已暂停；暂停前已经完成的数据仍会保留。',
    reason: '该范围的统计已暂停',
    status: '已暂停',
  },
  unavailable: {
    description: '上游无法提供当前范围的数据；不可用值不会按 0 展示。',
    emptyTitle: '当前范围统计不可用',
    reason: '上游站点无法提供该范围的数据',
    status: '不可用',
  },
}

const routeCases: StatisticsRouteCase[] = [
  {
    apiPath: '/api/statistics/global',
    heading: '全局统计',
    key: 'global',
    path: '/statistics/global',
    scope: 'global',
  },
  {
    apiPath: '/api/statistics/sites',
    heading: '站点统计',
    key: 'sites',
    path: '/statistics/sites',
    scope: 'site',
  },
  {
    apiPath: '/api/statistics/customers',
    heading: '客户统计',
    key: 'customers',
    path: '/statistics/customers',
    scope: 'customer',
  },
  {
    apiPath: '/api/statistics/accounts',
    heading: '账户统计',
    key: 'accounts',
    path: '/statistics/accounts',
    scope: 'account',
  },
  {
    apiPath: '/api/statistics/models',
    heading: '模型统计',
    key: 'models',
    path: '/statistics/models',
    scope: 'model',
  },
  {
    apiPath: '/api/statistics/channels',
    heading: '通道统计',
    key: 'channels',
    path: '/statistics/channels',
    scope: 'channel',
  },
  {
    apiPath: `/api/sites/${siteId}/stats`,
    detailApiPath: `/api/sites/${siteId}`,
    detailKind: 'site',
    heading: '华东站点 的站点统计',
    key: 'site-deep-link',
    path: `/sites/${siteId}/stats`,
    scope: 'site',
  },
  {
    apiPath: `/api/customers/${customerId}/stats`,
    detailApiPath: `/api/customers/${customerId}`,
    detailKind: 'customer',
    heading: '重点客户 的客户统计',
    key: 'customer-deep-link',
    path: `/customers/${customerId}/stats`,
    scope: 'customer',
  },
  {
    apiPath: `/api/accounts/${accountId}/stats`,
    detailApiPath: `/api/accounts/${accountId}`,
    detailKind: 'account',
    heading: 'managed@example.com 的账户统计',
    key: 'account-deep-link',
    path: `/accounts/${accountId}/stats`,
    scope: 'account',
  },
]

function envelope<T>(data: T, requestId = 'req_a50') {
  return {
    code: '',
    data,
    message: '',
    request_id: requestId,
    success: true,
  }
}

function rangeForState(state: A50State) {
  const index = stateOrder.indexOf(state)
  const start = baseRangeStart + index * rangeSeconds * 2
  return { end: start + rangeSeconds, start }
}

function stateForStart(start: number): A50State {
  return (
    stateOrder.find((state) => rangeForState(state).start === start) ??
    'partial'
  )
}

function statusForState(state: A50State) {
  return state === 'complete-zero' ? 'complete' : state
}

function reasonForState(state: A50State, start: number, end: number) {
  if (state === 'complete-zero') return null
  if (state === 'partial') {
    return {
      code: 'DATA_PARTIAL_SITES',
      params: { complete_site_count: 1, expected_site_count: 2 },
      technical_detail: '',
    }
  }
  if (state === 'missing') {
    return {
      code: 'DATA_WINDOW_MISSING',
      params: {
        end_timestamp: end,
        site_id: siteId,
        start_timestamp: start,
      },
      technical_detail: '',
    }
  }
  return {
    code:
      state === 'unavailable'
        ? 'DATA_UPSTREAM_UNAVAILABLE'
        : 'DATA_SCOPE_PAUSED',
    params: {},
    technical_detail: '',
  }
}

function siteBreakdown(
  quota: string,
  status: 'complete' | 'partial' = 'complete'
) {
  return [
    {
      data_status: status,
      quota,
      quota_per_unit: '500000',
      rate_source: 'site',
      rate_updated_at: baseRangeStart,
      site_id: siteId,
      site_name: '华东站点',
      usd_exchange_rate: '7.3',
    },
  ]
}

function trendPoint(
  state: A50State,
  start: number,
  end: number,
  retained = false
) {
  const status = retained ? 'complete' : statusForState(state)
  let value: string | null = null
  if (state === 'complete-zero') value = '0'
  else if (retained) value = '314'
  else if (state === 'partial') value = '42'
  let activeUsers: string | null = null
  let quota: string | null = null
  let tokenUsed: string | null = null
  let sites: ReturnType<typeof siteBreakdown> = []
  if (value != null) {
    const resolvedQuota = value === '42' ? '500000' : value
    activeUsers = value === '0' ? '0' : '1'
    quota = resolvedQuota
    tokenUsed = value === '42' ? '8400' : value
    sites = siteBreakdown(
      resolvedQuota,
      status === 'partial' ? 'partial' : 'complete'
    )
  }
  return {
    active_users: activeUsers,
    as_of: end - 1800,
    bucket_end: start + 3600,
    bucket_start: start,
    complete_site_count: status === 'complete' || status === 'partial' ? 1 : 0,
    data_status: status,
    expected_site_count: status === 'partial' ? 2 : 1,
    is_final: status === 'complete',
    quota,
    reason: retained ? null : reasonForState(state, start, end),
    request_count: value,
    site_breakdown: sites,
    token_used: tokenUsed,
  }
}

function breakdownDimensionName(state: A50State) {
  if (state === 'complete-zero') return '零值明细'
  if (state === 'paused') return '暂停前保留明细'
  return '部分已知明细'
}

function breakdownItem(
  scope: StatisticsScope,
  state: A50State,
  start: number,
  end: number
) {
  const retained = state === 'paused'
  const status = retained ? 'complete' : statusForState(state)
  let value = '42'
  if (state === 'complete-zero') value = '0'
  else if (retained) value = '314'
  const dimensionName = breakdownDimensionName(state)
  const base: Record<string, unknown> = {
    active_users: value === '0' ? '0' : '1',
    as_of: end - 1800,
    bucket_end: start + 3600,
    bucket_start: start,
    completeness_rate: status === 'partial' ? 0.5 : 1,
    data_status: status,
    dimension_id: scope === 'global' ? 'global' : '9007199254740993',
    dimension_name: dimensionName,
    dimension_type: scope,
    is_final: status === 'complete',
    quota: value === '42' ? '500000' : value,
    request_count: value,
    site_breakdown: siteBreakdown(
      value === '42' ? '500000' : value,
      status === 'partial' ? 'partial' : 'complete'
    ),
    site_id: scope === 'global' || scope === 'customer' ? null : siteId,
    site_name: scope === 'global' || scope === 'customer' ? null : '华东站点',
    token_used: value === '42' ? '8400' : value,
  }
  if (scope === 'global') {
    Object.assign(base, {
      complete_site_count: status === 'partial' ? 1 : 1,
      expected_site_count: status === 'partial' ? 2 : 1,
    })
  } else if (scope === 'site') {
    Object.assign(base, {
      auth_status: 'authorized',
      health_status: 'ok',
      management_status: 'active',
      online_status: 'online',
      rate: {
        quota_per_unit: '500000',
        source: 'site',
        updated_at: end - 1800,
        usd_exchange_rate: '7.3',
      },
      statistics_status: status === 'partial' ? 'partial' : 'ready',
    })
  } else if (scope === 'customer') {
    Object.assign(base, { account_count: 1, site_count: 1 })
  } else if (scope === 'account') {
    Object.assign(base, {
      customer_id: customerId,
      customer_name: '重点客户',
      remote_user_id: '9007199254740997',
    })
  } else if (scope === 'model') {
    base.model_name = dimensionName
  } else {
    Object.assign(base, {
      remote_channel_id: '0',
      remote_missing: false,
    })
  }
  return base
}

function completeness(state: A50State, start: number, end: number) {
  const status = statusForState(state)
  const complete = state === 'complete-zero'
  const partial = state === 'partial'
  let completenessRate = 0
  if (complete) completenessRate = 1
  else if (partial) completenessRate = 0.5
  return {
    complete_site_count: complete || partial ? 1 : 0,
    complete_unit_count: complete || partial ? 1 : 0,
    completeness_rate: completenessRate,
    data_status: status,
    expected_site_count: partial ? 2 : 1,
    expected_unit_count: partial ? 2 : 1,
    last_verified_at: end - 1800,
    missing_range_total: complete ? 0 : 1,
    missing_ranges: complete
      ? []
      : [
          {
            end_timestamp: end,
            reason: reasonForState(state, start, end),
            site_id: siteId,
            start_timestamp: start,
            status,
          },
        ],
    missing_ranges_truncated: false,
    missing_site_ids: complete ? [] : [siteId],
    unit_type: 'hour',
  }
}

function statisticsResponse(
  scope: StatisticsScope,
  state: A50State,
  start: number,
  end: number,
  granularity: string
) {
  const completeZero = state === 'complete-zero'
  const partial = state === 'partial'
  const paused = state === 'paused'
  let summaryValue: string | null = null
  if (completeZero) summaryValue = '0'
  else if (partial) summaryValue = '42'
  const items =
    state === 'missing' || state === 'unavailable'
      ? []
      : [breakdownItem(scope, state, start, end)]
  const trend = paused
    ? [
        trendPoint(state, start, start + 3600, true),
        trendPoint(state, start + 3600, end),
      ]
    : [trendPoint(state, start, end)]
  let summarySiteBreakdown: ReturnType<typeof siteBreakdown> = []
  if (completeZero) summarySiteBreakdown = siteBreakdown('0')
  else if (partial) summarySiteBreakdown = siteBreakdown('500000', 'partial')
  let summaryActiveUsers: string | null = null
  let summaryQuota: string | null = null
  let summaryTokenUsed: string | null = null
  if (summaryValue != null) {
    summaryActiveUsers = summaryValue === '0' ? '0' : '1'
    summaryQuota = summaryValue === '42' ? '500000' : '0'
    summaryTokenUsed = summaryValue === '42' ? '8400' : '0'
  }
  return {
    breakdown: {
      items,
      page: 1,
      page_size: 20,
      total: items.length,
    },
    completeness: completeness(state, start, end),
    granularity,
    range: {
      as_of: end - 1800,
      end_timestamp: end,
      start_timestamp: start,
      timezone: 'Asia/Shanghai',
    },
    scope,
    site_breakdown: summarySiteBreakdown,
    summary: {
      active_users: summaryActiveUsers,
      data_status: statusForState(state),
      is_partial: !completeZero,
      quota: summaryQuota,
      request_count: summaryValue,
      token_used: summaryTokenUsed,
    },
    trend,
  }
}

function escapeRegExp(value: string) {
  return value.replaceAll(/[.*+?^${}()|[\]\\]/g, '\\$&')
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
    await route.fulfill({ json: envelope(viewer, 'req_self_a50') })
  })
  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({ items: [], page: 1, page_size: 100, total: 0 }),
    })
  })
  await page.route(/\/api\/customers(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({ items: [], page: 1, page_size: 100, total: 0 }),
    })
  })
  await page.route(/\/api\/accounts(?:\?.*)?$/, async (route) => {
    await route.fulfill({
      json: envelope({ items: [], page: 1, page_size: 100, total: 0 }),
    })
  })
  await page.route(
    /\/api\/statistics\/options\/[a-z]+(?:\?.*)?$/,
    async (route) => {
      await route.fulfill({
        json: envelope({ items: [], page: 1, page_size: 50, total: 0 }),
      })
    }
  )
}

function detailFixture(kind: StatisticsRouteCase['detailKind']) {
  if (kind === 'site') return { id: siteId, name: '华东站点' }
  if (kind === 'customer') return { id: customerId, name: '重点客户' }
  return { id: accountId, username: 'managed@example.com' }
}

async function mockStatisticsRoute(page: Page, routeCase: StatisticsRouteCase) {
  const requests: URL[] = []
  let dayGate: Promise<void> | null = null
  let releaseDayGate: (() => void) | null = null

  if (routeCase.detailApiPath) {
    await page.route(
      new RegExp(`${escapeRegExp(routeCase.detailApiPath)}$`),
      async (route) => {
        await route.fulfill({
          json: envelope(detailFixture(routeCase.detailKind), 'req_detail_a50'),
        })
      }
    )
  }

  await page.route(
    new RegExp(`${escapeRegExp(routeCase.apiPath)}(?:\\?.*)?$`),
    async (route: Route) => {
      const url = new URL(route.request().url())
      requests.push(url)
      const start = Number(
        url.searchParams.get('start_timestamp') ?? baseRangeStart
      )
      const end = Number(
        url.searchParams.get('end_timestamp') ?? start + rangeSeconds
      )
      const granularity = url.searchParams.get('granularity') ?? 'hour'
      const held = granularity === 'day' ? dayGate : null
      if (held) await held
      const state = granularity === 'day' ? 'unavailable' : stateForStart(start)
      await route.fulfill({
        json: envelope(
          statisticsResponse(routeCase.scope, state, start, end, granularity),
          `req_${routeCase.key}_${state}`
        ),
      })
    }
  )

  return {
    holdDayResponse() {
      dayGate = new Promise<void>((resolve) => {
        releaseDayGate = resolve
      })
      return () => {
        releaseDayGate?.()
        releaseDayGate = null
        dayGate = null
      }
    },
    requests,
  }
}

function pageURL(routeCase: StatisticsRouteCase, state: A50State) {
  const range = rangeForState(state)
  const params = new URLSearchParams({
    end: String(range.end),
    granularity: 'hour',
    metric: 'request_count',
    start: String(range.start),
    view: 'chart',
  })
  return `${routeCase.path}?${params.toString()}`
}

function metric(summary: Locator, label: string) {
  return summary.getByText(label, { exact: true }).locator('..')
}

function beijingHour(timestamp: number) {
  const hour = new Date((timestamp + 8 * 3600) * 1000)
    .toISOString()
    .slice(0, 13)
    .replace('T', ' ')
  return `${hour}:00`
}

async function expectNoHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth
    )
  ).toBe(false)
}

async function expectState(page: Page, state: A50State) {
  const range = rangeForState(state)
  const summary = page.getByRole('region', { name: '范围汇总' })
  const requestMetric = metric(summary, '请求数')
  await expect(summary.getByText(/数据截至/)).toBeVisible()

  if (state === 'complete-zero') {
    await expect(summary.getByRole('status')).toHaveCount(0)
    await expect(requestMetric.getByText('0', { exact: true })).toBeVisible()
  } else {
    const notice = summary.getByRole('status')
    await expect(notice).toContainText(stateCopy[state].description)
    await expect(
      notice.getByText(stateCopy[state].status, { exact: true })
    ).toBeVisible()
    if (state === 'partial') {
      await expect(requestMetric.getByText('42', { exact: true })).toBeVisible()
    } else {
      await expect(
        requestMetric.getByText('不可用', { exact: true })
      ).toBeVisible()
      await expect(requestMetric.getByText('0', { exact: true })).toHaveCount(0)
    }
  }

  await expect(page.getByRole('img', { name: '统计趋势图' })).toBeVisible()
  const exactValues = page.getByTestId('statistics-chart-exact-values')
  if (state === 'complete-zero') {
    await expect(exactValues).toContainText('原始指标值 0')
    await expect(exactValues).not.toContainText('原始指标值 -')
  } else if (state === 'partial') {
    await expect(exactValues).toContainText('原始指标值 42')
  } else if (state === 'paused') {
    await expect(exactValues).toContainText('原始指标值 314')
    await expect(exactValues).toContainText('原始指标值 -')
  } else {
    await expect(exactValues).toContainText('原始指标值 -')
    await expect(exactValues).not.toContainText('原始指标值 0')
  }

  const completenessHeading = page.getByRole('heading', {
    name: '数据完整性',
  })
  const completenessSection = completenessHeading.locator('..').locator('..')
  await expect(completenessSection).toContainText('最近校验于')
  if (state === 'complete-zero') {
    await expect(
      completenessSection.getByText('完整', { exact: true })
    ).toBeVisible()
  } else {
    await expect(completenessSection).toContainText(stateCopy[state].reason)
    await expect(completenessSection).toContainText(
      `${beijingHour(range.start)} - ${beijingHour(range.end)}`
    )
    await expect(
      page.getByText(reasonForState(state, 0, 0)?.code ?? '')
    ).toHaveCount(0)
  }

  await page.getByRole('button', { name: '表格视图' }).click()
  if (state === 'missing' || state === 'unavailable') {
    await expect(
      page.getByText(stateCopy[state].emptyTitle ?? '', { exact: true })
    ).toBeVisible()
    await expect(page.getByText('范围内无流量', { exact: true })).toHaveCount(0)
  } else {
    const dimensionName = breakdownDimensionName(state)
    await expect(
      page.getByText(dimensionName, { exact: true }).filter({ visible: true })
    ).toBeVisible()
    await expect(page.getByText('范围内无流量', { exact: true })).toHaveCount(0)
  }
  await expectNoHorizontalOverflow(page)
}

test.describe('A50 statistics pages preserve all five data states', () => {
  for (const routeCase of routeCases) {
    test(`${routeCase.key} covers five states, refresh retention and reload`, async ({
      page,
    }) => {
      test.setTimeout(120_000)
      await seedAuth(page)
      await page.addStyleTag({
        content: `
          button[aria-label='Open TanStack Router Devtools'],
          button[aria-label='Open Tanstack query devtools'] {
            display: none !important;
          }
        `,
      })
      const mock = await mockStatisticsRoute(page, routeCase)

      for (const state of stateOrder) {
        await test.step(state, async () => {
          await page.goto(pageURL(routeCase, state))
          await expect(
            page.getByRole('heading', { name: routeCase.heading })
          ).toBeVisible()
          await expectState(page, state)
        })
      }

      await test.step('range refresh retains old data and reload restores URL', async () => {
        await page.goto(pageURL(routeCase, 'partial'))
        await expect(
          page.getByRole('heading', { name: routeCase.heading })
        ).toBeVisible()
        const summary = page.getByRole('region', { name: '范围汇总' })
        await expect(
          metric(summary, '请求数').getByText('42', { exact: true })
        ).toBeVisible()

        const release = mock.holdDayResponse()
        await page
          .getByRole('region', { name: '统计筛选' })
          .getByRole('button', { name: '日', exact: true })
          .click()
        await expect(
          page.getByText('正在加载新范围，当前继续展示上一范围的数据。')
        ).toBeVisible()
        await expect(
          metric(summary, '请求数').getByText('42', { exact: true })
        ).toBeVisible()

        release()
        await expect(
          page.getByText('上游无法提供当前范围的数据；不可用值不会按 0 展示。')
        ).toBeVisible()
        await expect(
          metric(summary, '请求数').getByText('不可用', { exact: true })
        ).toBeVisible()

        const beforeReload = new URL(page.url())
        const requestsBeforeReload = mock.requests.length
        await page.reload()
        await expect(
          page.getByRole('heading', { name: routeCase.heading })
        ).toBeVisible()
        const afterReload = new URL(page.url())
        expect(afterReload.pathname).toBe(routeCase.path)
        expect(afterReload.search).toBe(beforeReload.search)
        expect(afterReload.searchParams.get('granularity')).toBe('day')
        await expect(
          page.getByText('上游站点无法提供该范围的数据')
        ).toBeVisible()
        expect(mock.requests.length).toBeGreaterThan(requestsBeforeReload)
        await expectNoHorizontalOverflow(page)
      })
    })
  }
})
