import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

const viewer = {
  display_name: '排行榜只读员',
  id: '9007199254740991',
  must_change_password: false,
  role: 'viewer',
  status: 1,
  username: 'ranking_viewer',
}

interface ExportBody {
  filters: Record<string, unknown>
  format: 'csv' | 'xlsx'
  statistics_type: string
}

function envelope<T>(data: T, requestId = 'req_ranking_e2e') {
  return { code: '', data, message: '', request_id: requestId, success: true }
}

function assertAuthenticated(route: Route) {
  const headers = route.request().headers()
  expect(headers['new-api-user']).toBe(viewer.id)
  expect(headers['x-request-id']).toMatch(/^web_/)
}

async function seedAuth(page: Page, testInfo: TestInfo) {
  if (testInfo.project.name === 'chromium-mobile') {
    await page.setViewportSize({ height: 812, width: 375 })
  }
  await page.addInitScript((authUser) => {
    window.localStorage.setItem('pilot-auth-user', JSON.stringify(authUser))
    window.localStorage.setItem('uid', authUser.id)
  }, viewer)
  await page.route('**/api/user/self', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({ json: envelope(viewer, 'req_ranking_self') })
  })
}

function item(
  dimensionId: string,
  dimensionName: string,
  rank: number,
  growth: string | null
) {
  return {
    dimension_id: dimensionId,
    dimension_name: dimensionName,
    growth,
    quota: '900719925474099312345',
    rank,
    request_count: '9007199254740994',
    share: '0.3333333333',
    token_used: '900719925474099312345678',
  }
}

function response(vendors = false) {
  const main = vendors
    ? item('0', 'upstream-ignored-name', 1, null)
    : item('gpt-4o', 'gpt-4o', 1, null)
  return {
    as_of: 1_784_348_700,
    data_status: 'partial',
    droppers: [item(vendors ? '88' : 'drop-model', '下降项', 3, '-0.25')],
    end_timestamp: 1_784_380_800,
    history: [
      {
        bucket_start: 1_784_294_400,
        dimension_id: main.dimension_id,
        token_used: '900719925474099312345677',
      },
    ],
    items: [main],
    movers: [item(vendors ? '99' : 'move-model', '上升项', 2, '1.5')],
    period: 'today',
    site_breakdown: [
      {
        as_of: 1_784_348_600,
        data_status: 'unavailable',
        dimension_id: main.dimension_id,
        site_id: '9007199254740997',
        site_name: '华东统计站点',
        token_used: '900719925474099312345676',
      },
    ],
    start_timestamp: 1_784_294_400,
  }
}

function exportJob(body: ExportBody) {
  return {
    created_at: 1_784_348_800,
    data_snapshot_at: null,
    deduplicated: false,
    error: null,
    expires_at: null,
    file_name: '',
    file_size: '0',
    filters: body.filters,
    finished_at: null,
    format: body.format,
    id: '797',
    progress: 0,
    row_count: '0',
    started_at: null,
    statistics_type: body.statistics_type,
    status: 'pending',
  }
}

test('A97 keeps local rankings exact, bounded, exportable and responsive', async ({
  page,
}, testInfo) => {
  test.setTimeout(60_000)
  await seedAuth(page, testInfo)
  const rankingReads: URL[] = []
  const exports: ExportBody[] = []
  const rankingRequestPaths: string[] = []

  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/') && path.includes('ranking')) {
      rankingRequestPaths.push(path)
    }
  })
  await page.route(
    /\/api\/rankings\/(models|vendors)(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      rankingReads.push(url)
      await route.fulfill({
        json: envelope(response(url.pathname.endsWith('/vendors'))),
      })
    }
  )
  await page.route(
    /\/api\/sites\/9007199254740997\/rankings\/(models|vendors)(?:\?.*)?$/,
    async (route) => {
      assertAuthenticated(route)
      const url = new URL(route.request().url())
      expect(url.searchParams.has('site_ids')).toBe(false)
      rankingReads.push(url)
      await route.fulfill({
        json: envelope(response(url.pathname.endsWith('/vendors'))),
      })
    }
  )
  await page.route('**/api/statistics/export', async (route) => {
    assertAuthenticated(route)
    const body = route.request().postDataJSON() as ExportBody
    exports.push(body)
    await route.fulfill({ json: envelope(exportJob(body)) })
  })
  await page.route('**/api/statistics/exports/797', async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope(
        exportJob(
          exports.at(-1) ?? {
            filters: {},
            format: 'csv',
            statistics_type: 'model_rankings',
          }
        )
      ),
    })
  })
  await page.goto('/rankings?period=today&tab=models&siteIds=9007199254740997')
  await expect(
    page.getByRole('heading', { exact: true, name: '本地排行榜' })
  ).toBeVisible()
  await expect(
    page.getByText('900719925474099312345678').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('9007199254740994').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('0.3333333333').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(page.getByText('上升项').first()).toBeVisible()
  await expect(page.getByText('下降项').first()).toBeVisible()
  await expect(page.getByText('排名历史原值')).toBeVisible()
  await expect(page.getByText('华东统计站点')).toBeVisible()

  for (const [label, period] of [
    ['本周', 'week'],
    ['本月', 'month'],
    ['本年', 'year'],
    ['今日', 'today'],
  ] as const) {
    await page.getByRole('button', { exact: true, name: label }).click()
    await expect
      .poll(() => new URL(page.url()).searchParams.get('period'))
      .toBe(period)
    if (period !== 'today') {
      await expect
        .poll(() => rankingReads.at(-1)?.searchParams.get('period'))
        .toBe(period)
    }
  }

  await page.getByRole('tab', { name: '厂商' }).click()
  await expect
    .poll(() => new URL(page.url()).searchParams.get('tab'))
    .toBe('vendors')
  await expect(
    page.getByText('未知厂商').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('button', { name: '导出 CSV' }).click()
  await expect
    .poll(() => exports.at(-1)?.statistics_type)
    .toBe('vendor_rankings')
  expect(exports.at(-1)?.filters.ranking_period).toBe('today')
  expect(exports.at(-1)?.filters.site_ids).toEqual(['9007199254740997'])
  await page.getByRole('button', { name: '关闭' }).click()

  const scan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa'])
    .analyze()
  expect(scan.violations).toEqual([])
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)

  await page.goto(
    '/sites/9007199254740997/rankings?period=month&tab=models&siteIds=9007199254740995'
  )
  await expect(
    page.getByRole('heading', { exact: true, name: '站点本地排行榜' })
  ).toBeVisible()
  await page.getByRole('button', { name: '导出 XLSX' }).click()
  await expect
    .poll(() => exports.at(-1)?.statistics_type)
    .toBe('model_rankings')
  expect(exports.at(-1)?.filters.ranking_period).toBe('month')
  expect(exports.at(-1)?.filters.site_ids).toEqual(['9007199254740997'])
  expect(
    rankingRequestPaths.every((path) =>
      /^\/api\/(?:rankings\/(?:models|vendors)|sites\/9007199254740997\/rankings\/(?:models|vendors))$/.test(
        path
      )
    )
  ).toBe(true)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth
    )
  ).toBe(true)
})
