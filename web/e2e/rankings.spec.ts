import AxeBuilder from '@axe-core/playwright'
import {
  expect,
  test,
  type Page,
  type Route,
  type TestInfo,
} from '@playwright/test'

import { mockAuthenticatedShell } from './helpers/auth'

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
  await mockAuthenticatedShell(page)
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
  growth: string | null,
  movementType?: 'new' | 'up' | 'down' | 'stable' | 'removed'
) {
  let resolvedMovement = movementType
  if (resolvedMovement == null) {
    if (growth == null) resolvedMovement = 'stable'
    else resolvedMovement = Number(growth) > 0 ? 'up' : 'down'
  }
  return {
    dimension_id: dimensionId,
    dimension_name: dimensionName,
    growth,
    movement_type: resolvedMovement,
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
  const items = [
    main,
    ...Array.from({ length: 20 }, (_, index) =>
      item(
        `${vendors ? 'vendor' : 'model'}-${index + 2}`,
        `${vendors ? '厂商' : '模型'} ${index + 2}`,
        index + 2,
        '0.125'
      )
    ),
  ]
  return {
    as_of: 1_784_348_700,
    data_status: 'partial',
    droppers: [
      item(vendors ? '77' : 'removed-model', '退出项', 0, null, 'removed'),
      item(vendors ? '88' : 'drop-model', '下降项', 3, '-0.25'),
    ],
    end_timestamp: 1_784_380_800,
    history: Array.from({ length: 21 }, (_, index) => ({
      bucket_start: 1_784_294_400 + index * 3600,
      dimension_id: `history-${index + 1}`,
      token_used: `${900719925474099312345677n + BigInt(index)}`,
    })),
    items,
    movers: [
      item(vendors ? '100' : 'new-model', '新增项', 1, null, 'new'),
      item(vendors ? '99' : 'move-model', '上升项', 2, '1.5'),
    ],
    period: 'today',
    site_breakdown: Array.from({ length: 21 }, (_, index) => ({
      as_of: 1_784_348_600,
      data_status: index === 0 ? 'unavailable' : 'complete',
      dimension_id: `site-dimension-${index + 1}`,
      site_id: `${9007199254740997n + BigInt(index)}`,
      site_name: `华东统计站点 ${index + 1}`,
      token_used: `${900719925474099312345676n + BigInt(index)}`,
    })),
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

  await page.route(/\/api\/sites(?:\?.*)?$/, async (route) => {
    assertAuthenticated(route)
    await route.fulfill({
      json: envelope({
        items: [
          {
            id: '9007199254740997',
            name: '华东统计站点',
            status: 1,
          },
        ],
        page: 1,
        page_size: 100,
        total: '1',
      }),
    })
  })

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
  await page.addStyleTag({
    content: `
      button[aria-label='Open TanStack Router Devtools'],
      button[aria-label='Open Tanstack query devtools'] {
        display: none !important;
      }
    `,
  })
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
    page.getByText('33.33%').filter({ visible: true }).first()
  ).toBeVisible()
  await expect(page.getByText('0.3333333333')).toHaveCount(0)
  await expect(
    page.getByText('不可用').filter({ visible: true }).first()
  ).toBeVisible()
  await page.getByRole('button', { name: '下一页' }).click()
  await expect(page).toHaveURL(/page=2/)
  await expect(
    page.getByText('模型 21').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('tab', { name: '升降变化' }).click()
  await expect(page).toHaveURL(/view=movement/)
  await expect(page).toHaveURL(/page=1/)
  await expect(page.getByText('上升项').first()).toBeVisible()
  await expect(page.getByText('下降项').first()).toBeVisible()
  await expect(page.getByText('新增上榜').first()).toBeVisible()
  await expect(page.getByText('退出榜单').first()).toBeVisible()
  await expect(page.getByText('150%').first()).toBeVisible()
  await expect(page.getByText('-25%').first()).toBeVisible()
  await expect(page.getByRole('button', { name: '导出 CSV' })).toHaveCount(0)

  await page.getByRole('tab', { name: '历史原值' }).click()
  await expect(
    page.getByText('history-1').filter({ visible: true }).first()
  ).toBeVisible()
  await page.getByRole('button', { name: '下一页' }).click()
  await expect(page).toHaveURL(/page=2/)
  await expect(
    page.getByText('history-21').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('tab', { name: '站点明细' }).click()
  await expect(page).toHaveURL(/page=1/)
  await page.getByRole('button', { name: '下一页' }).click()
  await expect(
    page.getByText('华东统计站点 21').filter({ visible: true }).first()
  ).toBeVisible()

  await page.getByRole('tab', { name: '主榜' }).click()

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
